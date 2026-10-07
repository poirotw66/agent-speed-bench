#!/usr/bin/env python3
"""Prepare pinned additional real cases and require base-fails/fixed-passes gates."""
import json
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parent.parent
REPO = ROOT / "runs/repos/lazygit"
CASES = json.loads((ROOT / "benchmarks/real-go/cases.json").read_text())


def git(*args):
    return subprocess.run(["git", "-C", str(REPO), *args], check=True, capture_output=True, timeout=180).stdout


def main():
    if not REPO.exists():
        REPO.mkdir(parents=True)
        git("init", "--quiet", "--template=")
    for name, spec in CASES.items():
        for commit in (spec["base"], spec["fixed"]):
            exists = subprocess.run(["git", "-C", str(REPO), "cat-file", "-e", commit + "^{commit}"], capture_output=True, check=False)
            if exists.returncode:
                git("fetch", "--quiet", "--depth=2", spec["repository"], commit)
        if git("rev-parse", spec["fixed"] + "^1").decode().strip() != spec["base"]:
            raise RuntimeError("Unexpected fix parent for " + name)
        git("archive", spec["base"])
        with tempfile.TemporaryDirectory(prefix="agentspeedbench-case-gate-") as work:
            candidate = Path(work)
            for commit, layer, want in ((spec["base"], "core", False), (spec["base"], "regression", True), (spec["fixed"], "core", True), (spec["fixed"], "regression", True)):
                for source in spec["sources"]:
                    path = candidate / source
                    path.parent.mkdir(parents=True, exist_ok=True)
                    path.write_bytes(git("show", commit + ":" + source))
                result = subprocess.run([sys.executable, str(ROOT / "scripts/verify-real-go.py"), name, layer, str(candidate)], capture_output=True, text=True, timeout=300)
                print(result.stdout, end="")
                if (result.returncode == 0) != want or (not want and "--- FAIL: TestASBCore" not in result.stdout):
                    print(result.stderr, file=sys.stderr)
                    raise RuntimeError("Real-case gate failed: " + name + "/" + commit + "/" + layer)
        print(name + ": base fails core; base regression and fixed layers pass.")


if __name__ == "__main__":
    main()
