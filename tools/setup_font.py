"""Build SETUP's fixed-cell mask from the shipped DejaVu Sans Mono Book (Pillow and fontconfig)."""
import gzip
import struct
import subprocess
from pathlib import Path
from PIL import Image, ImageDraw, ImageFont

src = Path(__file__).resolve().parents[1] / "src"
master = src / "game/logview/fonts/DejaVuSansMono.ttf"
font = ImageFont.truetype(str(master), 16)
charset = subprocess.check_output(["fc-query", "--format=%{charset}", str(master)], text=True)
out = bytearray()
for interval in charset.split():
    ends = [int(end, 16) for end in interval.split("-")]
    for point in range(ends[0], ends[-1] + 1):
        cell = Image.new("L", (10, 20))
        ImageDraw.Draw(cell).text((0, 15), chr(point), anchor="ls", fill=255, font=font)
        out.extend(struct.pack("<I", point))
        out.extend(cell.tobytes())
(src / "internal/setupfont/cells.gz").write_bytes(gzip.compress(out, mtime=0))
