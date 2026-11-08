#!/usr/bin/env python3
"""Offline structural validator for api/openapi.yaml.

This performs dependency-free checks that catch the common ways an OpenAPI
document breaks: invalid YAML, a missing top-level structure, and dangling
local ``$ref`` pointers. When ``openapi-spec-validator`` is importable it also
runs the full specification validation. The goal is a check that runs in CI and
locally with only the Python standard library plus PyYAML available.
"""

from __future__ import annotations

import sys
from pathlib import Path

try:
    import yaml
except ModuleNotFoundError:  # pragma: no cover - environment guard
    sys.exit("PyYAML is required: pip install pyyaml")


def fail(message: str) -> "NoReturn":  # type: ignore[name-defined]
    sys.exit(f"openapi: {message}")


def resolve_ref(ref: str, root: dict) -> bool:
    """Return True if a local JSON pointer like '#/components/schemas/X' resolves."""
    if not ref.startswith("#/"):
        # External references are out of scope for this offline check.
        return True
    node: object = root
    for part in ref[2:].split("/"):
        part = part.replace("~1", "/").replace("~0", "~")
        if isinstance(node, dict) and part in node:
            node = node[part]
        else:
            return False
    return True


def walk_refs(node: object, root: dict, broken: list[str]) -> None:
    if isinstance(node, dict):
        for key, value in node.items():
            if key == "$ref" and isinstance(value, str):
                if not resolve_ref(value, root):
                    broken.append(value)
            else:
                walk_refs(value, root, broken)
    elif isinstance(node, list):
        for item in node:
            walk_refs(item, root, broken)


def main(path: Path) -> None:
    text = path.read_text(encoding="utf-8")
    try:
        doc = yaml.safe_load(text)
    except yaml.YAMLError as exc:
        fail(f"invalid YAML: {exc}")

    if not isinstance(doc, dict):
        fail("document root must be a mapping")
    for required in ("openapi", "info", "paths"):
        if required not in doc:
            fail(f"missing required top-level field: {required}")
    if not str(doc["openapi"]).startswith("3."):
        fail(f"unsupported OpenAPI version: {doc['openapi']!r}")
    if not isinstance(doc.get("paths"), dict) or not doc["paths"]:
        fail("'paths' must be a non-empty mapping")

    broken: list[str] = []
    walk_refs(doc, doc, broken)
    if broken:
        unique = sorted(set(broken))
        fail("unresolved local $ref(s):\n  " + "\n  ".join(unique))

    # Full validation when the optional dependency is present.
    extra = ""
    try:
        from openapi_spec_validator import validate  # type: ignore

        validate(doc)
        extra = " (full specification validation passed)"
    except ModuleNotFoundError:
        extra = " (structural checks only; install openapi-spec-validator for full validation)"
    except Exception as exc:  # noqa: BLE001 - surface any validator error
        fail(f"specification validation failed: {exc}")

    print(
        f"openapi: {path} is valid — "
        f"{len(doc['paths'])} paths, "
        f"{len(doc.get('components', {}).get('schemas', {}))} schemas{extra}."
    )


if __name__ == "__main__":
    target = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(__file__).resolve().parents[1] / "api" / "openapi.yaml"
    if not target.is_file():
        fail(f"file not found: {target}")
    main(target)
