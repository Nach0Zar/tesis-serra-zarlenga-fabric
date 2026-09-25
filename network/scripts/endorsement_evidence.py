"""Shared validation primitives for NET-6 and NET-9 evidence."""
from __future__ import annotations

import base64
import hashlib
import json
from pathlib import Path
from typing import Any


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValueError(message)


def load_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def transaction_evidence(
    block: dict, txid: str, expected_code: int, expected_channel: str = "snt-channel"
) -> dict:
    transactions = block["data"]["data"]
    matches = [
        index
        for index, tx in enumerate(transactions)
        if tx["payload"]["header"]["channel_header"]["tx_id"] == txid
    ]
    require(len(matches) == 1, "expected exactly one matching transaction ID")
    codes = base64.b64decode(block["metadata"]["metadata"][2], validate=True)
    require(len(codes) == len(transactions), "validation filter length mismatch")
    index = matches[0]
    header = transactions[index]["payload"]["header"]["channel_header"]
    require(
        header["channel_id"] == expected_channel,
        f"unexpected channel: expected {expected_channel}, got {header['channel_id']}",
    )
    require(
        codes[index] == expected_code,
        f"transaction {txid}: expected code {expected_code}, got {codes[index]}",
    )
    return {
        "transactionId": txid,
        "transactionIndex": index,
        "blockNumber": int(block["header"]["number"]),
        "validationCode": codes[index],
    }


def composite_key(*components: str) -> str:
    require(all("\x00" not in item for item in components), "invalid composite key component")
    return "\x00" + "\x00".join(components) + "\x00"


def private_key_hash(*components: str) -> str:
    digest = hashlib.sha256(composite_key(*components).encode("utf-8")).digest()
    return base64.b64encode(digest).decode("ascii")


def _read_varint(payload: bytes, offset: int) -> tuple[int, int]:
    value = 0
    shift = 0
    while offset < len(payload) and shift < 64:
        byte = payload[offset]
        offset += 1
        value |= (byte & 0x7F) << shift
        if byte < 0x80:
            return value, offset
        shift += 7
    raise ValueError("malformed protobuf varint")


def _protobuf_fields(payload: bytes) -> list[tuple[int, int, Any]]:
    fields = []
    offset = 0
    while offset < len(payload):
        tag, offset = _read_varint(payload, offset)
        field_number = tag >> 3
        wire_type = tag & 0x07
        require(field_number > 0, "invalid protobuf field number")
        if wire_type == 0:
            value, offset = _read_varint(payload, offset)
        elif wire_type == 1:
            require(offset + 8 <= len(payload), "truncated fixed64 field")
            value = payload[offset : offset + 8]
            offset += 8
        elif wire_type == 2:
            length, offset = _read_varint(payload, offset)
            require(offset + length <= len(payload), "truncated length-delimited field")
            value = payload[offset : offset + length]
            offset += length
        elif wire_type == 5:
            require(offset + 4 <= len(payload), "truncated fixed32 field")
            value = payload[offset : offset + 4]
            offset += 4
        else:
            raise ValueError(f"unsupported protobuf wire type {wire_type}")
        fields.append((field_number, wire_type, value))
    return fields


def decode_hashed_rwset(payload: bytes) -> dict:
    """Decode Fabric's rwset.kvrwset.HashedRWSet without generated bindings."""
    writes = []
    for field_number, wire_type, value in _protobuf_fields(payload):
        if field_number != 2:
            continue
        require(wire_type == 2, "malformed hashed_writes field")
        key_hash = None
        value_hash = None
        is_delete = False
        for nested_number, nested_wire, nested_value in _protobuf_fields(value):
            if nested_number == 1:
                require(nested_wire == 2, "malformed key_hash field")
                key_hash = base64.b64encode(nested_value).decode("ascii")
            elif nested_number == 2:
                require(nested_wire == 0, "malformed is_delete field")
                require(nested_value in (0, 1), "invalid is_delete value")
                is_delete = bool(nested_value)
            elif nested_number == 3:
                require(nested_wire == 2, "malformed value_hash field")
                value_hash = base64.b64encode(nested_value).decode("ascii")
        require(key_hash is not None, "hashed write has no key hash")
        entry = {"key_hash": key_hash, "is_delete": is_delete}
        if value_hash is not None:
            entry["value_hash"] = value_hash
        writes.append(entry)
    return {"hashed_writes": writes}
