#!/usr/bin/env python3
"""Grade one submitted source file against an immutable upstream snapshot."""
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parent.parent
REPO = ROOT / "runs/repos/lazygit"
BASE = "8f258a3650cef809b911df24881712bc6b5d96bd"
FIXED = "38dd035e289dd71ad16fb0caa34525ad03460d21"
SOURCE = Path("pkg/commands/git_commands/github.go")


def score(layer, candidate):
    if layer not in {"core", "regression"}:
        raise ValueError("Unknown scoring layer")
    if candidate.is_symlink() or not candidate.is_file() or candidate.stat().st_size > 8 * 1024 * 1024:
        raise ValueError("Invalid candidate source")
    submitted = candidate.read_bytes()
    with tempfile.TemporaryDirectory(prefix="agentspeedbench-lazygit-") as work:
        workspace = Path(work).resolve()
        archive = workspace / "base.tar"
        with archive.open("wb") as output:
            subprocess.run(["git", "-C", str(REPO), "archive", BASE, "go.mod", "go.sum", "pkg", "vendor"], stdout=output, check=True, timeout=90)
        with tarfile.open(archive) as snapshot:
            # Git archives contain no hard links; reject special entries and
            # traversal before extracting, including on Python 3.9.
            for member in snapshot.getmembers():
                target = (workspace / member.name).resolve()
                if workspace not in target.parents or not (member.isfile() or member.isdir()):
                    raise ValueError("Unsafe upstream archive entry: " + member.name)
            snapshot.extractall(workspace)
        archive.unlink()
        (workspace / SOURCE).write_bytes(submitted)
        tests = ROOT / "benchmarks/real-go/lazygit-owner" / (layer + "_test.go.txt")
        shutil.copyfile(tests, workspace / SOURCE.parent / "agentspeedbench_external_test.go")
        env = dict(os.environ, GOWORK="off", GOFLAGS="", GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local", CGO_ENABLED="0")
        pattern = "^TestASBCore" if layer == "core" else "^(TestASBRegression|TestGenerateGithubPullRequestMap)"
        result = subprocess.run(["go", "test", "-mod=vendor", "-count=1", "./pkg/commands/git_commands", "-run", pattern], cwd=workspace, env=env, check=False, timeout=120)
        return result.returncode


if __name__ == "__main__":
    if len(sys.argv) not in {2, 3}:
        sys.exit("Usage: verify-lazygit.py LAYER [CANDIDATE]")
    sys.exit(score(sys.argv[1], Path(sys.argv[2]) if len(sys.argv) == 3 else Path.cwd() / SOURCE))
