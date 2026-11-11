#!/usr/bin/env python3
"""Exercise the main-repository wrapper, including its Swagger failure path."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

SCRIPTS = Path(__file__).resolve().parent


class WrapperTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='zz-commit-window-')
        self.addCleanup(self.tmp.cleanup)
        self.repo = Path(self.tmp.name)
        self.env = dict(os.environ)
        for key in ('GIT_DIR', 'GIT_WORK_TREE', 'GIT_INDEX_FILE'):
            self.env.pop(key, None)
        self.env.update(GIT_AUTHOR_NAME='Fixture', GIT_COMMITTER_NAME='Fixture',
                        GIT_AUTHOR_EMAIL='fixture@example.invalid',
                        GIT_COMMITTER_EMAIL='fixture@example.invalid')
        self.git('init', '-q')
        self.base = self.commit('docs: 初始化', '正文')
        self.legacy = self.commit('legacy invalid subject', '旧历史')
        control = self.repo / 'control'
        control.mkdir()
        for name in ('commit-lint.sh', 'commit-policy.py', 'check-fix-body.awk'):
            shutil.copy2(SCRIPTS / name, control / name)
        (control / 'commit-policy-baseline').write_text(self.legacy + '\n')
        self.script = control / 'commit-lint.sh'

    def git(self, *args):
        return subprocess.check_output(
            ['git', '-c', 'core.hooksPath=/dev/null', '-c', 'commit.gpgSign=false', *args],
            cwd=self.repo, env=self.env, text=True, stderr=subprocess.STDOUT).strip()

    def commit(self, subject, body, path='fixture.txt'):
        target = self.repo / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(target.read_text() + 'next\n' if target.exists() else 'first\n')
        message = self.repo / 'message.txt'
        message.write_text(subject + '\n\n' + body + '\n')
        self.git('add', '--', path)
        self.git('commit', '-q', '-F', str(message), '--', path)
        return self.git('rev-parse', 'HEAD')

    def lint(self, *args, env=None, cwd=None):
        return subprocess.run(['sh', str(self.script), *args], cwd=cwd or self.repo,
                              env=env or self.env, text=True, capture_output=True)

    def test_default_initial_window_exempts_legacy(self):
        self.commit('docs: 更新说明', '正文')
        result = self.lint()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('adoption baseline ancestors excluded', result.stdout)

    def test_explicit_history_remains_strict(self):
        self.commit('docs: 更新说明', '正文')
        result = self.lint(self.base + '..HEAD')
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn('legacy invalid subject', result.stdout)

    def test_initial_window_does_not_guess_twenty(self):
        self.commit('new invalid subject', '正文')
        for index in range(21):
            self.commit('docs: 更新%d' % index, '正文')
        result = self.lint()
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn('new invalid subject', result.stdout)

    def test_swag_failure_is_not_false_green(self):
        self.commit('docs: 更新接口夹具', '正文', 'internal/handler/fixture.go')
        binary = self.repo / 'stub-bin'
        binary.mkdir()
        (binary / 'swag').write_text('#!/bin/sh\nexit 23\n')
        (binary / 'swag').chmod(0o755)
        result = self.lint(self.legacy + '..HEAD', env={
            **self.env, 'PATH': str(binary) + os.pathsep + self.env['PATH']})
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn('swagger gate: generator failed', result.stdout)

    def test_explicit_empty_range_in_shallow_clone_is_rejected(self):
        clone = self.repo / 'shallow'
        self.git('clone', '-q', '--depth=1', self.repo.as_uri(), str(clone))
        result = self.lint('HEAD..HEAD', cwd=clone)
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn('shallow history', result.stdout)


if __name__ == '__main__':
    unittest.main(verbosity=2)
