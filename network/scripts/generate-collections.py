#!/usr/bin/env python3
"""Generate Fabric explicit collection definitions from versioned sources."""

from __future__ import annotations

import argparse
import json
import sys
from itertools import combinations
from pathlib import Path
from typing import Any

try:
    import jsonschema
except ImportError as exc:  # pragma: no cover - dependency preflight
    print(
        f"ERROR: missing Python dependency {exc.name!r}; install jsonschema",
        file=sys.stderr,
    )
    raise SystemExit(2) from exc


class CollectionsError(ValueError):
    """Raised when the inputs cannot produce an unambiguous configuration."""


def parse_args() -> argparse.Namespace:
    script_dir = Path(__file__).resolve().parent
    network_dir = script_dir.parent
    repository_root = network_dir.parent
    parser = argparse.ArgumentParser(
        description=(
            "Generate Fabric private-data collections from the organization "
            "manifest and the authorized-transfer matrix."
        )
    )
    parser.add_argument("--manifest", type=Path, default=network_dir / "organizations-manifest.json")
    parser.add_argument("--manifest-schema", type=Path, default=network_dir / "organizations-manifest.schema.json")
    parser.add_argument("--matrix", type=Path, default=repository_root / "domain" / "authorized-transfers.json")
    parser.add_argument("--matrix-schema", type=Path, default=repository_root / "domain" / "authorized-transfers.schema.json")
    parser.add_argument("--output", type=Path, default=network_dir / "collections_config.json")
    parser.add_argument(
        "--check",
        action="store_true",
        help="fail when the versioned output differs instead of writing it",
    )
    return parser.parse_args()


def load_json(path: Path) -> Any:
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise CollectionsError(f"cannot read JSON {path}: {exc}") from exc


def validate_json(document: Any, schema: Any, label: str) -> None:
    try:
        jsonschema.Draft202012Validator.check_schema(schema)
    except jsonschema.SchemaError as exc:
        raise CollectionsError(f"invalid JSON Schema for {label}: {exc.message}") from exc
    validator = jsonschema.Draft202012Validator(schema, format_checker=jsonschema.FormatChecker())
    errors = sorted(validator.iter_errors(document), key=lambda item: list(item.path))
    if not errors:
        return
    details = "; ".join(
        f"{'.'.join(str(part) for part in error.absolute_path) or '<root>'}: {error.message}"
        for error in errors
    )
    raise CollectionsError(f"{label} does not match its schema: {details}")


def _unique(values: list[str], label: str) -> None:
    repeated = sorted({value for value in values if values.count(value) > 1})
    if repeated:
        raise CollectionsError(f"duplicate {label}: {', '.join(repeated)}")


def generate_collections(
    manifest: dict[str, Any], matrix: dict[str, Any]
) -> list[dict[str, Any]]:
    organizations = manifest.get("organizations", [])
    for field in ("mspId", "slug", "id", "peerHostname"):
        _unique(
            [organization[field] for organization in organizations],
            f"organization {field}",
        )
    _unique(
        [
            organization["ordererHostname"]
            for organization in organizations
            if "ordererHostname" in organization
        ],
        "ordererHostname",
    )
    regulators = [
        organization
        for organization in organizations
        if organization["agentType"] == "REGULATOR"
    ]
    if len(regulators) != 1:
        raise CollectionsError(
            f"expected exactly one REGULATOR organization, found {len(regulators)}"
        )
    regulator_msp = regulators[0]["mspId"]

    agent_codes = [agent_type["code"] for agent_type in matrix.get("agentTypes", [])]
    _unique(agent_codes, "matrix agent type")
    custodial_types = set(agent_codes)
    reference_ids = [
        reference["id"] for reference in matrix.get("normativeReferences", [])
    ]
    _unique(reference_ids, "normative reference id")
    known_references = set(reference_ids)

    rule_ids: list[str] = []
    directed_pairs: list[tuple[str, str]] = []
    for rule in matrix.get("authorizedTransfers", []):
        rule_ids.append(rule["id"])
        origin = rule["origin"]
        destination = rule["destination"]
        if origin not in custodial_types or destination not in custodial_types:
            raise CollectionsError(
                f"rule {rule['id']} references an agent type outside agentTypes"
            )
        missing_references = sorted(
            set(rule["normativeReferences"]) - known_references
        )
        if missing_references:
            raise CollectionsError(
                f"rule {rule['id']} has unknown normative reference: "
                f"{', '.join(missing_references)}"
            )
        directed_pairs.append((origin, destination))
    duplicate_pairs = sorted(
        {pair for pair in directed_pairs if directed_pairs.count(pair) > 1}
    )
    if duplicate_pairs:
        rendered = ", ".join(
            f"{origin}->{destination}" for origin, destination in duplicate_pairs
        )
        raise CollectionsError(f"duplicate authorized-transfer pair: {rendered}")

    authorized = set(directed_pairs)
    prohibited: set[tuple[str, str]] = set()
    for rule in matrix.get("prohibitedTransfers", []):
        rule_ids.append(rule["id"])
        missing_references = sorted(
            set(rule["normativeReferences"]) - known_references
        )
        if missing_references:
            raise CollectionsError(
                f"rule {rule['id']} has unknown normative reference: "
                f"{', '.join(missing_references)}"
            )
        origins = custodial_types if rule["origins"] == "*" else set(rule["origins"])
        destinations = (
            custodial_types
            if rule["destinations"] == "*"
            else set(rule["destinations"])
        )
        prohibited.update(
            (origin, destination)
            for origin in origins
            for destination in destinations
        )
    _unique(rule_ids, "transfer rule id")
    contradictions = sorted(authorized & prohibited)
    if contradictions:
        rendered = ", ".join(
            f"{origin}->{destination}" for origin, destination in contradictions
        )
        raise CollectionsError(f"contradictory transfer rules: {rendered}")

    custodial_organizations = sorted(
        (
            organization
            for organization in organizations
            if organization["agentType"] in custodial_types
        ),
        key=lambda organization: organization["mspId"],
    )
    collections: list[dict[str, Any]] = []
    names: set[str] = set()
    for first, second in combinations(custodial_organizations, 2):
        type_pair = (first["agentType"], second["agentType"])
        if type_pair not in authorized and tuple(reversed(type_pair)) not in authorized:
            continue
        first_msp, second_msp = sorted((first["mspId"], second["mspId"]))
        name = f"transfer_{first_msp}_{second_msp}"
        if name in names:
            raise CollectionsError(f"collection-name collision: {name}")
        names.add(name)
        collections.append(
            {
                "name": name,
                "policy": (
                    f"OR('{first_msp}.member','{second_msp}.member',"
                    f"'{regulator_msp}.member')"
                ),
                "requiredPeerCount": 1,
                "maxPeerCount": 2,
                "blockToLive": 0,
                "memberOnlyRead": True,
                "memberOnlyWrite": True,
                "endorsementPolicy": {
                    "signaturePolicy": f"OR('{first_msp}.peer','{second_msp}.peer')"
                },
            }
        )
    return sorted(collections, key=lambda collection: collection["name"])


def render_collections(collections: list[dict[str, Any]]) -> str:
    return json.dumps(collections, ensure_ascii=False, indent=2) + "\n"


def check_output(path: Path, expected: str) -> None:
    try:
        actual = path.read_text(encoding="utf-8")
    except OSError as exc:
        raise CollectionsError(f"cannot read generated output {path}: {exc}") from exc
    if actual != expected:
        raise CollectionsError(
            f"{path} is stale; run network/scripts/generate-collections.py"
        )


def main() -> int:
    args = parse_args()
    try:
        manifest = load_json(args.manifest)
        manifest_schema = load_json(args.manifest_schema)
        matrix = load_json(args.matrix)
        matrix_schema = load_json(args.matrix_schema)
        validate_json(manifest, manifest_schema, "organization manifest")
        validate_json(matrix, matrix_schema, "authorized-transfer matrix")
        rendered = render_collections(generate_collections(manifest, matrix))
        if args.check:
            check_output(args.output, rendered)
            print(f"OK: {args.output} is deterministic and up to date")
        else:
            args.output.parent.mkdir(parents=True, exist_ok=True)
            args.output.write_text(rendered, encoding="utf-8")
            print(f"OK: generated {args.output}")
        return 0
    except CollectionsError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
