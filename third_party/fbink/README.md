# FBInk binaries

[FBInk](https://github.com/NiLuJe/FBInk) by NiLuJe (GPL-3.0-or-later). Its
source is available at that URL.

## `kindlehf/fbink`

- **Version:** FBInk 1.25.0 for Kindle. Build features: `Draw=Yes, Bitmap=Yes,
  Fonts=Yes, OpenType=Yes, Image=Yes, Input=Yes`.
- **Source:** copied 2026-10-17 from the device's `/mnt/us/libkh/bin/fbink`,
  which the Véra jailbreak installed. The binary is dynamically linked against
  the device's hard-float glibc.
- **sha256:** `58938160f5cf1c9dac5fef45741eb9584bf698e6f939e8b4e9d4b637849df4dd`
- **Why this build:**
  - KOReader's newer build (`92e1270`, also on the device) was compiled with
    `Image=No`, so it can't draw PNGs.
  - Official FBInk releases ship source only.
- **Known issue:** `fbink -e` (state dump) segfaults. Drawing works.
  - Image draws to the screen work at 64–77 ms for a full-screen GC16 flash and
    about 13 ms for a small DU region.

Replace this with our own build (koxtoolchain `kindlehf`) if we need a newer
version or cgo linkage.
