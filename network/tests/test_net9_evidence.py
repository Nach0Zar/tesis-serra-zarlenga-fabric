from __future__ import annotations

import base64
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[1] / "scripts" / "verify-net9-evidence.py"
SPEC = importlib.util.spec_from_file_location("net9_evidence", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def block(txid: str, code: int):
    return {
        "header": {"number": "42"},
        "metadata": {
            "metadata": ["", "", base64.b64encode(bytes((code,))).decode()]
        },
        "data": {
            "data": [
                {
                    "payload": {
                        "header": {
                            "channel_header": {
                                "tx_id": txid,
                                "channel_id": "snt-channel",
                            }
                        }
                    }
                }
            ]
        },
    }


def varint(value: int) -> bytes:
    encoded = bytearray()
    while value > 0x7F:
        encoded.append((value & 0x7F) | 0x80)
        value >>= 7
    encoded.append(value)
    return bytes(encoded)


def bytes_field(number: int, value: bytes) -> bytes:
    return varint((number << 3) | 2) + varint(len(value)) + value


class HashedRWSetDecoderTest(unittest.TestCase):
    def test_put_and_delete_are_decoded(self):
        key_put = bytes(range(32))
        key_delete = bytes(reversed(range(32)))
        value_hash = b"v" * 32
        put = bytes_field(1, key_put) + bytes_field(3, value_hash)
        delete = bytes_field(1, key_delete) + varint(2 << 3) + varint(1)
        payload = bytes_field(2, put) + bytes_field(2, delete)

        self.assertEqual(
            MODULE.decode_hashed_rwset(payload),
            {
                "hashed_writes": [
                    {
                        "key_hash": base64.b64encode(key_put).decode(),
                        "is_delete": False,
                        "value_hash": base64.b64encode(value_hash).decode(),
                    },
                    {
                        "key_hash": base64.b64encode(key_delete).decode(),
                        "is_delete": True,
                    },
                ]
            },
        )

    def test_truncated_message_is_rejected(self):
        with self.assertRaisesRegex(ValueError, "truncated"):
            MODULE.decode_hashed_rwset(bytes_field(2, b"\x0a\x20")[:-1])


class Net9EvidenceTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.directory = Path(self.temporary.name)
        self.run_token = "regression"
        self.ids = {}
        self.write(
            "run-metadata.json",
            {
                "schemaVersion": "1.0.0",
                "runToken": self.run_token,
                "repositoryCommit": "a" * 40,
                "channel": "snt-channel",
                "chaincode": "snt",
                "packageID": "snt:fixture",
            },
        )

        for index, (label, code) in enumerate(MODULE.TRANSACTIONS.items()):
            txid = f"{index + 1:064x}"
            self.ids[label] = txid
            evidence = MODULE.transaction_evidence(block(txid, code), txid, code)
            self.write(f"{label}-txid.txt", txid)
            self.write(f"{label}-block.json", block(txid, code))
            self.write(f"{label}-transaction.json", evidence)
            self.write(f"{label}-status.txt", "0" if code == 0 else "1")
            suffix = "VALID" if code == 0 else "ENDORSEMENT_POLICY_FAILURE"
            self.write(f"{label}-invoke.txt", f"txid [{txid}] {suffix}")

        for label, msps in MODULE.SBE.items():
            self.write(
                f"{label}-sbe.json",
                {
                    "identities": [
                        {
                            "principal_classification": "ROLE",
                            "principal": {"role": "PEER", "msp_identifier": msp},
                        }
                        for msp in msps
                    ],
                    "rule": {
                        "n_out_of": {
                            "n": len(msps),
                            "rules": [{"signed_by": i} for i in range(len(msps))],
                        }
                    },
                },
            )

        for label, prefix in MODULE.REGULATORY_MARKERS.items():
            self.marker(label, "AnmatMSP", prefix)
        for label, prefix in MODULE.LAB_MARKERS.items():
            self.marker(label, "LabMSP", prefix)

        for index, (label, (prefix, state)) in enumerate(MODULE.TRANSIT.items()):
            unit_serial = MODULE.serial(prefix, self.run_token)
            dispatch_txid = f"{500 + index:064x}"
            self.write(f"{label}-dispatch-txid.txt", dispatch_txid)
            self.write(
                f"{label}-transfer-hashed-rwset.json",
                {
                    "hashed_writes": [
                        {
                            "key_hash": MODULE.private_key_hash(
                                "TransferOp", MODULE.GTIN, unit_serial, dispatch_txid
                            ),
                            "is_delete": False,
                            "value_hash": "dmFsdWU=",
                        },
                        {
                            "key_hash": MODULE.private_key_hash(
                                "TransferOpActive", MODULE.GTIN, unit_serial
                            ),
                            "is_delete": True,
                        },
                    ]
                },
            )
            self.write(f"{label}-unit.json", {"estado": state})

        for name, label in MODULE.SANITIZED.items():
            self.write(
                name,
                {
                    "transactionId": self.ids[label],
                    "assertions": {
                        "collectionNameVisible": True,
                        "privatePayloadIncluded": False,
                    },
                },
            )

        self.write(
            "lab-history-expired.json",
            [{"isDelete": False, "value": {"estado": "ACTIVA"}}],
        )
        states = [
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
        operations = (
            ["WITHDRAW_FROM_MARKET"] * 6
            + ["RESTOCK"] * 2
            + ["FINAL_DISPOSITION"] * 2
        )
        self.write(
            "lab-history-final.json",
            [
                {
                    "isDelete": False,
                    "value": {"estado": state, "operacion": operation},
                }
                for state, operation in zip(states, operations, strict=True)
            ],
        )
        self.write("lab-expired-operation-invoke.txt", "LAB_INTERVENTION_REQUIRED")
        self.write("lab-expired-operation-status.txt", "1")
        self.write("lab-expired-operation-unchanged-height.txt", "42")

        for name, state in {
            "transit-quarantine-unlocked-unit.json": "EN_CUSTODIA",
            "transit-expired-unlocked-unit.json": "DISPUESTO_FINAL",
            "transit-damaged-unlocked-unit.json": "DISPUESTO_FINAL",
            "regulatory-return-unit.json": "DEVUELTO",
            "regulatory-final-unit.json": "DISPUESTO_FINAL",
            "lab-final-unit.json": "DISPUESTO_FINAL",
        }.items():
            self.write(name, {"estado": state})

    def write(self, name, value):
        content = value if isinstance(value, str) else json.dumps(value)
        (self.directory / name).write_text(content, encoding="utf-8")

    def marker(self, label, msp, prefix):
        unit_serial = MODULE.serial(prefix, self.run_token)
        self.write(
            f"{label}-marker-{msp}.json",
            {
                "hashed_writes": [
                    {
                        "key_hash": MODULE.private_key_hash(
                            "Participacion",
                            "Unidad",
                            MODULE.GTIN,
                            unit_serial,
                            self.ids[label],
                        ),
                        "is_delete": False,
                        "value_hash": "dmFsdWU=",
                    }
                ]
            },
        )

    def test_complete_run(self):
        result = MODULE.verify_run(self.directory)
        self.assertEqual(result["schemaVersion"], "1.0.0")
        self.assertGreater(len(result["artifacts"]), 200)
        self.assertTrue(all(item["bytes"] > 0 for item in result["artifacts"].values()))

    def test_wrong_marker_key_is_rejected(self):
        path = self.directory / "lab-withdraw-marker-LabMSP.json"
        value = json.loads(path.read_text())
        value["hashed_writes"][0]["key_hash"] = "d3Jvbmc="
        self.write(path.name, value)
        with self.assertRaisesRegex(ValueError, "marker key hash"):
            MODULE.verify_run(self.directory)

    def test_active_transfer_must_be_deleted(self):
        path = self.directory / "transit-quarantine-transfer-hashed-rwset.json"
        value = json.loads(path.read_text())
        value["hashed_writes"][1]["is_delete"] = False
        self.write(path.name, value)
        with self.assertRaisesRegex(ValueError, "active operation was not deleted"):
            MODULE.verify_run(self.directory)

    def test_expiration_cannot_add_synthetic_history(self):
        self.write(
            "lab-history-expired.json",
            [
                {"isDelete": False, "value": {"estado": "ACTIVA"}},
                {"isDelete": False, "value": {"estado": "VENCIDA"}},
            ],
        )
        with self.assertRaisesRegex(ValueError, "derived condition"):
            MODULE.verify_run(self.directory)

    def test_summary_cannot_override_block_validation_code(self):
        label = "lab-final-missing-custodian"
        self.write(f"{label}-block.json", block(self.ids[label], 0))
        with self.assertRaisesRegex(ValueError, "expected code"):
            MODULE.verify_run(self.directory)


if __name__ == "__main__":
    unittest.main()
