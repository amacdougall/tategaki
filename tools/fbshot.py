#!/usr/bin/env python3
"""Screenshot the Kindle by reading /dev/fb0 over SSH.

The device has no fbgrab, and its FBInk CLI builds have no dump option, so we
read the raw framebuffer and crop it here. Geometry (see docs/DEVICE.md):
8 bpp grayscale, 1248-byte stride, 1236x1648 visible from row 0. The pan is
0,0; FBInk reports viewVertOrigin=8, but its draws land at row 0.

Usage: uv run --with pillow tools/fbshot.py [OUT.png] [--host kindle] [--raw FILE]
"""

import argparse
import subprocess
import sys

from PIL import Image

STRIDE = 1248
WIDTH = 1236
HEIGHT = 1648
Y_ORIGIN = 0


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("out", nargs="?", default="screenshot.png")
    ap.add_argument("--host", default="kindle")
    ap.add_argument("--raw", help="read a saved raw dump instead of SSH")
    args = ap.parse_args()

    if args.raw:
        with open(args.raw, "rb") as f:
            data = f.read()
    else:
        need = STRIDE * (Y_ORIGIN + HEIGHT)
        data = subprocess.run(
            ["ssh", args.host, f"head -c {need} /dev/fb0"],
            check=True,
            capture_output=True,
        ).stdout

    full = Image.frombytes("L", (STRIDE, len(data) // STRIDE), data)
    shot = full.crop((0, Y_ORIGIN, WIDTH, Y_ORIGIN + HEIGHT))
    shot.save(args.out)
    print(args.out)
    return 0


if __name__ == "__main__":
    sys.exit(main())
