from __future__ import annotations

import copy
import importlib.util
import unittest
from pathlib import Path


REPOSITORY_ROOT = Path(__file__).resolve().parents[2]
SCRIPT = REPOSITORY_ROOT / "network" / "scripts" / "sanitize-pdc-evidence.py"
SPEC = importlib.util.spec_from_file_location("sanitize_pdc_evidence", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


COLLECTION = "transfer_DrogueriaMSP_FarmaciaMSP"


def decoded_block() -> dict:
    return {
        "header": {
            "number": "40",
            "data_hash": "ZGF0YS1oYXNo",
            "previous_hash": "cHJldmlvdXMtaGFzaA==",
        },
        "data": {
            "data": [
                {
                    "payload": {
                        "header": {
                            "channel_header": {
                                "channel_id": "snt-channel",
                                "timestamp": "2026-08-28T12:04:05Z",
                                "tx_id": "abc123",
                                "extension": {
                                    "chaincode_id": {"name": "pdc-probe"}
                                },
                            }
                        },
                        "data": {
                            "actions": [
                                {
                                    "hashed": {
                                        "collection_name": COLLECTION,
                                        "hashed_rwset": "aGFzaGVkLXJ3c2V0",
                                        "pvt_rwset_hash": "cHZ0LXJ3c2V0LWhhc2g=",
                                    }
                                }
                            ]
                        },
                    }
                }
            ]
        },
    }


class SanitizePdcEvidenceTest(unittest.TestCase):
    def test_extracts_only_citable_hash_evidence(self) -> None:
        excerpt = MODULE.sanitize_block(decoded_block(), COLLECTION)
        self.assertEqual("1.0.0", excerpt["schemaVersion"])
        self.assertEqual(40, excerpt["blockNumber"])
        self.assertEqual("abc123", excerpt["transactionId"])
        self.assertEqual(COLLECTION, excerpt["collectionName"])
        self.assertEqual("aGFzaGVkLXJ3c2V0", excerpt["hashedReadWriteSet"])
        self.assertEqual(
            {"collectionNameVisible": True, "privatePayloadIncluded": False},
            excerpt["assertions"],
        )
        self.assertNotIn("payload", excerpt)

    def test_missing_collection_is_rejected(self) -> None:
        with self.assertRaisesRegex(MODULE.SanitizationError, "found 0"):
            MODULE.sanitize_block(decoded_block(), "transfer_MissingMSP_OtherMSP")

    def test_duplicate_collection_evidence_is_rejected(self) -> None:
        block = decoded_block()
        duplicate = copy.deepcopy(
            block["data"]["data"][0]["payload"]["data"]["actions"][0]
        )
        block["data"]["data"][0]["payload"]["data"]["actions"].append(duplicate)
        with self.assertRaisesRegex(MODULE.SanitizationError, "found 2"):
            MODULE.sanitize_block(block, COLLECTION)

    def test_invalid_hash_encoding_is_rejected(self) -> None:
        block = decoded_block()
        block["header"]["data_hash"] = "not base64!"
        with self.assertRaisesRegex(MODULE.SanitizationError, "base64"):
            MODULE.sanitize_block(block, COLLECTION)


if __name__ == "__main__":
    unittest.main()
