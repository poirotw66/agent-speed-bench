#!/usr/bin/env python3
"""Exercise installer preservation and corruption rejection with disposable files."""
import importlib.util
from pathlib import Path
import tempfile
import subprocess
import sys
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location("local_installer", Path(__file__).with_name("install-local.py"))
installer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(installer)


toolchain_spec = importlib.util.spec_from_file_location("toolchain_checker", Path(__file__).with_name("check-toolchain.py"))
toolchain = importlib.util.module_from_spec(toolchain_spec)
toolchain_spec.loader.exec_module(toolchain)


class ToolchainTests(unittest.TestCase):
    def test_missing_compiler_reports_actionable_error(self):
        with mock.patch.object(toolchain.shutil, "which", return_value=None):
            with self.assertRaisesRegex(ValueError, "persistent directory"):
                toolchain.check()

    def test_older_compiler_is_rejected(self):
        with mock.patch.object(toolchain.shutil, "which", return_value="/fixture/go"), mock.patch.object(toolchain.subprocess, "check_output", return_value="go version go1.25.0 darwin/arm64"):
            with self.assertRaisesRegex(ValueError, "is required"):
                toolchain.check()


class InstallerTests(unittest.TestCase):
    def test_upgrade_and_restore_preserve_both_versions(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            destination = root / "bin/agentspeedbench"
            destination.parent.mkdir()
            destination.write_bytes(b"previous executable")
            source = root / "new"
            source.write_bytes(b"new executable")
            old_hash = installer.digest(destination)
            installer.install(source, destination, root / "archive")
            old = root / "archive" / old_hash / "agentspeedbench"
            self.assertEqual(old.read_bytes(), b"previous executable")
            self.assertEqual(destination.read_bytes(), b"new executable")
            new_hash = installer.digest(destination)
            installer.install(old, destination, root / "archive")
            self.assertEqual(destination.read_bytes(), b"previous executable")
            self.assertEqual((root / "archive" / new_hash / "agentspeedbench").read_bytes(), b"new executable")

    def test_second_process_waits_for_installation_lock(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source, destination = root / "new", root / "bin/agentspeedbench"
            source.write_bytes(b"new executable")
            destination.parent.mkdir()
            destination.write_bytes(b"current executable")
            code = "import importlib.util, pathlib, sys; " \
                   "spec=importlib.util.spec_from_file_location('installer',sys.argv[1]); " \
                   "module=importlib.util.module_from_spec(spec); spec.loader.exec_module(module); " \
                   "print('ready',flush=True); " \
                   "module.install(pathlib.Path(sys.argv[2]),pathlib.Path(sys.argv[3]),pathlib.Path(sys.argv[4]))"
            with installer.installation_lock(destination):
                child = subprocess.Popen([sys.executable, "-c", code, str(Path(__file__).with_name("install-local.py")), str(source), str(destination), str(root / "archive")], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
                try:
                    self.assertEqual(child.stdout.readline().strip(), "ready")
                    with self.assertRaises(subprocess.TimeoutExpired):
                        child.wait(timeout=0.2)
                    self.assertEqual(destination.read_bytes(), b"current executable")
                except BaseException:
                    child.kill()
                    child.communicate()
                    raise
            stdout, stderr = child.communicate(timeout=5)
            self.assertEqual(child.returncode, 0, stdout + stderr)
            self.assertEqual(destination.read_bytes(), b"new executable")
            previous_hash = installer.hashlib.sha256(b"current executable").hexdigest()
            self.assertEqual((root / "archive" / previous_hash / "agentspeedbench").read_bytes(), b"current executable")

    def test_corrupt_archive_does_not_replace_installation(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source, destination = root / "new", root / "installed"
            source.write_bytes(b"new executable")
            destination.write_bytes(b"current executable")
            archive = root / "archive" / installer.digest(source) / "agentspeedbench"
            archive.parent.mkdir(parents=True)
            archive.write_bytes(b"corrupted")
            with self.assertRaises(ValueError):
                installer.install(source, destination, root / "archive")
            self.assertEqual(destination.read_bytes(), b"current executable")


if __name__ == "__main__":
    unittest.main()
