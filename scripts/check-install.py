#!/usr/bin/env python3
"""Exercise installer preservation and corruption rejection with disposable files."""
import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("local_installer", Path(__file__).with_name("install-local.py"))
installer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(installer)


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
