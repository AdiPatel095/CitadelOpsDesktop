"""Fail-closed provenance and artifact checks; no actual font files generated."""
import copy
import hashlib
import json
import runpy
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("build-manrope")
MODULE = runpy.run_path(str(SCRIPT))
ROOT = SCRIPT.resolve().parents[2]
PUBLIC = ROOT / "Client/public" if (ROOT / "Client/public").is_dir() else ROOT / "public"


class GateTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.output = Path(self.directory.name)
        self.manifest = json.loads((PUBLIC / "fonts/manrope/fonts-manifest.json").read_text())
        self.provenance = json.loads(SCRIPT.with_name("font-provenance.json").read_text())
        # WOFF2-signature payloads are exclusively integrity fixtures, not font validation.
        self.commit = MODULE["SOURCE_COMMIT"]
        for record in self.provenance["files"]:
            record["google_fonts_commit"] = self.commit
        for key in ("source", "license"):
            self.manifest[key]["google_fonts_commit"] = self.commit
        for entry in self.manifest["outputs"]:
            data = b"wOF2" + entry["script"].encode()
            entry.update(bytes=len(data), sha256=hashlib.sha256(data).hexdigest(), google_fonts_commit=self.commit)
            (self.output / entry["file"]).write_bytes(data)
        (self.output / "OFL.txt").write_bytes((PUBLIC / "fonts/manrope/OFL.txt").read_bytes())

    def check(self, manifest=None, provenance=None):
        manifest = manifest if manifest is not None else self.manifest
        provenance = provenance if provenance is not None else self.provenance
        MODULE["source_gate"](manifest, provenance)
        MODULE["check_outputs"](manifest, self.output)

    def test_complete_metadata_and_integrity(self):
        self.check()

    def test_unpinned_provenance_is_rejected(self):
        self.provenance["files"][0]["google_fonts_commit"] = None
        with self.assertRaisesRegex(ValueError, MODULE["GATE"]):
            self.check()

    def test_missing_manifest_metadata_is_rejected(self):
        for key in ("sha256", "bytes", "google_fonts_commit"):
            for section in ("source", "license", "output"):
                with self.subTest(key=key, section=section):
                    manifest = copy.deepcopy(self.manifest)
                    record = manifest["outputs"][0] if section == "output" else manifest[section]
                    record.pop(key)
                    with self.assertRaisesRegex(ValueError, MODULE["GATE"]):
                        self.check(manifest)

    def test_mismatched_provenance_is_rejected(self):
        self.manifest["source"]["google_fonts_commit"] = "b" * 40
        with self.assertRaisesRegex(ValueError, "does not match"):
            self.check()

    def test_missing_and_corrupt_outputs_are_rejected(self):
        path = self.output / self.manifest["outputs"][0]["file"]
        path.unlink()
        with self.assertRaisesRegex(ValueError, "integrity mismatch"):
            self.check()
        path.write_bytes(b"wOF2corrupt")
        with self.assertRaisesRegex(ValueError, "integrity mismatch"):
            self.check()

    def test_subset_cannot_claim_another_commit(self):
        self.manifest["outputs"][0]["google_fonts_commit"] = "b" * 40
        with self.assertRaisesRegex(ValueError, "contract mismatch"):
            self.check()

    def test_license_is_verified(self):
        (self.output / "OFL.txt").write_text("wrong license")
        with self.assertRaisesRegex(ValueError, "OFL.txt"):
            self.check()

    def test_unverified_pin_is_rejected_even_when_records_agree(self):
        for record in self.provenance["files"]:
            record["google_fonts_commit"] = "b" * 40
        for key in ("source", "license"):
            self.manifest[key]["google_fonts_commit"] = "b" * 40
        with self.assertRaisesRegex(ValueError, "verified google/fonts pin"):
            self.check()

    def test_cli_generation_gate_runs_before_source_or_fonttools(self):
        self.provenance["files"][0]["google_fonts_commit"] = None
        (self.output / "fonts-manifest.json").write_text(json.dumps(self.manifest))
        provenance = self.output / "font-provenance.json"
        provenance.write_text(json.dumps(self.provenance))
        result = subprocess.run([sys.executable, str(SCRIPT), "--source", str(self.output / "absent.ttf"),
                                 "--output", str(self.output), "--provenance", str(provenance)], capture_output=True, text=True)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stderr.strip(), MODULE["GATE"])


if __name__ == "__main__":
    unittest.main()
