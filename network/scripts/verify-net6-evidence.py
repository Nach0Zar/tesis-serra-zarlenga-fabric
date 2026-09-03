#!/usr/bin/env python3
"""Validate NET-6 evidence by transaction identity, not by block height."""
from __future__ import annotations

import argparse
import base64
import hashlib
import json
import sys
from pathlib import Path


TRANSACTIONS = {
    "registry-regulator-only": 0,
    "register-unit-regulator-only": 10,
    "register-unit-lab": 0,
    "dispatch-lab-drugstore": 0,
    "receive-one-party": 10,
    "receive-two-parties": 0,
    "receive-without-sender": 10,
    "receive-drugstore-pharmacy": 0,
    "dispense-regulator-only": 10,
    "reject-one-party": 10,
    "reject-two-parties": 0,
}
SBE = {
    "register-unit-lab": ["LabMSP"],
    "dispatch-lab-drugstore": ["LabMSP", "DrogueriaMSP"],
    "receive-two-parties": ["DrogueriaMSP"],
    "receive-drugstore-pharmacy": ["FarmaciaMSP"],
    "reject-two-parties": ["LabMSP"],
}
SANITIZED = {
    "registry-marker-sanitized.json": "registry-regulator-only",
    "register-unit-marker-sanitized.json": "register-unit-lab",
    "dispatch-lab-drugstore-sanitized.json": "dispatch-lab-drugstore",
}


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValueError(message)


def transaction_evidence(block: dict, txid: str, expected_code: int) -> dict:
    transactions = block["data"]["data"]
    matches = [
        index for index, tx in enumerate(transactions)
        if tx["payload"]["header"]["channel_header"]["tx_id"] == txid
    ]
    require(len(matches) == 1, "expected exactly one matching transaction ID")
    codes = base64.b64decode(block["metadata"]["metadata"][2], validate=True)
    require(len(codes) == len(transactions), "validation filter length mismatch")
    index = matches[0]
    header = transactions[index]["payload"]["header"]["channel_header"]
    require(header["channel_id"] == "snt-channel", "unexpected channel")
    require(codes[index] == expected_code, f"transaction {txid}: expected code {expected_code}, got {codes[index]}")
    return {
        "transactionId": txid,
        "transactionIndex": index,
        "blockNumber": int(block["header"]["number"]),
        "validationCode": codes[index],
    }


def load_json(path: Path):
    return json.loads(path.read_text(encoding="utf-8"))


def verify_run(directory: Path) -> dict:
    artifacts = {}

    def record(name: str) -> Path:
        path = directory / name
        data = path.read_bytes()
        require(bool(data), f"empty artifact: {name}")
        artifacts[name] = {"sha256": hashlib.sha256(data).hexdigest(), "bytes": len(data)}
        return path

    seen = set()
    ids = {}
    for label, code in TRANSACTIONS.items():
        txid = record(f"{label}-txid.txt").read_text().strip()
        require(len(txid) == 64 and all(c in "0123456789abcdef" for c in txid), "invalid transaction ID")
        require(txid not in seen, "transaction reused across scenarios")
        seen.add(txid)
        ids[label] = txid
        evidence = transaction_evidence(load_json(record(f"{label}-block.json")), txid, code)
        require(load_json(record(f"{label}-transaction.json")) == evidence, f"{label}: transaction evidence mismatch")
        status = int(record(f"{label}-status.txt").read_text())
        require((status == 0) == (code == 0), f"{label}: CLI status mismatch")
        log = record(f"{label}-invoke.txt").read_text()
        require(f"txid [{txid}]" in log, f"{label}: invocation does not identify transaction")
        if code == 10:
            require("ENDORSEMENT_POLICY_FAILURE" in log, f"{label}: missing platform rejection")

    for label, expected in SBE.items():
        policy = load_json(record(f"{label}-sbe.json"))
        identities = policy["identities"]
        actual = [item["principal"]["msp_identifier"] for item in identities]
        require(sorted(actual) == sorted(expected), f"{label}: wrong SBE principals")
        require(all(item["principal_classification"] == "ROLE" and item["principal"]["role"] == "PEER" for item in identities), f"{label}: wrong principal role")
        rule = policy["rule"]["n_out_of"]
        require(int(rule["n"]) == len(expected), f"{label}: wrong SBE threshold")
        require(sorted(int(item["signed_by"]) for item in rule["rules"]) == list(range(len(expected))), f"{label}: wrong SBE signed_by references")

    for name, label in SANITIZED.items():
        excerpt = load_json(record(name))
        require(excerpt["transactionId"] == ids[label], f"{name}: wrong transaction")
        require(excerpt["assertions"] == {"collectionNameVisible": True, "privatePayloadIncluded": False}, f"{name}: unexpected privacy assertions")

    verdict = load_json(record("core-unit-verdict.json"))
    require(verdict["autentica"] is True and verdict["estado"] == "EN_TRANSITO", "invalid Core verdict")
    require(len(verdict["verificaciones"]) == 4 and all(item["resultado"] == "OK" for item in verdict["verificaciones"]), "incomplete authenticity checks")
    history = load_json(record("core-history.json"))
    require(len(history) >= 6 and all(item["isDelete"] is False and item["value"] is not None for item in history), "incomplete Core history")
    for label, error in {
        "registry-wrong-creator": "REGULATORY_ONLY",
        "register-unit-duplicate": "UNIT_ALREADY_EXISTS",
    }.items():
        require(error in record(f"{label}-invoke.txt").read_text(), f"{label}: missing logic rejection")
        require(int(record(f"{label}-status.txt").read_text()) != 0, f"{label}: unexpected success")
        require(int(record(f"{label}-unchanged-height.txt").read_text()) > 0, f"{label}: invalid height")
    for name in (
        "matrix-divergent-receive-invoke.txt", "matrix-divergent-unchanged-height.txt",
        "matrix-restore-queryapproved.json", "matrix-canonical-receive-invoke.txt",
    ):
        record(name)
    require(load_json(record("dispense-unit.json"))["estado"] == "DISPENSADO", "dispense did not complete")
    require(load_json(record("reject-unit.json"))["estado"] == "DEVUELTO", "reject did not complete")
    require(load_json(record("matrix-canonical-unit.json"))["estado"] == "EN_CUSTODIA", "canonical package did not receive")
    return {"schemaVersion": "1.0.0", "artifacts": dict(sorted(artifacts.items()))}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    transaction = commands.add_parser("transaction")
    transaction.add_argument("--block", type=Path, required=True)
    transaction.add_argument("--txid", required=True)
    transaction.add_argument("--code", type=int, choices=(0, 10), required=True)
    run = commands.add_parser("run")
    run.add_argument("--directory", type=Path, required=True)
    run.add_argument("--manifest", type=Path)
    args = parser.parse_args()
    try:
        if args.command == "transaction":
            result = transaction_evidence(load_json(args.block), args.txid, args.code)
        else:
            result = verify_run(args.directory)
            if args.manifest:
                require(load_json(args.manifest) == result, "artifact manifest differs from verified content")
        print(json.dumps(result, indent=2, sort_keys=True))
    except (OSError, ValueError, KeyError, TypeError, IndexError) as exc:
        print(f"ERROR: NET-6 evidence: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
