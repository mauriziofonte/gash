// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// gitEnv is the environment for a setup git call: a private HOME, the
// test's PATH and a fixed, deterministic identity and date so the fixtures
// are reproducible.
func gitEnv(home string) []string {
	return []string{
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
		"LC_ALL=C",
		"GIT_CONFIG_GLOBAL=" + filepath.Join(home, "gitconfig-absent"),
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=Port Tester",
		"GIT_AUTHOR_EMAIL=port@example.test",
		"GIT_COMMITTER_NAME=Port Tester",
		"GIT_COMMITTER_EMAIL=port@example.test",
		"GIT_AUTHOR_DATE=2026-01-02T03:04:05 +0000",
		"GIT_COMMITTER_DATE=2026-01-02T03:04:05 +0000",
	}
}

func git(t *testing.T, dir, home string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv(home)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

// requireGitJQ skips when git or jq is not installed (the 2.x reference and
// the Go port both need them for these functions).
func requireGitJQ(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"git", "jq"} {
		if _, err := lookPathTest(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}
}

// mixedRepo builds one repository that exercises staged, modified,
// staged+modified, untracked, a staged rename and hostile/quoted file names.
func mixedRepo(t *testing.T) string {
	t.Helper()
	home := physTemp(t)
	d := physTemp(t)
	git(t, d, home, "init", "-b", "main")
	mustWrite(t, filepath.Join(d, "a.txt"), "base\n")
	mustWrite(t, filepath.Join(d, "file with space.txt"), "x\n")
	mustWrite(t, filepath.Join(d, "uni-é.txt"), "x\n")
	mustWrite(t, filepath.Join(d, "to-rename.txt"), "renameme\n")
	git(t, d, home, "add", "-A")
	git(t, d, home, "commit", "-m", "base")

	// modified (worktree only)
	mustWrite(t, filepath.Join(d, "a.txt"), "changed\n")
	// staged new file
	mustWrite(t, filepath.Join(d, "staged.txt"), "new\n")
	git(t, d, home, "add", "staged.txt")
	// staged + modified: stage, then edit again
	mustWrite(t, filepath.Join(d, "both.txt"), "one\n")
	git(t, d, home, "add", "both.txt")
	mustWrite(t, filepath.Join(d, "both.txt"), "two\n")
	// staged rename
	git(t, d, home, "mv", "to-rename.txt", "renamed.txt")
	// untracked, including hostile names
	mustWrite(t, filepath.Join(d, "untracked.txt"), "u\n")
	mustWrite(t, filepath.Join(d, `weird"quote.txt`), "q\n")
	mustWrite(t, filepath.Join(d, "line\nbreak.txt"), "n\n")
	return d
}

// aheadBehindRepo builds a repository two commits ahead and one behind its
// upstream, so the branch.ab line is exercised.
func aheadBehindRepo(t *testing.T) string {
	t.Helper()
	home := physTemp(t)
	bare := physTemp(t)
	git(t, bare, home, "init", "--bare", "-b", "main", ".")
	a := physTemp(t)
	git(t, a, home, "init", "-b", "main")
	mustWrite(t, filepath.Join(a, "f.txt"), "1\n")
	git(t, a, home, "add", "-A")
	git(t, a, home, "commit", "-m", "c1")
	git(t, a, home, "remote", "add", "origin", bare)
	git(t, a, home, "push", "-u", "origin", "main")
	// two local commits ahead
	mustWrite(t, filepath.Join(a, "f.txt"), "2\n")
	git(t, a, home, "commit", "-am", "c2")
	mustWrite(t, filepath.Join(a, "f.txt"), "3\n")
	git(t, a, home, "commit", "-am", "c3")
	// a second clone pushes one commit, so origin/main is one ahead of A
	b := physTemp(t)
	git(t, b, home, "clone", bare, ".")
	mustWrite(t, filepath.Join(b, "g.txt"), "b\n")
	git(t, b, home, "add", "-A")
	git(t, b, home, "commit", "-m", "cb")
	git(t, b, home, "push", "origin", "main")
	git(t, a, home, "fetch")
	return a
}

func logRepo(t *testing.T) string {
	t.Helper()
	home := physTemp(t)
	d := physTemp(t)
	git(t, d, home, "init", "-b", "main")
	for _, s := range []string{"first commit", `subject with "quotes" and \ slash`, "third"} {
		mustWrite(t, filepath.Join(d, "f.txt"), s+"\n")
		git(t, d, home, "add", "-A")
		git(t, d, home, "commit", "-m", s)
	}
	return d
}

func TestGitStatusMatches2x(t *testing.T) {
	requireGitJQ(t)
	m := New()

	mixed := mixedRepo(t)
	ahead := aheadBehindRepo(t)

	// detached HEAD
	detached := physTemp(t)
	dh := physTemp(t)
	git(t, detached, dh, "init", "-b", "main")
	mustWrite(t, filepath.Join(detached, "f.txt"), "x\n")
	git(t, detached, dh, "add", "-A")
	git(t, detached, dh, "commit", "-m", "c1")
	git(t, detached, dh, "checkout", "--detach", "HEAD")

	// clean repo
	clean := physTemp(t)
	ch := physTemp(t)
	git(t, clean, ch, "init", "-b", "main")
	mustWrite(t, filepath.Join(clean, "f.txt"), "x\n")
	git(t, clean, ch, "add", "-A")
	git(t, clean, ch, "commit", "-m", "c1")

	// merge conflict
	conflict := physTemp(t)
	cfh := physTemp(t)
	git(t, conflict, cfh, "init", "-b", "main")
	mustWrite(t, filepath.Join(conflict, "c.txt"), "base\n")
	git(t, conflict, cfh, "add", "-A")
	git(t, conflict, cfh, "commit", "-m", "base")
	git(t, conflict, cfh, "checkout", "-b", "feature")
	mustWrite(t, filepath.Join(conflict, "c.txt"), "feature\n")
	git(t, conflict, cfh, "commit", "-am", "feature")
	git(t, conflict, cfh, "checkout", "main")
	mustWrite(t, filepath.Join(conflict, "c.txt"), "mainline\n")
	git(t, conflict, cfh, "commit", "-am", "mainline")
	// a conflicting merge leaves an unmerged entry; ignore its non-zero exit
	mc := exec.Command("git", "merge", "feature")
	mc.Dir, mc.Env = conflict, gitEnv(cfh)
	_ = mc.Run()

	notRepo := physTemp(t)

	for _, tc := range []struct {
		name string
		cwd  string
		args []string
	}{
		{"clean", clean, nil},
		{"mixed", mixed, nil},
		{"mixed-path-arg", physTemp(t), []string{mixed}},
		{"ahead-behind", ahead, nil},
		{"detached", detached, nil},
		{"conflict", conflict, nil},
		{"outside-repo", notRepo, nil},
		{"help", clean, []string{"--help"}},
		{"help-h", clean, []string{"-h"}},
		{"unknown-option", clean, []string{"--bogus"}},
		{"second-path", clean, []string{".", "."}},
		{"traversal", clean, []string{"../etc"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := physTemp(t)
			same(t, m, tc.cwd, baseEnv(home, nil), "llm_git_status", tc.args...)
		})
	}
}

func TestGitDiffMatches2x(t *testing.T) {
	requireGitJQ(t)
	m := New()
	mixed := mixedRepo(t)
	notRepo := physTemp(t)

	for _, tc := range []struct {
		name string
		cwd  string
		args []string
	}{
		{"worktree", mixed, nil},
		{"staged", mixed, []string{"--staged"}},
		{"cached", mixed, []string{"--cached"}},
		{"path-arg", physTemp(t), []string{mixed}},
		{"staged-and-path", physTemp(t), []string{"--staged", mixed}},
		{"outside-repo", notRepo, nil},
		{"help", mixed, []string{"--help"}},
		{"unknown-option", mixed, []string{"--bogus"}},
		{"second-path", mixed, []string{".", "."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := physTemp(t)
			same(t, m, tc.cwd, baseEnv(home, nil), "llm_git_diff", tc.args...)
		})
	}
}

func TestGitLogMatches2x(t *testing.T) {
	requireGitJQ(t)
	m := New()
	repo := logRepo(t)

	empty := physTemp(t)
	eh := physTemp(t)
	git(t, empty, eh, "init", "-b", "main")

	notRepo := physTemp(t)

	for _, tc := range []struct {
		name string
		cwd  string
		args []string
	}{
		{"default", repo, nil},
		{"limit-n", repo, []string{"-n", "2"}},
		{"limit-long", repo, []string{"--limit", "1"}},
		{"path-arg", physTemp(t), []string{repo}},
		{"empty-repo", empty, nil},
		{"outside-repo", notRepo, nil},
		{"help", repo, []string{"--help"}},
		{"unknown-option", repo, []string{"--bogus"}},
		{"missing-value", repo, []string{"-n"}},
		{"invalid-limit", repo, []string{"-n", "abc"}},
		{"second-path", repo, []string{".", "."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := physTemp(t)
			same(t, m, tc.cwd, baseEnv(home, nil), "llm_git_log", tc.args...)
		})
	}
}

// TestLedger_LLM13: commits whose subject or author holds 0x1e, 0x1f or
// another control character stay in the llm_git_log array (2.x split on
// 0x1e/0x1f and dropped them).
func TestLedger_LLM13(t *testing.T) {
	requireGitJQ(t)
	m := New()
	home := physTemp(t)
	d := physTemp(t)
	git(t, d, home, "init", "-b", "main")
	subjects := []string{"plain", "rs\x1ehere", "us\x1fhere", "soh\x01here"}
	for _, s := range subjects {
		git(t, d, home, "commit", "--allow-empty", "-m", s)
	}
	git(t, d, home, "commit", "--allow-empty", "--author", "Odd\x1fName <odd@example.test>", "-m", "odd author")
	r := runGo(t, m, d, baseEnv(home, nil), "llm_git_log")
	require.Equal(t, 0, r.RC, r.Stderr)
	var log []struct{ Hash, Subject, Author, Date string }
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &log), r.Stdout)
	require.Len(t, log, 5)
	require.Equal(t, "odd author", log[0].Subject)
	require.Equal(t, "Odd\x1fName", log[0].Author)
	for i, s := range subjects {
		require.Equal(t, s, log[4-i].Subject)
		require.Equal(t, "Port Tester", log[4-i].Author)
		require.NotEmpty(t, log[4-i].Hash)
		require.NotEmpty(t, log[4-i].Date)
	}
	r = runGo(t, m, d, baseEnv(home, nil), "llm_git_log", "-n", "2")
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &log), r.Stdout)
	require.Len(t, log, 2)
}
