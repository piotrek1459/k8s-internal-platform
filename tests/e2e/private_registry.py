"""Authenticated pulls through the owned disposable registry, without host logins."""
import contextlib
import json
import secrets
import time
import urllib.error
import urllib.request


@contextlib.contextmanager
def authenticated_registry(environment):
    e = environment
    container = "k3d-" + e.name + "-registry"
    host = "localhost:" + e.env["PLATFORM_REGISTRY_PORT"]
    image = host + "/private-demo:acceptance"
    e.run("docker", "tag", "platform-demo:v1", image)
    e.run("docker", "push", image)
    # The disposable registry's image is the same image k3d provisions. Capture
    # only its command, never the container labels (which may contain tokens).
    command = json.loads(e.run("docker", "inspect", "--format", "{{json .Config.Cmd}}", container).stdout)
    config = next(arg for arg in command if arg.endswith((".yml", ".yaml")))
    original = e.run("docker", "exec", container, "cat", config, private=True).stdout
    password = secrets.token_urlsafe(24); e.secret_values.append(password)
    htpasswd = e.run("docker", "run", "--rm", "--entrypoint", "htpasswd", "httpd:2.4-alpine", "-Bbn", "acceptance", password, private=True).stdout
    auth_config = original + '\nauth:\n  htpasswd:\n    realm: acceptance\n    path: /tmp/platform-acceptance.htpasswd\n'
    def write(path, body):
        e.run("docker", "exec", container, "sh", "-c", 'printf "%s" "$1" > "$2"', "sh", body, path, private=True)
    def ready(expected):
        for _ in range(60):
            try:
                with urllib.request.urlopen("http://"+host+"/v2/",timeout=2) as response: code=response.status
            except urllib.error.HTTPError as error: code=error.code
            except OSError: code=None
            if code==expected: return
            time.sleep(0.5)
        raise AssertionError("Owned registry did not reach its expected authentication state")
    try:
        write("/tmp/platform-acceptance.htpasswd",htpasswd)
        write(config,auth_config)
        e.run("docker","restart",container);ready(401)
        yield {"image":"k3d-"+e.name+"-registry:"+e.env["PLATFORM_REGISTRY_PORT"]+"/private-demo:acceptance", "registry":"k3d-"+e.name+"-registry:"+e.env["PLATFORM_REGISTRY_PORT"], "username":"acceptance","password":password}
    finally:
        write(config,original)
        e.run("docker","restart",container,check=False);ready(200)
