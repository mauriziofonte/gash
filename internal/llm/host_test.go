// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/remote"
	"github.com/mauriziofonte/gash/internal/remote/sshtest"
)

// hostFixture is a tree that is both the local working directory and the
// fake remote home, a local HOME with the HOST: entry, and a remote PATH
// whose uname reports another platform, so every --host call takes the
// POSIX fallback: its answer must be the local one, byte for byte (plus
// "host" on a JSON object).
type hostFixture struct {
	dir  string
	env  map[string]string
	ssh  *sshtest.SSH
	m    *Module
	bare *Module
}

func newHostFixture(t *testing.T) *hostFixture {
	t.Helper()
	return newHostFixtureOn(t, false)
}

// newHostFixtureOn with bsd plays a remote userland without GNU realpath
// -m and without find -printf, so the portable paths of the fallbacks run.
func newHostFixtureOn(t *testing.T, bsd bool) *hostFixture {
	t.Helper()
	dir := physTemp(t)
	home := physTemp(t)
	mustWrite(t, filepath.Join(home, ".gash_env"), "HOST:web=deploy@web.example\n")
	stub := physTemp(t)
	mustWrite(t, filepath.Join(stub, "uname"), "#!/bin/sh\ncase $1 in -s) echo SunOS ;; -m) echo sparc64 ;; *) echo SunOS sparc64 ;; esac\n")
	require.NoError(t, os.Chmod(filepath.Join(stub, "uname"), 0o755))
	if bsd {
		findPath, err := exec.LookPath("find")
		require.NoError(t, err)
		mustWrite(t, filepath.Join(stub, "realpath"), "#!/bin/sh\necho 'realpath: unknown option' >&2; exit 1\n")
		mustWrite(t, filepath.Join(stub, "find"), "#!/bin/sh\nfor a; do [ \"$a\" = -printf ] && exit 1; done\nexec "+findPath+" \"$@\"\n")
		require.NoError(t, os.Chmod(filepath.Join(stub, "realpath"), 0o755))
		require.NoError(t, os.Chmod(filepath.Join(stub, "find"), 0o755))
	}
	ssh := &sshtest.SSH{Home: dir, Path: stub + ":" + os.Getenv("PATH")}
	m := New()
	m.run = ssh
	m.exe = func() (string, error) { return "", os.ErrNotExist }
	env := baseEnv(home, map[string]string{"PATH": sshtest.ClientPath(t, os.Getenv("PATH"))})
	return &hostFixture{dir: dir, env: env, ssh: ssh, m: m, bare: New()}
}

// same requires the --host answer to be the local one.
func (f *hostFixture) same(t *testing.T, fn string, args ...string) result {
	t.Helper()
	want := runGo(t, f.bare, f.dir, f.env, fn, args...)
	n := f.ssh.N()
	got := runGo(t, f.m, f.dir, f.env, fn, append([]string{"--host", "web"}, args...)...)
	if want.RC == 0 && fn == "llm_tree" && !strings.Contains(strings.Join(args, " "), "--text") {
		want.Stdout = string(remote.AddHost([]byte(want.Stdout), "web"))
	}
	require.Equal(t, want, got, "%s %q", fn, args)
	require.LessOrEqual(t, f.ssh.N()-n, 1, "one ssh invocation at most")
	return got
}

func (f *hostFixture) tree(t *testing.T) {
	t.Helper()
	for p, c := range map[string]string{
		"src/a.php": "<?php\nfunction alpha() {}\n// TODO alpha\n", "src/b.php": "<?php\nbeta();\n",
		"src/sub/c.php": "x\nTODO one\ny\nz\nw\nTODO two\n", "a.txt": "TODO top\n", "a b.txt": "spaces TODO\n",
		"a-b/x.md": "md TODO\n", "A/upper.php": "TODO upper\n", ".hidden/h.php": "TODO hidden\n", ".h.php": "TODO dotfile\n",
		".env": "TODO=secret\n", ".env.example": "TODO=template\n", "secrets/k.php": "TODO secretdir\n",
		"vendor/v.php": "TODO vendor\n", "node_modules/n.js": "TODO nm\n", "__pycache__/p.pyc": "x",
		".cache/c": "x", "conf/app.yaml": "a: 1\nb: [x, y]\n", "conf/app.json": "{\"a\": 1}\n", "conf/raw.yml": ": : :\n",
		"bin.dat": "TODO\x00bin\n",
	} {
		mustWrite(t, filepath.Join(f.dir, p), c)
	}
	require.NoError(t, os.Symlink("src", filepath.Join(f.dir, "link")))
	require.NoError(t, os.Symlink(".env", filepath.Join(f.dir, "env-link")))
	mustMkdir(t, filepath.Join(f.dir, "empty"))
}

func TestHostFindFallbackIsLocal(t *testing.T) {
	for _, bsd := range []bool{false, true} {
		testHostFind(t, newHostFixtureOn(t, bsd))
	}
}

func testHostFind(t *testing.T, f *hostFixture) {
	f.tree(t)
	before := f.ssh.N()
	f.same(t, "llm_find", "*.php")
	require.Equal(t, before+1, f.ssh.N())
	require.Contains(t, f.ssh.Scripts[len(f.ssh.Scripts)-1], "gash_resolve", "the fallback answered")
	for _, args := range [][]string{
		{"*.php"}, {"*"}, {"*", ".", "-n", "3"}, {"*", "-n", "0"}, {"*", "--type", "d"}, {"*.php", "src"},
		{"*.php", "src/"}, {"*.php", f.dir}, {"*.php", "link"}, {"[ab]*", "."}, {"*.txt", "a.txt"},
		{"*.php", "nope"}, {"vendor", "."}, {"*.php", "vendor"}, {"*", "--limit", "007"},
	} {
		f.same(t, "llm_find", args...)
	}
	for _, args := range [][]string{{""}, {"*", "--type", "x"}, {"*", "-n", "x"}, {"*", ".", "."}, {"*", "-n"}} {
		f.same(t, "llm_find", args...)
	}
}

func TestHostConfigFallbackIsLocal(t *testing.T) {
	for _, bsd := range []bool{false, true} {
		testHostConfig(t, newHostFixtureOn(t, bsd))
	}
}

func testHostConfig(t *testing.T, f *hostFixture) {
	f.tree(t)
	for _, args := range [][]string{
		{"conf/app.yaml"}, {"conf/app.json"}, {"conf/raw.yml"}, {"conf/app.json", "--max-bytes", "3"},
		{"conf/missing.json"}, {"conf"}, {".env"}, {"env-link"}, {}, {"conf/app.json", "--max-bytes", "x"},
		{"a", "b"}, {f.dir + "/conf/app.yaml"},
	} {
		f.same(t, "llm_config", args...)
	}
}

func TestHostTreeFallbackIsLocal(t *testing.T) {
	for _, bsd := range []bool{false, true} {
		testHostTree(t, newHostFixtureOn(t, bsd))
	}
}

func testHostTree(t *testing.T, f *hostFixture) {
	f.tree(t)
	for i := 0; i < 520; i++ {
		mustWrite(t, filepath.Join(f.dir, "many", fmt.Sprintf("f%03d", i)), "x")
	}
	for _, args := range [][]string{
		{}, {"--depth", "2"}, {"--depth", "3", "--stats"}, {"--stats"}, {"src", "--stats", "--depth", "1"},
		{"--text"}, {"--text", "--depth", "1"}, {"src", "--text"}, {"many"}, {"many", "--text"}, {"a.txt"},
		{"nope"}, {"--depth", "0"}, {"vendor"}, {"link", "--stats"},
	} {
		f.same(t, "llm_tree", args...)
	}
}

func TestHostGrepFallbackIsLocal(t *testing.T) {
	for _, bsd := range []bool{false, true} {
		testHostGrep(t, newHostFixtureOn(t, bsd))
	}
}

func testHostGrep(t *testing.T, f *hostFixture) {
	f.tree(t)
	for _, args := range [][]string{
		{"TODO"}, {"TODO", "-n", "4"}, {"TODO", "src", "-C", "1"}, {"TODO", ".", "--context", "2"}, {"TODO", "--ext", "php"},
		{"TODO", "--ext", "php,md"}, {"TODO", "src/sub/c.php"}, {"TODO", "src/sub/c.php", "-C", "1"}, {"alpha|beta"},
		{"^TODO (one|two)$"}, {"[Tt]ODO"}, {"TOD.", "-n", "0"}, {"nomatch"}, {"TODO", "nope"}, {"TODO", ".env"},
		{"(", "."}, {""}, {"x", "-C", "y"},
	} {
		f.same(t, "llm_grep", args...)
	}
}

func TestHostGrepFallbackAsksForTheBinary(t *testing.T) {
	f := newHostFixture(t)
	f.tree(t)
	for _, args := range [][]string{{`\d+`}, {"a{2}"}, {"(?i)todo"}, {"TODO", "bin.dat"}} {
		r := runGo(t, f.m, f.dir, f.env, "llm_grep", append(args, "--host", "web")...)
		require.Equal(t, 1, r.RC, args)
		require.Contains(t, r.Stderr, `"error":"remote_binary_unavailable"`, args)
	}
	mustWrite(t, filepath.Join(f.dir, "src", ".gitignore"), "b.php\n")
	r := runGo(t, f.m, f.dir, f.env, "llm_grep", "TODO", "--host", "web")
	require.Contains(t, r.Stderr, `"error":"remote_binary_unavailable"`)
	require.Contains(t, r.Stderr, "an ignore file")
}

func TestEreShared(t *testing.T) {
	for re, want := range map[string]bool{
		"TODO": true, "a.b": true, "^x$": true, "a|b": true, "(a|b)+": true, "[abc]": true, "[^a-z]": true,
		"[]a]": true, "[[:alpha:]_]": true, `\.php`: true, `a\*`: true, `\d`: false, `\bx`: false, "a{2}": false,
		"(?i)x": false, "a*?": false, "[a[b]]": false, "[a&&b]": false, `[\]]`: false, "é": true, "é+": false, "a++": false,
	} {
		require.Equal(t, want, ereShared(re), re)
	}
}

func TestHostSpecsCoverTheHookedFunctions(t *testing.T) {
	m := New()
	for _, fn := range []string{"llm_find", "llm_grep", "llm_tree", "llm_config"} {
		_, ok := m.hostSpecs()[fn]
		require.True(t, ok, fn)
	}
	require.Panics(t, func() { m.hostable("llm_nope", nil) })
}

// TestHostBinaryOnly: the reads without a fallback run the shipped binary
// with the arguments as given (--host taken out), one invocation once it
// is cached; their output gains "host". The "binary" is a script that
// echoes its function and arguments.
func TestHostBinaryOnly(t *testing.T) {
	if b, ok := remote.LocalBinary(nil); !ok || b.Platform == "" {
		t.Skip("no binary is shipped from this platform")
	}
	home, rhome := physTemp(t), physTemp(t)
	mustWrite(t, filepath.Join(home, ".gash_env"), "HOST:web=deploy@web.example\n")
	exe := filepath.Join(physTemp(t), "gash")
	mustWrite(t, exe, "#!/bin/sh\ncase $1 in version) echo 3.4.0 ;; *) f=$1; shift; printf '{\"fn\":\"%s\",\"args\":\"%s\"}\\n' \"$f\" \"$*\" ;; esac\n")
	require.NoError(t, os.Chmod(exe, 0o755))
	ssh := &sshtest.SSH{Home: rhome}
	m := New()
	m.run = ssh
	m.exe = func() (string, error) { return exe, nil }
	env := baseEnv(home, map[string]string{"PATH": sshtest.ClientPath(t, os.Getenv("PATH"))})
	d := physTemp(t)
	cases := [][]string{
		{"llm_stat", "a", "b", "--lines"},
		{"llm_read", "src/a.php:2-3", "--around", "TODO", "-C", "1"},
		{"llm_log", "storage/logs/app.log", "--level", "error", "-n", "5"},
		{"llm_hash", ".env", "--short", "12"},
		{"llm_count", "a", "--pat", "x"},
		{"llm_assert", "a", "--count", "x=2"},
		{"llm_textstats", "a"},
		{"llm_textcheck", "a", "--rules", "crlf"},
		{"llm_json", "composer.json", "--path", "require"},
		{"llm_csv", "a.csv", "--where", "id=1"},
		{"llm_dotenv", ".env", "APP_KEY", "--present"},
		{"llm_git_status", "--repos", "app", "api", "--fetch"},
		{"llm_git_log", "-n", "3", "--path", "src", "app"},
		{"llm_git_overview", "app", "--log", "2"},
		{"llm_procs", "--name", "php-fpm"},
		{"llm_ports", "--listen"},
		{"llm_service", "nginx", "--log", "5"},
		{"llm_docker_ps", "--running"},
		{"llm_docker_logs", "api", "--tail", "20", "--grep", "ERROR"},
		{"llm_docker_exec", "api", "--", "php", "artisan", "--host", "x"},
		{"llm_wait", "--timeout", "5s", "--file-match", "app.log", "ready"},
		{"llm_sysinfo", "--mount", "/srv"},
		{"llm_lint", "src", "--ext", "php"},
		{"llm_tools", "php", "node"},
		{"llm_project", "app"},
		{"llm_deps", "app", "--dev"},
		{"llm_env", "--filter", "^APP_"},
	}
	for _, tc := range cases {
		fn := tc[0]
		_, ok := m.hostSpecs()[fn]
		require.True(t, ok, fn)
		args := append([]string{"--host", "web"}, tc[1:]...)
		r := runGo(t, m, d, env, fn, args...)
		require.Equal(t, 0, r.RC, "%s: %s", fn, r.Stderr)
		require.Equal(t, `{"host":"web","fn":"`+fn+`","args":"`+strings.Join(tc[1:], " ")+`"}`+"\n", r.Stdout, fn)
	}
	require.Equal(t, len(cases)+1, ssh.N(), "the binary is shipped once, then one invocation per call")

	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"llm_json", "-", "--host", "web"}, `"error":"unexpected_argument","details":"-"`},
		{[]string{"llm_read", "/etc/ssh/sshd_config:1-5", "--host", "web"}, `"error":"forbidden_path"`},
		{[]string{"llm_stat", "../x", "--host", "web"}, `"error":"path_traversal_blocked"`},
		{[]string{"llm_wait", "--timeout", "1s", "--exists", "/proc/1/environ", "--host", "web"}, `"error":"forbidden_path"`},
		{[]string{"llm_sysinfo", "--mount", "/usr/bin", "--host", "web"}, `"error":"forbidden_path"`},
		{[]string{"llm_procs", "--host", "nope"}, `"error":"unknown_host"`},
	} {
		r := runGo(t, m, d, env, tc.args[0], tc.args[1:]...)
		require.Equal(t, 1, r.RC, "%q", tc.args)
		require.Contains(t, r.Stderr, tc.code, "%q", tc.args)
	}
	require.Equal(t, len(cases)+1, ssh.N(), "a refused call never connects")
	require.Equal(t, []string{"src/x"}, readPathForms("src/x:1-5"))
	require.Equal(t, []string{"src/x"}, readPathForms("src/x:1:5"))
	require.Nil(t, readPathForms("src/x"))
}
