# Gash itself

This area is Gash looking after itself: finding a function and its help,
checking the installation, installing the tools a function needs, reading
the settings, updating to a new release, and unloading or removing Gash.
Installing Gash the first time is in [install.md](../install.md).

Platforms: Linux, macOS and WSL. `gash_doctor --timing` needs Bash 5.0 or
newer. `gash_doctor --install` needs one of the package managers apt, dnf,
pacman, zypper, apk or Homebrew.

<!-- BEGIN generated: prerequisites (go test ./internal/docgen -update) -->
Beyond what Gash itself needs, this area uses (install commands for each system: [requirements](../requirements.md)):

- [ssh-keygen](../requirements.md#tool-ssh-keygen) (optional), for `gash_upgrade` (GASH_REQUIRE_SIGNED=1) and `gash update` (GASH_REQUIRE_SIGNED=1)
- [bc](../requirements.md#tool-bc) (optional), for `gash init` (the calc alias)
- [cosign](../requirements.md#tool-cosign) (optional), for `gash_upgrade` (GASH_REQUIRE_SIGNED=1) and `gash update` (GASH_REQUIRE_SIGNED=1)
- [gpg](../requirements.md#tool-gpg) (optional), for `gash_upgrade` (GASH_REQUIRE_SIGNED=1) and `gash update` (GASH_REQUIRE_SIGNED=1)
<!-- END generated: prerequisites -->

## Find a function and its help

`gash` alone prints the reference card: one line per topic, with the number
of functions and aliases in it. `gash TOPIC` lists the functions and
aliases of one topic (`gash git`, `gash docker`, `gash llm`), and
`gash all` the whole card. In an interactive shell the card lists only the
aliases the shell defines: an alias left out because a program of the same
name is installed does not appear.

`gash_help NAME` prints the full help of a function, with examples. It
accepts an alias (`gash_help flf` is the help of `files_largest`), and
every function also prints its help with `--help`. To find a function by
what it does, search all the help text:

<!-- example -->
```console
$ gash_help --search uninstall

  Search results for 'uninstall':

  gash_uninstall                        Uninstall Gash and clean up configurations
  nvm_prune_versions      (nvmp)        Remove duplicate/older NVM patch versions
```

`--short` prints the one-line description only, for scripts:

<!-- example -->
```console
$ gash_help --short files_largest
List the largest files under a path with size/exclude/json filters
```

`gash_help --list` lists every function by module, and
`gash_help --bash cd` shows the help of a Bash builtin. The generated
references of each area hold the same help text: start from the
[documentation index](../README.md).

See [`gash_help`](reference.md#gash_help) and [`gash`](reference.md#gash).

## Check the installation

`gash_doctor` (alias `gdoctor`) checks everything Gash depends on and
prints one line per check:

```bash
gash_doctor
```

| Section | What it checks |
|---|---|
| Runtime | The Bash version (4.4 or newer), the gash binary in use, and the files that failed to load in this shell |
| Core Files, Modules | The files of the install directory |
| Binary | The binary and `gash-hosts-helper` against the SHA-256 that the release pins in `release/SHA256SUMS` (a development build is not checked) |
| Shims | The generated Bash files against what this gash version generates |
| Configuration | The mode of `~/.gash_env` (600 or 400), the settings set in this shell, and each `SSH:` key of the file (it must exist and be private) |
| Prerequisites | Every external program Gash runs, grouped by area: found (with its path) or missing (with what needs it, its package and the install command for this system) |

It ends with `All checks passed.` or `N issue(s) found.` and returns 1 when
a check fails. A missing tool that Gash itself needs (Bash, git, curl or
wget, tar, gzip, sha256sum, grep, sed) is an issue. A missing tool that
only some functions use is a warning, and an optional one an `Info:` line:
neither makes the doctor fail.

The options that matter:

- `--prereqs` prints only the Prerequisites section.
- `--json` prints the prerequisites as one JSON object, for scripts and
  agents: `system` (distribution, package manager, root, sudo, WSL),
  `tools` (one entry per tool: `name`, `level`, `area`, `found`, and
  `path` when found, or `needed_by`, `package`, `install` and `advice`
  when missing), `missing` and `missing_required`. It returns 1 when a
  required tool is missing.
- `--timing` loads Gash in a new shell and lists what each file and step
  costs, then times whole new shells with your `~/.bashrc` and without it.
  Steps of 5 ms or more stand out.

[requirements.md](../requirements.md) lists every tool with its package on
each distribution. See [`gash_doctor`](reference.md#gash_doctor) for the
rest.

## Install a missing tool

When a function needs a program that is not installed, it stops before
doing anything and says so. In a terminal the first line is
`Error: NEED needs TOOL, which is not installed.`, where NEED is the
function (with the option that needs the tool, when only one does). The
next line says what TOOL is for and which package provides it on this
system, then `Install it:` gives the install command and `Or run:` the
`gash_doctor --install TOOL` that runs it for you. A few functions still
print the shorter `Error: Command 'TOOL' is not installed or not in PATH.`:
the fix is the same.

Run the install command it shows, or let Gash run it:

```bash
gash_doctor --install rsync ssh    # the named tools
gash_doctor --install              # every missing tool that a function needs
gash_doctor --install --all        # the optional tools too
```

`gash_doctor --install` prints the packages and the command, asks before it
runs anything (`--yes` skips the question), fetches the package lists first
where the manager needs it (apt, apk), then checks again that each tool is
found. Some tools have no package Gash can install: Docker, Compose,
composer, nvm, systemd and the Windows programs of WSL. Their line points
to the official instructions instead. A tool in a non-free repository
(`unrar`) is installed only when you name it.

When you are not root and `sudo` is missing, Gash cannot run the command:
it prints it, marked `(as root)`, for you to run as root.

An agent gets the same information as one JSON line on stderr: the error
code (`prerequisite_missing`, or a code of the tool such as `git_not_found`
or `docker_not_found`), the action `STOP`, and the members `missing` (the
tool) and `install` (the command, when there is one). `gash_doctor
--install` never installs anything for an agent: with `GASH_HEADLESS=1` it
fails with `install_needs_confirmation` and the command to show you.

## Settings

Gash reads its settings from `~/.gash_env`, a private file that also holds
SSH keys, database connections, AI tokens and remote hosts. Create it from
the template:

```bash
gash_env_init            # ~/.gash_env, mode 600; never overwrites an existing file
gash_env_init --force    # replace it with a fresh template
```

Your own aliases and shell code go in `~/.bash_aliases` and
`~/.bash_local`, which Gash loads last. Every setting, the file grammar and
the load order are in [configuration.md](../configuration.md).

## Update Gash

```bash
gash update               # or gash_upgrade: move to the newest release
gash update --rollback    # back to the version the last update replaced
```

`gash update` and `gash_upgrade` run the same steps:

1. They refuse an install directory with local changes (commit, stash or
   discard them first).
2. They fetch the release tags and pick the highest release. A checkout
   that is already newer (a pre-release, or a later commit) stays as it is.
3. They check the signature of the release (see below).
4. They download the binary of the new version into
   `~/.local/share/gash/versions/VERSION/` and verify it against the hash
   the release pins, before anything else changes.
5. They check out the new release and load it in a test shell. When that
   fails, the previous version is checked out again.

Open a new terminal afterwards to load the new version. The previous
version and its binary stay installed: `gash update --rollback` returns to
it, and a second rollback comes back. The exit status is 0 when Gash was
updated or was already up to date, 1 when the update was refused or failed
(nothing changed, or the previous version is back), 2 for a bad argument.

See [`gash_upgrade`](reference.md#gash_upgrade) and
[`gash update`](reference.md#gash-update).

### Require a signed release

By default a signature that cannot be checked is a warning. With
`GASH_REQUIRE_SIGNED=1` in the environment, the installer and
`gash update` refuse a release unless one of these verifies:

- the tag's own signature (PGP or SSH), checked with `git verify-tag`, or
- the Sigstore signature of the release (`checksums.txt.sigstore.json`),
  checked with `cosign` for the commit the tag points at. The release
  workflow signs every release this way.

They then accept only a binary whose hash the tagged tree pins in
`release/SHA256SUMS`.

```bash
GASH_REQUIRE_SIGNED=1 gash update
```

Without `cosign` on `PATH` an unsigned tag is refused, and the message says
where to get `cosign`. [security.md](../security.md#install-and-upgrade)
explains what each check covers.

## Check the version

```bash
gash version          # version, platform and path of the binary in use
gash version --json   # {"version", "os", "arch", "path"}
```

In a shell, `$GASH_VERSION` holds the version of the loaded tree.

## Unload Gash from this shell

`gash_unload` removes Gash from the current shell and changes nothing on
disk: every Gash function, alias and `GASH_*` variable goes, the prompt,
history settings, `PATH`, `EDITOR` and the umask return to how they were
before Gash loaded, and so do the variables Gash set (`LS_COLORS`,
`PAGER`, `PROMPT_COMMAND` and the like). Load it again with
`source ~/.gashrc`.

<!-- example -->
```console
$ type -t gash_doctor
function
$ gash_unload
$ type -t gash_doctor || echo "not defined"
not defined
```

`gash_unload` works in an interactive shell only. See
[`gash_unload`](reference.md#gash_unload).

## Remove Gash

If you installed `gash hte`, run `gash hte uninstall --yes` first: it stops
the stack and restores the system files it changed (see the
[HTE guide](../hte/README.md)). Then:

```bash
gash_uninstall
```

`gash_uninstall` asks first, in a terminal only: without one (a script, an
agent) it prints `Info: Uninstall cancelled.` and removes nothing. After
you confirm it:

1. removes the Gash block from `~/.bashrc`, `~/.bash_profile` and
   `~/.profile` (a profile that is a symlink stays one: the file it points
   to is edited);
2. deletes `~/.gashrc` and the install directory (only a directory that is
   a Gash install, never your home or `/`);
3. asks again before it deletes `~/.gash_env` (or the file `GASH_ENV_FILE`
   names).

It keeps each edited profile as `FILE.gash-uninstall.bak`, and
`~/.gashrc.gash-uninstall.bak` when your `~/.gashrc` held lines of your own,
and lists these backups at the end. It leaves behind, for you to remove by
hand: `~/.local/state/gash` (notices and update records),
`~/.local/share/gash/versions` (the downloaded binaries), `~/.cache/gash`,
and the `ssh-agent` Gash started with its socket. Open a new terminal
afterwards.

## Coming from Gash 1.x

Gash 3 does not migrate a Gash 1.x configuration. While `~/.gashrc` is
still the 1.x loader, an interactive shell reminds you at most once a day.
`gash migrate` (or `gash_migrate`) prints the route and changes nothing:

<!-- example
mask: v2\.1\.0 in [^ ]+, open
-->
```console
$ gash migrate
Error: gash_migrate: Gash 3 does not migrate a Gash 1.x configuration; check out v2.1.0 in ~/.gash, open a new terminal, run gash_migrate there, then gash_upgrade
```

[The 2.1.0 migration guide](https://github.com/mauriziofonte/gash/blob/v2.1.0/docs/migration.md)
has the steps.

## When something goes wrong

| Message | Meaning and fix |
|---|---|
| `NEED needs TOOL, which is not installed` | A function needs a program this system lacks. Run the install command the message shows, or `gash_doctor --install TOOL`. |
| `Unknown tool: NAME (gash_doctor --prereqs lists them)` | `gash_doctor --install` takes the names of the Prerequisites list (`ssh`, `7z`, `gnu-sed`), not package names. |
| `Installing packages needs root, and sudo is not installed` | Run the command it prints as root. |
| `Working tree at DIR has local modifications` | `gash update` never overwrites your edits. `git -C ~/.gash stash` keeps them, `git -C ~/.gash checkout -- .` discards them. |
| `GASH_REQUIRE_SIGNED=1 is set; not upgrading to TAG` | No signature of the release could be verified. Install `cosign` (or the key of a signed tag), or update without `GASH_REQUIRE_SIGNED=1`. |
| `TAG failed its smoke test (...)` | The new version did not load in a test shell; the previous one is checked out again and nothing else changed. Run `gash_doctor` and report the message. |
| `No previous version of DIR to return to` | `gash update --rollback` has nothing recorded; check out a release tag by hand: `git -C ~/.gash checkout vX.Y.Z`. |
| `gash VERSION for OS/ARCH is not installed and could not be downloaded (REASON)` | The binary of this version is missing. The hint names the missing download tool, or says to run `bash ~/.gash/install.sh --repair`. |

More symptoms and fixes are in [troubleshooting.md](../troubleshooting.md).

## See also

- [reference.md](reference.md): every function and command of this area.
- [install.md](../install.md): install, verify and the supported systems.
- [configuration.md](../configuration.md): `~/.gash_env`, every setting,
  the load order.
- [requirements.md](../requirements.md): every external tool and its
  package.
- [troubleshooting.md](../troubleshooting.md) and
  [security.md](../security.md).
