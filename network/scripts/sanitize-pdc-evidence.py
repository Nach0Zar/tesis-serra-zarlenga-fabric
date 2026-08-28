#!/usr/bin/env python3
"""Extract a citable, payload-free PDC excerpt from a decoded Fabric block."""

from __future__ import annotations

import argparse
import base64
import binascii
import json
import sys
from pathlib import Path
from typing import Any, Iterator


class SanitizationError(ValueError):
    """Raised when the decoded block does not contain the expected evidence."""


def walk_objects(value: Any) -> Iterator[dict[str, Any]]:
    if isinstance(value, dict):
        yield value
        for child in value.values():
            yield from walk_objects(child)
    elif isinstance(value, list):
        for child in value:
            yield from walk_objects(child)


def require_string(value: Any, field: str) -> str:
    if not isinstance(value, str) or not value:
        raise SanitizationError(f"missing {field}")
    return value


def require_base64(value: Any, field: str) -> str:
    encoded = require_string(value, field)
    try:
        base64.b64decode(encoded, validate=True)
    except (ValueError, binascii.Error) as exc:
        raise SanitizationError(f"{field} is not valid base64") from exc
    return encoded


def sanitize_block(block: dict[str, Any], collection: str) -> dict[str, Any]:
    header = block.get("header")
    transactions = block.get("data", {}).get("data")
    if not isinstance(header, dict) or not isinstance(transactions, list):
        raise SanitizationError("decoded block has no header or transaction data")

    matches: list[tuple[dict[str, Any], dict[str, Any]]] = []
    for transaction in transactions:
        if not isinstance(transaction, dict):
            continue
        for item in walk_objects(transaction):
            if item.get("collection_name") == collection:
                matches.append((transaction, item))
    if len(matches) != 1:
        raise SanitizationError(
            f"expected one hashed rwset for {collection}, found {len(matches)}"
        )

    transaction, hashed_collection = matches[0]
    channel_header = transaction.get("payload", {}).get("header", {}).get(
        "channel_header"
    )
    if not isinstance(channel_header, dict):
        raise SanitizationError("matching transaction has no channel header")
    chaincode_id = channel_header.get("extension", {}).get("chaincode_id", {})
    try:
        block_number = int(header.get("number"))
    except (TypeError, ValueError) as exc:
        raise SanitizationError("invalid block number") from exc

    return {
        "schemaVersion": "1.0.0",
        "channelId": require_string(channel_header.get("channel_id"), "channel id"),
        "blockNumber": block_number,
        "blockDataHash": require_base64(header.get("data_hash"), "block data hash"),
        "previousBlockHash": require_base64(
            header.get("previous_hash"), "previous block hash"
        ),
        "transactionId": require_string(
            channel_header.get("tx_id"), "transaction id"
        ),
        "transactionTimestamp": require_string(
            channel_header.get("timestamp"), "transaction timestamp"
        ),
        "chaincodeName": require_string(chaincode_id.get("name"), "chaincode name"),
        "collectionName": collection,
        "hashedReadWriteSet": require_base64(
            hashed_collection.get("hashed_rwset"), "hashed rwset"
        ),
        "privateReadWriteSetHash": require_base64(
            hashed_collection.get("pvt_rwset_hash"), "private rwset hash"
        ),
        "assertions": {
            "collectionNameVisible": True,
            "privatePayloadIncluded": False,
        },
    }


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--collection", required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--forbidden-value", required=True)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        raw = args.input.read_text(encoding="utf-8")
        if args.forbidden_value in raw:
            raise SanitizationError("decoded block contains the forbidden private value")
        block = json.loads(raw)
        if not isinstance(block, dict):
            raise SanitizationError("decoded block must be a JSON object")
        excerpt = sanitize_block(block, args.collection)
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(
            json.dumps(excerpt, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
        )
    except (OSError, json.JSONDecodeError, SanitizationError) as exc:
        print(f"ERROR: cannot sanitize PDC evidence: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
