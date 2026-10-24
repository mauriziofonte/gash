<!-- Gash 3.0 demo tapes. Author: Maurizio Fonte <fonte.maurizio@gmail.com> -->

# Demo tapes

The GIFs in `docs/assets/` are recorded from these [vhs](https://github.com/charmbracelet/vhs)
tapes. The [Demos workflow](../../.github/workflows/demos.yml) records them
again on `main` whenever a tape, the workflow or `VERSION` changes, and
commits the GIFs that changed. On a pull request it records them and uploads
them as the `demo-gifs` artifact.

| Tape | GIF | Shows |
|---|---|---|
| `demo.tape` | `demo.gif` | The `gash` card, `files_largest`, `tree_stats`, `sysinfo overview` |
| `files.tape` | `files.gif` | `dirs_largest`, `dirs_list_empty`, `archive_extract` refusing an unsafe tar, `file_backup` |
| `git.tape` | `git.gif` | `gl`, `git_sync` and `git_add_tag` with their plan and question, `git_list_tags` |
| `system.tape` | `system.gif` | `process_find`, `port_kill`, `hgrep` |
| `help.tape` | `help.gif` | `gash git`, `gash_help --search`, `gash_help` on an alias |
| `agents.tape` | `agents.gif` | Headless `llm_*` calls piped to `jq`, and the refusals |
| `hte.tape` | `hte.gif` | `gash hte install`, `site create`, `curl`, `site list`, `service status` |

## How a tape runs

Every tape starts with a hidden `source docs/tapes/env.sh` (`--headless` for
`agents.tape`). It needs `GASH_BIN`, a gash binary built from the checkout;
`render.sh` builds one, stamped with `VERSION`, together with
`gash-hosts-helper`. `env.sh` then:

1. builds the demo home with `sandbox.sh` in `/tmp/gash-demo`: a small
   Laravel-style project with a git history, an `origin`, a dirty file and
   an SQLite database, a `Downloads` folder, a shell history and two
   archives (one with a member that climbs out of its folder). Everything in
   it is made up, and the dates are fixed;
2. makes it `HOME`, with its own git configuration, `C.UTF-8` and
   `Europe/Rome`, so nothing of the recording machine's home shows;
3. loads this checkout's `gash.sh`, or for `--headless` links the checkout
   at `~/.gash` and leaves the shell bare, as an agent finds it.

Commands wait for the prompt (`Wait`), so a slow machine records the same
frames; the `Sleep` after each one is the time a reader gets to look.

## Recording locally

```bash
docs/tapes/render.sh                # every tape but hte.tape
docs/tapes/render.sh files git      # only these
```

It needs Go, vhs (with ttyd and ffmpeg), git, jq and sqlite3. Running as
root, set `VHS_NO_SANDBOX=true` for vhs's Chromium.

`hte.tape` runs `gash hte install`, which changes the machine: a sudoers
drop-in, the hosts helper, `net.ipv4.ip_unprivileged_port_start`, a block in
`/etc/hosts`. Its hidden tail removes the site and uninstalls again, but
record it only on a disposable machine with Apache, PHP-FPM, free ports 80
and 443 and passwordless sudo: `GASH_DEMO_HTE=1 docs/tapes/render.sh hte`.
