import hashlib
import importlib.util
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("split_oci", Path(__file__).with_name("split-oci.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class SplitOciTest(unittest.TestCase):
    def test_manifest_bytes_and_source_are_preserved(self):
        with tempfile.TemporaryDirectory() as directory:
            source, destination = str(Path(directory) / "release.tar"), str(Path(directory) / "candidate.tar")
            manifest = b'{"schemaVersion":2,"config":{},"layers":[]}'
            digest = "sha256:" + hashlib.sha256(manifest).hexdigest()
            name = "blobs/sha256/" + digest.split(":")[1]
            with tarfile.open(source, "w") as archive:
                for filename, content in {"index.json": b'{"schemaVersion":2,"manifests":[]}', name: manifest}.items():
                    header = tarfile.TarInfo(filename)
                    header.size = len(content)
                    archive.addfile(header, io.BytesIO(content))
            original = Path(source).read_bytes()
            module.split_oci(source, destination, digest)
            self.assertEqual(original, Path(source).read_bytes())
            with tarfile.open(destination) as archive:
                self.assertEqual(manifest, archive.extractfile(name).read())
                self.assertEqual(digest, json.load(archive.extractfile("index.json"))["manifests"][0]["digest"])
            with self.assertRaises(ValueError):
                module.split_oci(source, source, digest)
            with self.assertRaises(ValueError):
                module.split_oci(source, destination, "mutable-tag")


if __name__ == "__main__":
    unittest.main()