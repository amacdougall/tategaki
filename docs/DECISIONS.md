# Decisions log

Decisions made during implementation, newest last. LOCKED decisions live in
`plans/20261017-tategaki-reader.md`; this file records everything decided since.

## 2026-10-17

- **Jailbreak is Véra, with KPM and no KUAL.** Any plan step or third-party
  readme that assumes KUAL menus has to use `kpm launch …` over SSH or a
  documents-folder scriptlet instead. The plan's scriptlet launcher (§9 Phase 0)
  already fits this.
- **SSH route:** use the `dropbear-ssh` KPM package over Wi-Fi, on port 2022
  (see DEVICE.md). It is prebuilt and reported working on PW5, and it writes
  nothing to rootfs. The usbnetlite KPM build was rejected because it has no
  release and would need building.
- **Device facts confirmed** (DEVICE.md): firmware 5.19.2, `kindlehf`,
  1236×1648 at 8 bpp with a 1248-byte stride, and an MTK controller.
- **Deploying can't use rsync**, because the device doesn't have it. Use
  `scp`/`sftp` or `tar` over ssh. This changes how plan §5.8 is implemented, not
  what it does.
- **KPM calls in scripts always use `-y` and `</dev/null`**, after the
  interactive-prompt failure.
