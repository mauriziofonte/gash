// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/runner"
)

var headless = mode{env: []string{"GASH_HEADLESS=1"}}

// LEDGER GIT-01 (rc.A reproduced): tag push, tag deletion and git_sync
// act without a question, headless too, and no --yes exists.
func TestLedger_GIT01_2x(t *testing.T) {
	r := newScene(t, setupOrigin).same(headless, "w", "git_add_tag", "v1", "m")
	require.Equal(t, 0, r.RC)
	require.Contains(t, r.Err, "OK: Tag 'v1' pushed to remote.")
	require.Contains(t, r.State, "== ./origin.git refs/heads/main\nrefs/heads/main 98da8d01e82dd1bcd9dd3693a41ba40f1733aa7e\nrefs/tags/v1 ")

	r = newScene(t, setupTags).same(headless, "w", "git_delete_tag", "v2", "--yes")
	require.Equal(t, 0, r.RC)
	require.Contains(t, r.Err, "OK: Remote tag 'v2' deleted.")
	require.NotContains(t, r.State, "refs/tags/v2")

	r = newScene(t, setupOrigin+" && echo s >.env.local").same(headless, "w", "git_sync")
	require.Equal(t, 0, r.RC)
	require.Contains(t, r.Err, "OK: Pushed main to origin/main.")

	r = newScene(t, setupOrigin+" && echo s >.env.local").same(headless, "w", "git_sync", "--yes")
	require.Equal(t, 1, r.RC)
	require.Equal(t, "Error: Unknown option: --yes (usage: git_sync [-m MESSAGE] [--no-push] [--dry-run])\n", r.Err)

	r = newScene(t, setupPatch).same(headless, "w", "git_apply_patch", "main", "feature", "base")
	require.Equal(t, 0, r.RC)
}

// LEDGER GIT-02 (rc.A reproduced): an annotated tag is listed twice under
// "Remote tags:" and an unreachable origin still returns 0.
func TestLedger_GIT02_2x(t *testing.T) {
	r := newScene(t, setupTags).same(plain, "w", "git_list_tags")
	require.Equal(t, "Local tags:\nv1\nv2\nRemote tags:\nv0.9\nv0.9^{}\nv2\nv2^{}\n", r.Out)

	r = newScene(t, setupTags+" && git remote set-url origin ../none.git").same(plain, "w", "git_list_tags")
	require.Equal(t, 0, r.RC)
	require.Equal(t, "Local tags:\nv1\nv2\nRemote tags:\n", r.Out)
	require.Contains(t, r.Err, "fatal: '../none.git' does not appear to be a git repository")
}

// LEDGER GIT-03 (rc.A reproduced): an unreachable origin reads as "tag
// absent".
func TestLedger_GIT03_2x(t *testing.T) {
	unreachable := setupRepo + " && git remote add origin ../none.git && git tag v9"
	r := newScene(t, unreachable).same(plain, "w", "git_delete_tag", "v9")
	require.Equal(t, 0, r.RC)
	require.Equal(t, "OK: Local tag 'v9' deleted.\n", r.Err)

	r = newScene(t, unreachable).same(plain, "w", "git_add_tag", "v8")
	require.Equal(t, 1, r.RC)
	require.True(t, strings.HasPrefix(r.Err, "OK: Tag 'v8' created.\n"), r.Err)
	require.True(t, strings.HasSuffix(r.Err, "Error: Tag 'v8' was created locally but could not be pushed.\n"), r.Err)
	require.Contains(t, r.State, "refs/tags/v8 ")
}

// LEDGER GIT-04 (rc.A reproduced): option-like tag names reach git tag in
// option position.
func TestLedger_GIT04_2x(t *testing.T) {
	for _, name := range []string{"-s", "-f", "--list"} {
		r := newScene(t, setupOrigin).same(plain, "w", "git_add_tag", name)
		require.Equal(t, 1, r.RC, name)
		require.True(t, strings.HasSuffix(r.Err, "Error: Could not create tag '"+name+"'.\n"), r.Err)
		require.NotContains(t, r.State, "refs/tags/")
	}
	// The argv, recorded: `git tag -a -s -m "Release -s"`.
	f := runner.NewFake(t,
		runner.Expect{Argv: []string{"git", "rev-parse", "--is-inside-work-tree"}},
		runner.Expect{Argv: []string{"git", "remote", "get-url", "origin"}},
		runner.Expect{Argv: []string{"git", "check-ref-format", "refs/tags/-s"}},
		runner.Expect{Argv: []string{"git", "rev-parse", "-q", "--verify", "refs/tags/-s"}, Status: 1},
		runner.Expect{Argv: []string{"git", "ls-remote", "--exit-code", "--tags", "origin", "refs/tags/-s"}, Status: 2},
		runner.Expect{Argv: []string{"git", "tag", "-a", "-s", "-m", "Release -s"}, Status: 129},
	)
	out, errOut, rc := runFake(t, f, "git_add_tag", "-s")
	require.Equal(t, 1, rc)
	require.Empty(t, out)
	require.Equal(t, "Error: Could not create tag '-s'.\n", errOut)
	require.NoError(t, f.Verify())
}

// LEDGER GIT-05 (rc.A reproduced): FILE is not preceded by --, so a file
// named like a branch is ambiguous, and an absolute path gives empty
// files; the status is 0 either way.
func TestLedger_GIT05_2x(t *testing.T) {
	r := newScene(t, setupRepo+" && echo m >main && git add main && git commit -qm m").
		same(plain, "w", "git_dump_revisions", "main")
	require.Equal(t, 0, r.RC)
	require.Contains(t, r.Err, "fatal: ambiguous argument 'main': both revision and filename")
	require.NotContains(t, r.State, "main.001.")

	s := newScene(t, setupHistory)
	abs := filepath.Join(s.dir, "w/sub/f.txt")
	r = s.same(plain, "w", "git_dump_revisions", abs)
	require.Equal(t, 0, r.RC)
	require.Contains(t, r.State, "da39a3ee5e6b ./w/sub/f.txt.001.6dbc135\n") // empty
}

// LEDGER GIT-06 (rc.A reproduced): a FEATURE with a slash breaks the
// patch path, and the failure leaves FEATURE checked out; the Bash
// redirection diagnostic is not printed (DOCK-19).
func TestLedger_GIT06_2x(t *testing.T) {
	setup := setupOrigin + " && git branch feat/x && git tag base && git checkout -q -b other"
	s := newScene(t, setup)
	want := s.run2x(plain, "w", "git_apply_patch", "main", "feat/x", "base")
	got := s.runGo(plain, "w", "git_apply_patch", nil, "main", "feat/x", "base")
	require.Equal(t, want.RC, got.RC)
	require.Equal(t, 1, got.RC)
	require.Equal(t, want.Out, got.Out)
	require.Equal(t, want.State, got.State)
	require.Contains(t, got.State, "== ./w/.git refs/heads/feat/x\n")
	require.Contains(t, want.Err, "lib/modules/git.sh: line ")
	require.Equal(t, dropLines(want.Err, "feat/x.patch: No such file or directory"), got.Err)
	require.True(t, strings.HasSuffix(got.Err, "Error: Failed to create patch file.\n"), got.Err)
}

// LEDGER GIT-07 (rc.A reproduced): --dry-run promises a push to the
// upstream that a plain git push then refuses, after the commit.
func TestLedger_GIT07_2x(t *testing.T) {
	setup := setupOrigin + " && git checkout -q -b work --track origin/main && echo b >b"
	r := newScene(t, setup).same(plain, "w", "git_sync", "--dry-run")
	require.Contains(t, r.Out, "Would push work to origin/main.\n")

	r = newScene(t, setup).same(plain, "w", "git_sync", "-m", "x")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Err, "OK: Committed ")
	require.Contains(t, r.Err, "fatal: The upstream branch of your current branch does not match")
}

// LEDGER GIT-08 (rc.A reproduced): the origin URL reaches git clone
// without --, and the empty files are deleted before the clone.
func TestLedger_GIT08_2x(t *testing.T) {
	setup := setupOrigin + " && " + corrupt + ` && git config remote.origin.url "--upload-pack=git-upload-pack"`
	r := newScene(t, setup).same(plain, "w", "git_repair", "-y")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Err, "Cloning '--upload-pack=git-upload-pack' into a temporary bare repo...")
	require.Contains(t, r.Err, "/recovery.git' does not exist")
	require.NotContains(t, r.State, "/objects/", "the empty objects are gone")

	// An empty file that is not a loose object is deleted too.
	setup = setupOrigin + " && mkdir -p .git/objects/info && : >.git/objects/info/x.keep && " + corrupt
	r = newScene(t, setup).same(plain, "w", "git_repair", "-y")
	require.Contains(t, r.Err, "[1/5] Removing 3 corrupted (0-byte) loose object(s)...")
}

// LEDGER GIT-10 (kept): the git module functions run the user's hooks.
func TestLedger_GIT10(t *testing.T) {
	setup := setupRepo + ` && printf '#!/bin/sh\n: >"$HOME/../hook-ran"\n' >.git/hooks/pre-commit &&
chmod +x .git/hooks/pre-commit && echo x >x`
	s := newScene(t, setup)
	r := s.same(plain, "w", "git_sync", "--no-push")
	require.Equal(t, 0, r.RC)
	_, err := os.Stat(filepath.Join(s.base, "hook-ran"))
	require.NoError(t, err)
}

// dropLines removes the lines of s that contain sub.
func dropLines(s, sub string) string {
	var b strings.Builder
	for _, l := range strings.SplitAfter(s, "\n") {
		if !strings.Contains(l, sub) {
			b.WriteString(l)
		}
	}
	return b.String()
}
