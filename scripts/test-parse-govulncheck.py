#!/usr/bin/env python3
"""Offline real-CLI regression for evidence parsing; no Go tool or product scan."""
import copy
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

PARSER = Path(__file__).with_name("parse-govulncheck.py")


class EvidenceParserCLI(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="govulncheck-parser-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.config = {"protocol_version": "v1.0.0", "scanner_name": "govulncheck",
                       "scanner_version": "v1.4.0", "scan_mode": "source", "scan_level": "symbol",
                       "go_version": "go1.26.6", "db": "https://vuln.go.dev",
                       "db_last_modified": "2026-11-05T23:49:57Z"}
        self.sbom = {"go_version": "go1.26.6", "roots": ["example.org/a"],
                     "modules": [{"path": "example.org/a"}, {"path": "stdlib", "version": "v1.26.6"}]}
        self.snapshot = {"head": "0" * 40, "module": "example.org/a", "files": {"a.go": "abc"},
                         "go_version": "go1.26.6", "replace_directives": [], "worktree_clean": True}
        self.environment = {"GOOS": "linux", "GOARCH": "arm64", "CGO_ENABLED": "0", "GOWORK": "off",
                            "GOFLAGS": "-mod=readonly", "GOVERSION": "go1.26.6"}
        self.messages = [{"config": self.config}, {"progress": {"message": "multi\nline"}}, {"SBOM": self.sbom}]
        self.finding = {"finding": {"osv": "GO-2026-1234", "fixed_version": "v1.2.3", "trace": [
            {"module": "example.org/dep", "version": "v1.2.2", "package": "example.org/dep", "function": "Bad"},
            {"module": "example.org/a", "package": "example.org/a", "function": "main"}]}}

    def run_collect(self, messages=None, *, raw=None, scan_exit=0, after=None,
                    expected_go="go1.26.6", mode="source", environment=None):
        if raw is None:
            # Pretty-printed objects are concatenated with no line delimiter.
            raw = "".join(json.dumps(m, indent=2) for m in (self.messages if messages is None else messages))
        for name, data in [("before.json", self.snapshot), ("after.json", self.snapshot if after is None else after),
                           ("environment.json", self.environment if environment is None else environment)]:
            (self.root / name).write_text(json.dumps(data))
        (self.root / "raw.json").write_text(raw)
        (self.root / "exit").write_text(str(scan_exit))
        env = os.environ.copy()
        env["GITHUB_STEP_SUMMARY"] = str(self.root / "step-summary.md")
        argv = [sys.executable, str(PARSER), "collect", "--input", str(self.root / "raw.json"),
                "--exit-code", str(self.root / "exit"), "--mode", mode, "--expected-go", expected_go,
                "--before", str(self.root / "before.json"), "--after", str(self.root / "after.json"),
                "--go-env", str(self.root / "environment.json"), "--output", str(self.root / "result.json")]
        process = subprocess.run(argv, capture_output=True, text=True, env=env, timeout=10)
        self.assertTrue((self.root / "result.json").exists(), process.stderr)
        return process, json.loads((self.root / "result.json").read_text())

    def assert_failed(self, **kwargs):
        process, result = self.run_collect(**kwargs)
        self.assertNotEqual(process.returncode, 0)
        self.assertEqual(result["status"], "EVIDENCE_COLLECTION_FAILED")
        self.assertTrue(result["errors"])

    def test_multiline_concatenated_stream(self):
        process, result = self.run_collect()
        self.assertEqual(process.returncode, 0, process.stderr)
        self.assertEqual(result["message_count"], 3)
        self.assertFalse(result["findings_present"])
        self.assertEqual(result["status"], "EVIDENCE_COLLECTED_NO_FINDINGS_REPORTED")

    def test_exit_zero_findings_remain_pending(self):
        process, result = self.run_collect(self.messages + [self.finding])
        self.assertEqual(process.returncode, 0, process.stderr)
        self.assertTrue(result["findings_present"])
        self.assertEqual(result["go_ids"], ["GO-2026-1234"])
        self.assertEqual(result["security_verdict"], "PENDING_PROJECT_IMPACT_REVIEW")
        self.assertIn("::warning", process.stdout)

    def test_osv_alone_is_not_a_finding(self):
        process, result = self.run_collect(self.messages + [{"osv": {"id": "GO-2026-1234"}}])
        self.assertEqual(process.returncode, 0)
        self.assertFalse(result["findings_present"])

    def test_nonzero_execution_fails_even_with_valid_json(self):
        self.assert_failed(scan_exit=2)

    def test_truncated_json(self):
        self.assert_failed(raw=json.dumps(self.messages[0])[:-2])

    def test_invalid_json(self):
        self.assert_failed(raw="not json")

    def test_non_json_numeric_constant(self):
        self.assert_failed(raw='{"progress":{"value":NaN}}')

    def test_trailing_junk(self):
        self.assert_failed(raw="".join(json.dumps(m) for m in self.messages) + "junk")

    def test_missing_config(self):
        self.assert_failed(messages=self.messages[1:])

    def test_missing_sbom(self):
        self.assert_failed(messages=self.messages[:2])

    def test_invalid_sbom_roots_structure(self):
        self.sbom["roots"] = "not-a-package-array"
        self.assert_failed()

    def test_invalid_sbom_modules_structure(self):
        self.sbom["modules"] = {"path": "not-a-module-array"}
        self.assert_failed()

    def test_sbom_root_belongs_to_another_module(self):
        self.sbom["roots"] = ["example.org/other/package"]
        self.assert_failed()

    def test_source_module_missing_from_sbom(self):
        self.sbom["modules"] = [{"path": "stdlib", "version": "v1.26.6"}]
        self.assert_failed()

    def test_duplicate_config(self):
        self.assert_failed(messages=[self.messages[0]] + self.messages)

    def test_wrong_go_version(self):
        self.assert_failed(expected_go="go1.26.10")

    def test_source_snapshot_change(self):
        changed = copy.deepcopy(self.snapshot)
        changed["files"]["a.go"] = "changed"
        self.assert_failed(after=changed)

    def test_wrong_production_target(self):
        changed = dict(self.environment, GOARCH="amd64")
        self.assert_failed(environment=changed)

    def test_binary_config_can_omit_go_version(self):
        self.config["scan_mode"] = "binary"
        self.config.pop("go_version")
        self.sbom["go_version"] = "go1.26.10"
        process, result = self.run_collect(mode="binary", expected_go="go1.26.10")
        self.assertEqual(process.returncode, 0, process.stderr)
        self.assertEqual(result["SBOM"]["go_version"], "go1.26.10")

    def test_missing_frame_module_is_retained_for_triage(self):
        # Preserve partial provenance instead of silently discarding a finding.
        self.finding["finding"]["trace"][-1].pop("module")
        process, result = self.run_collect(self.messages + [self.finding])
        self.assertEqual(process.returncode, 0, process.stderr)
        self.assertTrue(result["findings_present"])
        self.assertEqual(result["frames_without_module"], 1)
        self.assertEqual(result["security_verdict"], "PENDING_PROJECT_IMPACT_REVIEW")


if __name__ == "__main__":
    unittest.main(verbosity=2)
