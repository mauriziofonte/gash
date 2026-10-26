// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/pathguard"
	"github.com/mauriziofonte/gash/internal/term"
)

type result struct {
	Stdout, Stderr string
	RC             int
}

// runIn runs an agent function with dir as the working directory and a
// private HOME.
func runIn(t *testing.T, dir, name string, args ...string) result {
	t.Helper()
	m := New()
	m.getwd = func() (string, error) { return dir, nil }
	m.rootHome = func() string { return "/root" }
	tbl := invoke.NewTable()
	m.Register(tbl)
	var out, errb bytes.Buffer
	env := invoke.EnvMap(map[string]string{"HOME": t.TempDir(), "PWD": dir, "GASH_HEADLESS": "1", "PATH": os.Getenv("PATH")})
	rc := tbl.Run(&invoke.Call{Ctx: context.Background(), Name: name, Args: args, Env: env,
		Stdout: &out, Stderr: &errb, Term: term.Fixed(env, false, false, false)})
	return result{out.String(), errb.String(), rc}
}

func code(t *testing.T, r result) string {
	t.Helper()
	var e struct{ Error, Action string }
	require.NoError(t, json.Unmarshal([]byte(r.Stderr), &e), r.Stderr)
	return e.Error + "/" + e.Action
}

// physDir is a temporary directory by its physical path (the guard
// resolves symlinks such as macOS /var -> /private/var).
func physDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return d
}

func write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), mode))
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

func TestReplaceWritesAllFiles(t *testing.T) {
	d := physDir(t)
	write(t, filepath.Join(d, "a.php"), "class OldName {}\n", 0o640)
	write(t, filepath.Join(d, "src", "b.php"), "new OldName();\nkeep\n", 0o755)
	r := runIn(t, d, "agent_replace", "a.php", "src/b.php", "--old", "OldName", "--new", "NewName")
	require.Equal(t, result{`{"files":2,"replaced":{"a.php":[1],"src/b.php":[1]},"written":true}` + "\n", "", 0}, r)
	require.Equal(t, "class NewName {}\n", read(t, filepath.Join(d, "a.php")))
	require.Equal(t, "new NewName();\nkeep\n", read(t, filepath.Join(d, "src", "b.php")))
	st, err := os.Stat(filepath.Join(d, "a.php"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o640), st.Mode().Perm())
	st, _ = os.Stat(filepath.Join(d, "src", "b.php"))
	require.Equal(t, os.FileMode(0o755), st.Mode().Perm())
	// No temporary file is left behind.
	entries, _ := os.ReadDir(d)
	for _, e := range entries {
		require.False(t, strings.Contains(e.Name(), ".gash-"), e.Name())
	}
}

// Plan E1 acceptance: a mismatch on the last file leaves every file
// untouched.
func TestReplaceAllOrNothing(t *testing.T) {
	d := physDir(t)
	write(t, filepath.Join(d, "1.txt"), "foo\n", 0o644)
	write(t, filepath.Join(d, "2.txt"), "foo\n", 0o644)
	write(t, filepath.Join(d, "3.txt"), "bar\n", 0o644)
	r := runIn(t, d, "agent_replace", "1.txt", "2.txt", "3.txt", "--old", "foo", "--new", "baz")
	require.Equal(t, 1, r.RC)
	require.Empty(t, r.Stdout)
	require.Equal(t, "replace_count_mismatch/RETRY", code(t, r))
	require.Contains(t, r.Stderr, "3.txt: pair 1 found 0, expected exactly 1")
	for _, f := range []string{"1.txt", "2.txt"} {
		require.Equal(t, "foo\n", read(t, filepath.Join(d, f)))
	}
}

func TestReplaceCountsAndPairs(t *testing.T) {
	d := physDir(t)
	write(t, filepath.Join(d, "x.js"), "var a;\nvar b;\nvar c;\n", 0o644)
	// Default is exactly one: three matches fail.
	r := runIn(t, d, "agent_replace", "x.js", "--old", "var ", "--new", "let ")
	require.Equal(t, "replace_count_mismatch/RETRY", code(t, r))
	require.Contains(t, r.Stderr, "found 3, expected exactly 1")
	r = runIn(t, d, "agent_replace", "x.js", "--old", "var ", "--new", "let ", "--count", "2")
	require.Equal(t, "replace_count_mismatch/RETRY", code(t, r))
	r = runIn(t, d, "agent_replace", "x.js", "--old", "var ", "--new", "let ", "--count", "3")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Equal(t, "let a;\nlet b;\nlet c;\n", read(t, filepath.Join(d, "x.js")))
	// Pairs apply in order, each to the result of the one before.
	r = runIn(t, d, "agent_replace", "x.js", "--old", "let a", "--new", "const a", "--old", "const a;", "--new", "const A;", "--min", "1")
	require.Equal(t, `{"files":1,"replaced":{"x.js":[1,1]},"written":true}`+"\n", r.Stdout)
	require.Equal(t, "const A;\nlet b;\nlet c;\n", read(t, filepath.Join(d, "x.js")))
}

func TestReplaceRegex(t *testing.T) {
	d := physDir(t)
	write(t, filepath.Join(d, "c.ini"), "v1\nv22\n", 0o644)
	r := runIn(t, d, "agent_replace", "c.ini", "--old", "v([0-9]+)", "--new", "v${1}-lts", "--regex", "--min", "1")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Equal(t, "v1-lts\nv22-lts\n", read(t, filepath.Join(d, "c.ini")))
	r = runIn(t, d, "agent_replace", "c.ini", "--old", "v(", "--new", "x", "--regex")
	require.Equal(t, "invalid_regex/RETRY", code(t, r))
}

func TestReplaceDryRunWritesNothing(t *testing.T) {
	d := physDir(t)
	write(t, filepath.Join(d, "f.txt"), "a\nold\nb\n", 0o644)
	r := runIn(t, d, "agent_replace", "f.txt", "--old", "old", "--new", "new1\nnew2", "--dry-run")
	require.Equal(t, `{"files":1,"replaced":{"f.txt":[1]},"written":false,"dry_run":true,"diff":{"f.txt":{"add":2,"del":1}}}`+"\n", r.Stdout)
	require.Equal(t, "a\nold\nb\n", read(t, filepath.Join(d, "f.txt")))

	r = runIn(t, d, "agent_replace", "f.txt", "--old", "old", "--new", "x", "--dry-run", "--format", "tsv")
	require.Equal(t, "file\tpair\tcount\nf.txt\t1\t1\n", r.Stdout)
	require.Equal(t, `{"dry_run":true,"written":false}`+"\n", r.Stderr)
}

func TestReplaceRefusals(t *testing.T) {
	d := physDir(t)
	write(t, filepath.Join(d, "f.txt"), "x\n", 0o644)
	write(t, filepath.Join(d, ".env"), "DB_PASSWORD=x\n", 0o600)
	write(t, filepath.Join(d, "bin.dat"), "x\x00y", 0o644)
	require.NoError(t, os.Mkdir(filepath.Join(d, "dir"), 0o755))
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"f.txt"}, "invalid_replacement/RETRY"},
		{[]string{"f.txt", "--old", "x"}, "invalid_replacement/RETRY"},
		{[]string{"f.txt", "--new", "x"}, "invalid_replacement/RETRY"},
		{[]string{"f.txt", "--old", "", "--new", "x"}, "invalid_replacement/RETRY"},
		{[]string{"f.txt", "--old", "a", "--old", "b"}, "invalid_replacement/RETRY"},
		{[]string{"--old", "x", "--new", "y"}, "missing_file/RETRY"},
		{[]string{"f.txt", "--old", "x", "--new", "y", "--count", "0"}, "invalid_count/RETRY"},
		{[]string{"f.txt", "--old", "x", "--new", "y", "--count", "1", "--min", "1"}, "conflicting_options/RETRY"},
		{[]string{"f.txt", "--old", "x", "--new", "y", "--bogus"}, "unknown_option/RETRY"},
		{[]string{"f.txt", "--old"}, "missing_value/RETRY"},
		{[]string{"nope.txt", "--old", "x", "--new", "y"}, "file_not_found/RETRY"},
		{[]string{"dir", "--old", "x", "--new", "y"}, "invalid_type/RETRY"},
		{[]string{".env", "--old", "x", "--new", "y"}, "secret_file_blocked/FATAL"},
		{[]string{"bin.dat", "--old", "x", "--new", "y"}, "binary_file/RETRY"},
		{[]string{"../f.txt", "--old", "x", "--new", "y"}, "path_traversal_blocked/FATAL"},
		{[]string{"/etc/shadow", "--old", "x", "--new", "y"}, "forbidden_path/FATAL"},
		{[]string{"/usr/bin/env", "--old", "x", "--new", "y", "--force"}, "write_path_denied/FATAL"},
		{[]string{"/etc/hosts", "--old", "x", "--new", "y"}, "sensitive_path/STOP"},
		{[]string{"f.txt", "./f.txt", "--old", "x", "--new", "y"}, "unexpected_argument/RETRY"},
		{[]string{"*.txt", "--old", "x", "--new", "y"}, "file_not_found/RETRY"},
	} {
		r := runIn(t, d, "agent_replace", tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Equal(t, tc.want, code(t, r), tc.args)
	}
	require.Equal(t, "x\n", read(t, filepath.Join(d, "f.txt")))
	require.Equal(t, "DB_PASSWORD=x\n", read(t, filepath.Join(d, ".env")))
}

// The ASK tier is waived for the caller's own work: below the working
// directory of a project in /var/www, below HOME; never for .ssh, never
// for a working directory that is an ASK root or below /etc.
func TestAskWaived(t *testing.T) {
	g := &pathguard.Guard{Home: "/root", Cwd: "/var/www/app"}
	require.True(t, askWaived(g, "/var/www/app/src/a.php", "/var"))
	require.False(t, askWaived(g, "/var/www/other/a.php", "/var"))
	require.True(t, askWaived(g, "/root/proj/x", "/root"))
	require.False(t, askWaived(g, "/root/.ssh/config", ".ssh"))
	g.Cwd = "/var"
	require.False(t, askWaived(&pathguard.Guard{Cwd: "/var"}, "/var/www/a", "/var"))
	require.False(t, askWaived(&pathguard.Guard{Cwd: "/etc/nginx"}, "/etc/nginx/nginx.conf", "/etc"))
	// macOS: the physical working directory of /etc/nginx is /private/etc/nginx.
	require.False(t, askWaived(&pathguard.Guard{Cwd: "/private/etc/nginx"}, "/private/etc/nginx/nginx.conf", "/etc"))
	require.False(t, askWaived(&pathguard.Guard{Cwd: "/private/var"}, "/private/var/www/a", "/var"))
	require.True(t, askWaived(&pathguard.Guard{Cwd: "/private/var/www/app"}, "/private/var/www/app/a.php", "/var"))
}

func TestReplaceHelp(t *testing.T) {
	d := physDir(t)
	r := runIn(t, d, "agent_replace", "--old", "-h")
	require.Equal(t, 0, r.RC)
	require.Contains(t, r.Stdout, "agent_replace FILE...")
}

func TestLineDelta(t *testing.T) {
	add, del := lineDelta([]byte("a\nb\nc\n"), []byte("a\nB\nc\nd\n"))
	require.Equal(t, 2, add)
	require.Equal(t, 1, del)
	add, del = lineDelta([]byte("x"), []byte("x"))
	require.Zero(t, add+del)
}
