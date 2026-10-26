# System

These functions handle the jobs of looking after a machine from its shell:
find a process and stop it, free a port that something still holds, stop the
usual development services at the end of the day, see which disks are
filling, run again with `sudo` the command that just failed for lack of
permission, and print a report of the system's configuration.

Platforms: Linux, macOS and WSL. `services_stop` needs systemd or the
`service` command (Linux, WSL). `sysinfo` reads what a Linux system keeps in
`/proc`, `/etc` and systemd: on macOS its report is partial.

<!-- BEGIN generated: prerequisites (go test ./internal/docgen -update) -->
Beyond what Gash itself needs, this area uses (install commands for each system: [requirements](../requirements.md)):

- [ps](../requirements.md#tool-ps) (feature), for `process_find`
- [pgrep](../requirements.md#tool-pgrep) (feature), for `process_kill`
- [lsof](../requirements.md#tool-lsof) (feature), for `port_kill`
- [sudo](../requirements.md#tool-sudo) (feature), for `services_stop`, `sudo_last` and `sysinfo` (the privileged sections)
- [systemctl](../requirements.md#tool-systemctl) (feature), for `services_stop`
<!-- END generated: prerequisites -->

Every function, alias and command of this area, with all its options: [reference.md](reference.md).

The outputs on this page depend on the machine, so they are described rather
than shown.

## Find a process

[`process_find`](reference.md#process_find) (`pf`) lists the processes whose
command line contains a text, ignoring case: one row per process with its
PID, user, CPU use and program.

```bash
pf nginx
pf "manage.py runserver"
```

The text is matched as written, not as a pattern, and only against the
command line, so `pf root` does not list every process of the root user. The
status is 1 when nothing matches, which makes
`pf mysql >/dev/null && echo running` a quick test.

## Stop a process, or whatever holds a port

[`process_kill`](reference.md#process_kill) (`pk`) stops the processes whose
program name is exactly NAME, ignoring case, and
[`port_kill`](reference.md#port_kill) (`ptk`) the ones listening on a TCP or
UDP port:

```bash
pk node         # every process named node
ptk 3000        # whatever listens on port 3000
ptk -y 3000     # the same, without the question
```

Both list what they found (PID and command) and ask before they act; `-y` (or
`--yes`, `-f`, `--force`) skips the question. Each process gets SIGTERM, and
SIGKILL if it is still running about three seconds later, and the function
says which one ended it. `port_kill` only touches listeners: a browser or an
SSH session connected to that port on another machine is left alone.

`process_kill` takes a name, not a pattern: `pk node` does not stop
`nodemon`. Use `pf` first when you are not sure of the name. Without a
terminal to ask on (a script, `GASH_HEADLESS=1`) it refuses unless you pass
`-y`; `port_kill` answers no for you there.

A process of another user cannot be signalled: the function reports
`Process PID: permission denied.` and returns 1. Stop it with `sudo kill PID`,
or through its service.

## Stop the usual services

[`services_stop`](reference.md#services_stop) (`svs`, also `quit`) stops the
development services that are running among a fixed list (Apache, nginx,
MySQL, MariaDB, PostgreSQL, Redis, Docker and a few more; the
[reference](reference.md#services_stop) has the list):

```bash
svs --dry-run   # which of them are running
svs             # list them, ask, then stop them
svs -y          # stop them without the question
```

It uses `systemctl` when systemd runs, and the `service` command otherwise
(WSL without systemd, containers). Stopping a service needs root, so it runs
`sudo`, which may ask for your password. Without a terminal to ask on (a
script, `GASH_HEADLESS=1`) it answers no unless you pass `-y`, and then runs
`sudo -n`: when `sudo` needs a password, it stops before stopping anything.
The status is 1 when you decline or when one service fails to stop.

## Check disk space

[`disk_usage`](reference.md#disk_usage) (`du2`) is `df` limited to the
filesystems on real disks: ext4, XFS, Btrfs, ZFS, FAT, NTFS, APFS and the
like, without tmpfs, overlay and other virtual mounts. Each row has the mount
point, device, type, size, used and free space, and a bar of the use that
turns yellow at 75% and red at 90%.

```bash
du2
```

Under WSL the Windows drives (`/mnt/c` and the others) are listed too. On
macOS, where `df` cannot show the type, every `/dev` device is listed without
it. To find what fills a disk, go on with
[files and archives](../files/README.md#find-what-fills-a-disk).

## Run the last command again with sudo

[`sudo_last`](reference.md#sudo_last) (`plz`) is for the moment a command
fails with "Permission denied":

```bash
apt update      # fails: not root
plz             # shows "apt update", asks, then runs it with sudo
plz systemctl restart nginx   # runs sudo systemctl restart nginx at once
```

Without arguments it takes the previous command from this shell's history,
prints `Warning: About to run as root: COMMAND` and asks; it runs the command
only when you answer `y`. The command then runs in a new Bash started by
`sudo`, so your aliases and Gash functions are not available there. With
arguments, `plz` is `sudo` followed by them, without a question.

## Get a system report

[`sysinfo`](reference.md#sysinfo) (`si`) reads the system's configuration and
prints it by section. Without arguments it prints the overview (host, uptime,
load, CPU, memory, swap, root disk and address), then every section; with one
section name it prints only that one:

```bash
si overview     # the summary card
si webstack     # Apache, nginx, PHP and its FPM pools, MySQL or MariaDB, PostgreSQL
si network      # resolv.conf, hosts, interfaces, netplan
si services --llm | less
```

The sections cover the system's identity, storage, services, users and SSH,
network, security tools, web stack, mail, infrastructure (FTP, Samba, VPN,
Docker, monitoring) and system settings (cron, certificates, kernel tuning);
the [reference](reference.md#sysinfo) says what each one reads.

Some sections read files only root may read. On a terminal, `sysinfo` asks
for your `sudo` password for those (`identity`, `storage`, `services`,
`auth`, `security`, `system` and the full report), once per run; `overview`,
`network`, `webstack`, `mail` and `infra` never ask. If you do not give the
password, or there is no terminal, the report goes on with what your user can
read.

`--llm` prints the same content as plain `key=value` lines and file excerpts
under `=== SECTION ===` headings, without colors or boxes: compact input for
an AI model. [`ai_sysinfo`](../ai/reference.md#ai_sysinfo) collects the same
report and asks an AI model to analyze it.

## Find your public IP address

[`ip_public`](reference.md#ip_public) (`myip`) asks `https://ipinfo.io/ip`
which address your connections come from, and gives up after 10 seconds.
`--raw` prints the bare address, for scripts:

```bash
myip
echo "deploy from $(myip --raw)"
```

## When something goes wrong

| Message | Meaning and what to do |
|---|---|
| `Error: No process found with name 'NAME'.` | `process_find` found no command line containing NAME. Status 1 |
| `Error: No process found with exact name 'NAME'. Use process_find (pf) to inspect candidates.` | `process_kill` matches the exact program name. `pf NAME` shows the candidates and their real names |
| `Error: N process(es) match 'NAME' (PID ...). Refusing to kill without a terminal to confirm; pass -y.` | `process_kill` ran without a terminal. Add `-y` once you have checked the list |
| `Error: Nothing is listening on port PORT.` | No TCP listener or UDP socket is bound to that port. A process of another user may be invisible to `lsof`: try `sudo lsof -i :PORT` |
| `Error: Invalid port: VALUE (expected 1-65535).` | Give the port as a number |
| `Error: This function requires 'lsof', which is not available.` | Install `lsof` ([requirements](../requirements.md#tool-lsof)) |
| `Error: Process PID: permission denied.` | The process belongs to another user: `sudo kill PID` |
| `Error: No service manager found (systemd or service).` | `services_stop` needs `systemctl` with systemd running, or the `service` command |
| `Error: sudo needs a password: run interactively or configure NOPASSWD.` | `services_stop -y` ran without a terminal and `sudo` cannot ask. Run it from a terminal |
| `Info: None of the known services is running.` | Nothing to stop; status 0 |
| `Error: No previous command in history.` | `sudo_last` found no previous command in this shell |
| `Error: Unknown argument: WORD` | `sysinfo` takes one section name from the list it prints, plus `--llm` and `--no-color` |
| `Error: Only one section allowed: WORD` | Run `sysinfo` once per section, or without a section for all of them |
| `Warning: Could not obtain sudo. Some data will be limited.` | The password was not given; the report shows what your user can read |
| `Error: Failed to retrieve public IP address.` | No network, a proxy or captive portal answered instead of `ipinfo.io`, or 10 seconds passed |

## See also

- [reference.md](reference.md): every function of this area, with all options.
- [Files and archives](../files/README.md): what fills a disk, and archives.
- [Windows and WSL](../windows/README.md): `wsl_restart`, `wsl_shutdown` and the other WSL functions.
- [Logs](../logs/README.md): log files that fill a disk.
- [AI assistants](../ai/README.md): `ai_sysinfo`, an analysis of the `sysinfo` report.
- [Tools for AI agents](../agents/README.md): `llm_procs`, `llm_ports`, `llm_sysinfo` and the other JSON reads.
