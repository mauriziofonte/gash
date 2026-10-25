// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// gitTestEnv is a fixed identity and date with no global or system git
// configuration, for the setup commands and the calls alike.
func gitTestEnv(home string) map[string]string {
	return map[string]string{
		"HOME": home, "PATH": os.Getenv("PATH"), "LC_ALL": "C", "GASH_HEADLESS": "1",
		"GIT_CONFIG_GLOBAL": filepath.Join(home, "gitconfig-absent"), "GIT_CONFIG_NOSYSTEM": "1",
		"GIT_AUTHOR_NAME": "Port Tester", "GIT_AUTHOR_EMAIL": "port@example.test",
		"GIT_COMMITTER_NAME": "Port Tester", "GIT_COMMITTER_EMAIL": "port@example.test",
		"GIT_AUTHOR_DATE": "2026-01-02T03:04:05 +0000", "GIT_COMMITTER_DATE": "2026-01-02T03:04:05 +0000",
	}
}

func envList(m map[string]string) []string {
	var out []string
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

// sh runs git in dir for the test's setup and returns its output.
func sh(t *testing.T, dir, home string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = envList(gitTestEnv(home))
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return string(out)
}

// commitCall runs agent_git_commit in dir with the test identity.
func commitCall(t *testing.T, dir, home string, args ...string) result {
	t.Helper()
	m := New()
	m.getwd = func() (string, error) { return dir, nil }
	m.rootHome = func() string { return "/root" }
	tbl := invoke.NewTable()
	m.Register(tbl)
	var out, errb bytes.Buffer
	vars := gitTestEnv(home)
	vars["PWD"] = dir
	env := invoke.EnvMap(vars)
	t.Chdir(dir)
	rc := tbl.Run(&invoke.Call{Ctx: context.Background(), Name: "agent_git_commit", Args: args, Env: env,
		Stdout: &out, Stderr: &errb, Term: term.Fixed(env, false, false, false)})
	return result{out.String(), errb.String(), rc}
}

// commitRepo is a clone tracking a bare remote, one commit pushed, with
// a.txt modified, n.txt untracked and dir/x.txt untracked.
func commitRepo(t *testing.T) (repo, bare, home string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	home = physDir(t)
	bare = physDir(t)
	sh(t, bare, home, "init", "-q", "--bare", "-b", "main", ".")
	repo = physDir(t)
	sh(t, repo, home, "init", "-q", "-b", "main")
	write(t, filepath.Join(repo, "a.txt"), "1\n", 0o644)
	write(t, filepath.Join(repo, ".gitignore"), "ignored.log\n", 0o644)
	sh(t, repo, home, "add", "-A")
	sh(t, repo, home, "commit", "-q", "-m", "base")
	sh(t, repo, home, "remote", "add", "origin", bare)
	sh(t, repo, home, "push", "-q", "-u", "origin", "main")
	write(t, filepath.Join(repo, "a.txt"), "1\n2\n", 0o644)
	write(t, filepath.Join(repo, "n.txt"), "new\n", 0o644)
	write(t, filepath.Join(repo, "dir", "x.txt"), "x\n", 0o644)
	return repo, bare, home
}

func decode(t *testing.T, r result) map[string]any {
	t.Helper()
	require.Equal(t, 0, r.RC, r.Stderr)
	var v map[string]any
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &v), r.Stdout)
	return v
}

func TestGitCommitAddAndPush(t *testing.T) {
	repo, bare, home := commitRepo(t)

	// Commit one path, no push: ahead of the upstream by one.
	r := commitCall(t, repo, home, "-m", "feat: a", "-m", "Body one.", "-m", "Body two.", "--add", "a.txt", "--author", "Ann <ann@example.test>")
	v := decode(t, r)
	require.Equal(t, map[string]any{"hash": v["hash"], "branch": "main", "files": 1.0, "pushed": false, "remote": "", "ahead": 1.0}, v)
	require.LessOrEqual(t, len(r.Stdout), 150)
	require.Equal(t, "feat: a\n\nBody one.\n\nBody two.\n\n", sh(t, repo, home, "log", "-1", "--format=%B"))
	require.Equal(t, "Ann <ann@example.test>\n", sh(t, repo, home, "log", "-1", "--format=%an <%ae>"))
	require.Equal(t, v["hash"], strings.TrimSpace(sh(t, repo, home, "log", "-1", "--format=%h")))
	// n.txt and dir/ were not named: still untracked.
	require.Contains(t, sh(t, repo, home, "status", "--short"), "?? n.txt")

	// A directory and a file, pushed to the upstream: ahead 0, the remote
	// has the commit.
	r = commitCall(t, repo, home, "-m", "feat: more", "--add", "dir", "n.txt", "--push")
	v = decode(t, r)
	require.Equal(t, 2.0, v["files"])
	require.Equal(t, true, v["pushed"])
	require.Equal(t, "origin", v["remote"])
	require.Equal(t, 0.0, v["ahead"])
	require.Equal(t, "feat: more\n", sh(t, bare, home, "log", "-1", "--format=%s", "main"))
	require.Empty(t, sh(t, repo, home, "status", "--short"))

	// The word after --push names a directory and no PATH was given: it is
	// the PATH.
	write(t, filepath.Join(repo, "a.txt"), "3\n", 0o644)
	r = commitCall(t, filepath.Dir(repo), home, "-m", "fix: via path", "--add", "a.txt", "--push", filepath.Base(repo))
	v = decode(t, r)
	require.Equal(t, true, v["pushed"])
	require.Equal(t, "fix: via path\n", sh(t, bare, home, "log", "-1", "--format=%s", "main"))
}

func TestGitCommitPushToNamedRemote(t *testing.T) {
	repo, bare, home := commitRepo(t)
	sh(t, repo, home, "checkout", "-q", "-b", "topic")
	r := commitCall(t, repo, home, "-m", "topic work", "--add", "a.txt", "--push")
	require.Equal(t, "no_upstream/RETRY", code(t, r))
	require.Equal(t, "base\n", sh(t, repo, home, "log", "-1", "--format=%s"), "nothing committed")
	require.Contains(t, sh(t, repo, home, "status", "--short"), " M a.txt", "nothing staged")

	r = commitCall(t, repo, home, "-m", "topic work", "--add", "a.txt", "--push", "nosuch")
	require.Equal(t, "unknown_remote/RETRY", code(t, r))

	r = commitCall(t, repo, home, "-m", "topic work", "--add", "a.txt", "--push", "origin")
	v := decode(t, r)
	require.Equal(t, "topic", v["branch"])
	require.Equal(t, true, v["pushed"])
	require.Equal(t, "topic work\n", sh(t, bare, home, "log", "-1", "--format=%s", "topic"))
}

func TestGitCommitDryRun(t *testing.T) {
	repo, _, home := commitRepo(t)
	before := sh(t, repo, home, "status", "--porcelain")
	r := commitCall(t, repo, home, "-m", "x", "-A", "--push", "--dry-run")
	require.Equal(t, `{"branch":"main","files":3,"paths":["a.txt","dir/x.txt","n.txt"],"push":true,"remote":"origin","ref":"refs/heads/main","ahead":0,"dry_run":true,"written":false}`+"\n", r.Stdout)
	require.Equal(t, before, sh(t, repo, home, "status", "--porcelain"))
	require.Equal(t, "base\n", sh(t, repo, home, "log", "-1", "--format=%s"))

	r = commitCall(t, repo, home, "-m", "x", "-A", "--dry-run", "--max-lines", "1")
	require.Contains(t, r.Stdout, `"files":3,"paths":["a.txt"],"truncated":2,"push":false,"remote":"","ahead":1,"dry_run":true,"written":false}`)
}

func TestGitCommitRefusals(t *testing.T) {
	repo, _, home := commitRepo(t)
	write(t, filepath.Join(repo, "ignored.log"), "x\n", 0o644)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no-message", []string{"--add", "a.txt"}, "missing_message/RETRY"},
		{"empty-message", []string{"-m", " ", "--add", "a.txt"}, "missing_message/RETRY"},
		{"no-target", []string{"-m", "x"}, "missing_target/RETRY"},
		{"both-targets", []string{"-m", "x", "-A", "--add", "a.txt"}, "conflicting_options/RETRY"},
		{"bad-author", []string{"-m", "x", "--add", "a.txt", "--author", "nobody"}, "invalid_author/RETRY"},
		{"add-empty", []string{"-m", "x", "--add", "--push"}, "missing_value/RETRY"},
		{"root-target", []string{"-m", "x", "--add", "."}, "invalid_target/RETRY"},
		{"traversal", []string{"-m", "x", "--add", "../elsewhere"}, "path_traversal_blocked/FATAL"},
		{"outside", []string{"-m", "x", "--add", home}, "invalid_target/RETRY"},
		{"missing-file", []string{"-m", "x", "--add", "nope.txt"}, "file_not_found/RETRY"},
		{"ignored", []string{"-m", "x", "--add", "ignored.log"}, "git_add_failed/RETRY"},
		{"amend", []string{"-m", "x", "--add", "a.txt", "--amend"}, "unknown_option/RETRY"},
		{"no-verify", []string{"-m", "x", "--add", "a.txt", "--no-verify"}, "unknown_option/RETRY"},
		{"format", []string{"-m", "x", "--add", "a.txt", "--format", "tsv"}, "invalid_format/RETRY"},
		{"not-a-repo", []string{"-m", "x", "-A", home}, "not_a_git_repo/CONTINUE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := commitCall(t, repo, home, tc.args...)
			require.Equal(t, 1, r.RC, r.Stdout)
			require.Equal(t, tc.want, code(t, r))
			require.Equal(t, "base\n", sh(t, repo, home, "log", "-1", "--format=%s"))
			require.NotContains(t, sh(t, repo, home, "status", "--porcelain"), "A ")
		})
	}
}

func TestGitCommitSecretFiles(t *testing.T) {
	repo, _, home := commitRepo(t)
	write(t, filepath.Join(repo, "dir", ".env"), "TOKEN=x\n", 0o644)
	r := commitCall(t, repo, home, "-m", "x", "-A")
	require.Equal(t, "secret_file_blocked/FATAL", code(t, r))
	require.Contains(t, r.Stderr, "dir/.env")
	r = commitCall(t, repo, home, "-m", "x", "--add", "dir")
	require.Equal(t, "secret_file_blocked/FATAL", code(t, r))
	r = commitCall(t, repo, home, "-m", "x", "--add", "dir/.env")
	require.Equal(t, "secret_file_blocked/FATAL", code(t, r))
	require.Equal(t, "base\n", sh(t, repo, home, "log", "-1", "--format=%s"))
	require.Contains(t, sh(t, repo, home, "status", "--porcelain"), "?? dir/")

	// A secret staged beforehand would go into the commit too.
	sh(t, repo, home, "add", "dir/.env")
	r = commitCall(t, repo, home, "-m", "x", "--add", "a.txt")
	require.Equal(t, "secret_file_blocked/FATAL", code(t, r))
	// Its deletion carries no content and is allowed.
	sh(t, repo, home, "rm", "-q", "--cached", "dir/.env")
	require.NoError(t, os.Remove(filepath.Join(repo, "dir", ".env")))
	r = commitCall(t, repo, home, "-m", "ok", "--add", "a.txt")
	require.Equal(t, 0, r.RC, r.Stderr)
}

func TestGitCommitStateRefusals(t *testing.T) {
	repo, _, home := commitRepo(t)

	// Nothing to commit under the path.
	sh(t, repo, home, "add", "-A")
	sh(t, repo, home, "commit", "-q", "-m", "all")
	r := commitCall(t, repo, home, "-m", "x", "--add", "a.txt")
	require.Equal(t, "nothing_to_commit/RETRY", code(t, r))

	// Detached HEAD.
	sh(t, repo, home, "checkout", "-q", "--detach", "HEAD")
	write(t, filepath.Join(repo, "a.txt"), "d\n", 0o644)
	r = commitCall(t, repo, home, "-m", "x", "--add", "a.txt")
	require.Equal(t, "detached_head/STOP", code(t, r))
	sh(t, repo, home, "checkout", "-q", "-f", "main")

	// Unresolved conflicts.
	sh(t, repo, home, "checkout", "-q", "-b", "side")
	write(t, filepath.Join(repo, "a.txt"), "side\n", 0o644)
	sh(t, repo, home, "commit", "-q", "-am", "side")
	sh(t, repo, home, "checkout", "-q", "main")
	write(t, filepath.Join(repo, "a.txt"), "main\n", 0o644)
	sh(t, repo, home, "commit", "-q", "-am", "main")
	mc := exec.Command("git", "merge", "side")
	mc.Dir, mc.Env = repo, envList(gitTestEnv(home))
	_ = mc.Run()
	r = commitCall(t, repo, home, "-m", "x", "--add", "a.txt")
	require.Equal(t, "merge_conflict/STOP", code(t, r))
}

func TestGitCommitPushRejected(t *testing.T) {
	repo, bare, home := commitRepo(t)
	other := physDir(t)
	sh(t, other, home, "clone", "-q", bare, ".")
	write(t, filepath.Join(other, "o.txt"), "o\n", 0o644)
	sh(t, other, home, "add", "-A")
	sh(t, other, home, "commit", "-q", "-m", "other")
	sh(t, other, home, "push", "-q", "origin", "main")

	// The remote moved and the clone does not know: the commit is made, the
	// push is refused (never forced), the envelope names the commit.
	r := commitCall(t, repo, home, "-m", "mine", "--add", "a.txt", "--push")
	require.Equal(t, "push_rejected/RETRY", code(t, r))
	require.Empty(t, r.Stdout)
	var e map[string]any
	require.NoError(t, json.Unmarshal([]byte(r.Stderr), &e))
	require.Equal(t, strings.TrimSpace(sh(t, repo, home, "log", "-1", "--format=%h")), e["hash"])
	require.Equal(t, "main", e["branch"])
	require.Contains(t, e["details"], "rejected")
	require.Equal(t, "other\n", sh(t, bare, home, "log", "-1", "--format=%s", "main"))

	// Once fetched, the branch is behind: refused before the commit.
	sh(t, repo, home, "fetch", "-q")
	write(t, filepath.Join(repo, "n.txt"), "changed\n", 0o644)
	r = commitCall(t, repo, home, "-m", "again", "--add", "n.txt", "--push")
	require.Equal(t, "push_rejected/RETRY", code(t, r))
	require.Contains(t, r.Stderr, "behind origin/main")
	require.Equal(t, "mine\n", sh(t, repo, home, "log", "-1", "--format=%s"))
}

func TestGitCommitHookFailure(t *testing.T) {
	repo, _, home := commitRepo(t)
	write(t, filepath.Join(repo, ".git", "hooks", "pre-commit"), "#!/bin/sh\necho 'lint: 2 errors in a.txt' >&2\nexit 1\n", 0o755)
	r := commitCall(t, repo, home, "-m", "x", "--add", "a.txt")
	require.Equal(t, "commit_failed/RETRY", code(t, r))
	require.Contains(t, r.Stderr, "lint: 2 errors in a.txt")
	require.Equal(t, "base\n", sh(t, repo, home, "log", "-1", "--format=%s"))
}

func TestCommitPathsAndTail(t *testing.T) {
	require.Equal(t, "b | c", tailLines([]byte("a\nhint: x\nb\n\nc\n"), 9))
	require.Equal(t, "c", tailLines([]byte("a\nb\nc\n"), 6))
	require.Equal(t, "abc…", cutBytes("abcdef", 3))
}
