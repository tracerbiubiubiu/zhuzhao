#!/usr/bin/env python3
"""Black-box commit-lint checks in disposable repositories, without project hooks."""

import os
from pathlib import Path
import subprocess
import tempfile
import unittest


LINTER = Path(__file__).resolve().with_name("commit-lint.sh")
COMPLETE = "改动摘要：修复边界\n同类扫描：已核同类入口\n\n影响面：认证\n\n验证证据：回归通过\n"


class FixBodyTests(unittest.TestCase):
    def check_message(self, subject, body):
        with tempfile.TemporaryDirectory(prefix="zz-commit-lint-test-") as tmp:
            repo = Path(tmp)
            env = dict(os.environ)
            # Do not inherit the caller's repository/index or commit identity.
            for key in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"):
                env.pop(key, None)
            env.update(GIT_AUTHOR_NAME="Commit lint fixture",
                       GIT_AUTHOR_EMAIL="fixture@example.invalid",
                       GIT_COMMITTER_NAME="Commit lint fixture",
                       GIT_COMMITTER_EMAIL="fixture@example.invalid")

            def git(*args):
                return subprocess.check_output(
                    ["git", "-c", "core.hooksPath=/dev/null",
                     "-c", "commit.gpgSign=false", *args],
                    cwd=repo, env=env, text=True, stderr=subprocess.STDOUT)

            git("init", "-q")
            path = repo / "fixture.txt"
            message = repo / "message.txt"
            path.write_text("before\n")
            git("add", "--", "fixture.txt")
            message.write_text("docs: 初始化测试\n\n测试夹具\n")
            git("commit", "-q", "-F", str(message), "--", "fixture.txt")
            baseline = git("rev-parse", "HEAD").strip()
            path.write_text("after\n")
            message.write_text(subject + "\n\n" + body)
            git("commit", "-q", "-F", str(message), "--", "fixture.txt")
            return subprocess.run(
                ["sh", str(LINTER), baseline + "..HEAD"], cwd=repo,
                env=env, text=True, capture_output=True)

    def test_complete_scoped_fix(self):
        r = self.check_message("fix(auth): 修复令牌撤权", COMPLETE)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertNotIn("[WARN]", r.stdout)

    def test_complete_scopeless_fix(self):
        r = self.check_message("fix: 修复令牌撤权", COMPLETE)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertNotIn("[WARN]", r.stdout)

    def test_missing_section_warns_without_blocking_history(self):
        r = self.check_message("fix(auth): 修复令牌撤权", COMPLETE.replace("影响面：认证\n", ""))
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("fix body missing section: impact", r.stdout)

    def test_empty_evidence_cannot_be_filled_by_refs_footer(self):
        r = self.check_message("fix(auth): 修复令牌撤权", COMPLETE.replace("验证证据：回归通过", "验证证据：\n\nRefs: ECO-20261104-B02-F01"))
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("fix body empty section: evidence", r.stdout)

    def test_titles_in_prose_are_not_sections(self):
        r = self.check_message("fix: 修复令牌撤权", "本次包含改动摘要、影响面和验证证据\n同类扫描：已核\n")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(r.stdout.count("fix body missing section:"), 3)

    def test_markdown_and_ascii_colons(self):
        body = "## 改动摘要\n修复边界\n同类扫描：已核\n\n**影响面：**\n认证\n\n验证证据: 回归通过\n"
        r = self.check_message("fix(auth): 修复令牌撤权", body)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertNotIn("[WARN]", r.stdout)

    def test_duplicate_section_warns(self):
        r = self.check_message("fix(auth): 修复令牌撤权", COMPLETE + "\n验证证据：重复\n")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("fix body duplicate section: evidence", r.stdout)

    def test_non_fix_body_keeps_existing_policy(self):
        r = self.check_message("docs: 更新审计说明", "补充说明\n")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertNotIn("fix body", r.stdout)

    def test_empty_body_still_fails_existing_gate(self):
        r = self.check_message("fix(auth): 修复令牌撤权", "")
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("[5] body empty", r.stdout)

    def test_same_class_scan_also_checks_scopeless_fix(self):
        r = self.check_message("fix: 修复令牌撤权", COMPLETE.replace("同类扫描：已核同类入口\n", ""))
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("lacks 'tonglei-scan' line", r.stdout)


if __name__ == "__main__":
    unittest.main(verbosity=2)
