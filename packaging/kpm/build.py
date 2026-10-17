#!/usr/bin/env python3
"""Build the kindlehf KPM package.

Cross-compiles the reader, stages it with the package hooks and FBInk, and
packs a manifest-v2 .kpkg (a gzipped tar, as kpm-helper.py does) into build/.

Usage: python3 packaging/kpm/build.py
"""

import json
import os
import shutil
import subprocess
import sys
import tarfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
PKG_SRC = ROOT / "packaging" / "kpm"
BUILD = ROOT / "build"
STAGE = BUILD / "kpm-stage"


def main() -> int:
    manifest = json.loads((PKG_SRC / "manifest.json").read_text())
    version = ".".join(str(n) for n in manifest["version"])
    platform = "-".join(manifest["supported_platforms"])

    shutil.rmtree(STAGE, ignore_errors=True)
    (STAGE / "bin").mkdir(parents=True)

    env = dict(os.environ, CGO_ENABLED="0", GOOS="linux", GOARCH="arm", GOARM="7")
    subprocess.run(
        ["go", "build", "-trimpath", "-ldflags=-s -w", "-o", str(STAGE / "bin" / "tategaki"), "./cmd/tategaki"],
        cwd=ROOT / "reader", env=env, check=True,
    )
    shutil.copy2(ROOT / "third_party" / "fbink" / platform / "fbink", STAGE / "bin" / "fbink")
    for name in ("manifest.json", "install.sh", "launch.sh", "uninstall.sh"):
        shutil.copy2(PKG_SRC / name, STAGE / name)
    shutil.copytree(PKG_SRC / "scriptlets", STAGE / "scriptlets")
    (STAGE / "fonts").mkdir()
    for font in ("BIZUDPGothic-Regular.ttf", "BIZUDPGothic-Bold.ttf", "OFL.txt"):
        shutil.copy2(ROOT / "data" / "fonts" / font, STAGE / "fonts" / font)

    out = BUILD / f"{manifest['id']}_{version}_{platform}.kpkg"

    def clean(info: tarfile.TarInfo) -> tarfile.TarInfo:
        info.uid = info.gid = 0
        info.uname = info.gname = "root"
        if info.isdir() or info.name.endswith(".sh") or info.name.startswith("bin/"):
            info.mode = 0o755
        else:
            info.mode = 0o644
        return info

    with tarfile.open(out, "w:gz", compresslevel=5) as tar:
        for item in sorted(STAGE.iterdir()):
            tar.add(item, arcname=item.name, filter=clean)
    print(out)
    return 0


if __name__ == "__main__":
    sys.exit(main())
