"""Exercise the same OCI/chart artifacts used by the native release installer."""
import importlib.util
import json
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile
import time
from types import SimpleNamespace
import urllib.request

from support import Client, ROOT, free_ports


def verify_bundle_installation(environment):
    e = environment
    module = importlib.util.spec_from_file_location("release_package", ROOT / "release/package.py")
    packaging = importlib.util.module_from_spec(module); module.loader.exec_module(packaging)
    architecture = e.run("docker", "info", "--format", "{{.Architecture}}", private=True).stdout.strip()
    architecture = {"aarch64": "arm64", "x86_64": "amd64"}.get(architecture, architecture)
    version = e.version
    with tempfile.TemporaryDirectory(prefix="platform-e2e-bundle-") as directory:
        folder = Path(directory); images = {}
        builder = e.name + "-builder"
        e.run("docker", "buildx", "create", "--name", builder, "--driver", "docker-container")
        e.builders.append(builder)
        for key in ("platform", "v1", "v2", "v3"):
            tag = "platform.local/platform-broker:" + version if key == "platform" else "platform.local/http-demo:" + key
            archive = folder / (key + ".oci.tar")
            args = ["docker", "buildx", "build", "--builder", builder, "--platform", "linux/" + architecture, "--provenance=false", "--tag", tag,
                    "--output", "type=oci,dest=" + str(archive), "--build-arg", "VERSION=" + (version if key == "platform" else key)]
            if key != "platform": args += ["--build-arg", "UNHEALTHY=" + str(key == "v3").lower()]
            args += [str(ROOT / ("broker" if key == "platform" else "examples/http-demo"))]
            e.run(*args, timeout=900)
            images[key] = archive
        bundle = packaging.package(version, architecture, images, folder, ROOT / ".local/bin/helm")
        extracted = folder / "extracted"; extracted.mkdir()
        with tarfile.open(bundle) as archive:
            for member in archive.getmembers():
                if not member.isfile() or "/" in member.name: raise AssertionError("Unexpected release member")
                with (extracted / member.name).open("wb") as file:
                    import shutil
                    shutil.copyfileobj(archive.extractfile(member), file)
        metadata = dict(line.split("=", 1) for line in (extracted / "release.env").read_text().splitlines())
        name = e.name + "-bundle"; port, ui = free_ports(2)
        config = ROOT / ".local" / (name + ".kubeconfig")
        e.additional.append((name, config))
        pins = packaging.versions()
        e.run("k3d", "cluster", "create", name, "--servers", "1", "--agents", "0", "--image", pins["K3S_IMAGE"], "--api-port", "127.0.0.1:" + str(port), "--kubeconfig-update-default=false", "--kubeconfig-switch-context=false", "--k3s-arg", "--secrets-encryption@server:0", "--k3s-arg", "--disable=traefik,servicelb@server:0", "--wait", "--timeout", "180s", timeout=240)
        config.write_text(e.run("k3d", "kubeconfig", "get", name, private=True).stdout);config.chmod(0o600)
        secondary_node = "k3d-" + name + "-server-0"
        for key in ("platform", "v1", "v2", "v3"):
            filename = "platform-image.oci.tar" if key == "platform" else "demo-" + key + ".oci.tar"
            e.run("docker", "cp", extracted / filename, secondary_node + ":/tmp/" + filename)
            reference = metadata["PLATFORM_IMAGE" if key == "platform" else "DEMO_" + key.upper() + "_IMAGE"]
            e.run("docker", "exec", secondary_node, "ctr", "--namespace", "k8s.io", "images", "import", "--local", "--digests", "--base-name", reference.split("@")[0], "/tmp/" + filename)
            imported = e.run("docker", "exec", secondary_node, "ctr", "--namespace", "k8s.io", "images", "ls", "--quiet", private=True).stdout.splitlines()
            if reference not in imported:
                raise AssertionError("Imported OCI content is missing its recorded native digest reference")
        e.run("kubectl", "--kubeconfig", config, "apply", "-f", extracted / "namespace.yaml")
        repository, digest = metadata["PLATFORM_IMAGE"].split("@")
        e.run(ROOT / ".local/bin/helm", "--kubeconfig", config, "upgrade", "--install", "platform", extracted / "platform-chart.tgz", "--namespace", "platform-system", "--create-namespace", "--set-string", "image.repository=" + repository, "--set-string", "image.digest=" + digest, "--set", "image.pullPolicy=Never", "--wait", "--timeout", "180s", timeout=240)
        log = (e.artifacts / "bundle-forward.log").open("w")
        process = subprocess.Popen(["kubectl", "--kubeconfig", str(config), "-n", "platform-system", "port-forward", "--address", "127.0.0.1", "service/platform", str(ui) + ":80"], stdout=log, stderr=log, start_new_session=True)
        e.forwards.append(process)
        try:
            url = "http://127.0.0.1:" + str(ui)
            for _ in range(90):
                try:
                    with urllib.request.urlopen(url + "/readyz", timeout=1) as response:
                        ready = json.load(response)
                    if ready["cluster"] == "available": break
                except OSError: time.sleep(0.5)
            else: raise AssertionError("Bundled platform did not become ready")
            logs = e.run("kubectl", "--kubeconfig", config, "-n", "platform-system", "logs", "deployment/platform", private=True).stdout
            token = re.search(r"(?m)^  ([A-Za-z0-9_-]{32})[ \t]*$", logs).group(1)
            e.secret_values.append(token)
            client = Client(SimpleNamespace(url=url, password=e.password))
            assert client.request("POST", "/setup", {"password": e.password}, {"Authorization": "Bearer " + token})[0] == 201
            client.login()
            assert client.get("/system")["version"] == version
            assert client.get("/system")["architecture"] == architecture
            body = {"name": "bundled-demo", "image": metadata["DEMO_V1_IMAGE"], "containerPort": 8080,
                    "profile": "http", "health": {"readinessPath": "/readyz"}, "exposure": {"mode": "internal"}}
            accepted, _ = client.mutate("POST", "/apps", body)
            app = accepted["applicationId"]
            current = client.get("/apps/" + app)
            client.mutate("PATCH", "/apps/" + app, {"version": current["version"], "image": metadata["DEMO_V2_IMAGE"]})
            resources = json.loads(e.run("kubectl", "--kubeconfig", config, "-n", "platform-apps", "get", "service", "-l", "platform.local/app-id=" + app, "-o", "json", private=True).stdout)
            service = resources["items"][0]["metadata"]["name"]
            app_port = free_ports(1)[0]
            forward = subprocess.Popen(["kubectl", "--kubeconfig", str(config), "-n", "platform-apps", "port-forward", "--address", "127.0.0.1", "service/" + service, str(app_port) + ":80"], stdout=log, stderr=log, start_new_session=True)
            e.forwards.append(forward)
            try:
                for _ in range(60):
                    try:
                        with urllib.request.urlopen(f"http://127.0.0.1:{app_port}/version", timeout=1) as response:
                            assert json.load(response)["version"] == "v2"
                        break
                    except OSError: time.sleep(0.5)
                else: raise AssertionError("Bundled demo did not serve HTTP")
            finally: e.stop_forward(forward)
            current = client.get("/apps/" + app)
            client.mutate("DELETE", "/apps/" + app, {"version": current["version"]})
        finally:
            e.stop_forward(process); log.close()
        e.record("Checksummed bundle starts the platform and deploys/updates/deletes a demo on a fresh node without a registry", architecture=architecture, version=version)
