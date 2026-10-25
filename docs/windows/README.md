# WSL and Windows interop (`gash wsl`, `gash win`)

When Gash runs inside a WSL2 distribution it can look at the Windows host and
convert paths between the two worlds — for a human, with eye-candy tables, and
for an agent, with `--json`. The rule that shapes everything here: **reading
Windows is free and automatic; changing Windows is human-only and goes through
a UAC prompt.**

These commands do nothing off WSL (`gash wsl info` says so and exits 0;
`gash wsl doctor` exits 1). On a real Linux host they are simply not useful.

## How the bridge works

Every Windows program Gash runs — `reg.exe`, `wevtutil.exe`, Windows
PowerShell — goes through one bridge, so a few guarantees hold in one place:

- **Programs are found by absolute path** under the Windows `System32`
  directory, never through `PATH`. This works even with
  `[interop] appendWindowsPath=false` in `wsl.conf`, the setting many people
  use to keep Windows `.exe`s off their Linux `PATH`.
- **The system drive is discovered**, not assumed. A distribution with
  `[automount] root=/` mounts the Windows drive on `/c`, not `/mnt/c`; Gash
  reads the mounts and finds it either way.
- **Every call has a timeout** (`GASH_WIN_TIMEOUT`, default 10s, max 120s), so
  a broken interop — a stale socket after a reattached `tmux`, a fragmented
  VM — fails fast with `interop_timeout` instead of hanging the shell.
- **Interop that is turned off is reported, not worked around**: a disabled
  handler or a missing socket is `interop_unavailable`.
- **PowerShell runs fixed, read-only scripts.** The scripts contain only
  `Get-*` and `query` verbs (a test proves it). Your arguments reach a script
  as data on standard input, never as part of the script text.

## `gash wsl` — facts, paths, repair

```bash
gash wsl info                       # a quick summary of the environment
gash wsl doctor                     # check interop, drives, networking, config
gash wsl path ~/project --windows   # /home/me/project -> \\wsl.localhost\...
gash wsl path 'C:\Users\me'         # -> /mnt/c/Users/me
gash wsl config                     # effective wsl.conf and .wslconfig
gash wsl interop                    # this shell's interop state
eval "$(gash wsl interop --export)" # repair a stale WSL_INTEROP in place
```

`path` converts either direction and auto-detects which way; force it with
`--windows`, `--mixed` (forward slashes: `C:/Users/me`) or `--linux`.

`interop --export` is the fix for the most common WSL annoyance: after you
reattach a `tmux` or `screen` session, `WSL_INTEROP` points at a socket that
is gone and every Windows program fails. `eval "$(gash wsl interop --export)"`
points it back at a live server without opening a new terminal.

## `gash win` — inspect the Windows host (read-only)

```bash
gash win info          # build, edition, host, user, elevation
gash win doctor        # a composed health check
gash win disk          # fixed drives, size and free space
gash win net           # adapters, DNS, proxy
gash win updates       # installed hotfixes, pending reboot
gash win defender      # Microsoft Defender status
gash win services ssh  # services, optionally filtered
gash win procs         # top processes by memory
gash win apps chrome   # installed programs, optionally filtered
gash win ports --port 443
gash win events --level error --since 24h
gash win reg 'HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion'
gash win file 'C:\Windows\System32\notepad.exe'
gash win policy        # policy registry roots and the device join
gash win hosts         # the Windows hosts file
```

Nothing under `gash win` writes to Windows. Data that needs an administrator
(some services, some registry keys) reads as `windows_access_denied` from a
normal terminal — `gash win info` tells you whether your Windows token is
elevated. Add `--json` to any of them for a documented, stable shape.

## `gash win fix` — elevated corrections (human-only, UAC)

`gash win fix` is the one path that changes Windows, and it is deliberately
narrow:

- it is **human-only**: a headless caller (an agent, `GASH_HEADLESS=1`, no
  terminal) is refused with `elevation_required`;
- it **always goes through UAC**: Gash launches the action with
  `Start-Process -Verb RunAs`, so Windows itself shows the consent dialog;
- before it runs anything it **prints the exact PowerShell command** and asks
  you to confirm (the UAC dialog is the second, authoritative gate);
- the actions are a **fixed allowlist**, and every argument is validated and
  embedded as data, never as code.

```bash
gash win fix                              # list the available corrections
gash win fix flush-dns                    # Clear-DnsClientCache
gash win fix restart-service Spooler
gash win fix hosts-add 127.0.0.1 app.local
gash win fix hosts-remove app.local
gash win fix flush-dns --dry-run          # print the command, run nothing
```

An agent cannot stand at a UAC prompt, so it uses `--dry-run` to read the
exact elevated command (also as `--json`) and hands it to a human to run.

## What Gash will not do (by design)

- **No UAC bypass, no silent elevation.** Interop runs Windows programs as the
  Windows user with the terminal's own elevation; there is no way to elevate
  without the user, and Gash does not pretend otherwise.
- **No reading Windows secrets.** The guardrails treat the user registry hives
  (`NTUSER.DAT`, `UsrClass.dat`), saved RDP credentials (`*.rdp`), answer files
  (`unattend.xml`), and the browser credential stores (`Login Data`,
  `logins.json`, `key4.db`) reachable over `/mnt` as secret files — see
  [docs/security.md](../security.md).
- **No arbitrary write path.** `gash win fix` is an allowlist, not a generic
  "run this elevated" escape hatch.
- **No `.exe` in `llm_exec`.** The agent command allowlist never includes
  Windows programs.

## Pinning the tests to a real machine

The Go side is tested with synthetic fixtures — a fake runner that returns the
JSON the read-only scripts would produce. To pin those shapes to a real
Windows build, run [`capture.ps1`](capture.ps1) on the Windows box (a normal
window, then an elevated one). It is read-only: it runs the same `Get-*`
queries `gash win` sends and saves their JSON. It collects metadata only — no
file contents, no credentials, no registry hives.
