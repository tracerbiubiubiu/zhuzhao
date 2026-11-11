#!/usr/bin/env python3
"""Black-box generator entry regressions; no SDK, network or service is changed.

Run: python3 scripts/test-go-tool.py. Real generator regeneration is a separate
SDK-backed gate; these stubs exercise failure paths and process boundaries.
"""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

SOURCE = Path(os.environ.get("GO_TOOL_TEST_SOURCE", Path(__file__).resolve().parents[1]))
REAL_MAKE = shutil.which("make")
PINS = {
    "wire": ["github.com/google/wire", "github.com/google/wire/cmd/wire", "v0.7.0",
             "h1:JxUKI6+CVBgCO2WToKy/nQk0sS+amI9z9EjVmdaocj4="],
    "swag": ["github.com/swaggo/swag", "github.com/swaggo/swag/cmd/swag", "v1.16.4",
             "h1:clWJtd9LStiG3VeijiCfOVODP6VpHtKdQy9ELFG3s1A="],
}
GO_STUB = r'''
import json, os, sys, time
from pathlib import Path
args = sys.argv[1:]
keys = ("GOTOOLCHAIN", "GOWORK", "GOFLAGS", "GOBIN", "GOPATH", "GOMODCACHE", "GOCACHE", "GOSUMDB", "GONOSUMDB", "GOPRIVATE", "GOINSECURE", "GOOS", "GOARCH")
with open(os.environ["MOCK_LOG"], "a") as output:
    output.write(json.dumps(dict(kind="go", args=args, cwd=os.getcwd(), env={k: os.environ.get(k) for k in keys})) + "\n")
if args == ["env", "-json", "GOVERSION", "GOHOSTOS", "GOHOSTARCH"]:
    print(json.dumps(dict(GOVERSION=os.environ.get("MOCK_GOVERSION", "go1.26.10"), GOHOSTOS="darwin", GOHOSTARCH="arm64")))
elif args[:2] == ["version", "-m"]:
    try:
        data = json.loads(Path(args[2]).read_text().splitlines()[1][6:])
        print(args[2] + ": " + data["compiler"])
        print("\tpath\t" + data["package"])
        print("\tmod\t" + data["module"] + "\t" + data["version"] + "\t" + data["checksum"])
        print("\tbuild\tGOOS=" + data["os"])
        print("\tbuild\tGOARCH=" + data["arch"])
        if data.get("replace"): print("\t=>\tlocal-replacement\t(devel)")
    except Exception:
        print("unrecognized executable", file=sys.stderr); sys.exit(1)
elif args[:1] == ["install"]:
    if os.environ.get("MOCK_INSTALL_SLEEP"): time.sleep(30)
    if os.environ.get("MOCK_INSTALL_FAIL"):
        print("test proxy unavailable", file=sys.stderr); sys.exit(71)
    pins = json.loads(os.environ["MOCK_PINS"])
    name = next(name for name, pin in pins.items() if args[1] == pin[1] + "@" + pin[2])
    module, package, version, checksum = pins[name]
    data = dict(module=module, package=package, version=version, checksum=checksum,
                compiler="go1.26.10", os="darwin", arch="arm64")
    executable = Path(os.environ["GOBIN"]) / name
    executable.write_text("#!" + sys.executable + "\n#META " + json.dumps(data) + "\n" + os.environ["MOCK_TOOL_SOURCE"])
    executable.chmod(0o700)
else: sys.exit(72)
'''
TOOL_STUB = r'''
import json, os, shutil, sys
with open(os.environ["MOCK_LOG"], "a") as output:
    output.write(json.dumps(dict(kind="tool", args=sys.argv[1:], cwd=os.getcwd(), go=shutil.which("go"), toolchain=os.environ.get("GOTOOLCHAIN"))) + "\n")
print("generator executed")
sys.exit(int(os.environ.get("MOCK_TOOL_EXIT", "0")))
'''


class GoToolTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix="zz-go-tools-")
        self.addCleanup(temporary.cleanup)
        self.base = Path(temporary.name).resolve()
        self.root = self.base / "source with spaces"
        (self.root / "scripts").mkdir(parents=True)
        for path in ("Makefile", "scripts/go-tool.py"):
            shutil.copy2(SOURCE / path, self.root / path)
        (self.root / "go.mod").write_text("module example.org/application\n\ngo 1.26.10\n")
        (self.root / "go.sum").write_text("application sums must not change\n")
        self.sdk = self.base / "selected sdk" / "bin"
        self.sdk.mkdir(parents=True)
        self.go = self.sdk / "go"
        self.go.write_text(f"#!{sys.executable}\n{GO_STUB}")
        self.go.chmod(0o700)
        # Arbitrary PATH generators must never be used by the Make targets.
        for name in PINS:
            decoy = self.sdk / name
            decoy.write_text(f"#!{sys.executable}\nimport sys; sys.exit(73)\n")
            decoy.chmod(0o700)
        self.cache = self.base / "private tool cache"
        self.log = self.base / "events.jsonl"
        self.env = {k: v for k, v in os.environ.items()
                    if not k.startswith(("MOCK_", "GO_TOOL_")) and k not in
                    ("GO", "WIRE_BIN", "SWAG_BIN", "MAKEFLAGS", "MFLAGS", "MAKELEVEL")}
        self.env.update(GO=str(self.go), PATH=str(self.sdk) + os.pathsep + self.env.get("PATH", ""),
                        GO_TOOL_CACHE=str(self.cache), MOCK_LOG=str(self.log),
                        MOCK_PINS=json.dumps(PINS), MOCK_TOOL_SOURCE=TOOL_STUB)

    def run_tool(self, *args, **env):
        return subprocess.run([sys.executable, str(self.root / "scripts/go-tool.py"), *args],
                              cwd=self.base, env={**self.env, **env}, text=True,
                              capture_output=True, timeout=10)

    def events(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def binary(self, name="wire"):
        return self.cache / "go1.26.10/darwin_arm64" / name / PINS[name][2] / name

    def test_install_is_pinned_isolated_and_sumdb_cannot_be_disabled(self):
        result = self.run_tool("install", GOSUMDB="off", GONOSUMDB="*", GOPRIVATE="*",
                               GOINSECURE="*", GOFLAGS="-mod=mod")
        self.assertEqual(result.returncode, 0, result.stderr)
        installs = [e for e in self.events() if e["args"][:1] == ["install"]]
        self.assertEqual([e["args"][1] for e in installs], [p[1] + "@" + p[2] for p in PINS.values()])
        for event in installs:
            self.assertTrue(Path(event["cwd"]).is_relative_to(self.cache))
            for key in ("GOPATH", "GOMODCACHE", "GOCACHE", "GOBIN"):
                self.assertTrue(Path(event["env"][key]).is_relative_to(self.cache))
            self.assertEqual(event["env"]["GOTOOLCHAIN"], "local")
            self.assertEqual(event["env"]["GOWORK"], "off")
            self.assertEqual(event["env"]["GOSUMDB"], "sum.golang.org")
            for key in ("GONOSUMDB", "GOPRIVATE", "GOINSECURE", "GOFLAGS"):
                self.assertEqual(event["env"][key], "")
        self.assertEqual((self.root / "go.mod").read_text(), "module example.org/application\n\ngo 1.26.10\n")
        self.assertEqual((self.root / "go.sum").read_text(), "application sums must not change\n")

    def test_cache_hit_is_reverified_without_reinstalling(self):
        self.assertEqual(self.run_tool("install", "wire").returncode, 0)
        self.log.unlink()
        result = self.run_tool("wire", "path with spaces", "--flag")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(any(e["args"][:1] == ["install"] for e in self.events()))
        self.assertTrue(any(e["args"][:2] == ["version", "-m"] for e in self.events()))
        event = next(e for e in self.events() if e["kind"] == "tool")
        self.assertEqual(event["args"], ["path with spaces", "--flag"])
        self.assertEqual(event["cwd"], str(self.root))
        self.assertEqual(event["go"], str(self.go))
        self.assertEqual(event["toolchain"], "local")

    def test_wrong_provenance_and_version_fail_without_run_or_reinstall(self):
        self.assertEqual(self.run_tool("install", "wire").returncode, 0)
        original = self.binary().read_text()
        for key, value in (("module", "example.org/impostor"), ("package", "example.org/impostor/cmd"),
                           ("version", "v0.6.0"), ("checksum", "h1:forged"), ("compiler", "go1.26.6"),
                           ("os", "linux"), ("arch", "amd64"), ("replace", True)):
            with self.subTest(key=key):
                lines = original.splitlines()
                data = json.loads(lines[1][6:]); data[key] = value
                lines[1] = "#META " + json.dumps(data)
                self.binary().write_text("\n".join(lines) + "\n")
                self.log.unlink(missing_ok=True)
                result = self.run_tool("wire")
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("来源/版本/平台不符或缓存已损坏", result.stderr)
                self.assertFalse(any(e["kind"] == "tool" or e["args"][:1] == ["install"] for e in self.events()))

    def test_corrupt_or_nonexecutable_cache_fails_closed(self):
        self.assertEqual(self.run_tool("install", "wire").returncode, 0)
        for executable in (True, False):
            with self.subTest(executable=executable):
                self.binary().write_text("corrupt cache")
                self.binary().chmod(0o700 if executable else 0o600)
                self.log.unlink()
                result = self.run_tool("wire")
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(any(e["kind"] == "tool" or e["args"][:1] == ["install"] for e in self.events()))

    def test_explicit_bin_is_verified_and_missing_bin_does_not_install(self):
        result = self.run_tool("wire", WIRE_BIN=str(self.base / "absent wire"))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("不可执行", result.stderr)
        self.assertFalse(self.cache.exists())
        self.assertEqual(self.run_tool("install", "wire").returncode, 0)
        self.log.unlink()
        result = self.run_tool("wire", WIRE_BIN=str(self.binary()))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(any(e["args"][:1] == ["install"] for e in self.events()))

    def test_missing_or_old_sdk_fails_before_any_install(self):
        for env in ({"GO": str(self.base / "no sdk")}, {"MOCK_GOVERSION": "go1.26.6"}):
            with self.subTest(env=env):
                self.log.unlink(missing_ok=True)
                result = self.run_tool("wire", **env)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("GO=/path/to/go", result.stderr)
                self.assertFalse(self.cache.exists())
                self.assertFalse(any(e["args"][:1] == ["install"] for e in self.events()))

    def test_install_failure_and_timeout_never_publish_a_binary(self):
        for env, message in (({"MOCK_INSTALL_FAIL": "1"}, "test proxy unavailable"),
                             ({"MOCK_INSTALL_SLEEP": "1", "GO_TOOL_INSTALL_TIMEOUT": "1"}, "超过 1 秒")):
            with self.subTest(env=env):
                result = self.run_tool("wire", **env)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(message, result.stderr)
                self.assertFalse(self.binary().exists())
                self.assertFalse(list(self.cache.rglob(".install-*")))
                self.assertFalse(any(e["kind"] == "tool" for e in self.events()))

    def test_generator_failure_propagates(self):
        result = self.run_tool("wire", MOCK_TOOL_EXIT="23")
        self.assertEqual(result.returncode, 23, result.stderr)

    @unittest.skipUnless(REAL_MAKE, "make unavailable")
    def test_original_make_targets_use_the_same_pinned_entry(self):
        for target, expected in (("wire", ["./internal/app/"]),
                                 ("swag", ["init", "-g", "cmd/server/main.go", "-o", "docs", "--parseDependency", "--parseInternal"])):
            with self.subTest(target=target):
                self.log.unlink(missing_ok=True)
                result = subprocess.run([REAL_MAKE, target], cwd=self.root, env=self.env,
                                        text=True, capture_output=True, timeout=10)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(next(e for e in self.events() if e["kind"] == "tool")["args"], expected)


if __name__ == "__main__":
    unittest.main(verbosity=2)
