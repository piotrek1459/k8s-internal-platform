#!/usr/bin/env bash
set -euo pipefail

usage() {
    cat <<'EOF'
Usage: ./scripts/build-book.sh

Compiles book/main.tex to book/build/main.pdf using Docker.
On macOS, starts Docker Desktop if the engine is not ready.
On Linux, requires a running Docker engine.
The first run downloads the image and LaTeX packages; later runs use the cache.
EOF
}

if [[ $# -gt 0 ]]; then
    if [[ $# -eq 1 && ( "$1" == "--help" || "$1" == "-h" ) ]]; then
        usage
        exit 0
    fi
    usage >&2
    exit 2
fi

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
BOOK_DIR="$REPO_DIR/book"
IMAGE="k8s-internal-platform-latex:bookworm"

if ! command -v docker >/dev/null 2>&1; then
    printf 'The docker command was not found. Install Docker Desktop (macOS) or Docker Engine (Linux).\n' >&2
    exit 1
fi

if [[ ! -f "$BOOK_DIR/main.tex" ]]; then
    printf 'File not found: %s/main.tex.\n' "$BOOK_DIR" >&2
    exit 1
fi

if ! docker info >/dev/null 2>&1; then
    if [[ "$(uname -s)" == "Darwin" ]]; then
        printf 'Starting Docker Desktop...\n'
        if ! open -g -a Docker; then
            printf 'Could not start Docker Desktop. Check that it is installed correctly.\n' >&2
            exit 1
        fi
        deadline=$((SECONDS + 120))
        until docker info >/dev/null 2>&1; do
            if (( SECONDS >= deadline )); then
                printf 'Docker is still not ready after 120 seconds. Check Docker Desktop and run docker info.\n' >&2
                exit 1
            fi
            sleep 2
        done
    else
        printf 'The Docker engine is unavailable. Start it and check access with docker info.\n' >&2
        exit 1
    fi
fi

printf 'Preparing the LaTeX image...\n'
docker build --tag "$IMAGE" "$REPO_DIR/docker/latex"

mkdir -p "$BOOK_DIR/build"
printf 'Compiling the thesis...\n'
docker run --rm --init \
    --network none \
    --user "$(id -u):$(id -g)" \
    --env HOME=/tmp \
    --mount "type=bind,source=$BOOK_DIR,target=/work" \
    "$IMAGE"

printf '\nDone: %s/build/main.pdf\n' "$BOOK_DIR"
