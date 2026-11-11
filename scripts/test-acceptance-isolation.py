#!/usr/bin/env python3
"""Black-box acceptance isolation regressions; all external commands are stubs.

Run with python3 scripts/test-acceptance-isolation.py. No Docker daemon,
PostgreSQL instance, application, or shared acceptance fixture is contacted.
"""

import concurrent.futures
import json
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from urllib.parse import parse_qs, quote, unquote, urlsplit


ROOT = Path(__file__).resolve().parents[1]
ROUNDTRIP = ROOT / "scripts/migration-roundtrip-check.sh"
PHASES = [ROOT / f"scripts/acceptance-phase{phase}.sh" for phase in ("1", "2a", "2b", "2c")]

STUB = r'''
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time

tool = Path(sys.argv[0]).name
args = sys.argv[1:]
event = {"tool": tool, "args": args, "invocation": os.environ.get("MOCK_INVOCATION", "")}

def record():
    fd = os.open(os.environ["MOCK_LOG"], os.O_CREAT | os.O_APPEND | os.O_WRONLY, 0o600)
    try:
        os.write(fd, (json.dumps(event) + "\n").encode())
    finally:
        os.close(fd)

def dsn():
    for arg in args:
        if arg.startswith(("postgres://", "postgresql://")):
            return arg
        if arg.startswith("--dbname=postgres"):
            return arg.split("=", 1)[1]
    return ""

if tool == "mktemp":
    result = subprocess.run([os.environ["MOCK_REAL_MKTEMP"], *args], capture_output=True, text=True)
    event["created"] = result.stdout.strip()
    record()
    sys.stdout.write(result.stdout)
    sys.stderr.write(result.stderr)
    sys.exit(result.returncode)

if tool == "psql":
    sql = "\n".join(args[index + 1] for index, arg in enumerate(args[:-1]) if arg in ("-c", "--command"))
    event["sql"] = sql
    event["dsn"] = dsn()
    if re.search(r"\bCREATE\s+DATABASE\b", sql, re.I):
        event["operation"] = "create"
        record()
        sys.exit(17 if os.environ.get("MOCK_FAIL_CREATE") == "1" else 0)
    if re.search(r"\bDROP\s+DATABASE\b", sql, re.I):
        event["operation"] = "drop"
        record()
        sys.exit(18 if os.environ.get("MOCK_FAIL_DROP") == "1" else 0)
    if "FROM menus" in sql:
        event["operation"] = "snapshot"
        record()
        print("menus\x1fstable seed")
        sys.exit(0)
    event["operation"] = "preflight"
    record()
    if os.environ.get("MOCK_FAIL_PREFLIGHT") == "1":
        sys.exit(21)
    print(os.environ.get("MOCK_CREATEDB", "t"))
    sys.exit(0)

if tool in ("migrate", "migrate with spaces"):
    event["dsn"] = dsn()
    event["operation"] = next((arg for arg in args if arg in ("up", "down", "version")), "other")
    record()
    if event["operation"] == "version":
        print("81")
    if event["operation"] == "up":
        time.sleep(float(os.environ.get("MOCK_MIGRATE_DELAY", "0")))
    sys.exit(19 if event["operation"] == os.environ.get("MOCK_FAIL_MIGRATE") else 0)

if tool == "curl":
    output = next((args[index + 1] for index, arg in enumerate(args[:-1]) if arg in ("-o", "--output")), None)
    event["output"] = output
    record()
    if os.environ.get("MOCK_CURL_UNTIL_OUTPUT") == "1":
        payload = {"status": "ok", "code": 0, "data": {"access_token": "dummy-at", "refresh_token": "dummy-rt"}}
        if output:
            # A mutation restoring /tmp/p1.json must never write the shared path.
            target = Path(output).resolve()
            parent = Path(os.environ["TMPDIR"]).resolve()
            if os.path.commonpath([str(target), str(parent)]) == str(parent):
                target.write_text(json.dumps(payload))
            sys.exit(22)
        print(json.dumps(payload))
        sys.exit(0)
    # Every API call fails locally, including nested phase scripts. Nothing is sent.
    print("stubbed curl failure")
    sys.exit(22)

record()
if tool == "docker":
    if args and args[0] == "ps":
        print("zhuzhao-dev-postgres\nzhuzhao-dev-redis")
    elif "redis-cli" in args:
        print("PONG")
    sys.exit(0)
sys.exit(97)
'''


class AcceptanceIsolationTest(unittest.TestCase):
    def setUp(self):
        self.workspace = tempfile.TemporaryDirectory(prefix="zz-isolation-test-")
        self.addCleanup(self.workspace.cleanup)
        self.work = Path(self.workspace.name)
        self.bin = self.work / "bin"
        self.bin.mkdir()
        self.parent = self.work / "temporary parent with spaces"
        self.parent.mkdir()
        self.sentinel = self.parent / "other-session-sentinel"
        self.sentinel.write_text("must survive\n")
        self.log = self.work / "commands.jsonl"
        for tool in ("psql", "migrate", "migrate with spaces", "docker", "curl", "mktemp"):
            executable = self.bin / tool
            executable.write_text(f"#!{sys.executable}\n{STUB}")
            executable.chmod(0o700)
        self.env = os.environ.copy()
        for key in list(self.env):
            if key.startswith(("PG", "ROUNDTRIP_", "MOCK_")) or key in (
                "MIG", "PSQL", "TMPDIR", "BASE_URL", "HEALTH_URL", "BASE", "HEALTH",
                "REDIS_CONTAINER", "MAKEFLAGS", "MFLAGS",
            ):
                self.env.pop(key, None)
        self.env.update(
            PATH=f"{self.bin}{os.pathsep}{os.environ['PATH']}",
            TMPDIR=str(self.parent),
            MIG=str(self.bin / "migrate"),
            MOCK_LOG=str(self.log),
            MOCK_REAL_MKTEMP=shutil.which("mktemp"),
        )

    def run_script(self, *args, env=None, make=False):
        command = ["make", "acceptance"] if make else ["bash", str(ROUNDTRIP), *args]
        return subprocess.run(
            command, cwd=ROOT, env={**self.env, **(env or {})},
            text=True, capture_output=True, timeout=20,
        )

    def events(self, tool=None):
        events = [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []
        return [event for event in events if tool is None or event["tool"] == tool]

    def assert_no_writes(self):
        events = self.events()
        self.assertFalse([event for event in events if event["tool"] in ("docker", "curl", "migrate")], events)
        self.assertFalse([event for event in events if event.get("operation") in ("create", "drop")], events)

    def assert_own_directory_cleaned(self):
        created = [Path(event["created"]) for event in self.events("mktemp") if event["created"]]
        self.assertTrue(created, self.events())
        self.assertEqual(len(created), len(set(created)))
        for directory in created:
            self.assertEqual(directory.parent.resolve(), self.parent.resolve())
            self.assertFalse(directory.exists(), directory)
        self.assertEqual(self.sentinel.read_text(), "must survive\n")
        return created

    @staticmethod
    def database_name(event):
        match = re.search(r'\b(?:CREATE|DROP)\s+DATABASE\s+(?:IF\s+EXISTS\s+)?"?([a-zA-Z0-9_]+)', event["sql"], re.I)
        if match is None:
            raise AssertionError(f"cannot find owned database in {event!r}")
        return match.group(1)

    def assert_same_roundtrip_target(self, expected_host, expected_port, expected_password, expected_user="zhuzhao"):
        creates = [event for event in self.events("psql") if event["operation"] == "create"]
        drops = [event for event in self.events("psql") if event["operation"] == "drop"]
        self.assertEqual(len(creates), 1, creates)
        self.assertEqual(len(drops), 1, drops)
        database = self.database_name(creates[0])
        self.assertRegex(database, r"^zz_roundtrip_[a-zA-Z0-9_]+$")
        self.assertEqual(self.database_name(drops[0]), database)
        self.assertNotIn(database, ("postgres", "zhuzhao"))
        migrations = self.events("migrate") + self.events("migrate with spaces")
        snapshots = [event for event in self.events("psql") if event["operation"] == "snapshot"]
        self.assertEqual([event["operation"] for event in migrations], ["up", "version", "down", "up"])
        self.assertEqual(len(snapshots), 2, snapshots)
        self.assertEqual(len({event["dsn"] for event in migrations + snapshots}), 1)
        for event in migrations + snapshots:
            uri = urlsplit(event["dsn"])
            self.assertEqual(uri.hostname, expected_host)
            self.assertEqual(uri.port, expected_port)
            self.assertEqual(unquote(uri.username or ""), expected_user)
            self.assertEqual(unquote(uri.password or ""), expected_password)
            self.assertEqual(unquote(uri.path), f"/{database}")
            self.assertEqual(parse_qs(uri.query)["connect_timeout"], ["5"])
        administration = [event for event in self.events("psql") if event["operation"] != "snapshot"]
        self.assertEqual(len({event["dsn"] for event in administration}), 1)
        for event in administration:
            uri = urlsplit(event["dsn"])
            self.assertEqual(uri.hostname, expected_host)
            self.assertEqual(uri.port, expected_port)
            self.assertEqual(unquote(uri.username or ""), expected_user)
            self.assertEqual(unquote(uri.password or ""), expected_password)
            self.assertEqual(unquote(uri.path), "/postgres")
        self.assertFalse(self.events("docker"), self.events("docker"))

    def test_check_is_read_only_and_does_not_create_temporary_directory(self):
        result = self.run_script("--check")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assert_no_writes()
        self.assertFalse(self.events("mktemp"))
        probes = self.events("psql")
        self.assertTrue(probes)
        for probe in probes:
            self.assertIn("-X", probe["args"])
            self.assertIn("-w", probe["args"])
            self.assertIn("ON_ERROR_STOP=1", probe["args"])

    def test_custom_container_requires_explicit_roundtrip_dsn_before_any_write(self):
        result = self.run_script(env={"PG_CONTAINER": "isolated-postgres"})
        self.assertNotEqual(result.returncode, 0)
        self.assert_no_writes()
        self.assertFalse(self.events("mktemp"))

    def test_unsafe_or_ambiguous_dsn_is_rejected_before_any_write(self):
        bad = (
            "postgres://u:p@127.0.0.1:5439/postgres?dbname=zhuzhao",
            "postgres://u:p@127.0.0.1:5439/postgres?options=-csearch_path%3Dpublic",
            "postgres://u:p@127.0.0.1:5439/postgres?sslmode=disable&sslmode=require",
            "postgres://u:p@127.0.0.1:5439/postgres?connect_timeout=5&connect_timeout=6",
            "postgres://u:p@127.0.0.1:5439/postgres#hidden",
            "postgres://u:p@127.0.0.1:5439/postgres?host=localhost",
            "dbname=zhuzhao host=localhost",
        )
        for dsn in bad:
            with self.subTest(dsn=dsn):
                result = self.run_script(env={"ROUNDTRIP_DSN": dsn})
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assert_no_writes()
                self.assertFalse(self.events("mktemp"))

    def test_role_without_createdb_fails_before_any_write(self):
        result = self.run_script(env={"MOCK_CREATEDB": "f"})
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(self.events("psql"))
        self.assert_no_writes()
        self.assertFalse(self.events("mktemp"))

    def test_failed_read_only_connection_does_not_create_or_migrate(self):
        result = self.run_script(env={"MOCK_FAIL_PREFLIGHT": "1"})
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(self.events("psql"))
        self.assert_no_writes()
        self.assertFalse(self.events("mktemp"))

    def test_make_checks_roundtrip_before_application_or_container_mutations(self):
        result = self.run_script(make=True, env={"MOCK_CREATEDB": "f"})
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(self.events("psql"))
        self.assert_no_writes()

    def test_default_development_target_remains_compatible(self):
        result = self.run_script()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assert_same_roundtrip_target("localhost", 5432, "zhuzhao_dev")
        self.assert_own_directory_cleaned()

    def test_custom_port_encoded_password_and_migrate_path_use_one_database(self):
        password = "p@:/?#% space"
        result = self.run_script(env={
            "PG_CONTAINER": "isolated-postgres",
            "ROUNDTRIP_DSN": f"postgresql://audit_user:{quote(password, safe='')}@127.0.0.1:55439/postgres?sslmode=disable",
            "MIG": str(self.bin / "migrate with spaces"),
        })
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assert_same_roundtrip_target("127.0.0.1", 55439, password, expected_user="audit_user")
        self.assert_own_directory_cleaned()

    def test_failed_create_does_not_drop_someone_elses_database(self):
        result = self.run_script(env={"MOCK_FAIL_CREATE": "1"})
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(len([event for event in self.events("psql") if event["operation"] == "create"]), 1)
        self.assertFalse([event for event in self.events("psql") if event["operation"] == "drop"])
        self.assertFalse(self.events("migrate"))
        self.assert_own_directory_cleaned()

    def test_migration_failure_drops_only_successfully_created_database(self):
        result = self.run_script(env={"MOCK_FAIL_MIGRATE": "up"})
        self.assertNotEqual(result.returncode, 0)
        creates = [event for event in self.events("psql") if event["operation"] == "create"]
        drops = [event for event in self.events("psql") if event["operation"] == "drop"]
        self.assertEqual(len(creates), 1)
        self.assertEqual(len(drops), 1)
        self.assertEqual(self.database_name(creates[0]), self.database_name(drops[0]))
        self.assert_own_directory_cleaned()

    def test_drop_failure_makes_gate_fail_and_still_cleans_owned_directory(self):
        result = self.run_script(env={"MOCK_FAIL_DROP": "1"})
        self.assertNotEqual(result.returncode, 0)
        self.assert_same_roundtrip_target("localhost", 5432, "zhuzhao_dev")
        self.assert_own_directory_cleaned()

    def test_term_cleans_created_database_and_owned_directory(self):
        process = subprocess.Popen(
            ["bash", str(ROUNDTRIP)], cwd=ROOT,
            env={**self.env, "MOCK_MIGRATE_DELAY": "0.3"},
            text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        )
        try:
            deadline = time.monotonic() + 5
            while not any(event.get("operation") == "up" for event in self.events("migrate")):
                self.assertIsNone(process.poll(), "roundtrip exited before the controlled migration wait")
                self.assertLess(time.monotonic(), deadline, "migration did not reach controlled wait")
                time.sleep(0.01)
            process.send_signal(signal.SIGTERM)
            stdout, stderr = process.communicate(timeout=5)
            self.assertEqual(process.returncode, 143, (stdout, stderr))
        finally:
            if process.poll() is None:
                process.kill()
                process.communicate(timeout=5)
        creates = [event for event in self.events("psql") if event["operation"] == "create"]
        drops = [event for event in self.events("psql") if event["operation"] == "drop"]
        self.assertEqual(len(creates), 1)
        self.assertEqual(len(drops), 1)
        self.assertEqual(self.database_name(creates[0]), self.database_name(drops[0]))
        self.assert_own_directory_cleaned()

    def test_parallel_invocations_have_distinct_owned_databases_and_directories(self):
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            results = list(pool.map(
                lambda invocation: self.run_script(env={"MOCK_MIGRATE_DELAY": "0.2", "MOCK_INVOCATION": str(invocation)}),
                range(2),
            ))
        for result in results:
            self.assertEqual(result.returncode, 0, result.stderr)
        creates = [event for event in self.events("psql") if event["operation"] == "create"]
        drops = [event for event in self.events("psql") if event["operation"] == "drop"]
        self.assertEqual(len(creates), 2)
        self.assertEqual(len({self.database_name(event) for event in creates}), 2)
        for invocation in ("0", "1"):
            own_create = [event for event in creates if event["invocation"] == invocation]
            own_drop = [event for event in drops if event["invocation"] == invocation]
            self.assertEqual(len(own_drop), 1)
            self.assertEqual(self.database_name(own_create[0]), self.database_name(own_drop[0]))
        self.assertEqual(len(self.assert_own_directory_cleaned()), 2)

    def test_each_acceptance_phase_and_child_owns_its_temporary_directory(self):
        expected_directories = 0
        for count, script in enumerate(PHASES, 1):
            with self.subTest(script=script.name):
                result = subprocess.run(
                    ["bash", str(script)], cwd=ROOT, env=self.env,
                    text=True, capture_output=True, timeout=20,
                )
                self.assertNotEqual(result.returncode, 0, "stubbed curl must prevent a false acceptance pass")
                expected_directories += count
                created = self.assert_own_directory_cleaned()
                self.assertEqual(len(created), expected_directories)
        self.assertTrue(self.events("curl"))

    def test_parallel_phase_responses_are_written_only_inside_each_owned_directory(self):
        def run_phase(invocation):
            return subprocess.run(
                ["bash", str(PHASES[0])], cwd=ROOT,
                env={**self.env, "MOCK_CURL_UNTIL_OUTPUT": "1", "MOCK_INVOCATION": str(invocation)},
                text=True, capture_output=True, timeout=20,
            )

        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            results = list(pool.map(run_phase, range(2)))
        for result in results:
            self.assertNotEqual(result.returncode, 0, "first output-file response deliberately stops the script")
        outputs = [event for event in self.events("curl") if event["output"]]
        directories = self.events("mktemp")
        self.assertEqual(len(outputs), 2, self.events())
        self.assertEqual(len({event["output"] for event in outputs}), 2, outputs)
        for invocation in ("0", "1"):
            own_directories = [event for event in directories if event["invocation"] == invocation]
            own_outputs = [event for event in outputs if event["invocation"] == invocation]
            self.assertEqual(len(own_directories), 1)
            self.assertEqual(len(own_outputs), 1)
            self.assertEqual(Path(own_outputs[0]["output"]).parent, Path(own_directories[0]["created"]))
        self.assertEqual(len(self.assert_own_directory_cleaned()), 2)

    def test_acceptance_sources_no_longer_use_fixed_shared_tmp_filenames(self):
        for script in [ROUNDTRIP, *PHASES]:
            with self.subTest(script=script.name):
                self.assertNotRegex(script.read_text(), r"/tmp/[a-zA-Z0-9_]+\.(?:json|txt|png)")


if __name__ == "__main__":
    unittest.main(verbosity=2)
