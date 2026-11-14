#!/usr/bin/env python3
"""Regression cases for observed image alias and lock-metadata false greens."""
import importlib.util
import json
from pathlib import Path
import shutil
import tempfile
import unittest

SOURCE = Path(__file__).resolve().parent.parent
SPEC = importlib.util.spec_from_file_location("versions", SOURCE / "scripts/check-component-versions.py")
CHECKER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CHECKER)


class VersionsTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="zz-version-contract-")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        paths = ["go.mod", "AGENTS.md", "deployments/Dockerfile", "deployments/component-lock.json",
                 "internal/testutil/testdb_integration.go", "internal/service/storage/e2e_integration_test.go",
                 "internal/service/user_security_state_integration_test.go"]
        paths += [str(p.relative_to(SOURCE)) for p in (SOURCE / "deployments").glob("docker-compose*.yaml")]
        lock = json.loads((SOURCE / "deployments/component-lock.json").read_text())
        for item in lock.get("local_tool_builds", {}).values():
            paths += [item["build_context"] + "/go.mod", item["build_context"] + "/Dockerfile"]
        for rel in paths:
            target = self.root / rel
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(SOURCE / rel, target)
        CHECKER.ROOT = self.root

    def change_builder(self, transform):
        p = self.root / "deployments/Dockerfile"
        p.write_text(transform(p.read_text()))

    def test_actual_combination_and_dockerhub_alias_are_equivalent(self):
        CHECKER.check()
        self.change_builder(lambda s: s.replace("FROM golang:", "FROM docker.io/library/golang:"))
        CHECKER.check()

    def test_alias_with_wrong_digest_is_rejected(self):
        lock = json.loads((self.root / "deployments/component-lock.json").read_text())
        pin = lock["images"]["golang"]["pin"]
        self.change_builder(lambda s: s.replace(pin, "docker.io/library/" + pin.split("@", 1)[0] + "@sha256:" + "0" * 64))
        with self.assertRaises(ValueError): CHECKER.check()

    def test_unknown_registry_or_floating_image_is_rejected(self):
        self.change_builder(lambda s: s.replace("FROM golang:", "FROM unselected.example/golang:"))
        with self.assertRaises(ValueError): CHECKER.check()

    def test_tag_or_index_metadata_mismatch_is_rejected(self):
        p = self.root / "deployments/component-lock.json"
        original = json.loads(p.read_text())
        for key, value in [("tag", "99.0-fake"), ("index_digest", "sha256:" + "0" * 64)]:
            lock = json.loads(json.dumps(original)); lock["images"]["postgres18"][key] = value
            p.write_text(json.dumps(lock))
            with self.assertRaises(ValueError): CHECKER.check()

    def test_local_tool_recipe_version_cannot_drift_from_lock(self):
        p = self.root / "deployments/component-lock.json"
        lock = json.loads(p.read_text()); lock["local_tool_builds"]["migrate"]["source_version"] = "v99.0.0"
        p.write_text(json.dumps(lock))
        with self.assertRaises(ValueError): CHECKER.check()


if __name__ == "__main__": unittest.main(verbosity=2)
