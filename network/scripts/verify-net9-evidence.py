#!/usr/bin/env python3
"""Validate NET-9 evidence against exact Fabric transactions and hashed keys."""
from __future__ import annotations

import argparse
import hashlib
import json
import sys
from pathlib import Path

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

from endorsement_evidence import (
    decode_hashed_rwset,
    load_json,
    private_key_hash,
    require,
    transaction_evidence,
)

GTIN = "07791234567898"

TRANSACTIONS = {
    "transit-quarantine-missing-sender": 10,
    "transit-quarantine-missing-receiver": 10,
    "transit-quarantine-missing-regulator": 10,
    "transit-quarantine": 0,
    "transit-expired": 0,
    "transit-stolen": 0,
    "transit-lost": 0,
    "transit-damaged": 0,
    "regulatory-quarantine-missing-regulator": 10,
    "regulatory-quarantine-missing-custodian": 10,
    "regulatory-quarantine": 0,
    "regulatory-release": 0,
    "regulatory-withdraw": 0,
    "regulatory-restock": 0,
    "regulatory-prohibit": 0,
    "regulatory-return": 0,
    "regulatory-final-quarantine": 0,
    "regulatory-final": 0,
    "lab-authorize-expired": 0,
    "lab-authorize-after-expiry": 0,
    "lab-revoke": 0,
    "lab-authorize-after-revoke": 0,
    "lab-authorize-replacement": 0,
    "lab-withdraw-missing-lab": 10,
    "lab-withdraw-missing-regulator": 10,
    "lab-withdraw-missing-custodian": 10,
    "lab-withdraw": 0,
    "lab-authorize-restock": 0,
    "lab-restock-missing-lab": 10,
    "lab-restock-missing-regulator": 10,
    "lab-restock-missing-custodian": 10,
    "lab-restock": 0,
    "lab-prep-regulatory-withdraw": 0,
    "lab-authorize-final": 0,
    "lab-final-missing-lab": 10,
    "lab-final-missing-regulator": 10,
    "lab-final-missing-custodian": 10,
    "lab-final": 0,
}

TRANSIT = {
    "transit-quarantine": ("N9TQ", "EN_CUARENTENA"),
    "transit-expired": ("N9TE", "VENCIDO"),
    "transit-stolen": ("N9TS", "ROBADO"),
    "transit-lost": ("N9TL", "EXTRAVIADO"),
    "transit-damaged": ("N9TD", "DETERIORADO"),
}

SBE = {
    **{label: ["LabMSP"] for label in TRANSIT},
    "lab-authorize-expired": ["AnmatMSP"],
    "lab-authorize-after-expiry": ["AnmatMSP"],
    "lab-authorize-after-revoke": ["AnmatMSP"],
    "lab-authorize-replacement": ["AnmatMSP"],
    "lab-authorize-restock": ["AnmatMSP"],
    "lab-authorize-final": ["AnmatMSP"],
}

REGULATORY_MARKERS = {
    **{label: prefix for label, (prefix, _state) in TRANSIT.items()},
    "regulatory-quarantine": "N9RA",
    "regulatory-release": "N9RA",
    "regulatory-withdraw": "N9RA",
    "regulatory-restock": "N9RA",
    "regulatory-prohibit": "N9RA",
    "regulatory-return": "N9RA",
    "regulatory-final-quarantine": "N9RF",
    "regulatory-final": "N9RF",
    "lab-authorize-expired": "N9LI",
    "lab-authorize-after-expiry": "N9LI",
    "lab-revoke": "N9LI",
    "lab-authorize-after-revoke": "N9LI",
    "lab-authorize-replacement": "N9LI",
    "lab-authorize-restock": "N9LI",
    "lab-prep-regulatory-withdraw": "N9LI",
    "lab-authorize-final": "N9LI",
    "lab-withdraw": "N9LI",
    "lab-restock": "N9LI",
    "lab-final": "N9LI",
}

LAB_MARKERS = {
    "lab-withdraw": "N9LI",
    "lab-restock": "N9LI",
    "lab-final": "N9LI",
}

SANITIZED = {
    "transit-quarantine-regulator-marker-sanitized.json": "transit-quarantine",
    "transit-quarantine-transfer-sanitized.json": "transit-quarantine",
    "lab-withdraw-lab-marker-sanitized.json": "lab-withdraw",
    "lab-withdraw-regulator-marker-sanitized.json": "lab-withdraw",
}


def serial(prefix: str, run_token: str) -> str:
    return prefix + run_token


def validate_sbe(policy: dict, expected: list[str], label: str) -> None:
    identities = policy["identities"]
    actual = [item["principal"]["msp_identifier"] for item in identities]
    require(sorted(actual) == sorted(expected), f"{label}: wrong SBE principals")
    require(
        all(
            item["principal_classification"] == "ROLE"
            and item["principal"]["role"] == "PEER"
            for item in identities
        ),
        f"{label}: wrong principal role",
    )
    rule = policy["rule"]["n_out_of"]
    require(int(rule["n"]) == len(expected), f"{label}: wrong SBE threshold")
    require(
        sorted(int(item["signed_by"]) for item in rule["rules"])
        == list(range(len(expected))),
        f"{label}: wrong SBE signed_by references",
    )


def hashed_writes(value: dict, label: str) -> list[dict]:
    writes = value.get("hashed_writes", [])
    require(isinstance(writes, list), f"{label}: malformed hashed writes")
    return writes


def validate_marker(
    directory: Path,
    label: str,
    owner_msp: str,
    unit_serial: str,
    txid: str,
) -> None:
    path = directory / f"{label}-marker-{owner_msp}.json"
    writes = hashed_writes(load_json(path), label)
    require(len(writes) == 1, f"{label}: expected exactly one marker write")
    write = writes[0]
    expected = private_key_hash("Participacion", "Unidad", GTIN, unit_serial, txid)
    require(write["key_hash"] == expected, f"{label}: wrong marker key hash for {owner_msp}")
    require(write.get("is_delete", False) is False, f"{label}: marker was deleted")
    require(bool(write.get("value_hash")), f"{label}: marker has no value hash")


def validate_transfer_closure(
    directory: Path, label: str, unit_serial: str
) -> None:
    dispatch_txid = (directory / f"{label}-dispatch-txid.txt").read_text().strip()
    require(
        len(dispatch_txid) == 64
        and all(char in "0123456789abcdef" for char in dispatch_txid),
        f"{label}: invalid dispatch transaction ID",
    )
    writes = hashed_writes(
        load_json(directory / f"{label}-transfer-hashed-rwset.json"), label
    )
    require(len(writes) == 2, f"{label}: expected historical put and active delete")
    by_hash = {item["key_hash"]: item for item in writes}
    historical = private_key_hash("TransferOp", GTIN, unit_serial, dispatch_txid)
    active = private_key_hash("TransferOpActive", GTIN, unit_serial)
    require(set(by_hash) == {historical, active}, f"{label}: unexpected private keys")
    require(
        by_hash[historical].get("is_delete", False) is False
        and bool(by_hash[historical].get("value_hash")),
        f"{label}: historical operation was not written",
    )
    require(
        by_hash[active].get("is_delete") is True,
        f"{label}: active operation was not deleted",
    )


def artifact_manifest(directory: Path) -> dict:
    artifacts = {}
    for path in sorted(directory.iterdir()):
        if path.name in {"artifacts.json", "result.json"} or not path.is_file():
            continue
        data = path.read_bytes()
        require(bool(data), f"empty artifact: {path.name}")
        artifacts[path.name] = {
            "sha256": hashlib.sha256(data).hexdigest(),
            "bytes": len(data),
        }
    return {"schemaVersion": "1.0.0", "artifacts": artifacts}


def verify_run(directory: Path, expected_channel: str = "snt-channel") -> dict:
    metadata = load_json(directory / "run-metadata.json")
    require(metadata["schemaVersion"] == "1.0.0", "wrong metadata schema")
    run_token = metadata["runToken"]
    require(
        isinstance(run_token, str)
        and 1 <= len(run_token) <= 14
        and run_token.isalnum(),
        "invalid run token",
    )

    seen = set()
    ids = {}
    for label, code in TRANSACTIONS.items():
        txid = (directory / f"{label}-txid.txt").read_text().strip()
        require(
            len(txid) == 64 and all(char in "0123456789abcdef" for char in txid),
            f"{label}: invalid transaction ID",
        )
        require(txid not in seen, f"{label}: transaction reused across scenarios")
        seen.add(txid)
        ids[label] = txid
        evidence = transaction_evidence(
            load_json(directory / f"{label}-block.json"),
            txid,
            code,
            expected_channel,
        )
        require(
            load_json(directory / f"{label}-transaction.json") == evidence,
            f"{label}: transaction evidence mismatch",
        )
        status = int((directory / f"{label}-status.txt").read_text())
        require((status == 0) == (code == 0), f"{label}: CLI status mismatch")
        log = (directory / f"{label}-invoke.txt").read_text()
        require(f"txid [{txid}]" in log, f"{label}: invocation does not identify transaction")
        if code == 10:
            require(
                "ENDORSEMENT_POLICY_FAILURE" in log,
                f"{label}: missing platform rejection",
            )

    for label, expected in SBE.items():
        validate_sbe(load_json(directory / f"{label}-sbe.json"), expected, label)

    for label, (prefix, expected_state) in TRANSIT.items():
        unit_serial = serial(prefix, run_token)
        validate_transfer_closure(directory, label, unit_serial)
        unit = load_json(directory / f"{label}-unit.json")
        require(unit["estado"] == expected_state, f"{label}: wrong resulting state")

    for label, prefix in REGULATORY_MARKERS.items():
        validate_marker(
            directory,
            label,
            "AnmatMSP",
            serial(prefix, run_token),
            ids[label],
        )
    for label, prefix in LAB_MARKERS.items():
        validate_marker(
            directory,
            label,
            "LabMSP",
            serial(prefix, run_token),
            ids[label],
        )

    for name, label in SANITIZED.items():
        excerpt = load_json(directory / name)
        require(excerpt["transactionId"] == ids[label], f"{name}: wrong transaction")
        require(
            excerpt["assertions"]
            == {"collectionNameVisible": True, "privatePayloadIncluded": False},
            f"{name}: unexpected privacy assertions",
        )

    expired_history = load_json(directory / "lab-history-expired.json")
    require(
        len(expired_history) == 1
        and expired_history[0]["isDelete"] is False
        and expired_history[0]["value"]["estado"] == "ACTIVA",
        "expiration must remain a derived condition without a synthetic history entry",
    )
    final_history = load_json(directory / "lab-history-final.json")
    expected_states = [
        "ACTIVA",
        "ACTIVA",
        "REVOCADA",
        "ACTIVA",
        "ACTIVA",
        "CONSUMIDA",
        "ACTIVA",
        "CONSUMIDA",
        "ACTIVA",
        "CONSUMIDA",
    ]
    expected_operations = (
        ["WITHDRAW_FROM_MARKET"] * 6
        + ["RESTOCK"] * 2
        + ["FINAL_DISPOSITION"] * 2
    )
    require(
        [entry["value"]["estado"] for entry in final_history] == expected_states,
        "unexpected LabIntervention state history",
    )
    require(
        [entry["value"]["operacion"] for entry in final_history]
        == expected_operations,
        "unexpected LabIntervention operation history",
    )
    require(
        all(entry["isDelete"] is False and entry["value"] is not None for entry in final_history),
        "LabIntervention history contains a deletion or empty snapshot",
    )

    logic_log = (directory / "lab-expired-operation-invoke.txt").read_text()
    require("LAB_INTERVENTION_REQUIRED" in logic_log, "missing logic rejection")
    require(
        int((directory / "lab-expired-operation-status.txt").read_text()) != 0,
        "expired authorization unexpectedly succeeded",
    )
    require(
        int((directory / "lab-expired-operation-unchanged-height.txt").read_text()) > 0,
        "invalid unchanged height for logic rejection",
    )
    require(
        not (directory / "lab-expired-operation-txid.txt").exists(),
        "proposal rejection unexpectedly has an ordered transaction",
    )

    expected_units = {
        "transit-quarantine-unlocked-unit.json": "EN_CUSTODIA",
        "transit-expired-unlocked-unit.json": "DISPUESTO_FINAL",
        "transit-damaged-unlocked-unit.json": "DISPUESTO_FINAL",
        "regulatory-return-unit.json": "DEVUELTO",
        "regulatory-final-unit.json": "DISPUESTO_FINAL",
        "lab-final-unit.json": "DISPUESTO_FINAL",
    }
    for name, state in expected_units.items():
        require(load_json(directory / name)["estado"] == state, f"{name}: wrong state")

    result_path = directory / "result.json"
    if result_path.exists():
        result = load_json(result_path)
        require(result["schemaVersion"] == "1.0.0", "wrong result schema")
        require(result["runToken"] == run_token, "result run token mismatch")
        require(
            result["channel"] == expected_channel and result["chaincode"] == "snt",
            "result targets the wrong deployment",
        )
        require(
            all(value is True for value in result["assertions"].values()),
            "result contains a failed assertion",
        )

    return artifact_manifest(directory)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    transaction = commands.add_parser("transaction")
    transaction.add_argument("--block", type=Path, required=True)
    transaction.add_argument("--txid", required=True)
    transaction.add_argument("--code", type=int, choices=(0, 10), required=True)
    transaction.add_argument("--channel", default="snt-channel")
    run = commands.add_parser("run")
    run.add_argument("--directory", type=Path, required=True)
    run.add_argument("--manifest", type=Path)
    run.add_argument("--channel", default="snt-channel")
    decode = commands.add_parser("hashed-rwset")
    decode.add_argument("--input", type=Path, required=True)
    decode.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    try:
        if args.command == "transaction":
            result = transaction_evidence(
                load_json(args.block), args.txid, args.code, args.channel
            )
        elif args.command == "hashed-rwset":
            result = decode_hashed_rwset(args.input.read_bytes())
            args.output.write_text(
                json.dumps(result, indent=2, sort_keys=True) + "\n",
                encoding="utf-8",
            )
        else:
            result = verify_run(args.directory, args.channel)
            if args.manifest:
                require(
                    load_json(args.manifest) == result,
                    "artifact manifest differs from verified content",
                )
        if args.command != "hashed-rwset":
            print(json.dumps(result, indent=2, sort_keys=True))
    except (OSError, ValueError, KeyError, TypeError, IndexError) as exc:
        print(f"ERROR: NET-9 evidence: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
