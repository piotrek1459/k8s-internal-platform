#!/usr/bin/env python3
"""Common entry point for local checks, regression suites and E2E workflows."""
import argparse
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
SUITES = ("broker", "demo", "python", "ui", "api", "chart", "syntax")


def run(*args, cwd=ROOT):
    print("+ " + " ".join(map(str, args)), flush=True)
    subprocess.run(list(map(str, args)), cwd=cwd, check=True)


def unit(suites):
    for suite in suites:
        if suite in ("broker", "ui", "api", "chart"):
            if suite == "chart":
                run(ROOT / "scripts/platform.sh", "tools")
            run(ROOT / "tests/checks" / (suite + ".sh"))
        elif suite == "demo":
            directory = ROOT / "examples/http-demo"
            files = subprocess.check_output(["gofmt", "-l", "."], cwd=directory, text=True)
            if files.strip():
                raise SystemExit("Unformatted demo files: " + files)
            run("go", "vet", "./...", cwd=directory)
            run("go", "test", "-count=1", "./...", cwd=directory)
        elif suite == "python":
            run(sys.executable, "-B", "-m", "unittest", "discover", "-s", "tests/regression", "-p", "test_*.py", "-v")
        elif suite == "syntax":
            for directory in ("scripts", "tests", "installer"):
                for script in sorted((ROOT / directory).rglob("*.sh")):
                    run("bash", "-n", script)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    checks = commands.add_parser("unit", help="Run fast suites; defaults to all.")
    checks.add_argument("--suite", choices=SUITES, action="append")
    integration = commands.add_parser("integration", help="Use a prepared disposable cluster.")
    integration.add_argument("--suite", choices=("broker", "helm"), default="broker")
    end = commands.add_parser("e2e", help="Build and test an isolated platform, then clean up.")
    end.add_argument("--artifacts", type=Path, default=ROOT / ".local/e2e")
    args = parser.parse_args()
    os.environ.setdefault("GOTOOLCHAIN", "local")
    if args.command == "unit":
        unit(args.suite or SUITES)
    elif args.command == "integration":
        if args.suite == "helm":
            run(ROOT / "tests/integration/helm-lifecycle.sh")
        else:
            run("go", "-C", ROOT / "broker", "test", "-count=1", "-tags", "integration", "./integration/...", "-v")
    else:
        sys.path.insert(0, str(ROOT / "tests/e2e"))
        from workflows import main as e2e_main
        return e2e_main(args.artifacts.resolve())
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except subprocess.CalledProcessError as error:
        raise SystemExit(error.returncode)
