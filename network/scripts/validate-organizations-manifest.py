#!/usr/bin/env python3
"""Validate the deployment manifest and its agreement with configtx.yaml."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any

try:
    import jsonschema
    import yaml
except ImportError as exc:  # pragma: no cover - dependency preflight
    print(
        f"ERROR: missing Python dependency {exc.name!r}; install jsonschema and PyYAML",
        file=sys.stderr,
    )
    raise SystemExit(2) from exc


FOUNDATIONAL_MSPS = {
    "AnmatMSP",
    "LabMSP",
    "DrogueriaMSP",
    "DistribuidorMSP",
    "FarmaciaMSP",
    "CentroMedicoMSP",
    "FinanciadorMSP",
}
ORDERER_MSPS = {"AnmatMSP", "LabMSP", "DrogueriaMSP"}


def parse_args() -> argparse.Namespace:
    script_dir = Path(__file__).resolve().parent
    network_dir = script_dir.parent
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--manifest",
        type=Path,
        default=network_dir / "organizations-manifest.json",
    )
    parser.add_argument(
        "--schema",
        type=Path,
        default=network_dir / "organizations-manifest.schema.json",
    )
    parser.add_argument(
        "--configtx",
        type=Path,
        default=network_dir / "configtx.yaml",
    )
    return parser.parse_args()


def load_json(path: Path) -> dict[str, Any]:
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError(f"cannot read JSON {path}: {exc}") from exc


def load_yaml(path: Path) -> dict[str, Any]:
    try:
        value = yaml.safe_load(path.read_text(encoding="utf-8"))
    except (OSError, yaml.YAMLError) as exc:
        raise ValueError(f"cannot read YAML {path}: {exc}") from exc
    if not isinstance(value, dict):
        raise ValueError(f"YAML root in {path} must be an object")
    return value


def gs1_check_digit_is_valid(value: str) -> bool:
    if len(value) != 13 or not value.isdigit():
        return False
    digits = [int(character) for character in value]
    weighted_sum = sum(
        digit * (3 if offset % 2 == 0 else 1)
        for offset, digit in enumerate(reversed(digits[:-1]))
    )
    expected = (10 - weighted_sum % 10) % 10
    return digits[-1] == expected


def duplicates(values: list[str]) -> set[str]:
    return {value for value in values if values.count(value) > 1}


def validate_manifest_invariants(manifest: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    organizations = manifest["organizations"]

    unique_fields = {
        "mspId": [organization["mspId"] for organization in organizations],
        "slug": [organization["slug"] for organization in organizations],
        "identity": [
            f'{organization["idType"]}:{organization["id"]}'
            for organization in organizations
        ],
        "peerHostname": [
            organization["peerHostname"] for organization in organizations
        ],
    }
    for field, values in unique_fields.items():
        repeated = duplicates(values)
        if repeated:
            errors.append(f"duplicate {field}: {', '.join(sorted(repeated))}")

    orderer_hosts = [
        organization["ordererHostname"]
        for organization in organizations
        if "ordererHostname" in organization
    ]
    repeated_orderers = duplicates(orderer_hosts)
    if repeated_orderers:
        errors.append(
            f"duplicate ordererHostname: {', '.join(sorted(repeated_orderers))}"
        )

    msp_ids = {organization["mspId"] for organization in organizations}
    missing_foundational = FOUNDATIONAL_MSPS - msp_ids
    if missing_foundational:
        errors.append(
            "missing foundational MSPs: " + ", ".join(sorted(missing_foundational))
        )

    regulators = [
        organization
        for organization in organizations
        if organization["agentType"] == "REGULATOR"
    ]
    if len(regulators) != 1:
        errors.append(f"expected exactly one REGULATOR, found {len(regulators)}")

    orderer_msps = {
        organization["mspId"]
        for organization in organizations
        if "ordererHostname" in organization
    }
    if orderer_msps != ORDERER_MSPS:
        errors.append(
            "orderer MSPs must be exactly "
            f"{', '.join(sorted(ORDERER_MSPS))}; found "
            f"{', '.join(sorted(orderer_msps))}"
        )

    for organization in organizations:
        slug = organization["slug"]
        expected_peer = f"peer0.{slug}.snt.local"
        if organization["peerHostname"] != expected_peer:
            errors.append(
                f'{organization["mspId"]}: peerHostname must be {expected_peer}'
            )
        if "ordererHostname" in organization:
            expected_orderer = f"orderer.{slug}.snt.local"
            if organization["ordererHostname"] != expected_orderer:
                errors.append(
                    f'{organization["mspId"]}: ordererHostname must be '
                    f"{expected_orderer}"
                )
        if organization["idType"] in {"GLN", "CUFE"} and not gs1_check_digit_is_valid(
            organization["id"]
        ):
            errors.append(
                f'{organization["mspId"]}: invalid GS1 check digit in '
                f'{organization["idType"]} {organization["id"]}'
            )

    return errors


def validate_configtx(
    manifest: dict[str, Any], configtx: dict[str, Any]
) -> list[str]:
    errors: list[str] = []
    organizations = manifest["organizations"]
    by_msp = {organization["mspId"]: organization for organization in organizations}

    definitions = configtx.get("Organizations", [])
    application_definitions = {
        definition["ID"]: definition
        for definition in definitions
        if "AnchorPeers" in definition
    }
    orderer_definitions = {
        definition["ID"]: definition
        for definition in definitions
        if "OrdererEndpoints" in definition
    }

    if set(application_definitions) != set(by_msp):
        errors.append(
            "configtx application MSPs differ from manifest: "
            f"configtx={sorted(application_definitions)}, manifest={sorted(by_msp)}"
        )

    expected_orderers = {
        msp_id
        for msp_id, organization in by_msp.items()
        if "ordererHostname" in organization
    }
    if set(orderer_definitions) != expected_orderers:
        errors.append(
            "configtx orderer MSPs differ from manifest: "
            f"configtx={sorted(orderer_definitions)}, manifest={sorted(expected_orderers)}"
        )

    for msp_id, organization in by_msp.items():
        definition = application_definitions.get(msp_id)
        if definition is None:
            continue
        expected_msp_dir = f'organizations/{organization["slug"]}/msp'
        if definition.get("MSPDir") != expected_msp_dir:
            errors.append(
                f"{msp_id}: MSPDir must be {expected_msp_dir}, "
                f"found {definition.get('MSPDir')}"
            )
        anchor_hosts = {
            anchor.get("Host") for anchor in definition.get("AnchorPeers", [])
        }
        if anchor_hosts != {organization["peerHostname"]}:
            errors.append(
                f"{msp_id}: AnchorPeers hosts {sorted(anchor_hosts)} do not match "
                f'{organization["peerHostname"]}'
            )

    for msp_id in expected_orderers:
        organization = by_msp[msp_id]
        definition = orderer_definitions.get(msp_id)
        if definition is None:
            continue
        expected_msp_dir = f'organizations/{organization["slug"]}/msp'
        if definition.get("MSPDir") != expected_msp_dir:
            errors.append(
                f"{msp_id} orderer: MSPDir must be {expected_msp_dir}, "
                f"found {definition.get('MSPDir')}"
            )
        endpoint_hosts = {
            str(endpoint).rsplit(":", 1)[0]
            for endpoint in definition.get("OrdererEndpoints", [])
        }
        if endpoint_hosts != {organization["ordererHostname"]}:
            errors.append(
                f"{msp_id}: OrdererEndpoints hosts {sorted(endpoint_hosts)} do not "
                f'match {organization["ordererHostname"]}'
            )

    profile = configtx.get("Profiles", {}).get("SNTChannelGenesis", {})
    profile_application_msps = {
        definition.get("ID")
        for definition in profile.get("Application", {}).get("Organizations", [])
    }
    profile_orderer_msps = {
        definition.get("ID")
        for definition in profile.get("Orderer", {}).get("Organizations", [])
    }
    if profile_application_msps != set(by_msp):
        errors.append("SNTChannelGenesis application organizations differ from manifest")
    if profile_orderer_msps != expected_orderers:
        errors.append("SNTChannelGenesis orderer organizations differ from manifest")

    consenters = profile.get("Orderer", {}).get("EtcdRaft", {}).get("Consenters", [])
    consenter_hosts = {consenter.get("Host") for consenter in consenters}
    expected_hosts = {
        by_msp[msp_id]["ordererHostname"] for msp_id in expected_orderers
    }
    if consenter_hosts != expected_hosts:
        errors.append(
            f"etcdraft consenters {sorted(consenter_hosts)} do not match manifest "
            f"{sorted(expected_hosts)}"
        )
    for consenter in consenters:
        host = consenter.get("Host")
        organization = next(
            (
                item
                for item in organizations
                if item.get("ordererHostname") == host
            ),
            None,
        )
        if organization is None:
            continue
        expected_tls = (
            f'organizations/{organization["slug"]}/orderers/{host}/tls/server.crt'
        )
        for field in ("ClientTLSCert", "ServerTLSCert"):
            if consenter.get(field) != expected_tls:
                errors.append(
                    f"{host}: {field} must be {expected_tls}, "
                    f"found {consenter.get(field)}"
                )

    return errors


def main() -> int:
    args = parse_args()
    try:
        manifest = load_json(args.manifest)
        schema = load_json(args.schema)
        configtx = load_yaml(args.configtx)
    except ValueError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1

    try:
        jsonschema.Draft202012Validator.check_schema(schema)
    except jsonschema.SchemaError as exc:
        print(f"ERROR: invalid JSON Schema: {exc.message}", file=sys.stderr)
        return 1

    validator_instance = jsonschema.Draft202012Validator(schema)
    errors = [
        f"schema {'.'.join(str(part) for part in error.absolute_path) or '<root>'}: "
        f"{error.message}"
        for error in sorted(
            validator_instance.iter_errors(manifest), key=lambda item: list(item.path)
        )
    ]
    if not errors:
        errors.extend(validate_manifest_invariants(manifest))
        errors.extend(validate_configtx(manifest, configtx))

    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1

    print(
        f"OK: {args.manifest} validates against schema and {args.configtx}",
        file=sys.stdout,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
