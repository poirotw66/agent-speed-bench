#!/usr/bin/env python3
"""Fetch pinned real-case commits and gate their external scoring tests."""
from pathlib import Path
import runpy
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parent.parent
metadata = runpy.run_path(str(ROOT / "scripts/verify-lazygit.py"))
repo, base, fixed, source = (metadata[key] for key in ("REPO", "BASE", "FIXED", "SOURCE"))


def git(*args):
    return subprocess.run(["git", "-C", str(repo), *args], check=True, capture_output=True, timeout=180).stdout


def main():
    if not repo.exists():
        repo.mkdir(parents=True)
        git("init", "--quiet", "--template=")
        git("remote", "add", "origin", "https://github.com/jesseduffield/lazygit.git")
        git("fetch", "--quiet", "--depth=2", "origin", fixed)
    for commit in (base, fixed):
        if git("rev-parse", "--verify", commit + "^{commit}").decode().strip() != commit:
            raise RuntimeError("Unexpected case commit")
    if git("rev-parse", fixed + "^").decode().strip() != base:
        raise RuntimeError("Case base is not the fix parent")
    # Materialize the complete base now; grading must not fetch filtered blobs.
    git("archive", base)
    with tempfile.TemporaryDirectory(prefix="agentspeedbench-gate-") as work:
        candidate = Path(work) / "github.go"
        for commit, layer, want in ((base, "core", False), (base, "regression", True), (fixed, "core", True), (fixed, "regression", True)):
            candidate.write_bytes(git("show", commit + ":" + source.as_posix()))
            result = subprocess.run([sys.executable, str(ROOT / "scripts/verify-lazygit.py"), layer, str(candidate)], text=True, capture_output=True, timeout=180)
            print(result.stdout, end="")
            if (result.returncode == 0) != want or (not want and "--- FAIL: TestASBCoreOwnerCasing" not in result.stdout):
                print(result.stderr, file=sys.stderr)
                raise RuntimeError("Case quality gate failed: " + commit + "/" + layer)
    print("Real-case gate passed: base fails core; base regression and fixed layers pass.")


if __name__ == "__main__":
    main()
