// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
)

// pinDebian pins the system the prerequisite reports describe (Debian
// with apt and sudo, a user that is not root); the programs it finds are
// those of the caller's PATH less the missing ones.
func pinDebian(t *testing.T, path string, missing ...string) {
	t.Helper()
	t.Cleanup(prereq.SetSystem(&prereq.System{
		OS:      platform.OS{GOOS: "linux", ID: "debian", Name: "Debian GNU/Linux 12 (bookworm)"},
		Manager: prereq.Apt, Sudo: true, Env: []string{"PATH=" + path},
		LookPath: func(name string, env []string) (string, bool) {
			if slices.Contains(missing, name) {
				return "", false
			}
			p, err := runner.LookPath(name, env, "")
			return p, err == nil
		},
	}))
}

// bashNote is the catalog's note on bash, JSON-escaped.
const bashNote = `On macOS, make the Homebrew bash your login shell: chsh -s \"$(brew --prefix)/bin/bash\"`

// fakeProc writes a Linux /proc: uptime 1000 s, 1000 MiB of memory, init
// (10.0% CPU, 10.0% memory) and a kernel thread.
func fakeProc(t *testing.T) string {
	t.Helper()
	root := physTemp(t)
	rss := 102400 * 1024 / os.Getpagesize()
	stat := func(pid, name, ppid string, utime, start, rss int) string {
		f := []string{pid, "(" + name + ")", "S", ppid, "1", "1", "0", "-1", "0", "0", "0", "0", "0",
			strconv.Itoa(utime), strconv.Itoa(utime), "0", "0", "20", "0", "1", "0", strconv.Itoa(start), "1000", strconv.Itoa(rss)}
		return strings.Join(f, " ") + "\n"
	}
	mustWrite(t, filepath.Join(root, "uptime"), "1000.00 900.00\n")
	mustWrite(t, filepath.Join(root, "meminfo"), "MemTotal:        1024000 kB\nMemFree: 1 kB\n")
	mustWrite(t, filepath.Join(root, "1/stat"), stat("1", "init", "0", 4975, 500, rss))
	mustWrite(t, filepath.Join(root, "1/cmdline"), "/sbin/init\x00splash\x00")
	mustWrite(t, filepath.Join(root, "2/stat"), stat("2", "kthreadd", "0", 0, 0, 0))
	mustWrite(t, filepath.Join(root, "2/cmdline"), "")
	mustWrite(t, filepath.Join(root, "300/stat"), stat("300", "sleep", "1", 0, 900, 0))
	mustWrite(t, filepath.Join(root, "300/cmdline"), "sleep\x0030\x00")
	return root
}

// TestProcsListPortable: the listing runs a ps both procps and BSD take
// (2.x: ps aux --no-headers, which BSD ps refuses, so macOS listed
// nothing), rows by PID.
func TestProcsListPortable(t *testing.T) {
	d := physTemp(t)
	m := New()
	m.lookPathFn = func(name string) (string, bool) { return "/bin/" + name, name == "ps" }
	m.run = runner.NewFake(t, runner.Expect{Argv: []string{"/bin/ps", "-A", "-o", "user=,pid=,%cpu=,%mem=,args="}, EnvHas: []string{"LC_ALL=C"},
		Stdout: "root   88  0.5  1.2 /usr/sbin/sshd -D\nroot    1  0.0  0.1 /sbin/launchd\n_www  120 12.3  0.0 /usr/sbin/httpd -D FOREGROUND\n"})
	r := runGo(t, m, d, baseEnv(physTemp(t), nil), "llm_procs")
	require.Equal(t, result{`[{"pid":1,"user":"root","cpu":0.0,"mem":0.1,"name":"/sbin/launchd"},` +
		`{"pid":88,"user":"root","cpu":0.5,"mem":1.2,"name":"/usr/sbin/sshd"},` +
		`{"pid":120,"user":"_www","cpu":12.3,"mem":0.0,"name":"/usr/sbin/httpd"}]` + "\n", "", 0}, r)
}

// TestProcsWithoutPS: without ps the listing, --name and --port read a
// Linux /proc; without either they report ps_not_found (2.x and 3.4
// before: [] or empty names).
func TestProcsWithoutPS(t *testing.T) {
	d := physTemp(t)
	env := baseEnv(physTemp(t), map[string]string{"PATH": "/nonexistent"})
	pinDebian(t, "/nonexistent")
	m := New()
	m.proc = fakeProc(t)
	m.getpid = func() int { return 4242 }
	m.lookPathFn = func(name string) (string, bool) { return "/bin/" + name, name == "lsof" }
	m.run = runner.NewFake(t).Respond(func(c runner.Call) (runner.Expect, bool) {
		return runner.Expect{Stdout: "300\n2\n"}, strings.Join(c.Argv, " ") == "/bin/lsof -i :80 -t"
	})
	user := psUser(os.Geteuid())
	r := runGo(t, m, d, env, "llm_procs")
	require.Equal(t, result{`[{"pid":1,"user":"` + user + `","cpu":10.0,"mem":10.0,"name":"/sbin/init"},` +
		`{"pid":2,"user":"` + user + `","cpu":0.0,"mem":0.0,"name":"[kthreadd]"},` +
		`{"pid":300,"user":"` + user + `","cpu":0.0,"mem":0.0,"name":"sleep"}]` + "\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_procs", "--name", "sle+p 30")
	require.Equal(t, result{`[{"pid":300,"name":"sleep"}]` + "\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_procs", "--name", "(")
	require.Equal(t, result{"[]\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_procs", "--port", "80")
	require.Equal(t, result{`[{"pid":300,"name":"sleep","port":80},{"pid":2,"name":"kthreadd","port":80}]` + "\n", "", 0}, r)

	m.proc = ""
	want := `{"error":"ps_not_found","details":"NEED needs ps, which is not installed","action":"CONTINUE","recoverable":true,` +
		`"hint":"Install procps: sudo apt-get install -y procps","missing":"ps","install":"sudo apt-get install -y procps"}` + "\n"
	for need, args := range map[string][]string{
		"llm_procs": nil, "llm_procs --name": {"--name", "x"}, "llm_procs --port": {"--port", "80"},
		"llm_procs --pid": {"--pid", "1"}, "llm_procs --tree": {"--tree"},
	} {
		r = runGo(t, m, d, env, "llm_procs", args...)
		require.Equal(t, result{"", strings.Replace(want, "NEED", need, 1), 1}, r, args)
	}
}

// TestProcsWithoutLsofOrPgrep: --port without lsof is reported on both
// paths (2.x and 3.4 before: no rows); --name without pgrep matches the
// process table (2.x: []).
func TestProcsWithoutLsofOrPgrep(t *testing.T) {
	d := physTemp(t)
	env := baseEnv(physTemp(t), map[string]string{"PATH": "/nonexistent"})
	pinDebian(t, "/nonexistent")
	m := procsModule(t)
	m.lookPathFn = func(name string) (string, bool) { return "/bin/" + name, name == "ps" }
	want := `{"error":"prerequisite_missing","details":"llm_procs --port needs lsof, which is not installed","action":"STOP","recoverable":true,` +
		`"hint":"Install lsof: sudo apt-get install -y lsof","missing":"lsof","install":"sudo apt-get install -y lsof"}` + "\n"
	for _, args := range [][]string{{"--port", "8080"}, {"--port", "8080", "--alive"}} {
		r := runGo(t, m, d, env, "llm_procs", args...)
		require.Equal(t, result{"", want, 1}, r, args)
	}
	r := runGo(t, m, d, env, "llm_procs", "--name", "sleep 30")
	require.Equal(t, result{`[{"pid":300,"name":"sleep"},{"pid":400,"name":"sleep"}]` + "\n", "", 0}, r)
}

// TestWaitWithoutPSOrBash: llm_wait --gone PID without /proc and ps, and
// --cmd-ok without bash, are reported at once (3.4 before: never met, or
// rc 127 on every run until the timeout).
func TestWaitWithoutPSOrBash(t *testing.T) {
	d := physTemp(t)
	env := baseEnv(physTemp(t), map[string]string{"PATH": "/nonexistent"})
	pinDebian(t, "/nonexistent")
	m := New()
	m.proc = ""
	m.lookPathFn = func(string) (string, bool) { return "", false }
	r := runGo(t, m, d, env, "llm_wait", "--gone", "1234", "--timeout", "1")
	require.Equal(t, result{"", `{"error":"ps_not_found","details":"llm_wait --gone needs ps, which is not installed","action":"CONTINUE","recoverable":true,` +
		`"hint":"Install procps: sudo apt-get install -y procps","missing":"ps","install":"sudo apt-get install -y procps"}` + "\n", 1}, r)
	r = runGo(t, m, d, env, "llm_wait", "--gone", "sleep", "--timeout", "1")
	require.Contains(t, r.Stderr, `"details":"llm_wait --gone needs ps, which is not installed"`)
	r = runGo(t, m, d, env, "llm_wait", "--cmd-ok", "true", "--timeout", "1")
	require.Equal(t, result{"", `{"error":"prerequisite_missing","details":"llm_wait --cmd-ok needs bash, which is not installed","action":"STOP","recoverable":true,` +
		`"hint":"Install bash: sudo apt-get install -y bash. ` + bashNote + `","missing":"bash","install":"sudo apt-get install -y bash"}` + "\n", 1}, r)

	// With ps and no /proc, --gone PID asks ps.
	m.lookPathFn = func(name string) (string, bool) { return "/bin/" + name, name == "ps" }
	m.run = runner.NewFake(t, runner.Expect{Argv: []string{"/bin/ps", "-p", "1234", "-o", "stat="}, Status: 1})
	r = runGo(t, m, d, env, "llm_wait", "--gone", "1234", "--timeout", "5")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Contains(t, r.Stdout, `"met":true`)
}

// TestExecWithoutBash: llm_exec and gash run --safe report a missing bash
// (2.x and 3.4 before: exit 127 with nothing on stderr).
func TestExecWithoutBash(t *testing.T) {
	d, home := execFixture(t)
	pinDebian(t, "/nonexistent")
	m := New()
	m.lookPathFn = func(string) (string, bool) { return "", false }
	env := baseEnv(home, map[string]string{"PWD": d, "PATH": "/nonexistent", "BASH": "/nonexistent/bash"})
	want := `{"error":"prerequisite_missing","details":"NEED needs bash, which is not installed","action":"STOP","recoverable":true,` +
		`"hint":"Install bash: sudo apt-get install -y bash. ` + bashNote + `","missing":"bash","install":"sudo apt-get install -y bash"}` + "\n"
	r := runGo(t, m, d, env, "llm_exec", "ls")
	require.Equal(t, result{"", strings.Replace(want, "NEED", "llm_exec", 1), 1}, r)

	var out, errb bytes.Buffer
	e := invoke.EnvMap(env)
	c := &invoke.Call{Ctx: context.Background(), Name: "gash", Env: e, Stdin: bytes.NewReader(nil), Stdout: &out, Stderr: &errb,
		Term: term.Fixed(e, false, false, false)}
	code, refused := SafeExec(c, []string{"ls"}, nil, &out, &errb)
	require.True(t, refused)
	require.Equal(t, 1, code)
	require.Equal(t, strings.Replace(want, "NEED", "gash run --safe", 1), errb.String())
	require.Empty(t, out.String())
}

// pinMacStubs pins a macOS without the Command Line Tools: git and
// python3 are the /usr/bin stubs, every other program comes from the
// caller's PATH.
func pinMacStubs(t *testing.T) {
	t.Helper()
	t.Cleanup(prereq.SetSystem(&prereq.System{
		OS:      platform.OS{GOOS: "darwin", ID: "macos", Name: "macOS 15.1"},
		Manager: prereq.Brew, Brew: "/opt/homebrew/bin/brew", Env: []string{"PATH=/usr/bin:/bin"},
		LookPath: func(name string, env []string) (string, bool) {
			if name == "git" || name == "python3" {
				return "/usr/bin/" + name, true
			}
			p, err := runner.LookPath(name, env, "")
			return p, err == nil
		},
		Xcode: func(string) bool { return false },
	}))
}

// TestGitXcodeStub: on macOS without the Command Line Tools /usr/bin/git
// is a stub, reported as git_not_found with the install command for
// every git function and llm_find --tracked (3.4 before: the stub ran,
// and its failure read as not_a_git_repo or a git error).
func TestGitXcodeStub(t *testing.T) {
	pinMacStubs(t)
	d := physTemp(t)
	m := New()
	m.run = runner.NewFake(t)
	env := baseEnv(physTemp(t), nil)
	want := `{"error":"git_not_found","details":"NEED needs git, which is not installed","action":"CONTINUE","recoverable":true,` +
		`"hint":"Install git: brew install git. On macOS git comes with the Command Line Tools: xcode-select --install","missing":"git","install":"brew install git"}` + "\n"
	for _, tc := range []struct {
		need string
		args []string
	}{
		{"llm_git_status", nil}, {"llm_git_status", []string{"--summary"}},
		{"llm_git_diff", nil}, {"llm_git_diff", []string{"--numstat"}},
		{"llm_git_log", nil}, {"llm_git_log", []string{"--fields", "subject"}},
		{"llm_git_show", []string{"HEAD"}}, {"llm_git_overview", nil},
		{"llm_find --tracked", []string{"*", "--tracked"}},
	} {
		name := strings.Fields(tc.need)[0]
		r := runGo(t, m, d, env, name, tc.args...)
		require.Equal(t, result{"", strings.Replace(want, "NEED", tc.need, 1), 1}, r, "%s %v", name, tc.args)
	}
}

// TestLintPythonXcodeStub: llm_lint on .py files with the macOS python3
// stub reports python3 as missing, with the install command (3.4
// before: the stub ran for each file and every file failed).
func TestLintPythonXcodeStub(t *testing.T) {
	pinMacStubs(t)
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "a.py"), "x = 1\n")
	mustWrite(t, filepath.Join(d, "b.py"), "y = 2\n")
	m := New()
	m.run = runner.NewFake(t)
	r := runGo(t, m, d, baseEnv(physTemp(t), nil), "llm_lint")
	require.Equal(t, result{"", `{"error":"runtime_not_installed","details":"llm_lint on 2 .py files needs python3, which is not installed","action":"STOP","recoverable":true,` +
		`"hint":"Install python: brew install python; or leave .py out with --ext","missing":"python3","install":"brew install python"}` + "\n", 1}, r)
}
