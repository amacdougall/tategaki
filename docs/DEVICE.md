# Device notes

Facts about the target Kindle and how we connect to it. Items marked
**UNVERIFIED** come from web sources and have not yet been checked on the
device.

## Device (verified over SSH 2026-10-17)

| Item | Value |
|---|---|
| Model | Kindle Paperwhite 11th gen (2021), called "PW5" in KOReader and FBInk. Device code `1PX` (id 0x6FF), codename Malbec, platform Bellatrix |
| SoC | MediaTek MT8110: two ARMv7 Cortex-A7 cores (`0xd03`), with NEON, VFPv4 and idiv |
| Firmware | 5.19.2 (`040-juno_1902_malbec_bellatrix-467076`, built 2026-02-12) |
| Kernel | `Linux kindle 4.9.77-lab126 #1 SMP PREEMPT Thu Feb 12 06:35:21 UTC 2026 armv7l` |
| Userland | hard-float (`/lib/ld-linux-armhf.so.3`), so KPM platform `kindlehf` |
| RAM | 474 MiB total, about 200 MiB available, plus 128 MiB swap |
| Storage | `/mnt/us` 6.2 GB (5.8 GB free); `/var/local` 230 MB (163 MB free) |
| Framework | `awesome` window manager running (the standard Kindle UI) |

### Framebuffer

- `fb0` is `hwtcon_v2`, the MTK e-ink controller, at 8 bpp grayscale (`Y8`).
- The visible area is **1236×1648**, which matches the `pw11` profile.
- Each row is **1248 bytes** (`stride`/`lineLength`), so every row has 12 padding
  bytes. Don't assume `width == stride`.
- `virtual_size` is 1248×3296, i.e. two buffers.
- **The visible image starts at buffer row 0** (`pan` is `0,0`). FBInk reports
  `viewVertOrigin=8`, but its draws at y=0 land on row 0 (checked 2026-10-17).
  `tools/fbshot.py` crops rows 0–1647.
- `rotate=3`, `currentRota=3`, `canRotate=0`.
- FBInk also reports `isMTK=1`, `canHWInvert=1`, `hasEclipseWfm=1` and
  `canWaitForSubmission=1`.
- From the `-e` dump, it reports no touch axis swap or mirroring:
  `touchSwapAxes=0`, `touchMirrorX=0`, `touchMirrorY=0`. Verify this with
  corner taps in Phase 0.

### FBInk binaries already on the device

- `/mnt/us/libkh/bin/fbink`, from the jailbreak. **Its `fbink -e` segfaults.**
- `/mnt/us/koreader/fbink`: `-e` works and reports `FBINK_VERSION='92e1270'`.
  `strings` finds "1.25.0 for Kindle", probably from this binary; this isn't
  certain.
- **Image support:** the libkh build is FBInk 1.25.0 with `Image=Yes`. KOReader's
  build has `Image=No`, even though its `--help` lists `-g`.
- **We vendor the libkh build** as `third_party/fbink/kindlehf/fbink` and ship
  it inside our package (see `third_party/fbink/README.md`).

### Input devices (`/proc/bus/input/devices`)

| Device | Name | Notes |
|---|---|---|
| `/dev/input/event0` | `bd71828-pwrkey` | Power button (gpio-keys) |
| `/dev/input/event1` | `pt_mt` | Touchscreen (Parade), multitouch, `ABS=e618000 0` |

Discover these at runtime by name; don't hardcode the event numbers.

**Touch (verified with corner taps, 2026-10-17):**
- `ABS_MT_POSITION_X` is 0..1235 and `ABS_MT_POSITION_Y` is 0..1647, the same as
  screen pixels.
- No axis swap or mirroring: raw coordinates are screen coordinates in portrait.
- `ABS_MT_SLOT` 0..31, multitouch protocol B.
- **Power key:** `EV_KEY` code 116 (`KEY_POWER`) on `event0`. powerd handles
  sleep by itself; the reader doesn't need to read this device.

## Power and sleep (verified 2026-10-17)

- **Events:** `lipc-wait-event -m -s 0 com.lab126.powerd '*'` emits
  `goingToScreenSaver 2`, `outOfScreenSaver 1`, `exitingScreenSaver` and
  `t1TimerReset`. Redraw on `outOfScreenSaver`.
- **Don't set `preventScreenSaver 1`.** It blocks the idle timeout, but it also
  makes the power button do nothing.
- **Instead, reset the idle timer** on input with
  `lipc-set-prop com.lab126.powerd touchScreenSaverTimeout 1`. This sets
  "Remaining time" back to `screenSaverTimeout` (600 s).
- With `awesome` stopped, sleeping turns the frontlight off and leaves our
  image on screen. Waking needs no unlock with the current settings.
- The reader uses this to draw the book's cover on `goingToScreenSaver`. The
  log line `SLEEP cover <id> drawn in …` shows the timing.

## Frontlight (verified 2026-10-18)

- **`com.lab126.powerd flIntensity`** (rw) is the step the Kindle's slider
  shows: 0..`flMaxIntensity`, which is **24** (25 positions). 0 is fully off.
- powerd maps the step to two LED channels, `/sys/class/backlight/fp9966-bl0`
  and `fp9966-bl1` (raw 0..2047), whose mix is the warmth. At step 9 they read
  930 and 632; at 0 both read 0. Don't write these directly: powerd would
  overwrite them and its slider would be out of sync.
- `flAuto` (auto-brightness) is 0 on this device. `flRawIntensity` is read-only.
- powerd isn't paused while the reader runs; `lipc-set-prop` takes about 30 ms
  with `awesome` stopped.

## Display latency (FBInk CLI, measured on device 2026-10-17)

| Operation | Time |
|---|---|
| Full-screen 1236×1648 PNG, GC16 + flash, from the Go app | 64–77 ms |
| The same, right after waking | 319–389 ms |
| 24×24 PNG region, DU | about 13 ms |

The CLI is well under the 250 ms page-turn budget, so we stay on the CLI and
don't need cgo (plan §9 Phase 0).

## Launching and stopping

- **Package install:** `kpm -y install file:///mnt/us/...kpkg` works with KPM
  0.2.2. Reinstalling the same version fails dependency resolution, so
  `tools/dev-install.sh` uninstalls first.
- **Library launch:** `documents/tategaki.sh` runs
  `kpm launch tategaki </dev/null`. Tested 2026-10-17.
- **Launch over SSH:**
  `ssh kindle 'nohup /var/local/kmc/bin/kpm launch tategaki </dev/null >/dev/null 2>&1 &'`
- **Stop over SSH:** `ssh kindle killall tategaki`. Even `killall -9` restores
  the UI, through the `launch.sh` trap. Verified by screenshot.
- **Log:** `/mnt/us/tategaki/log.txt`
- **Screenshot:** `uv run --with pillow tools/fbshot.py out.png`

### Tools on the device

- **Present:** `evtest`, `lipc-set-prop`, `lipc-wait-event`, `sqlite3`, busybox,
  `tar`, `gzip`.
- **Missing:** `fbgrab`, `evemu-*`, `rsync`, `python3`, `scp`, and `evtest` output is unusable over SSH (buffered). Use `/mnt/us/tategaki/bin/evinfo` (`reader/cmd/evinfo`) instead.
  - `scp` from the Mac still works, because OpenSSH 9.9's `scp` uses SFTP and the
    device has `sftp-server`.
  - **Without `rsync`, `tategaki deploy` must use `scp`/`sftp` or
    `tar | ssh tar x`**, not rsync (plan §5.8).
  - Screenshots will need FBInk's dump option or our own framebuffer reader.

## Jailbreak

- **Véra** (released 2026-08-10 by Ava / sparklerfish). Guide:
  <https://kindlemodding.org/jailbreaking/Vera/>
- Véra pre-installs **KPM** (Kindle Package Manager). **KUAL is not installed.**
  - KPM binary: `/var/local/kmc/bin/kpm`
  - Sources disagree on search-bar `;kpm …` commands. The dropbear-ssh readme
    says they don't work on current jailbreaks. Use scriptlets in `documents/`
    or SSH instead.
- Véra blocks OTA updates automatically. Don't place `update*.bin` files on
  the device (plan §0.1 guardrails).

## SSH access

Véra doesn't ship an SSH server. The official KPM repo (`KindleModding/repo`)
contains only `kompanion`, `koreader`, `kpm` and `kterm`, as of 2026-10-17.

**Chosen: `dropbear-ssh`** (<https://github.com/akshaylahudkar/dropbear-ssh>),
version 0.1.17, `kindlehf`.
- The author reports it was tested end to end on a Paperwhite 5 (kernel
  4.9.77), which is our model. It uses the `bin_11thgenplus` binaries, which are
  repackaged from notmarek/kindle-usbnetlite.
- **Install:** copy `dropbear-ssh.sh` into `documents/` over USB, then tap
  "Dropbear SSH" in the Library. **On our device the first tap silently did
  nothing.** The likely cause is that `kpm install` waits at a
  `Would you like to proceed? [Y/n]` prompt. A debug scriptlet that ran
  `kpm install dropbear-ssh </dev/null` (via the GitHub repo URL) worked.
  **Lesson for our own installer and launchers: always pass `kpm -y` and
  redirect stdin from `/dev/null`.** The script runs
  `kpm add-repo https://nealing.net/manifest.json`, `kpm install dropbear-ssh`
  and `kpm launch dropbear-ssh`. The repo URL is a Cloudflare proxy to the
  author's GitHub. The direct GitHub URL is
  `https://raw.githubusercontent.com/akshaylahudkar/dropbear-ssh/main/manifest.json`.
- **Start or stop the server:** use the on-device status app. The server keeps
  running after the app closes, but doesn't start at boot.
- **Connect:** `ssh -p 2022 root@<ip>`. Starting the server opens 2022 in
  iptables at runtime only; the rule doesn't survive a reboot.
- **Password:** random, stored at `/mnt/us/usbnetlite/etc/ssh_password` and shown
  in the app.
- **Host keys:** `/mnt/us/usbnetlite/etc/dropbear/`.
- **Key auth:** `authorized_keys` should go in the same directory, following the
  usbnetlite convention. **UNVERIFIED.**
- **What install writes**, from a review of `source/install.sh` on 2026-10-17.
  There are no rootfs writes.
  - `/mnt/us/usbnetlite/` (binaries, password, keys)
  - `/mnt/us/documents/dropbear-ssh.sh`
  - `/var/local/mesquite/dropbear-ssh/` (status web app)
  - rows in `/var/local/appreg.db` for app id `com.akshay.dropbearssh`.
    `uninstall.sh` removes these.
- **Uninstall:** `/var/local/kmc/bin/kpm uninstall dropbear-ssh`. This keeps
  `/mnt/us/usbnetlite/etc` and the launcher file.

Alternative, not used: the KPM build of usbnetlite
(<https://github.com/uybixd/kindle-usbnetlite-kpm>). It has no prebuilt release,
so it would have to be built from source.

### Connection

- The Mac is on Wi-Fi in `192.168.0.0/24` (Mac at `192.168.0.114`).
- Kindle IP: `192.168.0.133` (DHCP; reserve it on the router if it changes).
- `~/.ssh/config` alias:
  ```
  Host kindle
      HostName 192.168.0.133
      Port 2022
      User root
      IdentityFile ~/.ssh/id_rsa
      ConnectTimeout 5
  ```
- **Key auth works.** `~/.ssh/id_rsa.pub` is in
  `/mnt/us/usbnetlite/etc/dropbear/authorized_keys`; we wrote it over USB mass
  storage.
- The password, set by the user in the status app, is in
  `/mnt/us/usbnetlite/etc/ssh_password`.
- **The server must be started by hand after every reboot:** tap "Dropbear SSH"
  in the Library, then Start Server.
- While the Kindle is mounted over USB, `/mnt/us` is unavailable on the device.
  Eject before using SSH for anything that touches `/mnt/us`.

## Backups

Taken 2026-10-17 over USB mass storage, to `~/Tategaki-backup/2026-10-17/`:
`kmc/`, `libkh/` (jailbreak and KPM state), `My Clippings.txt`, `JAILBROKEN.txt`.

Not backed up, by the user's choice:
- `koreader/`: KOReader isn't wanted.
- `documents/`: holds only re-downloadable Amazon content (`Downloads/Items01`
  and two Oxford dictionaries), plus `toradora_01.epub`, a KOReader test book.
  That book is also a candidate converter test book.

## `/mnt/us` layout seen 2026-10-17

`audible/ documents/ fonts/ kmc/ koreader/ libkh/ screenshots/ system/ voice/`,
plus the jailbreak markers `privesc_marker.txt` and `.sdr`, and Calibre's
`metadata.calibre`. There is no `extensions/` folder, because KUAL isn't
installed.
