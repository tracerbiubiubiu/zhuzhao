#!/usr/bin/env python3
"""Real Git/gofmt boundary regressions; no application or shared state used."""
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

SCRIPT = Path(__file__).with_name("check-gofmt.py")


class GofmtBoundaryTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        (self.root / "scripts").mkdir()
        shutil.copy2(SCRIPT, self.root / "scripts" / SCRIPT.name)
        subprocess.run(["git", "init", "-q", str(self.root)], check=True)
        (self.root / ".gitignore").write_text(".codex/\nvendor/\n")
        (self.root / "main.go").write_text("package main\n\nfunc main() {}\n")
        subprocess.run(["git", "-C", str(self.root), "add", "main.go", ".gitignore"], check=True)

    def check(self):
        return subprocess.run([sys.executable, str(self.root / "scripts" / SCRIPT.name)],
                              cwd=self.root, capture_output=True, text=True)

    def test_ignored_sdk_and_vendor_do_not_enter_source_gate(self):
        for directory in (".codex/sdk/src", "vendor/example"):
            path = self.root / directory
            path.mkdir(parents=True)
            (path / "broken.go").write_text("unparseable local dependency cache")
        self.assertEqual(self.check().returncode, 0)

    def test_new_untracked_source_with_spaces_is_checked(self):
        path = self.root / "new source.go"
        path.write_text("package main\nfunc  untracked( ){}\n")
        result = self.check()
        self.assertEqual(result.returncode, 1)
        self.assertIn(path.name, result.stdout)
        subprocess.run(["gofmt", "-w", str(path)], check=True)
        self.assertEqual(self.check().returncode, 0)

    def test_tracked_drift_is_rejected(self):
        (self.root / "main.go").write_text("package main\nfunc main( ){}\n")
        self.assertEqual(self.check().returncode, 1)

    def test_parse_failure_is_rejected(self):
        (self.root / "main.go").write_text("package main\nfunc (\n")
        self.assertEqual(self.check().returncode, 1)

    def test_missing_git_inventory_is_not_success(self):
        shutil.rmtree(self.root / ".git")
        self.assertEqual(self.check().returncode, 1)


if __name__ == "__main__":
    unittest.main(verbosity=2)
