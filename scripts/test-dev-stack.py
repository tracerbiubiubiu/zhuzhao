#!/usr/bin/env python3
"""Black-box dev orchestration regressions; every external command is a stub.

Run: python3 scripts/test-dev-stack.py. No Docker daemon or service is touched.
DEV_STACK_TEST_SOURCE can point at a frozen original/mutated source archive.
"""
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import unittest

SOURCE = Path(os.environ.get("DEV_STACK_TEST_SOURCE", Path(__file__).resolve().parents[1]))
REAL_MAKE = shutil.which("make")
STUB = r'''
import json, os, sys
from pathlib import Path
name = Path(sys.argv[0]).name
args = sys.argv[1:]
event = {"tool": name, "args": args, "cwd": os.getcwd(), "inherited_project": os.environ.get("COMPOSE_PROJECT_NAME", "")}
if name in ("docker", "docker-compose") and (name == "docker-compose" or (args and args[0] == "compose")):
    rest = args[:] if name == "docker-compose" else args[1:]
    project, filename = "", ""
    while rest and rest[0].startswith("-"):
        flag = rest.pop(0)
        if flag in ("-p", "--project-name", "-f", "--file"):
            value = rest.pop(0)
            if flag in ("-p", "--project-name"): project = value
            else: filename = str((Path.cwd() / value).resolve())
    event.update(project=project or os.environ.get("COMPOSE_PROJECT_NAME", ""), explicit_project=bool(project), file=filename, operation=rest[0] if rest else "")
with open(os.environ["MOCK_LOG"], "a") as f: f.write(json.dumps(event) + "\n")
if name == "lsof":
    if os.environ.get("MOCK_PORT_BUSY") == "1":
        print("COMMAND PID USER FD TYPE DEVICE SIZE/OFF NODE NAME\nother-app 98765 someone 9u IPv4 0 0t0 TCP *:33333 (LISTEN)")
        sys.exit(0)
    sys.exit(1)
if name in ("go", "kill", "xargs", "sleep"): sys.exit(0)
if name == "make":
    sys.exit(82 if args and args[0] == "docker-dev-up" and os.environ.get("MOCK_FAIL_STACK") == "zhuzhao" else 0)
if name == "docker" and args and args[0] == "info":
    sys.exit(83 if os.environ.get("MOCK_FAIL_INFO") == "1" else 0)
if name == "docker" and args and args[0] == "ps":
    if os.environ.get("MOCK_PROJECT_IN_USE") and "label=com.docker.compose.project=" + os.environ["MOCK_PROJECT_IN_USE"] in args: print("existing-project-id")
    sys.exit(0)
if name == "docker" and args[:2] == ["container", "inspect"]:
    if args[-1] == "existing-project-id": print(os.environ.get("MOCK_PROJECT_SOURCE", "")); sys.exit(0)
    if args[-1] == "activelist-dev-postgres" and os.environ.get("MOCK_FIXED_OWNER"):
        print(os.environ["MOCK_FIXED_OWNER"]); sys.exit(0)
    sys.exit(1)
if name == "docker" and args[:2] == ["network", "inspect"]:
    sys.exit(1)
if name == "docker" and args[:2] == ["network", "create"]:
    sys.exit(84 if os.environ.get("MOCK_FAIL_NETWORK") == "1" else 0)
if "operation" in event:
    op = event["operation"]
    stack = Path.cwd().name
    if op == "config" and stack == os.environ.get("MOCK_FAIL_CONFIG"): sys.exit(85)
    if op == "up" and stack == os.environ.get("MOCK_FAIL_STACK"):
        if stack != "zhuzhao" or event["file"].endswith("docker-compose.dev.yaml"): sys.exit(86)
    if op == "down" and stack == os.environ.get("MOCK_FAIL_DOWN"): sys.exit(87)
    if op == "up" and stack == os.environ.get("MOCK_UNHEALTHY") and "--wait" in args: sys.exit(88)
    if op == "down" and any(x in args for x in ("-v", "--volumes")):
        for path in Path(os.environ["MOCK_DATA"]).glob("*"): path.unlink()
sys.exit(0)
'''


class DevStackTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="zz-dev-stack-")
        self.addCleanup(self.tmp.cleanup)
        self.workspace = Path(self.tmp.name).resolve() / "workspace with spaces"
        self.workspace.mkdir()
        self.root = self.workspace / "zhuzhao"
        for directory in ("scripts", "deployments"):
            (self.root / directory).mkdir(parents=True, exist_ok=True)
        for file in ("scripts/dev-stack.sh", "Makefile", "deployments/docker-compose.dev.yaml", "deployments/docker-compose.yaml"):
            shutil.copy2(SOURCE / file, self.root / file)
        self.al = self.workspace / "activelist"
        self.tr = self.workspace / "taskrunner"
        for root, file in ((self.al, "compose.dev.yaml"), (self.tr, "compose.yaml")):
            (root / "deploy").mkdir(parents=True)
            (root / "deploy" / file).write_text("services: {}\n")
        (self.workspace / "zhuzhao-ui").mkdir()
        self.bin = Path(self.tmp.name) / "bin"
        self.bin.mkdir()
        for name in ("docker", "docker-compose", "make", "lsof", "go", "kill", "xargs", "sleep"):
            executable = self.bin / name
            executable.write_text(f"#!{sys.executable}\n{STUB}")
            executable.chmod(0o700)
        self.log = Path(self.tmp.name) / "commands.jsonl"
        self.data = Path(self.tmp.name) / "existing-user-data"
        self.data.mkdir()
        for name in ("zhuzhao-dev_dev_pg_data", "deploy_pgdata", "taskrunner_pgdata"):
            (self.data / name).write_text("existing rows must survive")
        self.env = {k: v for k, v in os.environ.items() if not k.startswith(("MOCK_", "COMPOSE_")) and k not in ("MAKEFLAGS", "MFLAGS", "ACTIVELIST_DIR", "TASKRUNNER_DIR", "ZHUZHAO_DEV_PROJECT", "ACTIVELIST_DEV_PROJECT", "TASKRUNNER_PROJECT", "DEV_STACK_WAIT_TIMEOUT")}
        self.env.update(PATH=str(self.bin) + os.pathsep + os.environ["PATH"], MOCK_LOG=str(self.log), MOCK_DATA=str(self.data))

    def run_stack(self, command="up", **env):
        return subprocess.run(["bash", str(self.root / "scripts/dev-stack.sh"), command], cwd=self.workspace, env={**self.env, **env}, text=True, capture_output=True, timeout=15)

    def events(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def writes(self):
        return [e for e in self.events() if e.get("operation") in ("up", "down") or e["args"][:2] == ["network", "create"]]

    def test_projects_are_distinct_and_existing_volume_names_preserved(self):
        result = self.run_stack(COMPOSE_PROJECT_NAME="global-contaminated")
        self.assertEqual(result.returncode, 0, result.stderr)
        ups = [e for e in self.events() if e.get("operation") == "up"]
        self.assertEqual([e["project"] for e in ups], ["zhuzhao-dev", "deploy", "taskrunner"])
        self.assertTrue(all(e["explicit_project"] for e in ups))
        self.assertTrue(all("--wait" in e["args"] for e in ups))
        self.assertTrue(ups[0]["file"].endswith("docker-compose.dev.yaml"))
        self.assertEqual(ups[0]["args"][-2:], ["postgres", "redis"])
        self.assertIn("另行检查", result.stdout)
        self.assertEqual(len(list(self.data.iterdir())), 3)

    def test_start_failures_propagate_and_never_use_main_production_fallback(self):
        for stack in ("zhuzhao", "activelist", "taskrunner"):
            with self.subTest(stack=stack):
                self.log.unlink(missing_ok=True)
                result = self.run_stack(MOCK_FAIL_STACK=stack)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertNotIn("✅", result.stdout)
                self.assertFalse(any(e.get("file", "").endswith("/deployments/docker-compose.yaml") for e in self.events()))
                self.assertFalse(any(e["tool"] in ("kill", "xargs") or "-v" in e["args"] or "--volumes" in e["args"] for e in self.events()))

    def test_all_config_validation_happens_before_mutation(self):
        result = self.run_stack(MOCK_FAIL_CONFIG="taskrunner")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.writes(), self.events())

    def test_missing_sibling_or_compose_never_partially_starts(self):
        for path in (self.al / "deploy/compose.dev.yaml", self.tr / "deploy/compose.yaml"):
            with self.subTest(path=path):
                content = path.read_text(); path.unlink()
                self.log.unlink(missing_ok=True)
                result = self.run_stack()
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.writes(), self.events())
                path.write_text(content)
        shutil.rmtree(self.al)
        result = self.run_stack()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.writes())

    def test_network_or_daemon_failure_is_not_reported_as_success(self):
        for env in ({"MOCK_FAIL_NETWORK": "1"}, {"MOCK_FAIL_INFO": "1"}):
            with self.subTest(env=env):
                self.log.unlink(missing_ok=True)
                result = self.run_stack(**env)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("✅", result.stdout)
                self.assertFalse([e for e in self.events() if e.get("operation") == "up"])

    def test_unhealthy_stack_does_not_report_success(self):
        result = self.run_stack(MOCK_UNHEALTHY="taskrunner")
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertNotIn("✅", result.stdout)

    def test_equivalent_symlink_compose_source_keeps_existing_owner(self):
        alias = Path(self.tmp.name) / "checkout-alias"
        alias.symlink_to(self.al, target_is_directory=True)
        result = self.run_stack(MOCK_PROJECT_IN_USE="deploy", MOCK_PROJECT_SOURCE=str(alias / "deploy/compose.dev.yaml"))
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_fixed_container_owned_by_other_project_is_not_adopted(self):
        result = self.run_stack(MOCK_FIXED_OWNER="zhuzhao-dev|postgres")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.writes())

    def test_matching_existing_al_owner_and_source_are_allowed(self):
        result = self.run_stack(MOCK_FIXED_OWNER="deploy|postgres", MOCK_PROJECT_IN_USE="deploy", MOCK_PROJECT_SOURCE=str(self.al / "deploy/compose.dev.yaml"))
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_project_used_by_other_compose_file_is_not_adopted(self):
        result = self.run_stack(MOCK_PROJECT_IN_USE="taskrunner", MOCK_PROJECT_SOURCE="/another/checkout/compose.yaml")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.writes())

    def test_custom_projects_paths_and_wait_budget(self):
        result = self.run_stack(ZHUZHAO_DEV_PROJECT="own-main", ACTIVELIST_DEV_PROJECT="own-al", TASKRUNNER_PROJECT="own-tr", ACTIVELIST_DIR=str(self.al), TASKRUNNER_DIR=str(self.tr), DEV_STACK_WAIT_TIMEOUT="123")
        self.assertEqual(result.returncode, 0, result.stderr)
        ups = [e for e in self.events() if e.get("operation") == "up"]
        self.assertEqual([e["project"] for e in ups], ["own-main", "own-al", "own-tr"])
        self.assertTrue(all(e["args"][e["args"].index("--wait-timeout") + 1] == "123" for e in ups))

    def test_invalid_or_colliding_projects_fail_without_mutation(self):
        for env in ({"TASKRUNNER_PROJECT": "deploy"}, {"ZHUZHAO_DEV_PROJECT": "../other"}, {"DEV_STACK_WAIT_TIMEOUT": "0"}):
            with self.subTest(env=env):
                self.log.unlink(missing_ok=True)
                result = self.run_stack(**env)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.writes())

    def test_reset_stops_in_reverse_order_and_preserves_user_data(self):
        result = self.run_stack("reset")
        self.assertEqual(result.returncode, 0, result.stderr)
        changes = [e for e in self.events() if e.get("operation") in ("up", "down")]
        self.assertEqual([(e["operation"], e["project"]) for e in changes], [("down", "taskrunner"), ("down", "deploy"), ("down", "zhuzhao-dev"), ("up", "zhuzhao-dev"), ("up", "deploy"), ("up", "taskrunner")])
        self.assertTrue(all("-v" not in e["args"] and "--volumes" not in e["args"] for e in changes))
        self.assertEqual(len(list(self.data.iterdir())), 3)

    def test_down_failure_stops_reset_without_starting(self):
        result = self.run_stack("reset", MOCK_FAIL_DOWN="taskrunner")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse([e for e in self.events() if e.get("operation") == "up"])
        self.assertNotIn("✅", result.stdout)

    def test_unknown_command_is_side_effect_free(self):
        result = self.run_stack("destroy-everything")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.events())

    def run_make(self, *args, **env):
        return subprocess.run([REAL_MAKE, *args], cwd=self.root, env={**self.env, **env}, text=True, capture_output=True, timeout=15)

    def test_make_dev_refuses_busy_port_without_killing_owner(self):
        result = self.run_make("dev", MOCK_PORT_BUSY="1")
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("98765", result.stderr)
        self.assertFalse([e for e in self.events() if e["tool"] in ("go", "kill", "xargs")], self.events())

    def test_make_dev_on_free_port_starts_only_stubbed_go(self):
        result = self.run_make("dev", "APP_PORT=33334")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len([e for e in self.events() if e["tool"] == "go"]), 1)
        self.assertFalse([e for e in self.events() if e["tool"] in ("kill", "xargs")])

    def test_make_dependency_targets_ignore_global_project(self):
        for target in ("docker-dev-up", "docker-dev-down"):
            with self.subTest(target=target):
                result = self.run_make(target, COMPOSE_PROJECT_NAME="foreign")
                self.assertEqual(result.returncode, 0, result.stderr)
        commands = [e for e in self.events() if e.get("operation") in ("up", "down")]
        self.assertEqual([e["project"] for e in commands], ["zhuzhao-dev", "zhuzhao-dev"])
        self.assertTrue(all(e["tool"] == "docker" for e in commands))

    def test_production_ui_context_resolves_to_sibling_checkout(self):
        text = (self.root / "deployments/docker-compose.yaml").read_text()
        ui = text.split("\n  ui:\n", 1)[1]
        relative = re.search(r"^\s+context:\s*(\S+)", ui, re.M).group(1)
        resolved = (self.root / "deployments" / relative).resolve()
        self.assertEqual(resolved, self.workspace / "zhuzhao-ui")
        self.assertTrue(resolved.is_dir())


if __name__ == "__main__":
    unittest.main(verbosity=2)
