from __future__ import annotations

import base64
import copy
import importlib.util
import json
import re
import unittest
from pathlib import Path


REPOSITORY_ROOT = Path(__file__).resolve().parents[2]
SCRIPT = REPOSITORY_ROOT / "network" / "scripts" / "verify-committed-definition.py"
SPEC = importlib.util.spec_from_file_location("verify_committed_definition", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def varint(value: int) -> bytes:
    result = bytearray()
    while True:
        byte = value & 0x7F
        value >>= 7
        if value:
            result.append(byte | 0x80)
        else:
            result.append(byte)
            return bytes(result)


def principal(msp_id: str, role: int) -> str:
    encoded = b"\x0a" + varint(len(msp_id)) + msp_id.encode("utf-8")
    if role:
        encoded += b"\x10" + varint(role)
    return base64.b64encode(encoded).decode("ascii")


def fabric_policy(principals: list[tuple[str, int]]) -> dict:
    return {
        "rule": {
            "Type": {
                "NOutOf": {
                    "n": 1,
                    "rules": [
                        {"Type": {"SignedBy": index}}
                        for index in range(len(principals))
                    ],
                }
            }
        },
        "identities": [
            {"principal": principal(msp_id, role)} for msp_id, role in principals
        ],
    }


def dsl_principals(policy: str, role: int) -> list[tuple[str, int]]:
    return [(msp_id, role) for msp_id in re.findall(r"'([^']+)\.", policy)]


class VerifyCommittedDefinitionTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.collections = json.loads(
            (REPOSITORY_ROOT / "network" / "collections_config.json").read_text(
                encoding="utf-8"
            )
        )
        cls.manifest = json.loads(
            (REPOSITORY_ROOT / "network" / "organizations-manifest.json").read_text(
                encoding="utf-8"
            )
        )
        cls.matrix = json.loads(
            (REPOSITORY_ROOT / "domain" / "authorized-transfers.json").read_text(
                encoding="utf-8"
            )
        )

    def make_definition(self) -> dict:
        entries = []
        for collection in self.collections:
            member_policy = fabric_policy(
                dsl_principals(collection["policy"], role=0)
            )
            endorsement_policy = fabric_policy(
                dsl_principals(
                    collection["endorsementPolicy"]["signaturePolicy"], role=3
                )
            )
            entries.append(
                {
                    "Payload": {
                        "StaticCollectionConfig": {
                            "name": collection["name"],
                            "member_orgs_policy": {
                                "Payload": {"SignaturePolicy": member_policy}
                            },
                            "required_peer_count": collection["requiredPeerCount"],
                            "maximum_peer_count": collection["maxPeerCount"],
                            "member_only_read": collection["memberOnlyRead"],
                            "member_only_write": collection["memberOnlyWrite"],
                            "endorsement_policy": {
                                "Type": {"SignaturePolicy": endorsement_policy}
                            },
                        }
                    }
                }
            )
        return {
            "sequence": 2,
            "version": "1.0",
            "collections": {"config": entries},
        }

    def make_operational_policy(self) -> dict:
        expected = MODULE.expected_chaincode_policy(
            self.manifest, self.matrix, "operational"
        )
        principals = expected["principals"]
        return {
            "signature_policy": {
                "version": 0,
                "rule": {
                    "n_out_of": {
                        "n": expected["threshold"],
                        "rules": [
                            {"signed_by": index}
                            for index in range(len(principals))
                        ],
                    }
                },
                "identities": [
                    {
                        "principal": principal(msp_id, role),
                        "principal_classification": "ROLE",
                    }
                    for msp_id, role in principals
                ],
            }
        }

    def verify(self, definition: dict, policy: dict) -> None:
        MODULE.verify_definition(
            definition,
            policy,
            self.collections,
            self.manifest,
            self.matrix,
            sequence=2,
            version="1.0",
            init_required=False,
            policy_kind="operational",
        )

    def test_exact_operational_definition_passes(self) -> None:
        self.verify(self.make_definition(), self.make_operational_policy())

    def test_lifecycle_scalar_drift_is_rejected(self) -> None:
        definition = self.make_definition()
        definition["sequence"] = 3
        with self.assertRaisesRegex(MODULE.VerificationError, "lifecycle scalars"):
            self.verify(definition, self.make_operational_policy())

    def test_collection_property_drift_is_rejected(self) -> None:
        definition = self.make_definition()
        definition["collections"]["config"][0]["Payload"][
            "StaticCollectionConfig"
        ]["required_peer_count"] = 0
        with self.assertRaisesRegex(MODULE.VerificationError, "required_peer_count"):
            self.verify(definition, self.make_operational_policy())

    def test_collection_membership_drift_is_rejected(self) -> None:
        definition = self.make_definition()
        policy = definition["collections"]["config"][0]["Payload"][
            "StaticCollectionConfig"
        ]["member_orgs_policy"]["Payload"]["SignaturePolicy"]
        policy["identities"][2]["principal"] = principal("FinanciadorMSP", 0)
        with self.assertRaisesRegex(MODULE.VerificationError, "member_policy"):
            self.verify(definition, self.make_operational_policy())

    def test_collection_endorsement_drift_is_rejected(self) -> None:
        definition = self.make_definition()
        policy = definition["collections"]["config"][0]["Payload"][
            "StaticCollectionConfig"
        ]["endorsement_policy"]["Type"]["SignaturePolicy"]
        policy["rule"]["Type"]["NOutOf"]["n"] = 2
        with self.assertRaisesRegex(MODULE.VerificationError, "endorsement_policy"):
            self.verify(definition, self.make_operational_policy())

    def test_chaincode_policy_drift_is_rejected(self) -> None:
        policy = self.make_operational_policy()
        policy["signature_policy"]["rule"]["n_out_of"]["n"] = 2
        with self.assertRaisesRegex(MODULE.VerificationError, "chaincode endorsement"):
            self.verify(self.make_definition(), policy)

    def test_reordered_committed_collections_are_rejected(self) -> None:
        definition = self.make_definition()
        definition["collections"]["config"].reverse()
        with self.assertRaisesRegex(MODULE.VerificationError, "names/order"):
            self.verify(definition, self.make_operational_policy())

    def test_bootstrap_policy_requires_all_manifest_organizations(self) -> None:
        expected = MODULE.expected_chaincode_policy(
            self.manifest, self.matrix, "bootstrap"
        )
        self.assertEqual(7, expected["threshold"])
        self.assertEqual(7, len(expected["principals"]))
        self.assertIn(("FinanciadorMSP", 3), expected["principals"])


if __name__ == "__main__":
    unittest.main()
