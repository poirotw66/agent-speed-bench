#!/usr/bin/env python3
"""Check the Go compiler against the repository's minimum language version."""
from pathlib import Path
import re
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parent.parent


def check():
    required = re.search(r"^go (\d+)\.(\d+)\.(\d+)$", (ROOT / "go.mod").read_text(), re.MULTILINE)
    if not required:
        raise ValueError("Could not determine the required Go version from go.mod")
    executable = shutil.which("go")
    if not executable:
        raise ValueError("Go is missing from PATH; install the Go toolchain in a persistent directory and add its bin directory to PATH")
    version = subprocess.check_output([executable, "version"], text=True)
    observed = re.search(r"\bgo(\d+)\.(\d+)(?:\.(\d+))?\b", version)
    minimum = tuple(map(int, required.groups()))
    if not observed or tuple(int(part or 0) for part in observed.groups()) < minimum:
        raise ValueError("Go " + ".".join(required.groups()) + "+ is required; observed: " + version.strip())
    print("Toolchain:", Path(executable).resolve(), ";", version.strip())


if __name__ == "__main__":
    try:
        check()
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        sys.exit("Toolchain check failed: " + str(error))
