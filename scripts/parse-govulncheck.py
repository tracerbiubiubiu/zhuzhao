#!/usr/bin/env python3
"""Collect govulncheck evidence. Success means collection, never security approval."""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys

TOOL_VERSION = "v1.4.0"
TOOL_SUM = "h1:FpmTZiV4PyqY3lFfuCkz1JftEXb/+8M2NEkjJM5TF4g="
TOOL_MOD_SUM = "h1:FJ7XyKs83nAdxQ7PMsia2PoynwZMJ/QajXVMBBIgFe8="


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def write(path, data):
    Path(path).parent.mkdir(parents=True, exist_ok=True)
    Path(path).write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n")


def checked(argv, cwd):
    return subprocess.check_output(argv, cwd=cwd, text=True).strip()


def snapshot(args):
    root = Path(args.source).resolve()
    head = checked(["git", "rev-parse", "HEAD"], root)
    if head != args.expected_sha:
        raise ValueError(f"source HEAD mismatch: {head} != {args.expected_sha}")
    status = checked(["git", "status", "--porcelain", "--untracked-files=all"], root)
    if status:
        raise ValueError(f"source worktree is not clean: {status}")
    mod = json.loads(checked(["go", "mod", "edit", "-json"], root))
    if mod.get("Replace"):
        raise ValueError("replace directives are prohibited")
    if mod.get("Module", {}).get("Path") != args.expected_module:
        raise ValueError("unexpected source module path")
    names = subprocess.check_output(["git", "ls-files", "-z"], cwd=root).decode().split("\0")
    files = {}
    for name in filter(None, names):
        path = root / name
        data = os.readlink(path).encode() if path.is_symlink() else path.read_bytes()
        files[name] = hashlib.sha256(data).hexdigest()
    write(args.output, {"head": head, "module": args.expected_module, "files": files,
                        "go_version": checked(["go", "env", "GOVERSION"], root),
                        "replace_directives": [], "worktree_clean": True})


def verify_tool(args):
    module = json.loads(Path(args.module_json).read_text())
    if module.get("Error"):
        raise ValueError(f"tool download failed: {module['Error']}")
    for key, expected in [("Path", "golang.org/x/vuln"), ("Version", TOOL_VERSION),
                          ("Sum", TOOL_SUM), ("GoModSum", TOOL_MOD_SUM)]:
        if module.get(key) != expected:
            raise ValueError(f"tool {key} mismatch")
    info = Path(args.build_info).read_text()
    if not info.splitlines()[0].endswith(": " + args.expected_go):
        raise ValueError("unexpected scanner compiler version")
    sums = {}
    sum_path = Path(module["Dir"]) / "go.sum"
    for line in sum_path.read_text().splitlines():
        path, version, checksum = line.split()
        sums[(path, version)] = checksum
    dependencies = []
    main_module = False
    settings = {}
    for line in info.splitlines()[1:]:
        fields = line.strip().split()
        if not fields:
            continue
        if fields[0] == "=>":
            raise ValueError("scanner build contains a module replacement")
        if fields[0] == "mod":
            if fields[1:] != ["golang.org/x/vuln", TOOL_VERSION, TOOL_SUM]:
                raise ValueError("unexpected scanner main module")
            main_module = True
        if fields[0] == "dep":
            if len(fields) != 4 or sums.get((fields[1], fields[2])) != fields[3]:
                raise ValueError(f"scanner dependency does not match official go.sum: {line}")
            dependencies.append({"path": fields[1], "version": fields[2], "sum": fields[3]})
        if fields[0] == "build" and len(fields) == 2 and "=" in fields[1]:
            key, value = fields[1].split("=", 1)
            settings[key] = value
    if not main_module or not dependencies:
        raise ValueError("missing scanner module/dependency provenance")
    for key, expected in [("GOOS", args.expected_goos), ("GOARCH", args.expected_goarch),
                          ("CGO_ENABLED", "0")]:
        if settings.get(key) != expected:
            raise ValueError(f"scanner build {key} mismatch")
    write(args.output, {"status": "OFFICIAL_FIXED_TOOL_PROVENANCE_VERIFIED",
                        "version": TOOL_VERSION, "module_sum": TOOL_SUM,
                        "go_mod_sum": TOOL_MOD_SUM, "compiler": args.expected_go,
                        "binary_sha256": digest(args.binary), "settings": settings,
                        "dependencies": dependencies, "official_go_sum_sha256": digest(sum_path),
                        "build_info_sha256": digest(args.build_info),
                        "sumdb_required": "sum.golang.org"})


def decode_stream(text):
    def invalid_constant(value):
        raise ValueError(f"invalid JSON constant: {value}")

    decoder = json.JSONDecoder(parse_constant=invalid_constant)
    offset = 0
    messages = []
    while offset < len(text):
        if text[offset].isspace():
            offset += 1
            continue
        message, offset = decoder.raw_decode(text, offset)
        if not isinstance(message, dict) or len(message) != 1:
            raise ValueError("each protocol message must be an object with exactly one field")
        if next(iter(message)) not in {"config", "progress", "SBOM", "osv", "finding"}:
            raise ValueError("unknown govulncheck protocol message")
        messages.append(message)
    return messages


def collect(args):
    result = {"status": "EVIDENCE_COLLECTION_FAILED", "security_verdict": "UNKNOWN",
              "observed_utc": datetime.datetime.now(datetime.timezone.utc).isoformat(),
              "errors": [], "scan_exit": None}
    try:
        result["scan_exit"] = int(Path(args.exit_code).read_text().strip())
        if result["scan_exit"] != 0:
            result["errors"].append(f"scanner execution exited {result['scan_exit']}")
        raw = Path(args.input)
        result["raw_sha256"] = digest(raw)
        messages = decode_stream(raw.read_text())
        configs = [m["config"] for m in messages if "config" in m]
        sboms = [m["SBOM"] for m in messages if "SBOM" in m]
        if len(configs) != 1 or not messages or "config" not in messages[0]:
            raise ValueError("missing, duplicate or noninitial config")
        config = configs[0]
        for key, expected in [("protocol_version", "v1.0.0"), ("scanner_name", "govulncheck"),
                              ("scanner_version", TOOL_VERSION), ("scan_mode", args.mode),
                              ("scan_level", "symbol"),
                              ("db", "https://vuln.go.dev")]:
            if config.get(key) != expected:
                raise ValueError(f"config.{key} mismatch: {config.get(key)!r} != {expected!r}")
        if args.mode == "source" and config.get("go_version") != args.expected_go:
            raise ValueError("source config Go version mismatch")
        # v1.4.0 emits config before extracting the binary. Binary Go version
        # is authoritative in SBOM, and may be absent from the initial config.
        if args.mode == "binary" and config.get("go_version") not in (None, "", args.expected_go):
            raise ValueError("binary config unexpectedly reports another Go version")
        modified = config.get("db_last_modified")
        if not isinstance(modified, str):
            raise ValueError("missing actual vulnerability database modification time")
        datetime.datetime.fromisoformat(modified.replace("Z", "+00:00"))
        if len(sboms) != 1 or not sboms[0].get("roots") or not sboms[0].get("modules"):
            raise ValueError("missing, duplicate or empty SBOM roots/modules")
        if (not isinstance(sboms[0]["roots"], list)
                or any(not isinstance(root, str) or not root for root in sboms[0]["roots"])
                or not isinstance(sboms[0]["modules"], list)
                or any(not isinstance(module, dict) or not isinstance(module.get("path"), str)
                       or not module["path"] for module in sboms[0]["modules"])):
            raise ValueError("invalid SBOM roots/modules structure")
        if sboms[0].get("go_version") != args.expected_go:
            raise ValueError("SBOM Go version mismatch")
        result.update({"config": config, "SBOM": sboms[0], "message_count": len(messages)})
        if args.mode == "source":
            environment = json.loads(Path(args.go_env).read_text())
            for key, expected in [("GOOS", "linux"), ("GOARCH", "arm64"), ("CGO_ENABLED", "0"),
                                  ("GOWORK", "off"), ("GOFLAGS", "-mod=readonly"),
                                  ("GOVERSION", args.expected_go)]:
                if environment.get(key) != expected:
                    raise ValueError(f"actual source go env {key} mismatch")
            result["source_go_env"] = environment
            before = json.loads(Path(args.before).read_text())
            after = json.loads(Path(args.after).read_text())
            if before != after or before.get("go_version") != args.expected_go:
                raise ValueError("source content, HEAD, module or Go context changed")
            module_path = before["module"]
            if (module_path not in {module["path"] for module in sboms[0]["modules"]}
                    or any(root != module_path and not root.startswith(module_path + "/")
                           for root in sboms[0]["roots"])):
                raise ValueError("source SBOM does not correspond to the frozen module")
            result["source"] = {"head": before["head"], "module": before["module"],
                                "files": len(before["files"]), "unchanged": True,
                                "before_sha256": digest(args.before), "after_sha256": digest(args.after)}
        findings = []
        for message in messages:
            if "finding" not in message:
                continue
            finding = message["finding"]
            if not re.fullmatch(r"GO-\d{4}-\d+", finding.get("osv", "")):
                raise ValueError("invalid finding GO-ID")
            trace = finding.get("trace")
            if not isinstance(trace, list) or not trace or any(not isinstance(f, dict) for f in trace):
                raise ValueError("finding has no valid trace")
            first = trace[0]
            level = "symbol" if first.get("function") else "package" if first.get("package") else "module"
            findings.append({"level": level, **finding})
        ids = sorted({f["osv"] for f in findings})
        result.update({"findings": findings, "go_ids": ids, "finding_messages": len(findings),
                       "symbol_finding_messages": sum(f["level"] == "symbol" for f in findings),
                       "frames_without_module": sum(not frame.get("module") for finding in findings
                                                    for frame in finding["trace"]),
                       "findings_present": bool(findings),
                       "interpretation": "Source traces and binary symbol presence require separate project-impact review; not an exploitability certificate."})
        if not result["errors"]:
            result["status"] = ("EVIDENCE_COLLECTED_FINDINGS_REQUIRE_TRIAGE" if findings
                                else "EVIDENCE_COLLECTED_NO_FINDINGS_REPORTED")
            result["security_verdict"] = "PENDING_PROJECT_IMPACT_REVIEW" if findings else "NO_FINDINGS_REPORTED_UNDER_RECORDED_CONFIGURATION"
    except (ValueError, TypeError, KeyError, AttributeError, IndexError, OSError) as error:
        result["errors"].append(str(error))
    write(args.output, result)
    message = f"{result['status']}: {len(result.get('go_ids', []))} GO-IDs; scanner exit={result['scan_exit']}. Collection success is not security approval."
    print(message)
    if result.get("findings_present"):
        print("::warning title=Go vulnerability findings require triage::" + message)
    if result["errors"]:
        print("::error title=Go evidence collection failed::" + "; ".join(result["errors"]).replace("\n", " "))
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a") as handle:
            handle.write("\n### Go security evidence\n\n" + message + "\n\n")
            handle.write("Security verdict: **" + result["security_verdict"] + "**\n\n")
            handle.write("GO-IDs: " + (", ".join(result.get("go_ids", [])) or "none reported; see execution status") + "\n")
    return 1 if result["errors"] else 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    snap = commands.add_parser("snapshot")
    for field in ["source", "expected-sha", "expected-module", "output"]:
        snap.add_argument("--" + field, required=True)
    verify = commands.add_parser("verify-tool")
    for field in ["module-json", "build-info", "binary", "expected-go", "expected-goos", "expected-goarch", "output"]:
        verify.add_argument("--" + field, required=True)
    scan = commands.add_parser("collect")
    for field in ["input", "exit-code", "expected-go", "output"]:
        scan.add_argument("--" + field, required=True)
    scan.add_argument("--mode", choices=["source", "binary"], required=True)
    scan.add_argument("--before")
    scan.add_argument("--after")
    scan.add_argument("--go-env")
    args = parser.parse_args()
    if args.command == "snapshot":
        snapshot(args)
    elif args.command == "verify-tool":
        verify_tool(args)
    else:
        return collect(args)
    return 0


if __name__ == "__main__":
    sys.exit(main())
