// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// requireGit skips when git is not installed (the 3.4 modes need no jq).
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := lookPathTest("git"); err != nil {
		t.Skip("git is not installed")
	}
}

// errCode is the error and action of an envelope on stderr.
func errCode(t *testing.T, r result) string {
	t.Helper()
	var e struct{ Error, Action string }
	require.NoError(t, json.Unmarshal([]byte(r.Stderr), &e), r.Stderr)
	return e.Error + "/" + e.Action
}

// decodeOK decodes stdout into v (the call must succeed).
func decodeOK(t *testing.T, r result, v any) {
	t.Helper()
	require.Equal(t, 0, r.RC, r.Stderr)
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), v), r.Stdout)
}

// remoteRepo is a clone of a bare remote with three commits, one staged,
// one modified and one untracked path; the remote got one more commit
// from another clone that the clone has not fetched yet.
func remoteRepo(t *testing.T) (clone, bare, home string) {
	t.Helper()
	home = physTemp(t)
	bare = physTemp(t)
	git(t, bare, home, "init", "--bare", "-b", "main", ".")
	clone = physTemp(t)
	git(t, clone, home, "init", "-b", "main")
	for _, s := range []string{"one", "two", "three"} {
		mustWrite(t, filepath.Join(clone, "f.txt"), s+"\n")
		git(t, clone, home, "add", "-A")
		git(t, clone, home, "commit", "-m", "commit "+s)
	}
	git(t, clone, home, "remote", "add", "origin", bare)
	git(t, clone, home, "push", "-u", "origin", "main")
	other := physTemp(t)
	git(t, other, home, "clone", bare, ".")
	mustWrite(t, filepath.Join(other, "g.txt"), "g\n")
	git(t, other, home, "add", "-A")
	git(t, other, home, "commit", "-m", "remote side")
	git(t, other, home, "push", "origin", "main")
	mustWrite(t, filepath.Join(clone, "f.txt"), "three\nfour\nfive\n")
	mustWrite(t, filepath.Join(clone, "s.txt"), "staged\n")
	git(t, clone, home, "add", "s.txt")
	mustWrite(t, filepath.Join(clone, "u.txt"), "untracked\n")
	return clone, bare, home
}

func TestGitOverview(t *testing.T) {
	requireGit(t)
	m := New()
	clone, _, _ := remoteRepo(t)
	env := baseEnv(physTemp(t), nil)

	var ov overviewJSON
	r := runGo(t, m, clone, env, "llm_git_overview")
	decodeOK(t, r, &ov)
	require.Equal(t, "main", ov.Branch)
	require.Equal(t, "origin/main", ov.Upstream)
	require.Equal(t, 0, ov.Ahead)
	require.Equal(t, 0, ov.Behind)
	require.Equal(t, ovCounts{Staged: 1, Modified: 1, Untracked: 1}, ov.Counts)
	require.Equal(t, []ovLog{{ov.Log[0].Hash, "commit three"}, {ov.Log[1].Hash, "commit two"}, {ov.Log[2].Hash, "commit one"}}, ov.Log)
	require.Equal(t, ovDiffstat{Files: 2, Insertions: 3}, ov.Diffstat)
	require.Nil(t, ov.Files)
	require.NotContains(t, r.Stdout, `"files":{`)
	require.LessOrEqual(t, len(r.Stdout), 400)

	// --files N lists and caps; --log 0 drops the subjects.
	mustWrite(t, filepath.Join(clone, "u2.txt"), "x\n")
	ov = overviewJSON{}
	r = runGo(t, m, physTemp(t), env, "llm_git_overview", clone, "--files", "1", "--log", "0")
	decodeOK(t, r, &ov)
	require.Equal(t, []ovLog{}, ov.Log)
	require.Equal(t, &ovFiles{Staged: []string{"s.txt"}, Modified: []string{"f.txt"}, Untracked: []string{"u.txt"}, Conflicted: []string{}}, ov.Files)
	require.Equal(t, 1, ov.Truncated)

	// An unborn branch: no log, the staged file against the empty tree.
	empty := physTemp(t)
	git(t, empty, physTemp(t), "init", "-b", "main")
	mustWrite(t, filepath.Join(empty, "a.txt"), "1\n2\n")
	git(t, empty, physTemp(t), "add", "a.txt")
	ov = overviewJSON{}
	decodeOK(t, runGo(t, m, empty, env, "llm_git_overview"), &ov)
	require.Equal(t, "main", ov.Branch)
	require.Equal(t, []ovLog{}, ov.Log)
	require.Equal(t, ovDiffstat{Files: 1, Insertions: 2}, ov.Diffstat)

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"not-a-repo", []string{physTemp(t)}, "not_a_git_repo/CONTINUE"},
		{"bad-log", []string{"--log", "x"}, "invalid_log/RETRY"},
		{"bad-files", []string{"--files", "-1"}, "invalid_files/RETRY"},
		{"missing-value", []string{"--log"}, "missing_value/RETRY"},
		{"unknown", []string{"--bogus"}, "unknown_option/RETRY"},
		{"second-path", []string{".", "."}, "unexpected_argument/RETRY"},
		{"traversal", []string{"../x"}, "path_traversal_blocked/FATAL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runGo(t, m, clone, env, "llm_git_overview", tc.args...)
			require.Equal(t, 1, r.RC)
			require.Equal(t, tc.want, errCode(t, r))
		})
	}
}

func TestGitStatusModes(t *testing.T) {
	requireGit(t)
	m := New()
	clone, _, _ := remoteRepo(t)
	env := baseEnv(physTemp(t), nil)

	// The default stays the 2.x pretty object.
	r := runGo(t, m, clone, env, "llm_git_status")
	require.Equal(t, 0, r.RC)
	require.True(t, strings.HasPrefix(r.Stdout, "{\n  \"branch\": \"main\",\n"), r.Stdout)

	r = runGo(t, m, clone, env, "llm_git_status", "--summary")
	require.Equal(t, `{"branch":"main","upstream":"origin/main","ahead":0,"behind":0,"staged":1,"modified":1,"untracked":1,"conflicted":0}`+"\n", r.Stdout)
	r = runGo(t, m, clone, env, "llm_git_status", "--summary", "--format", "tsv")
	require.Equal(t, "branch\tupstream\tahead\tbehind\tstaged\tmodified\tuntracked\tconflicted\nmain\torigin/main\t0\t0\t1\t1\t1\t0\n", r.Stdout)

	r = runGo(t, m, clone, env, "llm_git_status", "--files")
	require.Equal(t, `[{"p":"f.txt","xy":".M"},{"p":"s.txt","xy":"A."},{"p":"u.txt","xy":"??"}]`+"\n", r.Stdout)
	r = runGo(t, m, clone, env, "llm_git_status", "--files", "--max-lines", "1", "--format", "jsonl")
	require.Equal(t, `{"p":"f.txt","xy":".M"}`+"\n", r.Stdout)
	require.Equal(t, `{"truncated":2}`+"\n", r.Stderr)

	// --repos: input order, a row per path, errors as rows.
	notRepo := physTemp(t)
	r = runGo(t, m, physTemp(t), env, "llm_git_status", "--repos", clone, notRepo, filepath.Join(notRepo, "missing"), "--format", "tsv", "-j", "2")
	require.Equal(t, 0, r.RC, r.Stderr)
	lines := strings.Split(strings.TrimSuffix(r.Stdout, "\n"), "\n")
	require.Equal(t, []string{
		"repo\tbranch\tupstream\tahead\tbehind\tstaged\tmodified\tuntracked\tconflicted\terror",
		clone + "\tmain\torigin/main\t0\t0\t1\t1\t1\t0\t\\N",
		notRepo + "\t\\N\t\\N\t\\N\t\\N\t\\N\t\\N\t\\N\t\\N\tnot_a_git_repo",
		filepath.Join(notRepo, "missing") + "\t\\N\t\\N\t\\N\t\\N\t\\N\t\\N\t\\N\t\\N\tpath_not_found",
	}, lines)
	r = runGo(t, m, clone, env, "llm_git_status", "--repos", "/proc")
	require.Contains(t, r.Stdout, `"error":"forbidden_path"`)

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"format-alone", []string{"--format", "tsv"}, "conflicting_options/RETRY"},
		{"files-summary", []string{"--files", "--summary"}, "conflicting_options/RETRY"},
		{"budget-summary", []string{"--summary", "--max-lines", "2"}, "conflicting_options/RETRY"},
		{"jobs-alone", []string{"--summary", "-j", "2"}, "conflicting_options/RETRY"},
		{"path-and-repos", []string{".", "--repos", "."}, "conflicting_options/RETRY"},
		{"repos-empty", []string{"--repos"}, "missing_value/RETRY"},
		{"bad-format", []string{"--files", "--format", "xml"}, "invalid_format/RETRY"},
		{"not-a-repo", []string{"--summary", notRepo}, "not_a_git_repo/CONTINUE"},
		{"unknown", []string{"--bogus"}, "unknown_option/RETRY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runGo(t, m, clone, env, "llm_git_status", tc.args...)
			require.Equal(t, 1, r.RC, r.Stdout)
			require.Equal(t, tc.want, errCode(t, r))
		})
	}
}

func TestGitStatusFetch(t *testing.T) {
	requireGit(t)
	m := New()
	clone, bare, home := remoteRepo(t)
	env := baseEnv(physTemp(t), nil)

	// The fetch brings the remote commit: behind 1, fetched true, and the
	// overview of the repository.
	var ov overviewJSON
	r := runGo(t, m, clone, env, "llm_git_status", "--fetch", "--timeout", "30")
	decodeOK(t, r, &ov)
	require.NotNil(t, ov.Fetched)
	require.True(t, *ov.Fetched)
	require.Equal(t, 1, ov.Behind)
	require.Empty(t, ov.Error)
	require.Len(t, ov.Log, 3)

	// Tags and other branches stay where they are.
	other := physTemp(t)
	git(t, other, home, "clone", bare, ".")
	git(t, other, home, "tag", "v9")
	git(t, other, home, "push", "origin", "v9", "main:side")
	r = runGo(t, m, clone, env, "llm_git_status", "--fetch", "--summary")
	require.Contains(t, r.Stdout, `"fetched":true`)
	out, err := exec.Command("git", "-C", clone, "for-each-ref", "--format=%(refname)").Output()
	require.NoError(t, err)
	require.NotContains(t, string(out), "refs/tags/v9")
	require.NotContains(t, string(out), "refs/remotes/origin/side")

	// The remote is gone: fetched false with the reason, the local state
	// still printed.
	moved := bare + ".moved"
	require.NoError(t, os.Rename(bare, moved))
	ov = overviewJSON{}
	decodeOK(t, runGo(t, m, clone, env, "llm_git_status", "--fetch"), &ov)
	require.False(t, *ov.Fetched)
	require.NotEmpty(t, ov.Error)
	require.Equal(t, "main", ov.Branch)
	require.NoError(t, os.Rename(moved, bare))

	// A repository whose own config names a program is not fetched.
	git(t, clone, home, "config", "core.sshCommand", "/bin/false")
	r = runGo(t, m, clone, env, "llm_git_status", "--fetch", "--summary")
	require.Contains(t, r.Stdout, `"fetched":false,"error":"not fetched: the repository configuration sets core.sshcommand`)
	git(t, clone, home, "config", "--unset", "core.sshCommand")

	// No upstream.
	git(t, clone, home, "checkout", "-q", "-b", "local")
	r = runGo(t, m, clone, env, "llm_git_status", "--fetch", "--summary")
	require.Contains(t, r.Stdout, `"upstream":""`)
	require.Contains(t, r.Stdout, `"fetched":false,"error":"no upstream branch to fetch"`)

	// --repos with --fetch adds the column.
	r = runGo(t, m, physTemp(t), env, "llm_git_status", "--repos", clone, "--fetch", "--format", "jsonl")
	require.Contains(t, r.Stdout, `"fetched":false,"error":"no upstream branch to fetch"`)
}

func TestBlockedFetchKey(t *testing.T) {
	for k, want := range map[string]bool{
		"core.sshcommand": true, "credential.helper": true, "credential.https://x.helper": true,
		"remote.origin.uploadpack": true, "url.ext::sh.insteadof": true, "protocol.ext.allow": true,
		"remote.origin.url": false, "core.editor": false, "branch.main.remote": false,
	} {
		got := blockedFetchKey(map[string]string{k: "x"})
		require.Equal(t, want, got != "", k)
	}
}

// diffRepo has a modified file, a staged rename, a binary file, a
// modified secret file and a long change.
func diffRepo(t *testing.T) (string, string) {
	t.Helper()
	home := physTemp(t)
	d := physTemp(t)
	git(t, d, home, "init", "-b", "main")
	mustWrite(t, filepath.Join(d, "a.txt"), "1\n2\n3\n")
	mustWrite(t, filepath.Join(d, "old.txt"), strings.Repeat("same line\n", 20))
	mustWrite(t, filepath.Join(d, ".env"), "SECRET=one\n")
	mustWrite(t, filepath.Join(d, "long.txt"), "x\n")
	require.NoError(t, os.WriteFile(filepath.Join(d, "bin.dat"), []byte{0, 1, 2}, 0o644))
	git(t, d, home, "add", "-A")
	git(t, d, home, "commit", "-m", "base")
	mustWrite(t, filepath.Join(d, "a.txt"), "1\nTWO\n3\n4\n")
	git(t, d, home, "mv", "old.txt", "new.txt")
	mustWrite(t, filepath.Join(d, ".env"), "SECRET=two\n")
	require.NoError(t, os.WriteFile(filepath.Join(d, "bin.dat"), []byte{0, 9, 9, 9}, 0o644))
	var long strings.Builder
	for i := 0; i < 30; i++ {
		long.WriteString("line\n")
	}
	mustWrite(t, filepath.Join(d, "long.txt"), "x\n"+long.String())
	return d, home
}

func TestGitDiffModes(t *testing.T) {
	requireGit(t)
	m := New()
	d, home := diffRepo(t)
	env := baseEnv(physTemp(t), nil)

	// The default is git's --stat text, unchanged.
	want, err := exec.Command("git", "-C", d, "diff", "--stat", "--no-color").Output()
	require.NoError(t, err)
	r := runGo(t, m, d, env, "llm_git_diff")
	require.Equal(t, string(want), r.Stdout)

	r = runGo(t, m, d, env, "llm_git_diff", "--summary")
	require.Equal(t, `{"files":4,"insertions":33,"deletions":2}`+"\n", r.Stdout)
	require.Less(t, len(r.Stdout), 100)

	r = runGo(t, m, d, env, "llm_git_diff", "--files")
	require.Equal(t, `[{"p":".env","add":1,"del":1,"status":"M"},{"p":"a.txt","add":2,"del":1,"status":"M"},`+
		`{"p":"bin.dat","add":null,"del":null,"status":"M"},{"p":"long.txt","add":30,"del":0,"status":"M"}]`+"\n", r.Stdout)

	r = runGo(t, m, d, env, "llm_git_diff", "--staged", "--files", "--format", "tsv")
	require.Equal(t, "p\tadd\tdel\tstatus\nnew.txt\t0\t0\tR\n", r.Stdout)

	r = runGo(t, m, d, env, "llm_git_diff", "--numstat", "--max-lines", "1")
	require.Equal(t, `[{"p":".env","add":1,"del":1}]`+"\n", r.Stdout)
	require.Equal(t, `{"truncated":3}`+"\n", r.Stderr)

	// --patch: the secret file's body is replaced, long.txt is capped.
	r = runGo(t, m, d, env, "llm_git_diff", "--patch", "--max-lines", "5")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.NotContains(t, r.Stdout, "SECRET")
	require.Contains(t, r.Stdout, "diff --git a/.env b/.env\n[... secret file: patch omitted]\n")
	require.Contains(t, r.Stdout, "+TWO\n")
	require.Contains(t, r.Stdout, "[... 27 lines omitted]\n")
	require.Contains(t, r.Stdout, "+++ b/long.txt\n")

	// A FILE as PATH limits the diff; a secret one is refused for --patch.
	r = runGo(t, m, physTemp(t), env, "llm_git_diff", "--summary", filepath.Join(d, "a.txt"))
	require.Equal(t, `{"files":1,"insertions":2,"deletions":1}`+"\n", r.Stdout)
	r = runGo(t, m, d, env, "llm_git_diff", "--patch", ".env")
	require.Equal(t, "secret_file_blocked/FATAL", errCode(t, r))

	// A range, as stat text and as a summary.
	git(t, d, home, "add", "-A")
	git(t, d, home, "commit", "-m", "second")
	r = runGo(t, m, d, env, "llm_git_diff", "HEAD~1..HEAD", "--summary")
	require.Equal(t, `{"files":5,"insertions":33,"deletions":2}`+"\n", r.Stdout)
	r = runGo(t, m, d, env, "llm_git_diff", "HEAD~1..HEAD")
	require.Contains(t, r.Stdout, "5 files changed")
	r = runGo(t, m, d, env, "llm_git_diff", "HEAD~1..HEAD", "--max-lines", "2")
	require.Contains(t, r.Stdout, "[... 4 lines omitted]")

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"unknown-range", []string{"nope..HEAD", "--summary"}, "unknown_revision/RETRY"},
		{"two-modes", []string{"--files", "--summary"}, "conflicting_options/RETRY"},
		{"staged-range", []string{"--staged", "HEAD~1..HEAD"}, "conflicting_options/RETRY"},
		{"format-stat", []string{"--format", "tsv"}, "conflicting_options/RETRY"},
		{"budget-summary", []string{"--summary", "--width", "10"}, "conflicting_options/RETRY"},
		{"not-a-repo", []string{"--files", physTemp(t)}, "not_a_git_repo/CONTINUE"},
		{"unknown", []string{"--name-only"}, "unknown_option/RETRY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runGo(t, m, d, env, "llm_git_diff", tc.args...)
			require.Equal(t, 1, r.RC, r.Stdout)
			require.Equal(t, tc.want, errCode(t, r))
		})
	}
}

func TestGitLogFields(t *testing.T) {
	requireGit(t)
	m := New()
	home := physTemp(t)
	d := physTemp(t)
	git(t, d, home, "init", "-b", "main")
	mustWrite(t, filepath.Join(d, "a b.txt"), "1\n")
	mustWrite(t, filepath.Join(d, "sub/c.txt"), "1\n")
	git(t, d, home, "add", "-A")
	git(t, d, home, "commit", "-m", "feat: first")
	git(t, d, home, "commit", "--allow-empty", "-m", "chore: empty")
	mustWrite(t, filepath.Join(d, "sub/c.txt"), "2\n")
	git(t, d, home, "commit", "-am", "fix: second")
	env := baseEnv(physTemp(t), nil)

	r := runGo(t, m, d, env, "llm_git_log", "--fields", "subject,files")
	require.Equal(t, `[{"subject":"fix: second","files":["sub/c.txt"]},{"subject":"chore: empty","files":[]},`+
		`{"subject":"feat: first","files":["a b.txt","sub/c.txt"]}]`+"\n", r.Stdout)

	r = runGo(t, m, d, env, "llm_git_log", "--fields", "files", "-n", "1", "--format", "tsv")
	require.Equal(t, "files\n[\"sub/c.txt\"]\n", r.Stdout)

	r = runGo(t, m, d, env, "llm_git_log", "--grep", "^(feat|fix):", "--fields", "subject,author,date")
	require.Equal(t, `[{"subject":"fix: second","author":"Port Tester","date":"2026-01-02 03:04:05 +0000"},`+
		`{"subject":"feat: first","author":"Port Tester","date":"2026-01-02 03:04:05 +0000"}]`+"\n", r.Stdout)

	r = runGo(t, m, d, env, "llm_git_log", "--path", "a b.txt", "--fields", "subject")
	require.Equal(t, `[{"subject":"feat: first"}]`+"\n", r.Stdout)

	r = runGo(t, m, d, env, "llm_git_log", "--author", "Nobody")
	require.Equal(t, "[]\n", r.Stdout)
	r = runGo(t, m, d, env, "llm_git_log", "--until", "2020-01-01")
	require.Equal(t, "[]\n", r.Stdout)
	r = runGo(t, m, d, env, "llm_git_log", "--since", "2025-01-01", "-n", "1")
	var rows []map[string]string
	decodeOK(t, r, &rows)
	require.Len(t, rows, 1)
	require.Equal(t, []string{"author", "date", "hash", "subject"}, keys(rows[0]))

	r = runGo(t, m, d, env, "llm_git_log", "--fields", "hash,subject", "--width", "6", "-n", "1")
	require.Contains(t, r.Stdout, `"subject":"fix: …"`)

	empty := physTemp(t)
	git(t, empty, home, "init", "-b", "main")
	r = runGo(t, m, empty, env, "llm_git_log", "--fields", "hash")
	require.Equal(t, "[]\n", r.Stdout)

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"bad-field", []string{"--fields", "hash,body"}, "invalid_fields/RETRY"},
		{"bad-regex", []string{"--grep", "("}, "invalid_regex/RETRY"},
		{"empty-path", []string{"--path", ""}, "missing_value/RETRY"},
		{"missing", []string{"--since"}, "missing_value/RETRY"},
		{"not-a-repo", []string{"--fields", "hash", physTemp(t)}, "not_a_git_repo/CONTINUE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runGo(t, m, d, env, "llm_git_log", tc.args...)
			require.Equal(t, 1, r.RC, r.Stdout)
			require.Equal(t, tc.want, errCode(t, r))
		})
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func TestParseLogZ(t *testing.T) {
	out := "\x00h1\x00s1\x00\nf1\x00f2\x00\x00h2\x00s2\x00\x00h3\x00s3\x00\n\nnl\x00"
	got := parseLogZ([]byte(out), 2)
	require.Equal(t, []logCommit{
		{header: []string{"h1", "s1"}, files: []string{"f1", "f2"}},
		{header: []string{"h2", "s2"}, files: []string{}},
		{header: []string{"h3", "s3"}, files: []string{"\nnl"}},
	}, got)
}

func TestGitShow(t *testing.T) {
	requireGit(t)
	m := New()
	d, home := diffRepo(t)
	git(t, d, home, "add", "-A")
	git(t, d, home, "commit", "-m", "second", "-m", "body line")
	git(t, d, home, "tag", "-a", "v1", "-m", "tag message")
	env := baseEnv(physTemp(t), nil)

	var obj struct {
		Hash, Subject, Author, Date string
		Files                       []map[string]any
		Stat                        *ovDiffstat
		Truncated                   int
	}
	r := runGo(t, m, d, env, "llm_git_show")
	decodeOK(t, r, &obj)
	require.Equal(t, "second", obj.Subject)
	require.Equal(t, "Port Tester", obj.Author)
	require.Equal(t, "2026-01-02 03:04:05 +0000", obj.Date)
	require.Len(t, obj.Files, 5)
	require.Equal(t, map[string]any{"p": "bin.dat", "add": nil, "del": nil}, obj.Files[2])
	require.Nil(t, obj.Stat)

	r = runGo(t, m, d, env, "llm_git_show", "v1", "--stat", "--files", "--max-lines", "2")
	obj.Stat, obj.Files = nil, nil
	decodeOK(t, r, &obj)
	require.Equal(t, &ovDiffstat{Files: 5, Insertions: 33, Deletions: 2}, obj.Stat)
	require.Equal(t, 3, obj.Truncated)
	require.Equal(t, map[string]any{"p": ".env", "add": 1.0, "del": 1.0, "status": "M"}, obj.Files[0])
	require.Equal(t, map[string]any{"p": "a.txt", "add": 2.0, "del": 1.0, "status": "M"}, obj.Files[1])

	// A FILE as PATH limits the files (git then sees no rename: its other
	// side is outside the pathspec).
	r = runGo(t, m, physTemp(t), env, "llm_git_show", "HEAD", "--files", filepath.Join(d, "new.txt"))
	require.Contains(t, r.Stdout, `"files":[{"p":"new.txt","add":20,"del":0,"status":"A"}]}`)
	r = runGo(t, m, d, env, "llm_git_show", "HEAD", "--files")
	require.Contains(t, r.Stdout, `{"p":"new.txt","add":0,"del":0,"status":"R"}`)

	r = runGo(t, m, d, env, "llm_git_show", "HEAD", "--patch", "--max-lines", "3")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.True(t, strings.HasPrefix(r.Stdout, "commit "), r.Stdout)
	require.Contains(t, r.Stdout, "Author: Port Tester\nDate:   2026-01-02 03:04:05 +0000\n\n    second\n\n    body line\n\ndiff --git")
	require.Contains(t, r.Stdout, "[... secret file: patch omitted]")
	require.NotContains(t, r.Stdout, "SECRET")
	require.Contains(t, r.Stdout, "[... 29 lines omitted]")

	r = runGo(t, m, d, env, "llm_git_show", "HEAD~1:a.txt")
	require.Equal(t, "1\n2\n3\n", r.Stdout)
	r = runGo(t, m, d, env, "llm_git_show", "HEAD:long.txt", "--head", "1", "--tail", "1")
	require.Equal(t, "x\n[... 29 lines omitted]\nline\n", r.Stdout)
	r = runGo(t, m, filepath.Join(d), env, "llm_git_show", "HEAD:", "--max-lines", "2")
	require.Equal(t, ".env\na.txt\n[... 3 lines omitted]\n", r.Stdout)

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"secret", []string{"HEAD:.env"}, "secret_file_blocked/FATAL"},
		{"binary", []string{"HEAD:bin.dat"}, "binary_file/RETRY"},
		{"unknown-rev", []string{"nope"}, "unknown_revision/RETRY"},
		{"unknown-rev-file", []string{"nope:a.txt"}, "unknown_revision/RETRY"},
		{"missing-file", []string{"HEAD:missing.txt"}, "file_not_found/RETRY"},
		{"not-a-commit", []string{"HEAD:a.txt", "--stat"}, "conflicting_options/RETRY"},
		{"patch-stat", []string{"--patch", "--stat"}, "conflicting_options/RETRY"},
		{"not-a-repo", []string{"HEAD", physTemp(t)}, "not_a_git_repo/CONTINUE"},
		{"third", []string{"HEAD", ".", "."}, "unexpected_argument/RETRY"},
		{"unknown", []string{"--name-only"}, "unknown_option/RETRY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runGo(t, m, d, env, "llm_git_show", tc.args...)
			require.Equal(t, 1, r.RC, r.Stdout)
			require.Equal(t, tc.want, errCode(t, r))
		})
	}
}

func TestSplitRevFileAndRange(t *testing.T) {
	for in, want := range map[string][2]string{
		"HEAD:a.txt": {"HEAD", "a.txt"}, ":a.txt": {"", "a.txt"}, "HEAD@{10:00}:f": {"HEAD@{10:00}", "f"},
		"main:dir/x:y": {"main", "dir/x:y"},
	} {
		r, f, ok := splitRevFile(in)
		require.True(t, ok, in)
		require.Equal(t, want, [2]string{r, f}, in)
	}
	for _, in := range []string{"HEAD", ":/fix bug", "HEAD@{1:2}"} {
		_, _, ok := splitRevFile(in)
		require.False(t, ok, in)
	}
	t.Chdir(physTemp(t))
	require.True(t, isRevRange("main..HEAD"))
	require.True(t, isRevRange("origin/main...feature"))
	require.False(t, isRevRange("../x"))
	require.False(t, isRevRange("./a..b"))
	require.False(t, isRevRange("plain"))
	mustMkdir(t, "a..b")
	require.False(t, isRevRange("a..b"))
}

func TestPatchNames(t *testing.T) {
	require.Equal(t, []string{"a b.txt", "a b.txt", "a b.txt"}, patchNames([]string{
		"diff --git a/a b.txt b/a b.txt", "--- a/a b.txt", "+++ b/a b.txt"}))
	require.Equal(t, []string{"old/.env", "new.txt"}, patchNames([]string{
		"diff --git a/old/.env b/new.txt", "rename from old/.env", "rename to new.txt"}))
	require.Equal(t, []string{"t\tab", "t\tab"}, patchNames([]string{`diff --git "a/t\tab" "b/t\tab"`}))
	require.Equal(t, []string{"n.txt"}, patchNames([]string{"diff --git a/n.txt b/n.txt", "--- /dev/null"}))
}
