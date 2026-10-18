<!-- Gash 3.0 design: 2.x to 3.x upgrade path. Author: Maurizio Fonte <fonte.maurizio@gmail.com> -->

# Upgrade path (PRD 5.3, the highest-risk area)

## 1. Facts this design relies on (verified in the S1 audit)

| Fact | Source |
|---|---|
| 2.x installs are git clones (default `~/.gash`) on a detached tag | `install.sh`, `lib/modules/gash.sh` |
| 2.x `gash_upgrade`: refuses a dirty `git status --porcelain`, `git fetch --quiet --tags origin`, picks `git tag --list --sort=v:refname \| grep -E '^v?[0-9]+(\.[0-9]+)*$' \| tail -n 1`, compares it with `git describe --tags --exact-match HEAD` for equality only, verifies the tag signature (`GASH_REQUIRE_SIGNED=1` refuses unsigned), `git checkout --quiet <tag>`, then prints "Open a new terminal (or run 'source ~/.gashrc') to load it". No build step, no smoke test, no rollback, no re-source | `lib/modules/gash.sh` (`__gash_latest_tag`, `__gash_verify_tag`, `gash_upgrade`) |
| Pre-release tags such as `v3.0.0-rc.A` do not match the 2.x regex: 2.x installs never move onto an rc by themselves | same |
| 1.x `gash_upgrade` picks the newest tag by creation date (`git for-each-ref --sort=-creatordate`), whatever its name, then `checkout` + `reset --hard` + reflog expiry | `v1.5.3:lib/modules/gash.sh` |
| A shell that loaded 2.x keeps running 2.x code after the checkout and reads some repository files at call time | audit list in section 3 |

Consequence: the 3.x tree must work right after a plain `git checkout`, with
nothing but the files git puts there.

## 2. Binary distribution

* Release workflow on tags only (`.github/workflows/release.yml`, goreleaser):
  linux and darwin, amd64 and arm64, `CGO_ENABLED=0`, `-trimpath`,
  `-buildvcs=false`, the version taken from `VERSION`, no commit hash in the
  binary (reproducible builds). Assets: `gash_<version>_<os>_<arch>.tar.gz`
  (containing `gash` and `gash-hosts-helper`) and `checksums.txt`.
* The agent never creates tags or releases; tagging is the operator's.
* Integrity, default: the extracted binaries' SHA-256 must match
  `checksums.txt` of the same release, fetched over HTTPS from github.com
  (the same trust as the `git fetch` that brought the tree).
* Integrity, stronger (decision requested, D-3): the binaries' SHA-256 are
  committed in the tagged tree (`release/SHA256SUMS`, produced by a
  reproducible build before tagging and re-checked by the release workflow).
  The bootstrap then verifies against the tree, so a signed tag covers the
  binary too. With `GASH_REQUIRE_SIGNED=1` the bootstrap refuses a binary
  that the tree does not pin.
* Test and developer override (environment only, new): `GASH_RELEASE_URL`
  (base URL, `file://` accepted) and `GASH_BIN` (a binary to use as is).

## 3. Scenarios and required outcomes

### 3.1 A 2.x interactive user runs `gash_upgrade` onto 3.x

1. 2.x code checks out `v3.0.x`. The open shell keeps its 2.x functions.
   Runtime reads of an open 2.x shell after the checkout: the 3.x tree keeps
   every file that 2.x reads at call time either present with compatible
   content or absent where 2.x already fails soft (`[[ -r ]] || return 0`);
   the per-file list is in section 7.
2. The next shell (new terminal, or `source ~/.gashrc` in the same one) loads
   the 3.x `gash.sh`. The binary for `VERSION` is missing, so the bootstrap
   downloads and verifies it, prints one line (`Gash 3.0.0: installed the
   gash binary for linux/amd64.`) and continues: same functions, aliases,
   prompt, history policy, toolchains, completion and banner as 2.x.
3. `source ~/.gashrc` in the same shell runs 3.x over the loaded 2.x state:
   supported (`compat.md` section 5).

### 3.2 A 2.x headless caller after the upgrade

`GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; llm_tree'`: sourcing prints
nothing and returns 0; the first call fetches the binary silently (lock-
serialized), then runs. Never a non-zero return while sourcing.

### 3.3 The binary cannot be obtained

No network, no `curl`/`wget`, a 404, a checksum mismatch: interactive shells
get one stderr line (`Gash 3.0.0: the gash binary is not installed and could
not be downloaded (reason). Run: bash ~/.gash/install.sh --repair`) and keep
working without Gash features; headless callers get the `binary_not_found`
envelope (STOP) from each function call. The shell is never broken and
sourcing never returns non-zero. A failed download leaves nothing behind
(temporary file in the versions directory, removed).

### 3.4 Rollback with `git checkout v2.1.0`

* The install directory holds only tracked files (the binary and every 3.x
  state live under XDG directories), so `git status --porcelain` stays clean
  and 2.x `gash_upgrade` keeps working after the rollback.
* 3.x never rewrites `~/.gashrc`, `~/.gash_env`, `~/.bashrc` or other profiles
  and never migrates configuration in a way 2.x cannot read: rollback needs no
  undo step.
* Proven by a test that installs 2.1.0 from a fixture clone, upgrades to the
  branch head (tagged locally in the fixture as `v3.0.0`), runs headless and
  interactive checks, checks out `v2.1.0`, and runs the 2.x checks again,
  `git status --porcelain` empty at every step.

### 3.5 A 1.x install landing on 3.x

1.x picks the newest tag by creation date, whatever its name, so it can land
on any 3.x tag, release candidates included (operator note: a tag created
after the last 2.x tag reaches every 1.x install that runs its upgrade). The
audit of the 1.x loaders (section 7.2) shows that every 1.x `~/.gashrc`
sources `$HOME/.gash/gash.sh` and `$HOME/.gash/bash_completion`, which stay
at the root, so a 1.x install landing on 3.x gets the 3.x shell with no step:

* interactive with a terminal: the 3.x shell, then the 2.x migration review
  (`gash_migrate --auto`) starts by itself, as it does on 2.x;
* interactive without a terminal: the shell, plus the 2.x once-a-day stderr
  notice pointing to `gash migrate`;
* headless: the functions, nothing printed (2.x prints the notice there too;
  3.x cannot, MUST NOT 4: forced divergence, LEDGER).

`gash_migrate` is ported (MATRIX), so the pointer leads somewhere; the
alternative (drop it and keep only the pointer) is decision D-6 in
`README.md`. Proven by tests on fixture checkouts of `v1.0.6`, `v1.4.9` and
`v1.5.3` with their own `~/.gashrc` variants, moved onto the branch head by
their own 1.x `gash_upgrade`.

### 3.6 `gash update` (3.x)

1. Refuse a dirty tree (as 2.x).
2. `git fetch --tags`, pick the highest release tag (same regex as 2.x, semver
   comparison, never lower than the current version: fixes the 2.x
   equality-only downgrade, LEDGER).
3. Verify the tag signature (`GASH_REQUIRE_SIGNED` as 2.x).
4. Download and verify the target binary into `versions/<target>/` before
   touching the tree.
5. Check that the new binary runs (`gash version --json` answers the target
   version).
6. `git checkout <tag>`; then a smoke test in a child shell: headless source
   prints nothing, returns 0, `$GASH_VERSION` is the target, one function
   answers; interactive `bash -i -c` source returns 0.
7. Any failure in 6: `git checkout <previous>` and report; the previous
   binary is still installed. Two versions are kept; `gash update --rollback`
   returns to the previous one.

`gash_upgrade` keeps its name, its output lines and its exit codes, and runs
this flow (MATRIX: redesign). In rc.A it keeps the 2.x tag choice and the
equality-only comparison (pure port; the downgrade is a LEDGER defect fixed in
3.0): on an rc install, `gash_upgrade` therefore moves back to the latest
final tag (`v2.1.0`), which is also the rollback an rc tester may want.

### 3.7 Fresh install

`install.sh` keeps its flags and environment variables (`--assume-yes`,
`--quiet`, `GASH_DIR`, `GASH_INSTALL_GIT_REPO`, `GASH_INSTALL_REF`, `PROFILE`,
`GASH_REQUIRE_SIGNED`), then fetches and verifies the binary, and on a TTY
offers to link `~/.gash/bin/gash` into `~/.local/bin`. New: `--repair`
(fetch the binary for the checked-out `VERSION`, nothing else).

## 4. Bootstrap fetch (Bash, in `lib/compat/bootstrap.sh`)

* `curl -fsSL --max-time 120` or `wget -q -T 120`; neither → failure reason
  "no curl or wget".
* Download into `versions/<v>/.tmp.<pid>/`, extract with `tar`, verify with
  `sha256sum` or `shasum -a 256` (macOS), `chmod 0755`, rename into place,
  remove the temporary directory. Lock: `mkdir versions/<v>/.lock`, stale after
  10 minutes.
* errexit-safe, nothing printed in headless mode, all diagnostics to stderr.
* Never runs at source time in headless mode.

## 5. What 3.x never does to an install

* Leave an untracked, non-ignored file in the install directory.
* Require any step after `git checkout` beyond what 2.x `gash_upgrade` does.
* Rewrite `~/.gashrc`, `~/.gash_env` or shell profiles without consent.
* Make `git checkout v2.1.0` lose anything.

## 6. Tests

testscript scripts (`testdata/script/upgrade_*.txtar`) with fixture
repositories built in the script (a bare clone of this repository with local
tags), a `file://` release directory holding the test build, and a temporary
`HOME`:

| Script | Proves |
|---|---|
| `upgrade_2x_headless` | 2.1.0 install, 2.x `gash_upgrade`, headless source silent and status 0, first call fetches and answers, `GASH_VERSION` |
| `upgrade_2x_interactive` | same, interactive `bash -i`: functions, aliases, prompt hook, completion, banner |
| `upgrade_2x_resource` | `source ~/.gashrc` in the shell that ran 2.x, then `gash_unload` restores the pre-2.x state |
| `upgrade_no_binary` | release URL unreachable: one stderr line interactive, envelope headless, status 0 while sourcing, nothing left behind |
| `upgrade_rollback` | checkout `v2.1.0`, porcelain empty, 2.x loads and upgrades again |
| `upgrade_1x_landing` | 1.0.6, 1.4.9 and 1.5.3 fixture installs moved onto 3.x by their own 1.x `gash_upgrade`: interactive shell works, headless prints nothing |
| `update_verify_rollback` | `gash update` with a broken target build rolls back |

## 7. Audit results (S1)

### 7.1 Files an open 2.x shell reads at call time

Found by strace on calls and a grep of every `GASH_DIR` use in `lib/`, then
checked by checking out a fake 3.x layout under a loaded 2.x shell. Help
data, completion data and prompt code are in memory and never re-read.

| 2.x reads | Used by | In the 3.x tree | Effect on an open 2.x shell after the checkout |
|---|---|---|---|
| `quotes/list.txt` | `gash_inspiring_quote` | kept, same format | none |
| `.gash_env.template` | `gash_env_init` | kept | none |
| `.gashrc` | `gash_migrate` (new `~/.gashrc`) | kept, unchanged | none |
| `gash.sh`, `lib/core/` (layout check before `rm -rf`) | `gash_uninstall` | both kept | none |
| `.git/` | `gash_upgrade` | unchanged | none: 2.x can still upgrade or roll back |
| `gash.sh`, `bash_completion` in a child shell | `gash_doctor --timing` | generated 3.x files | times the 3.x load |
| `lib/aliases/*.sh` (parsed with awk) | `gash`, `gash TOPIC` (alias tables and counts) | removed at cutover | the card shows no aliases, no error |
| the 12 core files and 13 module files (existence) | `gash_doctor` | 4 core files remain | doctor lists the missing files as warnings, status 0 |
| `VERSION` | load only | kept | `GASH_VERSION` stays `2.1.0` until a new terminal |

The two soft degradations (card alias tables, doctor warnings) last only
until the next shell, which `gash_upgrade` already tells the user to open;
keeping `lib/aliases/` and the 2.x modules for them would keep 2.x code that
no 3.x code reads, so they are accepted.

### 7.2 The 1.x side

| 1.x item | Content | 3.x consequence |
|---|---|---|
| `~/.gashrc`, v1.0.6 to v1.4.8 | `export GASH_BASH=$HOME/.gash/gash.sh`, `export GASH_COMPLETION=$HOME/.gash/bash_completion`, `source $GASH_BASH`, `source $GASH_COMPLETION` (unconditional) | both files exist at the root: loads |
| `~/.gashrc`, v1.4.9 to v1.5.0 | quoted, `[ -f "$GASH_COMPLETION" ] && source ...` | loads |
| `~/.gashrc`, v1.5.3 | if-block with a `GASH_HEADLESS` check | loads; headless prints nothing |
| `~/.gashrc` in general | paths hard-coded to `~/.gash`; no trailing newline | `GASH_DIR` comes from the path of `gash.sh`, as 2.x; 3.x never rewrites `~/.gashrc` (the migration does it only with consent, as 2.x) |
| Profile block | v1.0.6 to v1.2.0: 5 lines (`# Load Gash Bash` ...); v1.3.0 on: the marker block | sources `~/.gashrc`; uninstall removes either form, as 2.x |
| 1.x `gash_upgrade` | v1.2.0 to v1.5.3: newest tag by `creatordate`, any name; `checkout` + `reset --hard` + `reflog expire` + `gc`; dirty check only from v1.5.1; no re-source | lands on any 3.x tag, including an rc; the binary is fetched by the next shell (3.1) |
| 1.x load (v1.5.3) | `HISTCONTROL=ignoreboth:erasedups`, unlimited history, history synced every prompt, `PROMPT_COMMAND` replaced, `umask 022` | the 2.x behavior applies (the migration does not mention the dropped history sync: LEDGER) |
| 1.x quote reader | any random line | can print a `#` comment line of the 2.x format in a still-open 1.x shell (LEDGER, low) |

Detection in 3.x stays the 2.x rule: `~/.gashrc` contains `GASH_BASH` and
`${XDG_STATE_HOME:-~/.local/state}/gash/schema` is missing or lower than the
current schema.
