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
- **FBInk reports `viewVertOrigin=8`:** the visible image starts 8 rows down in
  the buffer. FBInk handles this; it only matters if we ever write to
  `/dev/fb0` directly.
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
- **Ship our own** `kindlehf` FBInk in the package, as plan §4 says. Don't rely
  on either of these.

### Input devices (`/proc/bus/input/devices`)

| Device | Name | Notes |
|---|---|---|
| `/dev/input/event0` | `bd71828-pwrkey` | Power button (gpio-keys) |
| `/dev/input/event1` | `pt_mt` | Touchscreen (Parade), multitouch, `ABS=e618000 0` |

Discover these at runtime by name; don't hardcode the event numbers.

### Tools on the device

- **Present:** `evtest`, `lipc-set-prop`, `lipc-wait-event`, `sqlite3`, busybox,
  `tar`, `gzip`.
- **Missing:** `fbgrab`, `evemu-*`, `rsync`, `python3`, `scp`.
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
