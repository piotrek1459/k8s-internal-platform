# HTTP demonstration application

This stateless Go application provides a controlled workload for M0/M1 deployment
experiments. Independently built v1 and v2 images expose their embedded versions;
v3 deliberately fails readiness while retaining a healthy process. This permits
observation of a failed rollout without introducing a crash loop.

The application uses the Go standard library and requires Go 1.25 or later.
Container builds use Go 1.27.2 with the official builder image pinned by digest.

## Deployment contract

| Property | Contract |
| --- | --- |
| Process | One HTTP server; no background services or external dependencies. |
| Listen address | `0.0.0.0:8080` by default. |
| Architectures | Linux ARM64 and AMD64; cross-compilation is supported by the Dockerfile. |
| Identity | `GET /` and `GET /version` return JSON containing `version` and `hostname`. |
| Readiness | `GET /readyz` returns 200 when ready and 503 when readiness failure is enabled. |
| Liveness/startup | `GET /healthz` returns 200, including during deliberate readiness failure. |
| Logs | One JSON object per line on stdout, including lifecycle and request events. Query values are omitted. |
| Storage | No application filesystem writes or persistent storage. |
| Container identity | UID/GID 65532; no elevated privileges or additional capabilities are required. |
| Shutdown | SIGTERM/SIGINT stop the listener and permit in-flight requests to complete for up to 10 seconds. A Kubernetes termination grace period of at least 15 seconds is required. |

Example identity response:

```json
{"version":"v1","hostname":"demo-7cf787cc5f-cb2mr"}
```

`hostname` identifies the responding container or pod. It can be used to observe
load distribution after horizontal scaling.

| Runtime variable | Default | Behaviour |
| --- | --- | --- |
| `PORT` | `8080` | Integer from 1 to 65535; a changed port must also be reflected in the Service and probes. |
| `READINESS_FAIL` | Embedded image setting | Optional boolean override for readiness experiments. Invalid values prevent startup. |

An unset or empty variable retains its default. The version is embedded at build
time and has no runtime override. A v3 image therefore reports `v3` even if
readiness failure is disabled explicitly.

## Local tests

Commands in this section run from the repository root:

```sh
cd examples/http-demo
go test -race ./...
go vet ./...
```

The tests cover version responses, the separation between readiness and
liveness, invalid configuration, unsupported routes/methods, and structured
request logging without query values.

## Local images

The following commands run from the repository root and build three separate
images for the current Docker platform. No registry push is performed.

```sh
docker build --build-arg VERSION=v1 --build-arg UNHEALTHY=false \
  -t platform-demo:v1 examples/http-demo
docker build --build-arg VERSION=v2 --build-arg UNHEALTHY=false \
  -t platform-demo:v2 examples/http-demo
docker build --build-arg VERSION=v3 --build-arg UNHEALTHY=true \
  -t platform-demo:v3 examples/http-demo
```

A hardened local run is available with:

```sh
docker run --rm --name platform-http-demo --read-only --cap-drop=ALL \
  --security-opt=no-new-privileges -p 127.0.0.1:8080:8080 platform-demo:v1
```

Requests can be issued from a separate terminal:

```sh
curl --fail http://127.0.0.1:8080/version
curl --fail http://127.0.0.1:8080/readyz
docker stop --timeout 15 platform-http-demo
```

Replacing the image with `platform-demo:v3` produces a 503 readiness response and
a 200 liveness response. The application chart must use `/readyz` for readiness
and `/healthz` for startup and liveness checks.

## ARM64 and AMD64 builds

Docker Buildx can produce a local OCI archive containing both architecture
variants without publishing an image:

```sh
docker buildx build --platform linux/arm64,linux/amd64 \
  --build-arg VERSION=v1 --build-arg UNHEALTHY=false \
  --output type=oci,dest=/tmp/platform-demo-v1.oci examples/http-demo
```

A Buildx builder with support for the OCI exporter and the selected target
platforms is required. A successful cross-build establishes compilation and
packaging support; Raspberry Pi deployment and performance remain separate
validation activities.
