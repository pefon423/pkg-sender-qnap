# Windows self-hosted deployment

Runs the same Go server used by the Synology SPK (`../spk/`) and QNAP QPKG
(`../qpkg/`) directly on a Windows machine, no NAS involved. Unlike the
SPK/QPKG scaffolds, this has actually been built and run end-to-end on real
hardware (a Windows PC on the same LAN as the PS5).

## Why this exists

The QNAP QPKG path turned out to be unreliable on the target NAS (real
hardware, unrelated to this daemon, kept becoming unresponsive). Running on
a Windows PC that's already on the same LAN sidesteps NAS packaging
entirely, and can be built/tested locally instead of blind.

## Layout

```text
windows/
├── build.ps1          # go test + go build -> pkg-sender-nas.exe
├── run.ps1            # loads data/config.env into env vars, launches the exe
├── install-task.ps1   # registers the auto-start Scheduled Task
├── pkg-sender-nas.exe # built binary (not committed; run build.ps1)
└── data/
    ├── config.env      # KEY="value" lines, same format as Synology/QNAP
    ├── history.json     # created at runtime
    └── aliases.json     # created at runtime, optional
```

## Build

Requires Go on `PATH` (or set `$env:GO_BIN` to a full path to `go.exe`):

```powershell
cd nas\windows
.\build.ps1
```

## Configure

Edit `data\config.env`:

```env
PKGSENDER_PACKAGE_DIR="O:\PS5\PKG"
PKGSENDER_LISTEN=":9898"
PKGSENDER_PUBLIC_BASE_URL="http://<this-PC-LAN-IP>:9898"
PKGSENDER_PS5_IP="<ps5-lan-ip>"
PKGSENDER_PS5_PORT="12800"
```

`run.ps1` parses this file itself (Windows has no POSIX shell to source a
`.env` file the way the Synology/QNAP control scripts do) and additionally
sets `PKGSENDER_HISTORY_FILE`, `PKGSENDER_TITLE_ALIASES_FILE`, and
`PKGSENDER_CONFIG_FILE` to paths under `data/` if not already set. Changing
the PS5 IP or library paths from the Web UI (`POST /api/settings/ps5` and
`/api/settings/libraries`) persists back into this same file, exactly like
on Synology/QNAP.

## Run once (foreground, for testing)

```powershell
.\run.ps1
```

## Auto-start

```powershell
.\install-task.ps1
```

Registers a Scheduled Task (`PKGSenderNAS`) that starts `run.ps1` **when the
current Windows user logs in**, and restarts it automatically if it exits
(up to 999 times, 1-minute backoff). This does not require an elevated
PowerShell.

This is an `AtLogOn` trigger for the current user, not an `AtStartup`/SYSTEM
one: `Register-ScheduledTask` refuses to create a SYSTEM-context task without
admin rights. For a machine that should serve PKGs even before anyone logs
in, re-run `install-task.ps1` from an elevated PowerShell after changing its
trigger/principal to `AtStartup` / `SYSTEM` (see the comments in that
script) -- not done here since it wasn't needed for this deployment.

Manage the task like any other: `Get-ScheduledTask -TaskName PKGSenderNAS`,
`Stop-ScheduledTask` / `Start-ScheduledTask`, or `Unregister-ScheduledTask`
to remove it. Logs are not redirected by the task itself; use
`Start-Process ... -RedirectStandardOutput` if you need persistent logs
(see how this was tested, in git history / conversation, for an example).

## Verify

```powershell
Invoke-RestMethod http://127.0.0.1:9898/health
Invoke-RestMethod http://127.0.0.1:9898/api/packages
```

Open `http://<this-PC-LAN-IP>:9898/ui/` from a browser on the LAN.

## Known Windows gaps

- File permission bits (e.g. `config.env`/`history.json` mode `0600`) are
  requested the same way as on Linux but NTFS has no POSIX owner/group/other
  model, so they aren't enforced or observable the same way. Not a
  functional issue, just weaker at-rest file permission isolation than on a
  real Unix host.
