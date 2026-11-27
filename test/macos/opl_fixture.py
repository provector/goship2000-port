"""Exercise the helper bytes embedded in the packaged executable with a synthetic keyed note."""
from pathlib import Path
import gzip
import hashlib
import json
import struct
import subprocess
import sys
import tempfile

root = Path(__file__).resolve().parents[2]
game, platform = Path(sys.argv[1]), sys.argv[2]
component = root / "src/internal/audioopl"
blob = (component / "bin" / ("audio-bake-" + platform + ".gz")).read_bytes()
digest = json.loads((component / "component.json").read_text())["helpers"][platform]
assert hashlib.sha256(blob).hexdigest() == digest["compressed_sha256"]
offset = game.read_bytes().index(blob)
with game.open("rb") as f:
    f.seek(offset)
    helper = gzip.decompress(f.read(len(blob)))
assert hashlib.sha256(helper).hexdigest() == digest["sha256"]
# A two-operator sine tone with a key release at frame 3072.
writes = [(0, reg, val) for reg, val in [(0x20,1),(0x23,1),(0x40,0x20),(0x43,0),
          (0x60,0xf1),(0x63,0xf1),(0x80,0x44),(0x83,0x44),(0xa0,0x98),(0xb0,0x31)]]
writes.append((3072,0xb0,0x11))
data = struct.pack("<8sIIQI", b"OPL27\0\0\0", 49716, len(writes), 4096, 1)
data += b"".join(struct.pack("<QHBx", *w) for w in writes)
with tempfile.TemporaryDirectory() as tmp:
    path = Path(tmp) / "audio-opl-helper"
    path.write_bytes(helper)
    path.chmod(0o700)
    pcm = subprocess.run([path], input=data, stdout=subprocess.PIPE, check=True, timeout=20).stdout
assert len(pcm)==4096*4 and any(pcm)
actual = hashlib.sha256(pcm).hexdigest()
print(platform, "4096 frames", actual, flush=True)
assert actual == "5171a0dccef1cf8262de3a8757c210a2531f5b8f99d576a50af3a8a20cdfd901", "synthetic note differs from the accepted W5.4 Linux helper"
