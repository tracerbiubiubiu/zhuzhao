#!/usr/bin/env python3
"""Check tracked and new repository Go files, excluding ignored local caches."""
from pathlib import Path
import os
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]


def main():
    inventory = subprocess.run(
        ["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", "*.go"],
        cwd=ROOT, capture_output=True,
    )
    if inventory.returncode:
        sys.stderr.write("ERROR: cannot enumerate repository Go sources\n")
        return 1
    files = sorted({os.fsdecode(name) for name in inventory.stdout.split(b"\0") if name})
    files = [name for name in files if not name.startswith("vendor/") and (ROOT / name).is_file()]
    if not files:
        sys.stderr.write("ERROR: no repository Go sources found\n")
        return 1
    failed = False
    for start in range(0, len(files), 100):
        try:
            result = subprocess.run(["gofmt", "-l", *files[start:start + 100]],
                                    cwd=ROOT, capture_output=True, text=True)
        except OSError:
            sys.stderr.write("ERROR: gofmt is unavailable; select the required Go SDK\n")
            return 1
        sys.stdout.write(result.stdout)
        sys.stderr.write(result.stderr)
        failed = failed or bool(result.returncode or result.stdout)
    if failed:
        sys.stderr.write("ERROR: repository gofmt check failed; format the listed sources\n")
    return int(failed)


if __name__ == "__main__":
    sys.exit(main())
