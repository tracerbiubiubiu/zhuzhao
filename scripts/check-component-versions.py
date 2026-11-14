#!/usr/bin/env python3
"""Reject deployment/test image drift against the selected OCI combination."""
import json
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parent.parent
DIGEST = re.compile(r"sha256:[0-9a-f]{64}")
LOCAL_BUILDS = {"zhuzhao", "zhuzhao-test", "zhuzhao-ui", "zhuzhao-ui-test",
                "activelist-test", "taskrunner-test", "zhuzhao-rclone", "zhuzhao-migrate"}


def repository(value):
    for host in ("docker.io/", "index.docker.io/", "registry-1.docker.io/"):
        if value.startswith(host):
            value = value[len(host):]
            break
    return value.removeprefix("library/")


def reference(value):
    tagged, digest = value.rsplit("@", 1)
    repo, tag = tagged.rsplit(":", 1)
    if not repo or not tag or not DIGEST.fullmatch(digest):
        raise ValueError(f"patch tag and SHA256 digest required: {value}")
    return repository(repo), tag, digest


def check():
    lock = json.loads((ROOT / "deployments/component-lock.json").read_text())
    images = lock["images"]
    selected = {}
    go = re.search(r"(?m)^go\s+(\S+)$", (ROOT / "go.mod").read_text())[1]
    local_tools = {repository(v["image"].rsplit(":", 1)[0]): v for v in lock.get("local_tool_builds", {}).values()}
    for item in local_tools.values():
        context = (ROOT / item["build_context"]).resolve()
        if not context.is_relative_to(ROOT.resolve()):
            raise ValueError("tool build context must stay in this repository")
        module = (context / "go.mod").read_text()
        if item["go_sdk"] != go or not re.search(r"(?m)^go\s+" + re.escape(go) + r"$", module):
            raise ValueError("local tool compiler floor differs from selected Go SDK")
        if not re.search(re.escape(item["source_module"]) + r"\s+" + re.escape(item["source_version"]) + r"\b", module):
            raise ValueError("local tool upstream version differs from module lock")
        if item["main_package"] not in (context / "Dockerfile").read_text():
            raise ValueError("local tool main package differs from Docker build")
    for name, item in images.items():
        repo, tag, digest = reference(item["pin"])
        if repo != repository(item["registry"] + "/" + item["repository"]) or tag != item["tag"] or digest != item["index_digest"]:
            raise ValueError(f"{name}: pin/repository/tag/index_digest disagree")
        if set(item["platforms"]) != {"amd64", "arm64"} or any(
                not DIGEST.fullmatch(v["digest"]) for v in item["platforms"].values()):
            raise ValueError(f"{name}: exact Linux amd64/arm64 manifest digests required")
        selected[repo] = (repo, tag, digest)
    files = [ROOT / "deployments/Dockerfile", *sorted((ROOT / "deployments").glob("docker-compose*.yaml")),
             *sorted((ROOT / "deployments/tools").glob("*/Dockerfile"))]
    for path in files:
        text = path.read_text()
        blocks = re.split(r"(?m)^  [a-zA-Z0-9_-]+:\s*$", text) if path.suffix == ".yaml" else [text]
        for block in blocks:
            for line in block.splitlines():
                match = re.match(r"\s*(?:FROM|image:)\s+(\S+)", line)
                if not match:
                    continue
                pin = match[1]
                repo = repository(pin.split("@", 1)[0].rsplit(":", 1)[0])
                if repo in LOCAL_BUILDS and re.search(r"(?m)^    build:", block):
                    if repo in local_tools:
                        item = local_tools[repo]
                        expected_repo, expected_tag = item["image"].rsplit(":", 1)
                        if pin.rsplit(":", 1)[-1] != expected_tag:
                            raise ValueError("local tool image tag differs from selected recipe")
                        context = re.search(r"(?m)^    build:\s*([^\n]+)$", block)
                        if not context or (path.parent / context[1].strip()).resolve() != (ROOT / item["build_context"]).resolve():
                            raise ValueError("local tool compose build context differs from selected recipe")
                    continue
                actual = reference(pin)
                if actual[0] not in selected or actual != selected[actual[0]]:
                    raise ValueError(f"{path.relative_to(ROOT)}: unselected image {pin}")
    fixtures = {"internal/testutil/testdb_integration.go": ("postgres18", "return "),
                "internal/service/storage/e2e_integration_test.go": ("rustfs", "Image:"),
                "internal/service/user_security_state_integration_test.go": ("redis8", "Image:")}
    for path, (key, prefix) in fixtures.items():
        expression = re.escape(prefix) + r'\s*"' + re.escape(images[key]["pin"]) + '"'
        if not re.search(expression, (ROOT / path).read_text()):
            raise ValueError(f"{path}: default test image differs from deployment")
    if not images["golang"]["tag"].startswith(go + "-"):
        raise ValueError("go.mod and builder SDK differ")
    if f"Go {go}" not in (ROOT / "AGENTS.md").read_text():
        raise ValueError("AGENTS.md compiler instruction differs")
    print(f"component versions: {len(images)} selected image pins; Go {go}; deployment/test fixtures agree")


if __name__ == "__main__":
    try:
        check()
    except (ValueError, KeyError, TypeError, OSError, AttributeError) as error:
        print(f"ERROR: {error}", file=sys.stderr)
        sys.exit(1)
