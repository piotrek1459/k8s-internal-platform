"""Owned disposable environments and sanitized diagnostics for real workflows."""
import contextlib
import http.cookiejar
import json
import os
from pathlib import Path
import secrets
import signal
import socket
import subprocess
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[2]


def free_ports(count):
    sockets = []
    try:
        for _ in range(count):
            sock = socket.socket(); sock.bind(("127.0.0.1", 0)); sockets.append(sock)
        return [sock.getsockname()[1] for sock in sockets]
    finally:
        for sock in sockets: sock.close()


class Client:
    def __init__(self, environment):
        self.environment = environment
        self.cookies = http.cookiejar.CookieJar()
        self.http = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.cookies))

    def request(self, method, path, body=None, headers=None, csrf=True):
        fields = {"Content-Type": "application/json", **(headers or {})}
        if csrf: fields["X-Requested-With"] = "XMLHttpRequest"
        request = urllib.request.Request(self.environment.url + "/api/v1" + path, data=None if body is None else json.dumps(body).encode(), method=method, headers=fields)
        try:
            response = self.http.open(request, timeout=40)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            raw = response.read()
            return response.code, json.loads(raw) if raw else None

    def get(self, path):
        code, body = self.request("GET", path)
        if code != 200: raise AssertionError(f"GET {path}: {code}: {body}")
        return body

    def login(self):
        code, body = self.request("POST", "/auth/session", {"password": self.environment.password})
        if code != 204: raise AssertionError(f"Login failed: {code}: {body}")

    def mutate(self, method, path, body, key=None, terminal="succeeded"):
        key = key or secrets.token_hex(16)
        code, accepted = self.request(method, path, body, {"Idempotency-Key": key})
        if code != 202: raise AssertionError(f"{method} {path}: {code}: {accepted}")
        operation = self.poll(accepted["operationId"], terminal)
        return accepted, operation

    def poll(self, identity, terminal="succeeded"):
        deadline = time.monotonic() + 390
        while time.monotonic() < deadline:
            operation = self.get("/operations/" + identity)
            if operation["state"] in ("succeeded", "failed", "needs_attention"):
                if operation["state"] != terminal: raise AssertionError(f"Unexpected operation outcome: {operation}")
                return operation
            time.sleep(1)
        raise AssertionError("Operation did not finish within the bounded polling budget")


class Environment:
    def __init__(self, artifacts):
        self.artifacts = artifacts
        artifacts.mkdir(parents=True, exist_ok=True)
        self.name = "platform-e2e-" + secrets.token_hex(4)
        registry, api, ui, app = free_ports(4)
        self.env = dict(os.environ, PLATFORM_CLUSTER_NAME=self.name, PLATFORM_REGISTRY_PORT=str(registry), PLATFORM_API_PORT=str(api), PLATFORM_UI_PORT=str(ui), PLATFORM_APP_PORT=str(app), PLATFORM_BUILD_VERSION=(ROOT / "release/version.txt").read_text().strip())
        self.config = ROOT / ".local" / (self.name + ".kubeconfig")
        self.url = "http://127.0.0.1:" + str(ui)
        self.app_port = app
        self.password = secrets.token_urlsafe(24)
        self.token = ""
        self.secret_values = [self.password]
        self.steps = []
        self.forwards = []
        self.owns_cluster = False
        self.additional = []
        self.builders = []

    def redact(self, text):
        for value in self.secret_values:
            if value: text = text.replace(value, "[REDACTED]")
        import re
        return re.sub(r"(?m)^  [A-Za-z0-9_-]{32}[ \t]*$", "  [REDACTED]", text)

    def record(self, message, **details):
        self.steps.append({"step": message, **details})
        (self.artifacts / "steps.json").write_text(self.redact(json.dumps(self.steps, indent=2)))
        print(message, flush=True)

    def run(self, *command, private=False, timeout=900, check=True):
        result = subprocess.run(list(map(str, command)), cwd=ROOT, env=self.env, capture_output=True, text=True, timeout=timeout)
        if not private:
            with (self.artifacts / "commands.log").open("a") as log:
                log.write(self.redact("+ " + " ".join(map(str, command)) + "\n" + result.stdout + result.stderr))
        if check and result.returncode:
            raise RuntimeError(self.redact("Command failed: " + " ".join(map(str, command)) + "\n" + result.stderr[-3000:]))
        return result

    def cli(self, *args, **kwargs):
        return self.run(ROOT / "scripts/platform.sh", *args, **kwargs).stdout.strip()

    def kubectl(self, *args, **kwargs):
        return self.run("kubectl", "--kubeconfig", self.config, "--context", "k3d-" + self.name, *args, private=True, **kwargs)

    def resource(self, kind, app):
        return json.loads(self.kubectl("-n", "platform-apps", "get", kind, "-l", "platform.local/app-id=" + app, "-o", "json").stdout)["items"]

    def wait_replicas(self, app, count):
        deadline = time.monotonic() + 120
        while time.monotonic() < deadline:
            deployments = self.resource("deployments", app)
            pods = [p for p in self.resource("pods", app) if not p["metadata"].get("deletionTimestamp")]
            if len(deployments) == 1:
                d = deployments[0]
                if d["spec"]["replicas"] == count and d["status"].get("readyReplicas", 0) == count and d["status"].get("observedGeneration", 0) >= d["metadata"]["generation"] and len(pods) == count:
                    return pods
            time.sleep(1)
        raise AssertionError("Actual Kubernetes replica state did not converge")

    def image(self, version):
        return self.cli("image", version)

    def start(self):
        names = json.loads(self.run("k3d", "cluster", "list", "-o", "json", private=True).stdout)
        if any(c["name"] == self.name for c in names): raise RuntimeError("Refusing to reuse an E2E cluster")
        self.owns_cluster = True
        self.cli("up", "--no-ui", timeout=1500)
        self.start_management()
        self.token = self.cli("token", private=True)
        self.secret_values.append(self.token)
        self.client = Client(self)
        self.record("Built UI, broker and demos; installed a fresh isolated K3s platform")

    def start_management(self):
        self.management = self.forward("ui", port=int(self.env["PLATFORM_UI_PORT"]), path="/readyz")
        self.management.__enter__()

    def restart_broker(self):
        self.management.__exit__(None, None, None)
        self.kubectl("-n", "platform-system", "rollout", "restart", "deployment/platform")
        self.kubectl("-n", "platform-system", "rollout", "status", "deployment/platform", "--timeout=180s")
        self.start_management()

    @contextlib.contextmanager
    def forward(self, command, *args, port, path="/version"):
        with (self.artifacts / "forwards.log").open("a") as log:
            process = subprocess.Popen([str(ROOT / "scripts/platform.sh"), command, *args], cwd=ROOT, env=self.env, stdout=log, stderr=log, start_new_session=True)
            self.forwards.append(process)
            try:
                for _ in range(90):
                    if process.poll() is not None: raise RuntimeError("Owned port-forward exited")
                    try:
                        with urllib.request.urlopen(f"http://127.0.0.1:{port}{path}", timeout=1) as response:
                            if response.status == 200: break
                    except (OSError, urllib.error.URLError): time.sleep(0.5)
                else: raise RuntimeError("Owned port-forward did not become ready")
                yield
            finally:
                self.stop_forward(process)

    def stop_forward(self, process):
        if process.poll() is None:
            os.killpg(process.pid, signal.SIGTERM)
            try: process.wait(timeout=8)
            except subprocess.TimeoutExpired:
                os.killpg(process.pid, signal.SIGKILL); process.wait()

    def http_version(self, app, expected):
        with self.forward("forward", app, port=self.app_port):
            with urllib.request.urlopen(f"http://127.0.0.1:{self.app_port}/version", timeout=5) as response:
                body = json.load(response)
            if body["version"] != expected: raise AssertionError(body)
            return body

    def create(self, name, version="v1", **extra):
        accepted, _ = self.client.mutate("POST", "/apps", {"name": name + "-" + secrets.token_hex(3), "image": self.image(version), "containerPort": 8080, "profile": "http", "replicas": 1, "health": {"readinessPath": "/readyz"}, "exposure": {"mode": "internal"}, **extra})
        app = accepted["applicationId"]
        self.wait_replicas(app, 1)
        return app

    def delete(self, app):
        version = self.client.get("/apps/" + app)["version"]
        self.client.mutate("DELETE", "/apps/" + app, {"version": version})
        if self.resource("deployments,services", app): raise AssertionError("Deleted app resources remain")

    def diagnostics(self):
        if not self.config.exists(): return
        result = self.kubectl("-n", "platform-system", "logs", "deployment/platform", "--tail=250", check=False)
        (self.artifacts / "broker.log").write_text(self.redact(result.stdout + result.stderr))
        result = self.kubectl("-n", "platform-apps", "get", "deployment,pod,service", "-o", "wide", check=False)
        (self.artifacts / "resources.txt").write_text(self.redact(result.stdout + result.stderr))

    def close(self):
        for process in self.forwards: self.stop_forward(process)
        for builder in self.builders:
            self.run("docker", "buildx", "rm", builder, check=False)
        for name, config in reversed([(self.name, self.config)] + self.additional):
            if name == self.name and not self.owns_cluster: continue
            self.run("k3d", "registry", "delete", "k3d-" + name + "-registry", check=False)
            # Select the owned kubeconfig so deletion cannot edit the user's default.
            result = subprocess.run(["k3d", "cluster", "delete", name], env=dict(self.env, KUBECONFIG=str(config)), capture_output=True, text=True, timeout=120)
            if result.returncode: raise RuntimeError(self.redact(result.stderr))
            config.unlink(missing_ok=True)
            import shutil
            state = ROOT / ".local" / name
            if state.exists(): shutil.rmtree(state)
        for path in self.artifacts.rglob("*"):
            if path.is_file() and path.suffix in (".log", ".txt", ".json", ".xml"):
                path.write_text(self.redact(path.read_text()))
