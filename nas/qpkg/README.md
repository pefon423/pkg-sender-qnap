# QNAP native QPKG packaging

This directory builds a native QNAP App Center package (`.qpkg`) for the same
Go sender used by the Synology SPK (`../spk/`) and the Docker image
(`../Dockerfile`). It packages the identical `pkg-sender-nas` binary; nothing
under `nas/cmd` or `nas/internal` changes for QNAP.

**Status: unverified scaffold.** Nobody has installed this on a real QNAP NAS
yet. The manifest fields, control-script conventions, and hook contract below
were checked against QNAP's official QDK-Guide example
(`github.com/qnap-dev/QDK-Guide`, `example/QPKG/helloWorld/`), not against
real hardware. Treat the first real install as a validation pass, the same
way the Synology SPK needed real-hardware acceptance testing before its
README could claim it actually works (see `../spk/README.md`).

## Why this differs from the Synology SPK

QNAP's QPKG format is unrelated to Synology's SPK: a different manifest
(`qpkg.cfg` vs `INFO`), a different lifecycle-hook contract
(`package_routines` vs `postinst`/`preuninst`/...), and no `/var/packages`
equivalent — installed packages live under
`/share/<volume>/.qpkg/<QPKG_NAME>/`, resolved at runtime via
`/sbin/getcfg <name> Install_Path -f /etc/config/qpkg.conf`, never hardcoded.

## Determine your ARCH first

**Confirmed target: TS-459 Pro → `ARCH=x86`.** The TS-459 Pro is an Intel
Atom D510 (dual-core, x86) NAS — not ARM. QTS 4.2.6 is this model's final,
EOL firmware. Although the D510 supports 64-bit, QTS 4.2.6 runs a **32-bit**
userspace on this model, so the package needs a 32-bit `x86` build
(`GOARCH=386`), not `x86_64`. `build.sh` already handles this.

If this package is later built for a different (ARM) NAS, confirm first via
SSH:

```sh
uname -m
cat /etc/platform.conf 2>/dev/null | head -5
```

| `uname -m` | Platform hint | `ARCH` |
| --- | --- | --- |
| `i686`/`i386` | Intel Atom, 32-bit QTS (e.g. TS-459 Pro) | `x86` |
| `armv5*` | Marvell Kirkwood | `arm-x19` |
| `armv7l` | `ARM_MS` in `/etc/platform.conf` | `arm-x31` |
| `armv7l` | `ARM_AL` in `/etc/platform.conf` | `arm-x41` |
| `aarch64` | Annapurna Alpine 64-bit | `arm_64` |

If this doesn't cleanly match, hold off on a real build and share the command
output before guessing.

## Build

Requires a Linux build host (or WSL) with `go` and QNAP's official `qbuild`
installed (`https://github.com/qnap-dev/QDK` — it's a portable, GPL-licensed
CLI; it does not need to run on the NAS itself). This Windows machine cannot
run this script directly, the same limitation `../spk/build.sh` already has
for `sh`/`go`.

```sh
cd nas/qpkg
ARCH=x86 VERSION=0.1.0-0001 ./build.sh
```

Output: `dist/PKGSenderNAS_0.1.0-0001_x86.qpkg` (exact filename is whatever
`qbuild` names it).

Then run the best-effort structural check:

```sh
./verify.sh dist/PKGSenderNAS_0.1.0-0001_x86.qpkg
```

## Install on real hardware

QTS 4.2.6 will refuse an unsigned package by default. In App Center →
Settings → General, enable **"Allow installation of applications without a
valid digital signature"** (wording varies slightly by QTS version), then
install the `.qpkg` manually.

## Persistent configuration

On first install, `package_routines` copies `config/config.env.example` to
`<Install_Path>/data/config.env` (mode `600`). Edit it and set:

```env
PKGSENDER_PACKAGE_DIR="/share/PS5/PKG"
PKGSENDER_PUBLIC_BASE_URL="http://192.168.1.20:9898"
PKGSENDER_PS5_IP="192.168.1.50"
PKGSENDER_PS5_PORT="12800"
```

Then start the package from App Center (or `<Install_Path>/PKGSenderNAS.sh
start`). If the config is missing or still contains `CHANGE_ME`, the control
script reports that configuration is required and does not launch the
daemon, exactly like the Synology package's behavior.

## Runtime files

Persistent data lives under `<Install_Path>/data/`:

```text
<Install_Path>/
├── pkg-sender-nas              # binary
├── data/
│   ├── config.env
│   ├── history.json
│   ├── aliases.json
│   ├── pkg-sender-nas.log
│   └── pkg-sender-nas.pid
```

`data/` survives an in-place App Center **upgrade** (qbuild-generated
installers extract new payload over the existing `Install_Path` rather than
wiping it). A full **uninstall** removes `Install_Path` entirely, including
`data/` — QNAP QPKGs have no equivalent of DSM's separate `target`/`var`
split, so uninstalling reasonably means removing everything, matching how
most QNAP apps behave. If you want to preserve history/aliases across an
uninstall, back up `<Install_Path>/data/` yourself first.

## Shared-folder permission

Unlike the Synology package (which runs under a restricted `run-as: package`
identity), classic QTS QPKGs of this vintage typically run as root, so this
package does not need a separate ACL grant to read the PKG library directory.
If you'd rather it run as a non-root user, that's a manual QTS-side change,
not something this package currently sets up.

## Known gaps versus the Synology package

- Icon files (`icons/icon80.png`, `icons/icon128.png`) are placeholders
  resized from the existing Synology icon source. Confirm the exact
  filenames/sizes `qbuild` expects once it's installed, and adjust `qpkg.cfg`
  / the `icons/` directory if it complains.
- `QPKG_RC_NUM="150"` is a plausible default start/stop ordering, not a
  verified one. Adjust if it conflicts with another installed package.
- No firewall/port-registration manifest is declared for the PS5 discovery
  listener (UDP `12801`), unlike the Synology package's `conf/resource`. If
  QTS's firewall is enabled, you may need to add a manual allow rule.
