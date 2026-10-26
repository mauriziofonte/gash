# Windows and WSL

Inspect and repair WSL, and read the Windows host from inside it.

When Gash runs inside a WSL distribution it can tell you how the
distribution is set up, repair its link to Windows, convert paths between
the two systems, read the state of the Windows host, apply a few fixes to
Windows through a UAC prompt, open Explorer and Task Manager, and restart
WSL. The rule that shapes this area: reading Windows needs nothing from you;
changing Windows is for a person at the desktop, through UAC.

Platforms: WSL only (a Linux distribution under the Windows Subsystem for
Linux). `gash wsl` reads files of the distribution and starts no Windows
program. `gash win` and the `wsl_*` functions start Windows programs
(`powershell.exe`, `cmd.exe`, `reg.exe`, `explorer.exe`) through WSL interop.
Off WSL, `gash wsl info` says so and exits 0; the other commands and
functions exit 1.

<!-- BEGIN generated: prerequisites (go test ./internal/docgen -update) -->
Beyond what Gash itself needs, this area uses (install commands for each system: [requirements](../requirements.md)):

- [sudo](../requirements.md#tool-sudo) (feature), for `wsl_compact`
- [powershell.exe](../requirements.md#tool-powershell.exe) (feature), for `gash win`
- [cmd.exe](../requirements.md#tool-cmd.exe) (feature), for `wsl_restart`, `wsl_shutdown` and `wsl_taskmanager`
- [explorer.exe](../requirements.md#tool-explorer.exe) (feature), for `wsl_explorer`
<!-- END generated: prerequisites -->

Every function, alias and command of this area, with all its options: [reference.md](reference.md).

## Check the WSL environment

```bash
gash wsl info     # WSL version, distribution, kernel, interop, drives, systemd, networking
gash wsl doctor   # the same facts as checks, each with a remedy when it fails
```

`gash wsl doctor` checks the interop (can this shell start Windows
programs), whether the Windows `PATH` is appended, the mounted drives and
their `metadata` option, the networking mode, DNS tunneling and systemd.
It exits 1 when a check fails. It also reminds you to build projects on the
Linux disk: `/mnt/c` is slow for compiling.

`gash wsl config` prints the settings in force: `/etc/wsl.conf`, then the
`.wslconfig` of your Windows profile, each key with its value and whether
it comes from the file or is WSL's default. `gash wsl config FILE` reads
another file.

Every `gash wsl` and `gash win` command takes `--json` for one JSON object
on stdout (`gash win fix` with `--dry-run`). See
[`gash wsl`](reference.md#gash-wsl).

## Convert a path

`gash wsl path` converts in either direction and guesses which one you want:

```bash
gash wsl path 'C:\Users\me\Downloads'   # /mnt/c/Users/me/Downloads
gash wsl path ~/project --windows       # \\wsl.localhost\Ubuntu\home\me\project
gash wsl path /mnt/c/Users/me --mixed   # C:/Users/me
```

`--windows`, `--mixed` (forward slashes, for tools that accept both) and
`--linux` force the direction. A path on a mounted drive becomes `X:\...`;
any other Linux path becomes the `\\wsl.localhost\<distribution>\...` path
that Windows programs open. The drive mount is read from the system, so a
distribution with `[automount] root=/` gets `/c/...`. A drive mounted more
than once (an extra `mount -t drvfs C: /win/c`, or a bind of
`C:\Users` on `/x`) converts from every one of its mount points; a Windows
path converts to the automount one (`/mnt/c`). `gash wsl info --json` lists
them all under each drive's `mounts`, and `gash win` finds the Windows
system drive on any of them.

## Repair Windows programs in this shell

After you reattach a `tmux` or `screen` session, `WSL_INTEROP` often points
at a socket that no longer exists, and every Windows program (`code .`,
`explorer.exe`, `clip.exe`) fails. `gash wsl interop` shows the state, and
its `--export` form fixes the shell in place:

```bash
gash wsl interop                        # Registered, Enabled, WSL_INTEROP, Live socket, Usable
eval "$(gash wsl interop --export)"     # point WSL_INTEROP at a live socket
```

`--export` prints nothing when no live socket exists; open a new terminal
then.

## Read the Windows host

`gash win` reads Windows and changes nothing:

```bash
gash win info                  # build, edition, host name, user, and whether the terminal is elevated
gash win doctor                # a health check: version, pending reboot, Defender, free disk space
gash win disk                  # fixed drives, size and free space
gash win net                   # adapters, DNS servers, proxy
gash win updates               # installed hotfixes, pending reboot
gash win defender              # Microsoft Defender status
gash win services ssh          # services, filtered by name
gash win procs chrome          # top processes by memory, filtered
gash win apps python           # installed programs, filtered
gash win ports --port 443      # TCP ports and their processes
gash win events --level error --since 24h
gash win reg 'HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion' --value ProductName
gash win file 'C:\Windows\System32\notepad.exe'   # owner, ACL, signature
gash win policy                # where policy is set, and the device join
gash win hosts                 # the Windows hosts file
```

Most of them run a fixed Windows PowerShell script; `reg` runs `reg.exe`,
`policy` also runs `dsregcmd.exe`, and `hosts` reads the file through the
mounted drive. Some data needs an administrator (some services, some
registry keys): from a normal terminal it reads as `windows_access_denied`.
`gash win info` says whether your Windows token is elevated. See
[`gash win`](reference.md#gash-win) for each command's options.

## Apply a fix to Windows

`gash win fix` is the only command that changes Windows. It offers a fixed
list of actions:

<!-- example -->
```console
$ gash win fix

────────────────────────────────────────────────────────────
gash win fix — elevated Windows corrections

action                 description                                    
──────────────────────────────────────────────────────────────────────
flush-dns              Clear the Windows DNS client cache
restart-service NAME   Restart a Windows service by its service name
hosts-add IP HOSTNAME  Add a mapping to the Windows hosts file
hosts-remove HOSTNAME  Remove a hostname from the Windows hosts file
each action asks for a Windows UAC prompt; add --dry-run to see the exact command
```

Each action shows the exact PowerShell command it will run as
administrator, asks you, then starts it with `Start-Process -Verb RunAs`, so
Windows shows its own UAC prompt: two confirmations, the second one
Windows'. `--yes` skips the first, never the UAC prompt. The arguments are
checked (an IP address, a host name, a service name) and passed as quoted
data, never as code. `--dry-run` prints the command and runs nothing, which
is also how an agent prepares a fix for you:

<!-- example -->
```console
$ gash win fix hosts-add 127.0.0.1 app.local --dry-run

────────────────────────────────────────────────────────────
Would run elevated

Action Add 127.0.0.1 app.local to the Windows hosts file
Warning: runs elevated as administrator (Windows will ask for UAC)

    powershell Add-Content -LiteralPath (Join-Path $env:WINDIR 'System32\drivers\etc\hosts') -Value '127.0.0.1 app.local  # added by gash'

this is a preview; run it without --dry-run on the Windows desktop to apply it
$ GASH_HEADLESS=1 gash win fix flush-dns
{"error":"elevation_required","details":"win fix changes Windows and needs a human to approve the UAC prompt; it will not run headless","action":"STOP","recoverable":true,"hint":"run it yourself in an interactive shell on the Windows desktop, or use --dry-run to see the exact command"}
```

`--dry-run --json` gives the command as JSON.

## Open Explorer or Task Manager

```bash
wex                # Explorer in the current directory (alias of wsl_explorer)
wex /mnt/c/Users   # Explorer in another directory
wtm                # Task Manager (alias of wsl_taskmanager)
```

[`wsl_explorer`](reference.md#wsl_explorer) translates the directory to its
Windows form first; [`wsl_taskmanager`](reference.md#wsl_taskmanager) starts
Task Manager in the background.

## Restart or shut down WSL

[`wsl_restart`](reference.md#wsl_restart) (alias `wr`) and
[`wsl_shutdown`](reference.md#wsl_shutdown) (alias `wsd`) do the same thing:
they save this shell's history, then run `wsl --shutdown` through `cmd.exe`.
Every distribution stops, Docker Desktop's included, with the servers
running in them; WSL starts again with the next terminal. They ask first;
`-y` skips the question.

## Fix Windows programs that time out

On a WSL 2 VM that has run for a long time, memory gets fragmented, and
starting any Windows program can fail with `UtilAcceptVsock ... accept4
failed 110` (the kernel log says `page allocation failure: order:7`).
[`wsl_compact`](reference.md#wsl_compact) (alias `wslc`) counts those
failures, drops the page cache and compacts memory (with `sudo`), shows the
free memory blocks before and after, and checks that `cmd.exe` starts:

```bash
wslc --status   # the failure count and the free blocks, nothing changed
wslc            # compact; in headless mode it needs -y
```

When it still fails, run `wsl --shutdown` from Windows.

## How Gash talks to Windows

Every Windows program that `gash win` runs goes through one bridge, so a few
rules hold everywhere:

- Programs are found by their absolute path under the Windows `System32`
  directory, never through `PATH`, so they work with
  `[interop] appendWindowsPath=false` in `wsl.conf`.
- The drive that holds Windows is found from the mounts, not assumed to be
  `/mnt/c`.
- Every call has a timeout, `GASH_WIN_TIMEOUT` (an environment variable:
  seconds or a duration such as `8s`; default 10 seconds, at most 120). A
  stuck interop fails with `interop_timeout` instead of hanging the shell.
- Interop that is turned off is reported as `interop_unavailable`, not
  worked around.
- The PowerShell scripts are fixed and only read; your arguments reach them
  as JSON data on standard input, never as part of the script.

What Gash does not do: it never elevates without you (there is no UAC
bypass), and `gash win fix` is a fixed list rather than a way to run any
command elevated. The agent command runner `llm_exec` has no Windows program
on its list of allowed commands, and under `GASH_LLM_SANDBOX=1` no Windows
program can start from its sandbox. Windows secrets reachable over `/mnt` (the user registry
hives, saved RDP files, answer files, browser password stores) are secret
files for every Gash reader; see [security](../security.md).

## When something goes wrong

`gash win` errors are JSON objects on stderr, with status 1; `error` is the
code:

| Code or message | What it means and what to do |
|---|---|
| `interop_unavailable`, "not running under WSL" | You are not in WSL. |
| `interop_unavailable`, "WSL_INTEROP points at a socket that is gone" | Run `eval "$(gash wsl interop --export)"`, or open a new terminal. |
| `interop_unavailable`, "Windows interop is off" or "the WSLInterop handler is not registered" | Interop is disabled: set `[interop] enabled=true` in `/etc/wsl.conf`, then restart WSL (`wr`). |
| `interop_unavailable`, "no Windows system drive is mounted" | Check `[automount]` in `/etc/wsl.conf` (`gash wsl config`). |
| `interop_timeout` | Windows did not answer within `GASH_WIN_TIMEOUT`. Try again; if Windows programs fail everywhere, run `wslc`, then `wr`. |
| `windows_access_denied` | Windows refused the read: the data needs an elevated terminal (open Windows Terminal as administrator), or the file's ACL denies you. |
| `elevation_required` | `gash win fix` without a person at a terminal, or an operation that needs an administrator. Run it yourself in an interactive shell, or use `--dry-run`. |
| `windows_not_found` | The Windows program, file or key does not exist, or a Windows program could not start. |
| `invalid_windows_argument` | An argument failed the check before anything ran: a registry key must start with `HKLM\`, `HKCU\`, `HKCR\`, `HKU\` or `HKCC\`; a fix needs a valid IP address, host name or service name. |
| `Error: This command requires WSL (Windows Subsystem for Linux).` | A `wsl_*` function ran outside WSL. |
| `Error: wsl_compact needs root: pass -y to run it unattended.` | `wsl_compact` in headless mode: add `-y`; `sudo` must then work without a password. |
| `Error: Windows interop still fails. Last resort, from Windows: wsl --shutdown` | Compacting did not free a large enough block: restart WSL from Windows. |

`gash wsl doctor` prints a `remedy:` line under each failed check, which is
usually the fastest way to the fix.

## See also

- [reference.md](reference.md): every function and command of this area with all its options.
- [configuration](../configuration.md#environment-only-variables): `GASH_WIN_TIMEOUT`.
- [security](../security.md): the Windows secret files, and what `llm_exec` refuses under WSL.
- [Local web stack (HTE)](../hte/README.md): `gash hte hosts windows` prints the lines for the Windows hosts file.
- [capture.ps1](capture.ps1): for contributors, a read-only PowerShell script that records what the `gash win` scripts read on a real Windows machine, to pin the test fixtures.
