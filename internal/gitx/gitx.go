// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package gitx holds the git module functions of 2.x (lib/modules/git.sh):
// git_list_tags, git_add_tag, git_delete_tag, git_dump_revisions,
// git_apply_patch, git_repair and git_sync.
//
// Every git call is the git CLI started through internal/runner with the
// caller's exported environment (c.Env.Child()) and no hardening flags,
// so the user's configuration, hooks, signing and credential helpers
// apply as in 2.x (LEDGER GIT-10: they are the user's own repository
// operations). 3.0 asks before a tag push or deletion, a sync and a
// patch, with -y/--yes for headless use (GIT-01), and fixes the 2.x
// defects GIT-02 to GIT-08.
package gitx

import (
	"context"
	"io"
	"os"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
)

// Module is the git handler set with its process seam.
type Module struct {
	// Run starts every process (git, and rm, cp, mkdir in git_repair).
	Run runner.Runner
}

// New returns the module wired to the real system.
func New() *Module { return &Module{Run: runner.Exec{}} }

// Register binds the git functions to t.
func Register(t *invoke.Table) { New().Register(t) }

// Register binds the functions of m to t.
func (m *Module) Register(t *invoke.Table) {
	t.Handle("git_list_tags", m.listTags)
	t.Handle("git_add_tag", m.addTag)
	t.Handle("git_delete_tag", m.deleteTag)
	t.Handle("git_dump_revisions", m.dumpRevisions)
	t.Handle("git_apply_patch", m.applyPatch)
	t.Handle("git_repair", m.repair)
	t.Handle("git_sync", m.sync)
	m.registerLog(t)
}

// call is one invocation with its printer and the process helpers.
type call struct {
	*invoke.Call
	m *Module
	s ui.Style
	p ui.Printer
}

func (m *Module) begin(c *invoke.Call) *call {
	s := ui.StyleOf(c.Term, false)
	return &call{Call: c, m: m, s: s, p: s.Printer(c.Stderr)}
}

func (x *call) ctx() context.Context {
	if x.Ctx != nil {
		return x.Ctx
	}
	return context.Background()
}

// cmd runs NAME ARGS... as the 2.x shell did: the caller's exported
// environment, the caller's stdin, stdout and stderr to the given writers
// (nil captures into the result, io.Discard is >/dev/null). A program that
// cannot start gets Bash's status (127 or 126); Bash's own "command not
// found" line, which names a 2.x script line, is not printed (LEDGER
// DOCK-19).
func (x *call) cmd(stdout, stderr io.Writer, name string, args ...string) runner.Result {
	res, _ := x.m.Run.Run(x.ctx(), runner.Cmd{Path: name, Args: args, Options: runner.Options{
		Env: x.Env.Child(), Stdin: x.Stdin, Stdout: stdout, Stderr: stderr,
	}})
	return res
}

// git runs git with stdout and stderr to the caller's streams.
func (x *call) git(args ...string) runner.Result {
	return x.cmd(x.Stdout, x.Stderr, "git", args...)
}

// gitQuiet is git ARGS >/dev/null 2>&1.
func (x *call) gitQuiet(args ...string) bool {
	return x.cmd(io.Discard, io.Discard, "git", args...).Success()
}

// gitOut is $(git ARGS) with stderr to the caller's stderr: the output
// without its trailing newlines, and whether git succeeded.
func (x *call) gitOut(args ...string) (string, bool) {
	res := x.cmd(nil, x.Stderr, "git", args...)
	return runner.CommandSubst(res.Stdout), res.Success()
}

// gitOutQuiet is $(git ARGS 2>/dev/null).
func (x *call) gitOutQuiet(args ...string) (string, bool) {
	res := x.cmd(nil, io.Discard, "git", args...)
	return runner.CommandSubst(res.Stdout), res.Success()
}

// requireArg is __gash_require_arg VALUE NAME USAGE.
func (x *call) requireArg(value, name, usage string) bool {
	if value != "" {
		return true
	}
	if usage != "" {
		x.p.Error("Missing " + name + ". Usage: " + usage)
	} else {
		x.p.Error("Missing required " + name + ".")
	}
	return false
}

// requireRepo is __gash_require_git_repo.
func (x *call) requireRepo() bool {
	if x.gitQuiet("rev-parse", "--is-inside-work-tree") {
		return true
	}
	x.p.Error("Not in a git repository.")
	return false
}

// requireRepoWithRemote is __gash_require_git_repo_with_remote (origin).
func (x *call) requireRepoWithRemote() bool {
	if !x.requireRepo() {
		return false
	}
	if x.gitQuiet("remote", "get-url", "origin") {
		return true
	}
	x.p.Error("Remote 'origin' is not configured.")
	return false
}

// isFile is [[ -f PATH ]]: a regular file after symlinks.
func isFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular()
}

// exists is [[ -e PATH ]].
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
