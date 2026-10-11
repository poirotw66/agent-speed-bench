#!/usr/bin/env python3
"""Build clean Git sources and install atomically while preserving binary hashes."""
import argparse
from contextlib import contextmanager
import fcntl
import hashlib
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent


def digest(path):
    with path.open("rb") as source:
        hash_value = hashlib.sha256()
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            hash_value.update(chunk)
        return hash_value.hexdigest()


def atomic_copy(source, destination):
    destination.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    descriptor, name = tempfile.mkstemp(prefix=".agentspeedbench-", dir=destination.parent)
    try:
        with os.fdopen(descriptor, "wb") as output, source.open("rb") as input_file:
            shutil.copyfileobj(input_file, output)
            output.flush()
            os.fsync(output.fileno())
        os.chmod(name, 0o755)
        os.replace(name, destination)
    finally:
        Path(name).unlink(missing_ok=True)


def archive(source, archive_dir):
    sha = digest(source)
    target = archive_dir / sha / "agentspeedbench"
    if not target.exists():
        atomic_copy(source, target)
    if digest(target) != sha:
        raise ValueError("Archived binary hash mismatch: " + sha)
    return target


@contextmanager
def installation_lock(destination):
    destination.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    # Keep the lock file in place: unlinking it could split concurrent locks.
    with (destination.parent / ".agentspeedbench-install.lock").open("a+b") as lock:
        fcntl.flock(lock.fileno(), fcntl.LOCK_EX)
        try:
            yield
        finally:
            fcntl.flock(lock.fileno(), fcntl.LOCK_UN)


def install(source, destination, archive_dir):
    with installation_lock(destination):
        install_locked(source, destination, archive_dir)


def install_locked(source, destination, archive_dir):
    # Validate both archives before replacing the currently installed executable.
    new_archive = archive(source, archive_dir)
    if destination.exists():
        previous = archive(destination, archive_dir)
        print("Previous binary preserved:", previous)
    atomic_copy(new_archive, destination)
    if digest(destination) != digest(new_archive):
        raise ValueError("Installed binary hash mismatch")
    print("Installed:", destination)
    print("SHA-256:", digest(destination))


def clean_revision():
    if subprocess.check_output(["git", "status", "--porcelain", "--untracked-files=all"], cwd=ROOT, text=True).strip():
        raise ValueError("Clean committed sources are required for installation")
    return subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bin-dir", type=Path, default=Path.home() / ".local/bin")
    parser.add_argument("--archive-dir", type=Path, default=Path.home() / ".local/share/agentspeedbench/binaries")
    parser.add_argument("--restore", help="Restore an archived SHA-256 to the original installation path")
    args = parser.parse_args()
    destination = args.bin_dir.resolve() / "agentspeedbench"
    archive_dir = args.archive_dir.resolve()
    if args.restore:
        if not re.fullmatch(r"[0-9a-f]{64}", args.restore):
            raise ValueError("Restore requires a full lowercase SHA-256")
        source = archive_dir / args.restore / "agentspeedbench"
        if digest(source) != args.restore:
            raise ValueError("Archived binary hash mismatch")
        install(source, destination, archive_dir)
        return
    subprocess.run(["python3", str(ROOT / "scripts/check-toolchain.py")], check=True)
    revision = clean_revision()
    with tempfile.TemporaryDirectory(prefix="agentspeedbench-install-") as temporary:
        source = Path(temporary) / "agentspeedbench"
        subprocess.run(["go", "build", "-trimpath", "-buildvcs=true", "-o", str(source), "./cmd/agentspeedbench"], cwd=ROOT, check=True)
        version = subprocess.check_output([str(source), "version"], text=True)
        if "commit=" + revision + " dirty=false" not in version or clean_revision() != revision:
            raise ValueError("Build metadata or source revision changed during compilation")
        install(source, destination, archive_dir)
        print(version.strip())


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        raise SystemExit("Installation failed: " + str(error))
