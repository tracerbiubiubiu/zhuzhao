#!/usr/bin/env python3
"""Black-box CLI checks; real PostgreSQL evidence is retained in the audit batch."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = "migration-template-preflight.py"


def rule(number, action, template, biz="ticket", enabled=True):
    return dict(id=number, biz_type=biz, event_action=action, template_code=template,
                recipient_type="creator", enabled=enabled, version=1, created_by=None)


def fresh():
    return {"read_only": "on", "checkpoint": [{"version": 81, "dirty": False}],
            "incoming_foreign_keys": [{"name": "fk_message_rules_template_code", "schema": "public",
                "relation": "message_rules", "columns": ["template_code"], "target_columns": ["code"],
                "on_delete": "a", "on_update": "a", "validated": True, "deferrable": False}],
            "rules": [rule(1, "ticket.created", "ticket_created"),
                      rule(2, "ticket.cc_added", "ticket_cc_notify"),
                      rule(3, "ticket.timeout", "ticket_timeout_notify"),
                      rule(4, "ops.cron_failed", "ops_cron_alert", "ops"),
                      rule(5, "ticket.deadline", "ticket_deadline_notify"),
                      rule(6, "ticket.action_notify", "ticket_action_notify"),
                      rule(7, "ops.storage_usage_high", "ops_storage_usage_alert", "ops")]}


class PreflightCLI(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="zz-template-preflight-test-")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        (self.root / "scripts").mkdir()
        shutil.copyfile(ROOT / "scripts" / SCRIPT, self.root / "scripts" / SCRIPT)
        shutil.copytree(ROOT / "migrations", self.root / "migrations")
        self.bin = self.root / "bin"
        self.bin.mkdir()
        psql = self.bin / "psql"
        psql.write_text("#!" + sys.executable + "\n" + r'''
import json, os, pathlib, sys
pathlib.Path(os.environ["MOCK_CAPTURE"]).write_text(json.dumps({
    "argv": sys.argv[1:], "sql": sys.stdin.read(),
    "pg_env": {k:v for k,v in os.environ.items() if k.startswith("PG")}}))
if os.environ.get("MOCK_ERROR"):
    print("sensitive-server-output/password=DO_NOT_ECHO", file=sys.stderr)
    sys.exit(1)
print(os.environ["MOCK_SNAPSHOT"])
''')
        psql.chmod(0o755)
        self.env = {k: v for k, v in os.environ.items() if not k.startswith("PG")}
        self.env.update(PATH=str(self.bin), MOCK_CAPTURE=str(self.root / "capture.json"),
                        ROLLBACK_PREFLIGHT_DSN="postgresql://audit:secret%21@127.0.0.1:25432/audit?sslmode=disable")

    def run_check(self, snapshot=None, target=78, code=0, raw=None):
        self.env["MOCK_SNAPSHOT"] = raw if raw is not None else json.dumps(snapshot if snapshot is not None else fresh())
        run = subprocess.run([sys.executable, str(self.root / "scripts" / SCRIPT), "--target-version", str(target)],
                             env=self.env, text=True, capture_output=True, timeout=10)
        self.assertEqual(run.returncode, code, run.stdout + run.stderr)
        self.assertEqual(run.stderr, "")
        output = json.loads(run.stdout)
        self.assertNotIn("secret!", run.stdout)
        self.assertNotIn("postgresql://", run.stdout)
        self.assertNotIn("DO_NOT_ECHO", run.stdout)
        return output

    def test_fresh_range_removes_own_seeds(self):
        out = self.run_check(target=61)
        self.assertEqual([s["migration"] for s in out["steps"]], [80, 79, 71, 64, 62])
        self.assertTrue(out["requires_code_rollback"])
        self.assertFalse(out["down_executed"])
        self.assertTrue(all(not s["surviving_references"] for s in out["steps"]))
        self.assertEqual([r["id"] for r in out["steps"][3]["rules_matching_down_delete"]], [3, 4])

    def test_custom_including_disabled_rules_block_all_five_downs(self):
        for version, template in [(62, "ticket_cc_notify"), (64, "ticket_timeout_notify"),
                                  (71, "ticket_deadline_notify"), (79, "ticket_action_notify"),
                                  (80, "ops_storage_usage_alert")]:
            with self.subTest(version=version):
                snapshot = fresh()
                snapshot["rules"].append(rule(20, "ticket.transitioned", template, enabled=False))
                out = self.run_check(snapshot, target=61, code=1)
                self.assertEqual(out["first_blocking_migration"], version)
                self.assertEqual(next(s for s in out["steps"] if s["migration"] == version)["surviving_references"][0]["id"], 20)

    def test_later_down_removes_reference_before_older_template(self):
        snapshot = fresh()
        snapshot["rules"][-1]["template_code"] = "ticket_action_notify"
        out = self.run_check(snapshot)
        self.assertEqual(out["steps"][0]["rules_matching_down_delete"][0]["id"], 7)
        self.assertFalse(out["steps"][1]["surviving_references"])

    def test_79_template_qualified_predicate_retains_reconfigured_seed_key(self):
        snapshot = fresh()
        snapshot["rules"][5]["template_code"] = "ticket_deadline_notify"
        out = self.run_check(snapshot, target=70, code=1)
        self.assertFalse(next(s for s in out["steps"] if s["migration"] == 79)["rules_matching_down_delete"])
        self.assertEqual(out["first_blocking_migration"], 71)

    def test_current_checkpoint_and_exact_target_limit_range(self):
        snapshot = fresh()
        snapshot["checkpoint"][0]["version"] = 79
        out = self.run_check(snapshot)
        self.assertEqual([s["migration"] for s in out["steps"]], [79])
        self.assertFalse(out["requires_code_rollback"])
        self.assertEqual(self.run_check(target=80)["steps"], [])

    def test_unsupported_or_dirty_checkpoint_is_unproven(self):
        for target in [0, 60, 81, 82, -1]:
            with self.subTest(target=target):
                self.run_check(target=target, code=2)
        for checkpoints in [None, [], [None], [{"version": 81, "dirty": True}],
                            [{"version": "81", "dirty": False}], [{"version": 82, "dirty": False}],
                            [{"version": 81, "dirty": False}] * 2]:
            snapshot = fresh()
            snapshot["checkpoint"] = checkpoints
            self.run_check(snapshot, code=2)

    def test_source_drift_unknown_related_down_and_range_gap_fail_closed(self):
        path = self.root / "migrations/000079_action_notify_seed.down.sql"
        original = path.read_bytes()
        path.write_bytes(original + b"\n-- changed historical down\n")
        self.run_check(code=2)
        path.write_bytes(original)
        new = self.root / "migrations/000082_new.down.sql"
        new.write_text("DELETE FROM message_templates WHERE code='future';")
        snapshot = fresh()
        snapshot["checkpoint"][0]["version"] = 82
        self.run_check(snapshot, code=2)
        new.unlink()
        (self.root / "migrations/000080_storage_usage_alert_seed.down.sql").unlink()
        self.run_check(code=2)

    def test_unknown_missing_or_changed_incoming_fk_fails_closed(self):
        known = fresh()["incoming_foreign_keys"][0]
        for fks in [[], [None], [dict(known, schema="custom")], [dict(known, on_delete="c")],
                    [dict(known, validated=False)], [dict(known, deferrable=True)],
                    [known, dict(known, relation="custom_refs")]]:
            snapshot = fresh()
            snapshot["incoming_foreign_keys"] = fks
            self.run_check(snapshot, code=2)

    def test_bad_snapshot_and_non_read_only_fail_closed(self):
        for raw in ["not-json", "null", "[]"]:
            self.run_check(raw=raw, code=2)
        for change in [dict(read_only="off"), dict(rules=None), dict(rules=[None]),
                       dict(rules=[dict(rule(1, "event", "code"), enabled="false")])]:
            snapshot = fresh()
            snapshot.update(change)
            self.run_check(snapshot, code=2)

    def test_connection_validation_no_default_or_injected_options(self):
        for dsn in ["", "dbname=audit", "postgresql:///audit", "postgresql://user@host/",
                    "postgresql://user@host/a/b", "postgresql://user@host/a%00b",
                    "postgresql://user@host,audit/a", "postgresql://user@host:bad/a",
                    "postgresql://user@host/a?options=-cdefault_transaction_read_only=off",
                    "postgresql://user@host/a?host=other", "postgresql://user@host/a#fragment",
                    "postgresql://user@host/a?sslmode=disable&sslmode=require",
                    "postgresql://user@host/a?connect_timeout=0", "postgresql://user@host/a%XZ",
                    "postgresql://user@host/a?secret!", "postgresql://user@host/a?sslmode=disable;secret!"]:
            self.env["ROLLBACK_PREFLIGHT_DSN"] = dsn
            self.run_check(code=2)
            self.assertFalse((self.root / "capture.json").exists())

    def test_read_only_sql_and_credentials_stay_out_of_argv_and_output(self):
        self.env.update(PGHOST="wrong", PGOPTIONS="-c default_transaction_read_only=off", PGSERVICE="wrong")
        self.run_check()
        capture = json.loads((self.root / "capture.json").read_text())
        self.assertEqual(capture["argv"], ["-X", "-w", "-qAt", "-v", "ON_ERROR_STOP=1"])
        self.assertEqual(capture["pg_env"]["PGHOST"], "127.0.0.1")
        self.assertEqual(capture["pg_env"]["PGPASSWORD"], "secret!")
        self.assertNotIn("PGOPTIONS", capture["pg_env"])
        self.assertNotIn("PGSERVICE", capture["pg_env"])
        sql = capture["sql"]
        self.assertTrue(sql.startswith("BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;"))
        self.assertTrue(sql.endswith("ROLLBACK;\n"))
        self.assertNotRegex(sql, r"(?i)\b(DELETE|INSERT|UPDATE|DROP|TRUNCATE|ALTER)\s")

    def test_tool_error_and_absence_are_unproven_and_redacted(self):
        self.env["MOCK_ERROR"] = "1"
        self.run_check(code=2)
        self.env.pop("MOCK_ERROR")
        (self.bin / "psql").unlink()
        self.run_check(code=2)


if __name__ == "__main__":
    unittest.main(verbosity=2)
