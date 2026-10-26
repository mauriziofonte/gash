# Install, update and remove Gash

Gash installs into your home directory, for your user only: a git checkout
in `~/.gash`, one binary in `~/.local/share/gash`, a `~/.gashrc` file and
a short block in your Bash profile. It needs no root, except to install
missing system packages, which it asks about first.

Platforms: Linux (amd64, arm64), WSL 2, macOS (Apple silicon and Intel).
Gash runs in Bash 4.4 or newer. 32-bit systems and ARMv7 have no release
binary.

## Supported systems

CI installs Gash on each of these systems at every change: from the bare
image, `install.sh` installs what Gash needs with the system's package
manager, then `gash_doctor` installs every package Gash can use and checks
that each one provides its tool. The last column is the end-to-end test of
the local web stack ([`gash hte`](hte/README.md)).

<!-- BEGIN generated: systems (go test ./internal/docgen -update) -->
| System | Version | amd64 | arm64 | Local sites (`gash hte`) |
|---|---|---|---|---|
| Rocky Linux | 8 | tested | tested | not tested |
| Rocky Linux | 9 (minimal image) | tested | tested | not tested |
| AlmaLinux | 9 | tested | tested | tested |
| AlmaLinux | 10 | tested | tested | not tested |
| Oracle Linux | 9 | tested | tested | not tested |
| Amazon Linux | 2023 | tested | tested | not tested |
| Fedora | 44 | tested | tested | tested |
| Debian | 12 | tested | tested | tested |
| Ubuntu | 24.04 | tested | tested | tested |
| Alpine Linux | 3.22 | tested | tested | not tested |
| openSUSE Leap | 15.6 | tested | tested | not tested |
| openSUSE Tumbleweed | rolling | tested | tested | not tested |
| Arch Linux | rolling | tested | no image | tested |
| macOS (Homebrew Bash) | current | built, not tested | tested | tested (Homebrew httpd and PHP) |
<!-- END generated: systems -->

Other distributions work when they have Bash 4.4 and the tools of
[the requirements](requirements.md); `gash_doctor` recognizes the families
above (Debian, the RHEL family, Arch, SUSE, Alpine) through
`/etc/os-release`, so a derivative such as Linux Mint, Pop!_OS, CentOS
Stream or Manjaro gets the same install commands as its parent.

WSL 2 runs the Linux binary inside your distribution; the Windows
functions ([Windows and WSL](windows/README.md)) also need the Windows
interop of WSL (`powershell.exe` on PATH), which WSL enables by default.

## What Gash needs

To install and load, Gash needs Bash 4.4 or newer, `git`, `curl` or
`wget`, `tar`, `gzip`, `sha256sum` or `shasum`, `grep` and `sed`. Most
systems have them all; minimal container images often lack `git`, `curl`
or `tar`. Check without changing anything:

```bash
curl -fsSL https://raw.githubusercontent.com/mauriziofonte/gash/main/install.sh | bash -s -- --check
```

It names what is missing and prints the command that installs it on this
system, with status 1, or confirms that everything is there.

Everything else Gash can use (Docker, rsync, expect, Apache, the database
clients of remote hosts...) is needed only by the functions that use it.
[docs/requirements.md](requirements.md) lists every tool with its package
on each system. After the install, `gash_doctor` shows what this machine
lacks and installs it on request.

macOS ships Bash 3.2, which is too old. Install a current Bash first:

```bash
brew install bash
chsh -s "$(brew --prefix)/bin/bash"
```

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/mauriziofonte/gash/main/install.sh | bash
```

or, with wget:

```bash
wget -qO- https://raw.githubusercontent.com/mauriziofonte/gash/main/install.sh | bash
```

To read the installer before it runs, clone the repository and run it from
there:

```bash
git clone https://github.com/mauriziofonte/gash.git ~/.gash
bash ~/.gash/install.sh
```

The installer:

1. checks the Bash version and asks once whether to continue;
2. checks the tools above, and when one is missing shows the command that
   installs it and asks whether to run it (with `sudo` when you are not
   root);
3. clones the repository into `~/.gash` and checks out the newest release
   tag;
4. adds this block to `~/.bashrc` (or `~/.bash_profile`, or `~/.profile`,
   the first that exists), unless the profile already sources `~/.gashrc`:

   ```bash
   # >>> GASH START >>>
   # Load Gash Bash - Do not edit this block, it is managed by Gash installer
   if [ -f "$HOME/.gashrc" ]; then
     source "$HOME/.gashrc"
   fi
   # <<< GASH END <<<
   ```

5. creates `~/.gashrc`, the file the block loads, which you can edit and
   which is never overwritten;
6. downloads the `gash` binary of that release for your system into
   `~/.local/share/gash/versions/VERSION/` and checks its SHA-256 against
   the hash the release tag pins in `release/SHA256SUMS`;
7. offers to install, through `gash_doctor --install`, the tools that
   single functions use and this system lacks.

Open a new terminal when it is done, or run `source ~/.bashrc`. Running the
installer again on an existing install updates it to the newest release
tag; it refuses when the checkout has local changes.

### Options

| Option | Effect |
|---|---|
| `--assume-yes`, `-y` | Install without asking. Missing required tools are installed with the package manager; the tools of single functions are not (run `gash_doctor --install` later). |
| `--quiet`, `-q` | Print only warnings and errors. |
| `--check` | Report what is missing and change nothing (status 1 when something is). |
| `--repair` | Download and verify the binary of the installed version again, and nothing else. |
| `--help`, `-h` | Print the usage. |

Pass options through the pipe with `bash -s --`:
`curl -fsSL .../install.sh | bash -s -- --assume-yes`.

| Variable | Effect |
|---|---|
| `GASH_DIR` | Install location (default `~/.gash`). `~/.gashrc` remembers it. |
| `PROFILE` | The profile file to edit; `/dev/null` edits none. |
| `GASH_INSTALL_REF` | A branch, tag or commit to install instead of the newest release tag. |
| `GASH_INSTALL_GIT_REPO` | The repository to clone (a mirror, or a local path). |
| `GASH_RELEASE_URL` | The base URL of the release archives (a mirror; `file://` works). |
| `GASH_REQUIRE_SIGNED=1` | Refuse a release whose signature does not verify (see [Verify a release](#verify-a-release)). |

### Install without a terminal

In a Dockerfile, a CI job or a provisioning script there is nobody to
answer the questions, so pass `--assume-yes`:

```bash
curl -fsSL https://raw.githubusercontent.com/mauriziofonte/gash/main/install.sh | bash -s -- --assume-yes --quiet
```

As root it installs missing required packages directly; as another user it
needs `sudo` without a password prompt, or the packages installed
beforehand. Without `--assume-yes` and without a terminal, the installer
stops with a message instead of waiting for an answer.

An AI agent loads Gash with `GASH_HEADLESS=1` instead of through the
profile: see [AGENTS.md](../AGENTS.md).

## Verify a release

Every install and update checks the binary it downloads against the
SHA-256 that the release tag pins in `release/SHA256SUMS`, so a binary
that differs from the one built for that tag is refused.

`GASH_REQUIRE_SIGNED=1` adds a check of the release itself: the release tag
must carry a signature that `git verify-tag` accepts, or the release must
carry the Sigstore signature that the release workflow makes, verified with
[cosign](https://docs.sigstore.dev/cosign/system_config/installation/) for
the tag's commit. Releases are signed with Sigstore, so this needs `cosign`
on PATH:

```bash
curl -fsSL https://raw.githubusercontent.com/mauriziofonte/gash/main/install.sh | GASH_REQUIRE_SIGNED=1 bash
```

Export it in `~/.gashrc` to apply it to every later `gash update`. How to verify a
release archive by hand, with cosign or `gh attestation verify`, is in
[the security model](security.md).

## First steps

```bash
gash                 # the reference card: every topic with its functions
gash_doctor          # check the install, and list what this system lacks
gash_doctor --install   # install the missing tools, after asking
```

[docs/gash/README.md](gash/README.md) explains the card, the help and the
doctor; [docs/README.md](README.md) lists every area.

## Update

```bash
gash update            # the same as gash_upgrade
gash update --rollback # back to the version you had before
```

`gash update` moves to the highest release tag of the repository, never to
an older one, and refuses a checkout with local changes. Before it changes
anything it downloads and verifies the new binary (and the signature, with
`GASH_REQUIRE_SIGNED=1`); after the checkout it loads the new version in
two new shells, and goes back to the previous version if either fails.
The previous version and its binary are kept for `--rollback`. Open a new
terminal afterwards.

If the binary of the installed version is missing or damaged (a failed
download, a cleaned cache), `gash_doctor` says so and this restores it:

```bash
bash ~/.gash/install.sh --repair
```

### From Gash 2.x

Run `gash_upgrade` as usual: it moves you to the newest 3.x release. To go
back, run `git -C ~/.gash checkout v2.1.1` and open a new terminal.

### From Gash 1.x

Gash 3 does not read a 1.x configuration. Move to 2.1.0 first and let its
guided migration convert your files, then upgrade:

```bash
git -C ~/.gash fetch --tags
git -C ~/.gash checkout v2.1.0
exec bash             # the 2.x migration starts in this shell
gash_upgrade
```

The 2.1.0 migration is described in
[its guide](https://github.com/mauriziofonte/gash/blob/v2.1.0/docs/migration.md).

## Remove Gash

```bash
gash_unload      # this shell only: back to how it was before Gash loaded
gash_uninstall   # remove Gash, its profile block and ~/.gashrc (asks first)
```

`gash_uninstall` keeps a copy of each profile as it was before the edit,
and of a `~/.gashrc` that holds your own lines, as
`FILE.gash-uninstall.bak`. If you installed the local web stack, run
`gash hte uninstall --yes` first: it stops the stack and restores the
system files it changed. Packages installed for Gash stay installed.

## When the install fails

| Message | What to do |
|---|---|
| `Gash needs these tools, which are not installed: ...` | Run the command printed after it, or the installer with `--assume-yes`. |
| `Gash requires Bash 4.4 or newer` | Install a current Bash (macOS: `brew install bash`), then run the installer with it. |
| `No terminal to ask on` / `No terminal to ask for confirmation` | Run the installer with `--assume-yes`, or from a terminal. |
| `... has local modifications` | The checkout in `~/.gash` was edited: `git -C ~/.gash stash`, then run the installer again. |
| `... exists and is not a Gash checkout` | Move that directory away, or install elsewhere with `GASH_DIR`. |
| `Gash cannot run until its binary is installed` | The download or its check failed (network, proxy, a mirror out of date): run `bash ~/.gash/install.sh --repair` when that is fixed. |

More symptoms and fixes are in [Troubleshooting](troubleshooting.md).
