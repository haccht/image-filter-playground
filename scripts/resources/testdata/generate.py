"""Regenerate synthetic decoder fixtures; requires cjpeg and ImageMagick.

Normal Go tests and the resource harness use the checked-in files directly.
No user images, network requests, or application dependencies are involved.
"""
import hashlib
import json
from pathlib import Path
import struct
import subprocess

ROOT = Path(__file__).resolve().parent
manifest = {}


def save(name, data):
    assert len(data) <= 2 << 20, name
    (ROOT / name).write_bytes(data)
    manifest[name] = {"bytes": len(data), "sha256": hashlib.sha256(data).hexdigest()}


for width, height, sample in [(1024, 1024, "2x2"), (1024, 1024, "1x1"), (257, 129, "2x2"), (1, 1, "2x2"), (512, 512, "1x1")]:
    state = 0x12345678
    pixels = bytearray()
    for _ in range(width * height * 3):
        state ^= (state << 13) & 0xffffffff
        state ^= state >> 17
        state ^= (state << 5) & 0xffffffff
        pixels.append(state & 255)
    ppm = f"P6\n{width} {height}\n255\n".encode() + pixels
    name = f"progressive-{width}x{height}-{'420' if sample == '2x2' else '444'}.jpg"
    result = subprocess.run(["cjpeg", "-quality", "90", "-dct", "int", "-sample", sample, "-progressive"], input=ppm, capture_output=True, check=True)
    save(name, result.stdout)

for width, height, depth in [(1024, 1024, 16), (513, 257, 16), (1, 1, 16), (1024, 1024, 8), (512, 512, 16), (512, 512, 8)]:
    pixels = bytearray()
    for y in range(height):
        for x in range(width):
            channels = ((x * 64 + 123) & 65535, (y * 64 + 456) & 65535, ((x + y) * 32 + 789) & 65535, (x * 64 + 327) & 65535)
            if depth == 16:
                pixels.extend(struct.pack(">4H", *channels))
            else:
                pixels.extend(v >> 8 for v in channels)
    result = subprocess.run(["magick", "-size", f"{width}x{height}", "-depth", str(depth), "-endian", "MSB", "rgba:-", "-define", "png:color-type=6", "-define", "png:exclude-chunk=all", "-depth", str(depth), "-interlace", "PNG", "png:-"], input=pixels, capture_output=True, check=True)
    assert result.stdout[24:29] == bytes([depth, 6, 0, 0, 1])
    save(f"adam7-rgba{depth}-{width}x{height}.png", result.stdout)

(ROOT / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
print(json.dumps(manifest, indent=2))
