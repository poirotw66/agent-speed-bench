#!/usr/bin/env python3
"""Score allowlisted source files against pinned dependencies and external tests."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parent.parent
REPO = ROOT / "runs/repos/lazygit"
CASES = json.loads((ROOT / "benchmarks/real-go/cases.json").read_text())


def score(case, layer, candidate, retained=False):
    if case not in CASES or layer not in {"core", "regression"}:
        raise ValueError("Unknown scoring case or layer")
    spec = CASES[case]
    submissions = {}
    hashes = {}
    if retained:
        records = json.loads((candidate.parent / "source_manifest.json").read_text())
        hashes = {record["path"]: record["sha256"] for record in records}
    for source in spec["sources"]:
        path = candidate / (source + ".txt" if retained else source)
        resolved = path.resolve(strict=True)
        root = candidate.resolve(strict=True)
        if path.is_symlink() or root not in resolved.parents or not resolved.is_file() or resolved.stat().st_size > 8 * 1024 * 1024:
            raise ValueError("Invalid submitted source: " + source)
        submissions[source] = resolved.read_bytes()
        if retained and hashlib.sha256(submissions[source]).hexdigest() != hashes.get(source):
            raise ValueError("Retained source hash mismatch: " + source)
    with tempfile.TemporaryDirectory(prefix="agentspeedbench-real-go-") as work:
        workspace = Path(work).resolve()
        archive = workspace / "base.tar"
        with archive.open("wb") as output:
            subprocess.run(["git", "-C", str(REPO), "archive", spec["base"], "go.mod", "go.sum", "pkg", "vendor"], stdout=output, check=True, timeout=90)
        with tarfile.open(archive) as snapshot:
            for entry in snapshot.getmembers():
                target = (workspace / entry.name).resolve()
                if workspace not in target.parents or not (entry.isfile() or entry.isdir()):
                    raise ValueError("Unsafe archive entry")
            snapshot.extractall(workspace)
        archive.unlink()
        for source, data in submissions.items():
            (workspace / source).write_bytes(data)
        env = dict(os.environ, GOWORK="off", GOFLAGS="", GOENV="off", GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local", CGO_ENABLED="0")
        failed = False
        for check in spec["checks"]:
            fixture = ROOT / "benchmarks/real-go" / spec["test_directory"] / (layer + "_" + check["name"] + "_test.go.txt")
            shutil.copyfile(fixture, workspace / check["package"] / "agentspeedbench_external_test.go")
            pattern = "^TestASBCore" if layer == "core" else "^(TestASBRegression|" + check["public_pattern"] + ")"
            result = subprocess.run(["go", "test", "-mod=vendor", "-count=1", "./" + check["package"], "-run", pattern], cwd=workspace, env=env, timeout=120, check=False)
            failed = failed or result.returncode != 0
        return 1 if failed else 0


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("case", choices=sorted(CASES))
    parser.add_argument("layer", choices=["core", "regression"])
    parser.add_argument("candidate", type=Path, nargs="?", default=Path.cwd())
    parser.add_argument("--retained", action="store_true", help="Read retained source files with .txt suffixes")
    args = parser.parse_args()
    raise SystemExit(score(args.case, args.layer, args.candidate, args.retained))
