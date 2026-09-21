import hashlib
import io
import json
import re
import sys
import tarfile


def split_oci(source, destination, manifest_digest):
    if not re.fullmatch(r"sha256:[0-9a-f]{64}", manifest_digest):
        raise ValueError("expected immutable platform manifest digest")
    if source == destination:
        raise ValueError("source archive must remain unchanged")
    blob_path = "blobs/sha256/" + manifest_digest.split(":", 1)[1]
    with tarfile.open(source, "r:") as archive:
        manifest = archive.extractfile(blob_path).read()
        if "sha256:" + hashlib.sha256(manifest).hexdigest() != manifest_digest:
            raise ValueError("platform manifest digest mismatch")
        document = json.loads(manifest)
        if "config" not in document:
            raise ValueError("expected image manifest, not an index")
        index = json.dumps({
            "schemaVersion": 2,
            "manifests": [{
                "mediaType": document.get("mediaType", "application/vnd.oci.image.manifest.v1+json"),
                "digest": manifest_digest,
                "size": len(manifest),
            }],
        }, separators=(",", ":")).encode()
        with tarfile.open(destination, "w") as output:
            replaced = False
            for member in archive:
                if member.name.removeprefix("./") == "index.json":
                    if replaced:
                        raise ValueError("duplicate root index")
                    replaced = True
                    replacement = tarfile.TarInfo("index.json")
                    replacement.size = len(index)
                    replacement.mode = 0o644
                    output.addfile(replacement, io.BytesIO(index))
                elif member.isfile():
                    output.addfile(member, archive.extractfile(member))
                elif member.isdir():
                    output.addfile(member)
                else:
                    raise ValueError("unsupported archive entry")
            if not replaced:
                raise ValueError("missing root index")


if __name__ == "__main__":
    split_oci(*sys.argv[1:])