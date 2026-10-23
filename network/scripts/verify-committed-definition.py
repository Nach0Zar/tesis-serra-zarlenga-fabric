#!/usr/bin/env python3
"""Compare a Fabric lifecycle definition with the versioned NET-4/NET-5 inputs."""

from __future__ import annotations

import argparse
import base64
import binascii
import json
import re
import sys
from pathlib import Path
from typing import Any


class VerificationError(ValueError):
    """Raised when a committed or approved definition has semantic drift."""


def load_json(path: Path) -> Any:
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise VerificationError(f"cannot read JSON {path}: {exc}") from exc


def read_varint(data: bytes, offset: int) -> tuple[int, int]:
    value = 0
    shift = 0
    while offset < len(data) and shift < 64:
        byte = data[offset]
        offset += 1
        value |= (byte & 0x7F) << shift
        if byte < 0x80:
            return value, offset
        shift += 7
    raise VerificationError("invalid MSPRole protobuf varint")


def decode_msp_role(encoded: str, context: str) -> tuple[str, int]:
    try:
        data = base64.b64decode(encoded, validate=True)
    except (ValueError, binascii.Error) as exc:
        raise VerificationError(f"{context}: invalid base64 principal") from exc

    offset = 0
    msp_id: str | None = None
    role = 0
    while offset < len(data):
        key, offset = read_varint(data, offset)
        field_number = key >> 3
        wire_type = key & 0x07
        if field_number == 1 and wire_type == 2:
            length, offset = read_varint(data, offset)
            end = offset + length
            if end > len(data):
                raise VerificationError(f"{context}: truncated MSP identifier")
            try:
                msp_id = data[offset:end].decode("utf-8")
            except UnicodeDecodeError as exc:
                raise VerificationError(f"{context}: MSP identifier is not UTF-8") from exc
            offset = end
        elif field_number == 2 and wire_type == 0:
            role, offset = read_varint(data, offset)
        else:
            raise VerificationError(
                f"{context}: unsupported MSPRole field {field_number}/{wire_type}"
            )
    if not msp_id:
        raise VerificationError(f"{context}: principal has no MSP identifier")
    return msp_id, role


def normalize_signature_policy(policy: dict[str, Any], context: str) -> dict[str, Any]:
    identities = policy.get("identities")
    rule = policy.get("rule")
    if not isinstance(identities, list) or not isinstance(rule, dict):
        raise VerificationError(f"{context}: malformed signature policy")

    principals: list[tuple[str, int]] = []
    for index, identity in enumerate(identities):
        if not isinstance(identity, dict) or not isinstance(identity.get("principal"), str):
            raise VerificationError(f"{context}: malformed identity {index}")
        classification = identity.get("principal_classification")
        if classification not in (None, "ROLE"):
            raise VerificationError(
                f"{context}: identity {index} is not an MSP role principal"
            )
        principals.append(
            decode_msp_role(identity["principal"], f"{context} identity {index}")
        )

    n_out_of = rule.get("n_out_of")
    if n_out_of is None:
        n_out_of = rule.get("Type", {}).get("NOutOf")
    if not isinstance(n_out_of, dict):
        raise VerificationError(f"{context}: expected an NOutOf rule")
    threshold = n_out_of.get("n")
    rules = n_out_of.get("rules")
    if not isinstance(threshold, int) or not isinstance(rules, list):
        raise VerificationError(f"{context}: malformed NOutOf rule")

    signed_by: list[int] = []
    for child in rules:
        if not isinstance(child, dict):
            raise VerificationError(f"{context}: malformed child rule")
        if "signed_by" in child:
            index = child["signed_by"]
        else:
            index = child.get("Type", {}).get("SignedBy")
        if not isinstance(index, int):
            raise VerificationError(f"{context}: nested policies are not expected")
        signed_by.append(index)

    expected_indexes = list(range(len(principals)))
    if sorted(signed_by) != expected_indexes:
        raise VerificationError(
            f"{context}: rules must reference every identity exactly once"
        )
    selected = [principals[index] for index in signed_by]
    if len(set(selected)) != len(selected):
        raise VerificationError(f"{context}: duplicate MSP role principal")
    return {"threshold": threshold, "principals": sorted(selected)}


def parse_dsl_policy(value: Any, role_name: str, role_number: int, context: str) -> dict[str, Any]:
    if not isinstance(value, str):
        raise VerificationError(f"{context}: policy must be a string")
    matches = re.findall(r"'([^']+)\.(member|peer)'", value)
    if not matches:
        raise VerificationError(f"{context}: unsupported policy {value!r}")
    canonical = "OR(" + ",".join(f"'{msp}.{role}'" for msp, role in matches) + ")"
    if value != canonical or any(role != role_name for _, role in matches):
        raise VerificationError(f"{context}: unsupported policy {value!r}")
    principals = [(msp, role_number) for msp, _ in matches]
    if len(set(principals)) != len(principals):
        raise VerificationError(f"{context}: duplicate principal")
    return {"threshold": 1, "principals": sorted(principals)}


def expected_collections(config: Any) -> list[dict[str, Any]]:
    if not isinstance(config, list):
        raise VerificationError("collections config must be an array")
    result: list[dict[str, Any]] = []
    for index, item in enumerate(config):
        if not isinstance(item, dict) or not isinstance(item.get("name"), str):
            raise VerificationError(f"collections config item {index} is malformed")
        name = item["name"]
        endorsement = item.get("endorsementPolicy", {})
        result.append(
            {
                "name": name,
                "required_peer_count": item.get("requiredPeerCount"),
                "maximum_peer_count": item.get("maxPeerCount"),
                "block_to_live": item.get("blockToLive", 0),
                "member_only_read": item.get("memberOnlyRead"),
                "member_only_write": item.get("memberOnlyWrite"),
                "member_policy": parse_dsl_policy(
                    item.get("policy"), "member", 0, f"collection {name} membership"
                ),
                "endorsement_policy": parse_dsl_policy(
                    endorsement.get("signaturePolicy"),
                    "peer",
                    3,
                    f"collection {name} endorsement",
                ),
            }
        )
    names = [item["name"] for item in result]
    if names != sorted(names) or len(set(names)) != len(names):
        raise VerificationError("expected collections are not unique and sorted")
    return result


def actual_collections(definition: dict[str, Any]) -> list[dict[str, Any]]:
    entries = definition.get("collections", {}).get("config")
    if not isinstance(entries, list):
        raise VerificationError("lifecycle definition has no static collection config")
    result: list[dict[str, Any]] = []
    for index, entry in enumerate(entries):
        static = entry.get("Payload", {}).get("StaticCollectionConfig")
        if not isinstance(static, dict) or not isinstance(static.get("name"), str):
            raise VerificationError(f"committed collection item {index} is malformed")
        name = static["name"]
        member_policy = (
            static.get("member_orgs_policy", {})
            .get("Payload", {})
            .get("SignaturePolicy")
        )
        endorsement_policy = static.get("endorsement_policy", {}).get("Type", {}).get(
            "SignaturePolicy"
        )
        if not isinstance(member_policy, dict) or not isinstance(
            endorsement_policy, dict
        ):
            raise VerificationError(f"collection {name}: missing signature policy")
        result.append(
            {
                "name": name,
                "required_peer_count": static.get("required_peer_count"),
                "maximum_peer_count": static.get("maximum_peer_count"),
                "block_to_live": static.get("block_to_live", 0),
                "member_only_read": static.get("member_only_read"),
                "member_only_write": static.get("member_only_write"),
                "member_policy": normalize_signature_policy(
                    member_policy, f"collection {name} membership"
                ),
                "endorsement_policy": normalize_signature_policy(
                    endorsement_policy, f"collection {name} endorsement"
                ),
            }
        )
    return result


def expected_chaincode_policy(
    manifest: dict[str, Any], matrix: dict[str, Any], policy_kind: str
) -> dict[str, Any]:
    organizations = manifest.get("organizations")
    if not isinstance(organizations, list):
        raise VerificationError("manifest has no organizations")
    if policy_kind == "bootstrap":
        msp_ids = [organization.get("mspId") for organization in organizations]
        threshold = len(msp_ids)
    else:
        agent_types = matrix.get("agentTypes")
        if not isinstance(agent_types, list):
            raise VerificationError("transfer matrix has no agentTypes")
        custodial = {item.get("code") for item in agent_types}
        msp_ids = [
            organization.get("mspId")
            for organization in organizations
            if organization.get("agentType") == "REGULATOR"
            or organization.get("agentType") in custodial
        ]
        threshold = 1
    if not msp_ids or any(not isinstance(msp_id, str) for msp_id in msp_ids):
        raise VerificationError(f"cannot derive {policy_kind} chaincode policy")
    if len(set(msp_ids)) != len(msp_ids):
        raise VerificationError("manifest contains duplicate MSP identifiers")
    return {
        "threshold": threshold,
        "principals": sorted((msp_id, 3) for msp_id in msp_ids),
    }


def compare_collections(actual: list[dict[str, Any]], expected: list[dict[str, Any]]) -> None:
    actual_names = [item["name"] for item in actual]
    expected_names = [item["name"] for item in expected]
    if actual_names != expected_names:
        raise VerificationError(
            f"collection names/order differ: expected {expected_names}, got {actual_names}"
        )
    for actual_item, expected_item in zip(actual, expected, strict=True):
        name = expected_item["name"]
        for field, expected_value in expected_item.items():
            if field == "name":
                continue
            actual_value = actual_item[field]
            if actual_value != expected_value:
                raise VerificationError(
                    f"collection {name} field {field} differs: "
                    f"expected {expected_value!r}, got {actual_value!r}"
                )


def verify_definition(
    definition: dict[str, Any],
    decoded_policy: dict[str, Any],
    collections_config: Any,
    manifest: dict[str, Any],
    matrix: dict[str, Any],
    sequence: int,
    version: str,
    init_required: bool,
    policy_kind: str,
) -> None:
    actual_init_required = definition.get("init_required", False)
    expected_scalars = {
        "sequence": sequence,
        "version": version,
        "init_required": init_required,
    }
    actual_scalars = {
        "sequence": definition.get("sequence"),
        "version": definition.get("version"),
        "init_required": actual_init_required,
    }
    if actual_scalars != expected_scalars:
        raise VerificationError(
            f"lifecycle scalars differ: expected {expected_scalars}, got {actual_scalars}"
        )

    compare_collections(
        actual_collections(definition), expected_collections(collections_config)
    )
    signature_policy = decoded_policy.get("signature_policy")
    if not isinstance(signature_policy, dict):
        raise VerificationError("validation parameter is not a signature policy")
    actual_policy = normalize_signature_policy(
        signature_policy, "chaincode endorsement policy"
    )
    expected_policy = expected_chaincode_policy(manifest, matrix, policy_kind)
    if actual_policy != expected_policy:
        raise VerificationError(
            f"chaincode endorsement policy differs: expected {expected_policy!r}, "
            f"got {actual_policy!r}"
        )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--definition", type=Path, required=True)
    parser.add_argument("--decoded-policy", type=Path, required=True)
    parser.add_argument("--collections", type=Path, required=True)
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--matrix", type=Path, required=True)
    parser.add_argument("--sequence", type=int, required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--init-required", choices=("true", "false"), required=True)
    parser.add_argument(
        "--policy-kind", choices=("bootstrap", "operational"), required=True
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        verify_definition(
            load_json(args.definition),
            load_json(args.decoded_policy),
            load_json(args.collections),
            load_json(args.manifest),
            load_json(args.matrix),
            args.sequence,
            args.version,
            args.init_required == "true",
            args.policy_kind,
        )
    except VerificationError as exc:
        print(f"ERROR: lifecycle definition mismatch: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
