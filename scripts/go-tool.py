#!/usr/bin/env python3
"""Run pinned Go generators without adding their CLI dependencies to the app module."""
import json
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parent.parent
TOOLS = {
    "wire": ("github.com/google/wire", "github.com/google/wire/cmd/wire", "v0.7.0",
             "h1:JxUKI6+CVBgCO2WToKy/nQk0sS+amI9z9EjVmdaocj4="),
    # CLI and runtime library are pinned explicitly; installer verifies module sum and compiler.
    "swag": ("github.com/swaggo/swag", "github.com/swaggo/swag/cmd/swag", "v1.16.6",
             "h1:qBNcx53ZaX+M5dxVyTrgQ0PJ/ACK+NzhwcbieTt+9yI="),
}


class ToolError(Exception):
    pass


def seconds(name, default):
    value = os.environ.get(name, str(default))
    if not value.isdigit() or not 1 <= int(value) <= 600:
        raise ToolError(f"{name} 必须为 1–600 秒的整数")
    return int(value)


def run(argv, *, env, cwd, timeout):
    try:
        process = subprocess.Popen(argv, cwd=cwd, env=env, text=True,
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                   start_new_session=(os.name == "posix"))
    except OSError as error:
        raise ToolError(f"无法执行 {argv[0]}: {error}") from error
    try:
        stdout, stderr = process.communicate(timeout=timeout)
    except subprocess.TimeoutExpired as error:
        # Go can spawn compiler/linker children. Stop this invocation's whole
        # process group rather than leaving installers behind after a timeout.
        def stop(sig):
            try:
                if os.name == "posix":
                    os.killpg(process.pid, sig)
                elif sig == signal.SIGTERM:
                    process.terminate()
                else:
                    process.kill()
            except ProcessLookupError:
                pass
        stop(signal.SIGTERM)
        try:
            process.communicate(timeout=5)
        except subprocess.TimeoutExpired:
            stop(signal.SIGKILL)
            process.communicate()
        raise ToolError(f"{argv[0]} 超过 {timeout} 秒，已停止本次进程；未重试") from error
    return process.returncode, stdout, stderr


def go_context():
    requested = os.environ.get("GO", "go")
    found = shutil.which(requested)
    if not found:
        raise ToolError("找不到 Go；先安装 go.mod 要求的 SDK，或设置 GO=/path/to/go")
    go = str(Path(found).resolve())
    env = dict(os.environ, GOTOOLCHAIN="local", GOWORK="off")
    code, stdout, stderr = run([go, "env", "-json", "GOVERSION", "GOHOSTOS", "GOHOSTARCH"],
                               env=env, cwd=ROOT, timeout=30)
    if code:
        raise ToolError("Go SDK 不满足项目要求；请先准备 go.mod 要求的 SDK，"
                        "或设置 GO=/path/to/go（不自动下载 SDK）。\n" + stderr.strip())
    try:
        context = json.loads(stdout)
        if any(not re.fullmatch(r"[A-Za-z0-9_.-]+", context[k])
               for k in ("GOVERSION", "GOHOSTOS", "GOHOSTARCH")):
            raise ValueError("invalid Go context")
    except (ValueError, KeyError, TypeError) as error:
        raise ToolError("无法识别 Go SDK 构建信息") from error
    # `go env` itself succeeds with an older local SDK. Check the module floor
    # explicitly before installing anything, rather than failing later in Wire.
    floor = re.search(r"(?m)^go\s+(\d+\.\d+(?:\.\d+)?)\s*$", (ROOT / "go.mod").read_text())
    current = re.fullmatch(r"go(\d+\.\d+(?:\.\d+)?)", context["GOVERSION"])
    if not floor or not current:
        raise ToolError("无法识别 go.mod 的 SDK 下限或当前 SDK 正式版本")
    def version_tuple(value):
        return tuple(int(part) for part in value.split(".")) + (0,) * (3 - len(value.split(".")))
    if version_tuple(current[1]) < version_tuple(floor[1]):
        raise ToolError(f"Go SDK {context['GOVERSION']} 低于 go.mod 要求的 go{floor[1]}；"
                        "请设置 GO=/path/to/go（不自动下载 SDK）。")
    # Generators invoke `go list` themselves; make them use the selected SDK too.
    env["PATH"] = str(Path(go).parent) + os.pathsep + env.get("PATH", "")
    return go, env, context


def verify(binary, name, go, env, context):
    if not binary.is_file() or not os.access(binary, os.X_OK):
        raise ToolError(f"{name} 不可执行: {binary}")
    code, stdout, stderr = run([go, "version", "-m", str(binary)], env=env, cwd=ROOT, timeout=30)
    module, package, version, checksum = TOOLS[name]
    lines = stdout.splitlines()
    fields = [line.strip().split() for line in lines[1:]]
    required = [["path", package], ["mod", module, version, checksum],
                ["build", "GOOS=" + context["GOHOSTOS"]],
                ["build", "GOARCH=" + context["GOHOSTARCH"]]]
    if (code or not lines or not lines[0].endswith(": " + context["GOVERSION"])
            or any(row not in fields for row in required)
            or any(row and row[0] == "=>" for row in fields)):
        raise ToolError(f"{name} 来源/版本/平台不符或缓存已损坏: {binary}；"
                        f"需要 {module}@{version}，编译器 {context['GOVERSION']}。"
                        "显式 BIN 请重新构建；缓存文件请核查后删除该文件再重试。\n" + stderr.strip())


def obtain(name, go, env, context, cache):
    override = os.environ.get(name.upper() + "_BIN")
    if override:
        binary = Path(override).expanduser().resolve()
        verify(binary, name, go, env, context)
        return binary
    module, package, version, _ = TOOLS[name]
    directory = cache / context["GOVERSION"] / (context["GOHOSTOS"] + "_" + context["GOHOSTARCH"]) / name / version
    binary = directory / name
    if binary.exists():
        verify(binary, name, go, env, context)
        return binary
    directory.mkdir(parents=True, exist_ok=True)
    timeout = seconds("GO_TOOL_INSTALL_TIMEOUT", 180)
    print(f"安装固定工具 {package}@{version} → {binary}（最多 {timeout} 秒，sumdb 开启）", file=sys.stderr, flush=True)
    with tempfile.TemporaryDirectory(prefix=".install-", dir=directory) as temporary:
        install_env = dict(env, GO111MODULE="on", GOFLAGS="", GOBIN=temporary,
                           GOPATH=str(cache / "gopath"), GOMODCACHE=str(cache / "modcache"),
                           GOCACHE=str(cache / "buildcache"), GOSUMDB="sum.golang.org",
                           GONOSUMDB="", GOPRIVATE="", GOINSECURE="",
                           GOOS=context["GOHOSTOS"], GOARCH=context["GOHOSTARCH"], CGO_ENABLED="0")
        code, stdout, stderr = run([go, "install", package + "@" + version], env=install_env,
                                   cwd=temporary, timeout=timeout)
        if code:
            raise ToolError(f"安装 {name}@{version} 失败；检查网络/GOPROXY 后重试 make tools-install。\n" + stdout + stderr)
        candidate = Path(temporary) / name
        verify(candidate, name, go, env, context)
        # Publish only a fully verified executable. Concurrent installs use
        # independent temporary directories and replace the same pinned target.
        os.replace(candidate, binary)
    return binary


def main():
    args = sys.argv[1:]
    if not args or args[0] in ("-h", "--help"):
        print("usage: go-tool.py install [wire|swag ...] | wire|swag [args...]\n"
              "GO=/path/to/go GO_TOOL_CACHE=/path/to/cache WIRE_BIN=/path/to/wire SWAG_BIN=/path/to/swag")
        return 0 if args else 2
    install = args[0] == "install"
    names = (args[1:] or list(TOOLS)) if install else args[:1]
    if any(name not in TOOLS for name in names):
        raise ToolError("只支持固定版本的 wire、swag")
    go, env, context = go_context()
    cache = Path(os.environ.get("GO_TOOL_CACHE", str(ROOT / ".codex" / "tools"))).expanduser().resolve()
    for name in names:
        binary = obtain(name, go, env, context, cache)
        if not install:
            code, stdout, stderr = run([str(binary), *args[1:]], env=env, cwd=ROOT,
                                       timeout=seconds("GO_TOOL_TIMEOUT", 300))
            sys.stdout.write(stdout)
            sys.stderr.write(stderr)
            return code if code >= 0 else 128 - code
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except ToolError as error:
        print(f"ERROR: {error}", file=sys.stderr)
        sys.exit(1)
