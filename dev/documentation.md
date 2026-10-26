# Writing the Gash documentation

This page is for whoever changes the documentation: how it is organized,
which parts are generated, how examples are verified, and the rules every
page follows. The documentation is part of the product. A change that
alters what a user sees updates the page that describes it, in the same
pull request.

## Layout

```text
README.md              what Gash is, install, support matrix, tour, links
AGENTS.md              the contract for AI agents (invocation, output, errors, rails)
SECURITY.md            how to report a vulnerability
CHANGELOG.md           release history (the only place for "what's new")
docs/
  README.md            index of the areas (the table is generated)
  install.md           install, verify, update, uninstall, supported systems
  requirements.md      every external tool and its package per system (generated)
  configuration.md     ~/.gash_env and every GASH_* setting
  security.md          the guardrails: what they do and what they do not
  troubleshooting.md   symptoms, causes, fixes
  <area>/README.md     the guide of an area (written by hand)
  <area>/reference.md  every function, alias and command of the area (generated)
  assets/              images used by the pages
dev/                   for people who develop Gash (this page, architecture,
                       testing, releasing, the demo tapes, the port archive)
```

The areas are listed in `internal/docgen/areas.go`, with the function of
the help registry each one documents and the `gash` commands it covers.

## Generated parts

Run `go test ./internal/docgen ./internal/prereq -update` after changing
help text, the prerequisites catalog, the areas or an example. The same
tests, without `-update`, fail in CI when a generated part is stale.

| What | Source | Generator |
|---|---|---|
| `docs/<area>/reference.md` | the help registry (`gash_help NAME`), the alias packs, `gash COMMAND --help` | `internal/docgen` TestReference |
| the `prerequisites` block of each guide | `internal/registry/needs.go` and the catalog | `internal/docgen` TestBlocks |
| the `areas` table of `docs/README.md` | `internal/docgen/areas.go` | `internal/docgen` TestBlocks |
| `docs/requirements.md` | `internal/prereq/catalog.go` | `internal/prereq` TestRequirementsDoc |
| the outputs of verified examples | running Gash | `internal/docgen` TestExamples |
| AGENTS.md routing table | `internal/registry` routes | `go test ./internal/registry -update-docs` |

A generated block in a hand-written page sits between
`<!-- BEGIN generated: NAME ... -->` and `<!-- END generated: NAME -->`.
Edit around it, never inside it.

## Verified examples

An example that shows output must show the output Gash prints. Mark it so
CI runs it:

````markdown
<!-- example
mkdir -p project/src
printf 'hello\n' >project/src/main.go
mask: Scan time: +[0-9]+ms
-->
```console
$ cd project
$ tree_stats .
Tree statistics for ~/project
  Files:       1
  Directories: 2
  Total size:  6 B (6 bytes)
  Avg file:    6 B
  Max depth:   1
  Empty dirs:  0
  Scan time:   0ms

  Top 5 extensions by count:
    go                1 files  6 B

  Top 5 extensions by size:
    go           6 B
```
````

- The comment's lines are a setup script, run first in an empty sandbox
  directory that is also `HOME`. Lines starting with `mask: ` are regular
  expressions instead: text they match (times, durations, dates) may differ
  between the page and a run.
- Each `$ ` line runs in one interactive Bash with Gash loaded, in that
  sandbox, so `cd` carries over. A line ending in `\` continues on the
  next. Everything up to the next `$ ` is the output (stdout and stderr).
  The sandbox path prints as `~`.
- Write the commands, run `go test ./internal/docgen -run TestExamples
  -update`, and read the output it wrote into the page: that is what the
  user will see. If it is not what the page should teach, change the
  example, not the output.
- Examples run on the Linux CI runners (Ubuntu, with git and the base
  tools). Only verify what is deterministic there: no network, no root, no
  Docker, no services, no tools the runner may lack. Show those as a
  plain ```` ```bash ```` block of commands, and say in prose what happens.

## Rules for every page

1. **Say what the page covers, where it works, and what it needs.** The
   first paragraph says what the area is for. A `Platforms:` line says
   where it works (Linux, macOS, WSL, and any limit). The prerequisites
   block is generated.
2. **Never write by hand a list that exists in the code**: functions,
   options, error codes, settings, prerequisites. Link to the generated
   reference (`reference.md#function_name`) or table instead. A guide
   explains tasks and choices; the reference lists everything.
3. **Show real output, or none.** Output appears only in verified
   examples. An unverifiable command is shown without output.
4. **Task first.** Sections are what the reader wants to do ("Find what
   fills a disk"), not function names. Each section: one or two sentences,
   the command, what it prints or changes, the options that matter, and
   the link to the reference for the rest.
5. **Say what fails and what to do.** When a function can refuse or fail
   in a way a user will meet (a missing tool, a guard, a permission), say
   what the message means and how to resolve it.
6. **Plain English.** Short sentences, present tense, active voice, no
   marketing ("blazing", "powerful", "seamless"), no version history
   ("since 3.2", "new in"): that belongs in CHANGELOG.md. Name things as
   the user types them. Prefer a table for comparisons and a numbered list
   for steps.
7. **Links are checked.** Every relative link and `#anchor` between
   Markdown files must resolve (`internal/docgen` TestLinks). GitHub
   anchors keep underscores: `reference.md#archive_extract`.
8. **Agents are users too, but AGENTS.md is their contract.** A guide may
   show an `llm_*` call for a person to try; the rules agents follow live
   in AGENTS.md only.
