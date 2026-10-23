from __future__ import annotations

import base64
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[1] / "scripts" / "verify-net6-evidence.py"
SPEC = importlib.util.spec_from_file_location("net6_evidence", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def block(ids=("other", "target"), codes=(0, 10)):
    return {
        "header": {"number": "42"},
        "metadata": {"metadata": ["", "", base64.b64encode(bytes(codes)).decode()]},
        "data": {"data": [
            {"payload": {"header": {"channel_header": {
                "tx_id": txid, "channel_id": "snt-channel",
            }}}} for txid in ids
        ]},
    }


class TransactionEvidenceTest(unittest.TestCase):
    def test_exact_index(self):
        result = MODULE.transaction_evidence(block(), "target", 10)
        self.assertEqual(result, {
            "transactionId": "target", "transactionIndex": 1,
            "blockNumber": 42, "validationCode": 10,
        })

    def test_other_transaction_failure_cannot_pass(self):
        with self.assertRaisesRegex(ValueError, "expected code"):
            MODULE.transaction_evidence(block(codes=(10, 0)), "target", 10)

    def test_missing_and_duplicate_ids(self):
        for ids in (("other",), ("target", "target")):
            with self.subTest(ids=ids), self.assertRaisesRegex(ValueError, "exactly one"):
                MODULE.transaction_evidence(block(ids=ids), "target", 10)

    def test_malformed_filter(self):
        for encoded in ("!", "", "AA=="):
            value = block()
            value["metadata"]["metadata"][2] = encoded
            with self.subTest(encoded=encoded), self.assertRaises(ValueError):
                MODULE.transaction_evidence(value, "target", 10)

    def test_wrong_channel(self):
        value = block()
        value["data"]["data"][1]["payload"]["header"]["channel_header"]["channel_id"] = "other"
        with self.assertRaisesRegex(ValueError, "channel"):
            MODULE.transaction_evidence(value, "target", 10)

    def test_configured_channel(self):
        value = block(ids=("target",), codes=(0,))
        value["data"]["data"][0]["payload"]["header"]["channel_header"]["channel_id"] = "custom-channel"
        result = MODULE.transaction_evidence(value, "target", 0, "custom-channel")
        self.assertEqual(result["validationCode"], 0)


class RunEvidenceTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.directory = Path(self.temporary.name)
        self.ids = {}
        for index, (label, code) in enumerate(MODULE.TRANSACTIONS.items()):
            txid = f"{index + 1:064x}"
            self.ids[label] = txid
            value = block(ids=(txid,), codes=(code,))
            self.write(f"{label}-txid.txt", txid)
            self.write(f"{label}-block.json", value)
            self.write(f"{label}-transaction.json", MODULE.transaction_evidence(value, txid, code))
            self.write(f"{label}-status.txt", "1" if code else "0")
            self.write(f"{label}-invoke.txt", f"txid [{txid}] " + ("ENDORSEMENT_POLICY_FAILURE" if code else "VALID"))
        for label, msps in MODULE.SBE.items():
            self.write(f"{label}-sbe.json", {
                "identities": [{"principal_classification": "ROLE", "principal": {"role": "PEER", "msp_identifier": msp}} for msp in msps],
                "rule": {"n_out_of": {"n": len(msps), "rules": [{"signed_by": i} for i in range(len(msps))]}},
            })
        for name, label in MODULE.SANITIZED.items():
            self.write(name, {"transactionId": self.ids[label], "assertions": {"collectionNameVisible": True, "privatePayloadIncluded": False}})
        self.write("core-unit-verdict.json", {"autentica": True, "estado": "EN_TRANSITO", "verificaciones": [{"resultado": "OK"}] * 4})
        self.write("core-history.json", [{"isDelete": False, "value": {}}] * 6)
        for label, error in (("registry-wrong-creator", "REGULATORY_ONLY"), ("register-unit-duplicate", "UNIT_ALREADY_EXISTS")):
            self.write(f"{label}-invoke.txt", error)
            self.write(f"{label}-status.txt", "1")
            self.write(f"{label}-unchanged-height.txt", "42")
        for name in ("matrix-divergent-receive-invoke.txt", "matrix-divergent-unchanged-height.txt", "matrix-restore-queryapproved.json", "matrix-canonical-receive-invoke.txt"):
            self.write(name, "fixture")
        for label, state in (("dispense", "DISPENSADO"), ("reject", "DEVUELTO"), ("matrix-canonical", "EN_CUSTODIA")):
            self.write(f"{label}-unit.json", {"estado": state})

    def write(self, name, value):
        (self.directory / name).write_text(value if isinstance(value, str) else json.dumps(value), encoding="utf-8")

    def test_manifest_records_real_files(self):
        result = MODULE.verify_run(self.directory)
        self.assertGreater(len(result["artifacts"]), 70)
        for artifact in result["artifacts"].values():
            self.assertEqual(len(artifact["sha256"]), 64)
            self.assertGreater(artifact["bytes"], 0)

    def test_missing_artifact(self):
        (self.directory / "core-history.json").unlink()
        with self.assertRaises(OSError):
            MODULE.verify_run(self.directory)

    def test_sbe_wrong_principal(self):
        path = self.directory / "receive-two-parties-sbe.json"
        policy = json.loads(path.read_text())
        policy["identities"][0]["principal"]["msp_identifier"] = "AnmatMSP"
        self.write(path.name, policy)
        with self.assertRaisesRegex(ValueError, "principals"):
            MODULE.verify_run(self.directory)

    def test_sanitized_evidence_wrong_transaction(self):
        self.write("registry-marker-sanitized.json", {"transactionId": "unrelated"})
        with self.assertRaisesRegex(ValueError, "wrong transaction"):
            MODULE.verify_run(self.directory)

    def test_summary_cannot_override_decoded_block(self):
        label = "receive-one-party"
        self.write(f"{label}-block.json", block(ids=(self.ids[label],), codes=(0,)))
        with self.assertRaisesRegex(ValueError, "expected code"):
            MODULE.verify_run(self.directory)


if __name__ == "__main__":
    unittest.main()
