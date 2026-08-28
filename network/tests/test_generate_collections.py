from __future__ import annotations

import copy
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path


REPOSITORY_ROOT = Path(__file__).resolve().parents[2]
SCRIPT = REPOSITORY_ROOT / "network" / "scripts" / "generate-collections.py"
SPEC = importlib.util.spec_from_file_location("generate_collections", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class GenerateCollectionsTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
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

    def generate(self, manifest=None, matrix=None):
        return MODULE.generate_collections(
            copy.deepcopy(manifest or self.manifest),
            copy.deepcopy(matrix or self.matrix),
        )

    def test_current_dataset_generates_ten_exact_collections(self) -> None:
        collections = self.generate()
        self.assertEqual(10, len(collections))
        self.assertEqual(
            sorted(collection["name"] for collection in collections),
            [collection["name"] for collection in collections],
        )
        for collection in collections:
            first_msp, second_msp = collection["name"].removeprefix(
                "transfer_"
            ).split("_")
            self.assertEqual(
                f"OR('{first_msp}.member','{second_msp}.member','AnmatMSP.member')",
                collection["policy"],
            )
            self.assertEqual(1, collection["requiredPeerCount"])
            self.assertEqual(2, collection["maxPeerCount"])
            self.assertEqual(0, collection["blockToLive"])
            self.assertIs(collection["memberOnlyRead"], True)
            self.assertIs(collection["memberOnlyWrite"], True)
            self.assertEqual(
                f"OR('{first_msp}.peer','{second_msp}.peer')",
                collection["endorsementPolicy"]["signaturePolicy"],
            )

    def test_non_custodial_organizations_never_form_a_pair(self) -> None:
        names = [collection["name"] for collection in self.generate()]
        self.assertFalse(any("AnmatMSP" in name for name in names))
        self.assertFalse(any("FinanciadorMSP" in name for name in names))

    def test_bidirectional_rules_collapse_into_one_collection(self) -> None:
        matrix = copy.deepcopy(self.matrix)
        matrix["authorizedTransfers"].append(
            {
                "id": "DISTRIBUTOR_TO_LABORATORY_TEST",
                "origin": "DISTRIBUTOR",
                "destination": "LABORATORY",
                "allowed": True,
                "normativeReferences": ["DEC_1299_1997_ART_4"],
                "rationale": "Only used to verify unordered-pair collapse.",
            }
        )
        names = [collection["name"] for collection in self.generate(matrix=matrix)]
        self.assertEqual(1, names.count("transfer_DistribuidorMSP_LabMSP"))

    def test_same_agent_type_creates_pair_for_distinct_organizations(self) -> None:
        manifest = copy.deepcopy(self.manifest)
        second_drugstore = copy.deepcopy(
            next(
                organization
                for organization in manifest["organizations"]
                if organization["agentType"] == "DRUGSTORE"
            )
        )
        second_drugstore.update(
            {
                "mspId": "DrogueriaDosMSP",
                "slug": "drogueriados",
                "id": "7791234500062",
                "peerHostname": "peer0.drogueriados.snt.local",
            }
        )
        second_drugstore.pop("ordererHostname", None)
        manifest["organizations"].append(second_drugstore)
        names = [collection["name"] for collection in self.generate(manifest=manifest)]
        self.assertIn("transfer_DrogueriaDosMSP_DrogueriaMSP", names)

    def test_input_order_does_not_change_output(self) -> None:
        manifest = copy.deepcopy(self.manifest)
        matrix = copy.deepcopy(self.matrix)
        manifest["organizations"].reverse()
        matrix["authorizedTransfers"].reverse()
        self.assertEqual(self.generate(), self.generate(manifest, matrix))

    def test_duplicate_directed_pair_is_rejected(self) -> None:
        matrix = copy.deepcopy(self.matrix)
        duplicate = copy.deepcopy(matrix["authorizedTransfers"][0])
        duplicate["id"] = "DUPLICATE_PAIR_TEST"
        matrix["authorizedTransfers"].append(duplicate)
        with self.assertRaisesRegex(MODULE.CollectionsError, "duplicate.*pair"):
            self.generate(matrix=matrix)

    def test_unknown_normative_reference_is_rejected(self) -> None:
        matrix = copy.deepcopy(self.matrix)
        matrix["authorizedTransfers"][0]["normativeReferences"] = ["UNKNOWN_REFERENCE"]
        with self.assertRaisesRegex(MODULE.CollectionsError, "unknown normative reference"):
            self.generate(matrix=matrix)

    def test_authorized_and_prohibited_pair_is_rejected(self) -> None:
        matrix = copy.deepcopy(self.matrix)
        matrix["prohibitedTransfers"][0]["destinations"].append(
            "HEALTHCARE_FACILITY"
        )
        with self.assertRaisesRegex(MODULE.CollectionsError, "contradictory"):
            self.generate(matrix=matrix)

    def test_check_output_detects_drift(self) -> None:
        expected = MODULE.render_collections(self.generate())
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "collections_config.json"
            output.write_text("[]\n", encoding="utf-8")
            with self.assertRaisesRegex(MODULE.CollectionsError, "stale"):
                MODULE.check_output(output, expected)
            output.write_text(expected, encoding="utf-8")
            MODULE.check_output(output, expected)


if __name__ == "__main__":
    unittest.main()
