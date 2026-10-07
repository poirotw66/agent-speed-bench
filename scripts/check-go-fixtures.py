#!/usr/bin/env python3
"""Require every broken fixture to fail core scoring and every fixed fixture to pass."""
from pathlib import Path
import importlib.util
import sys

path = Path(__file__).with_name("verify-go-case.py")
spec = importlib.util.spec_from_file_location("grader", path)
grader = importlib.util.module_from_spec(spec)
spec.loader.exec_module(grader)
root = path.parent.parent / "benchmarks" / "go-cases"
for case in ("clamp", "dedupe", "range"):
    if grader.score(case, "core", root / case / "base.go.txt") == 0:
        sys.exit("Broken fixture unexpectedly passed: " + case)
    for layer in ("core", "regression"):
        if grader.score(case, layer, root / case / "fixed.go.txt") != 0:
            sys.exit("Fixed fixture failed: " + case + "/" + layer)
print("All three fixture gates passed: base fails core; fixed passes core and regression.")
