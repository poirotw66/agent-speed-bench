#!/usr/bin/env python3
"""Score only submitted Go source using trusted external tests and module metadata."""
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile


def score(case, layer, candidate):
    root = Path(__file__).resolve().parent.parent / "benchmarks" / "go-cases"
    if case not in {"clamp", "dedupe", "range"} or layer not in {"core", "regression"}:
        raise ValueError("Unknown scoring case or layer")
    source = candidate.resolve(strict=True)
    if not source.is_file() or source.stat().st_size > 8 * 1024 * 1024:
        raise ValueError("Invalid candidate source")
    with tempfile.TemporaryDirectory(prefix="agentspeedbench-score-") as work:
        workspace = Path(work)
        shutil.copyfile(source, workspace / "candidate.go")
        shutil.copyfile(root / case / (layer + "_test.go.txt"), workspace / "behavior_test.go")
        (workspace / "go.mod").write_text("module candidate\n\ngo 1.26.0\n")
        env = dict(os.environ, GOWORK="off", GOFLAGS="", GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local")
        result = subprocess.run(["go", "test", "-count=1", "."], cwd=workspace, env=env, timeout=50, check=False)
        return result.returncode


if __name__ == "__main__":
    if len(sys.argv) not in {3, 4}:
        sys.exit("Usage: verify-go-case.py CASE LAYER [CANDIDATE]")
    sys.exit(score(sys.argv[1], sys.argv[2], Path(sys.argv[3]) if len(sys.argv) == 4 else Path.cwd() / "candidate.go"))
