// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/remote/sshtest"
)

// hostScene is a local HOME with a HOST: entry, a fake remote home and the
// fake ssh.
type hostScene struct {
	t     *testing.T
	home  string
	rhome string
	ssh   *sshtest.SSH
	env   map[string]string
	exe   string
}

func newHostScene(t *testing.T) *hostScene {
	t.Helper()
	home, rhome := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gash_env"), []byte("HOST:web=deploy@web.example\nHOST:db=admin@db.example\nHOST:bad=-oProxyCommand=x@h\n"), 0o600))
	// The "binary" shipped in these tests: a script that answers version
	// and echoes the rest.
	exe := filepath.Join(t.TempDir(), "gash")
	require.NoError(t, os.WriteFile(exe, []byte("#!/bin/sh\ncase $1 in version) echo 3.4.0 ;; *) printf '{\"fn\":\"%s\",\"argc\":%d}\\n' \"$1\" $(($#-1)) ;; esac\n"), 0o755))
	return &hostScene{t: t, home: home, rhome: rhome, ssh: &sshtest.SSH{Home: rhome}, exe: exe,
		env: map[string]string{"HOME": home, "PATH": os.Getenv("PATH"), "GASH_HEADLESS": "1"}}
}

func (s *hostScene) hoster() Hoster {
	return Hoster{Run: s.ssh, Exe: func() (string, error) { return s.exe, nil }}
}

func (s *hostScene) call(h invoke.Handler, args ...string) (string, string, int) {
	var out, errb bytes.Buffer
	c := &invoke.Call{Ctx: context.Background(), Name: "llm_x", Args: args, Env: invoke.EnvMap(s.env), Stdout: &out, Stderr: &errb}
	rc := h(c)
	return out.String(), errb.String(), rc
}

// stubUname puts a uname that reports another platform first on the remote
// PATH, so the dispatch takes the fallback.
func (s *hostScene) stubUname() {
	dir := s.t.TempDir()
	require.NoError(s.t, os.WriteFile(filepath.Join(dir, "uname"), []byte("#!/bin/sh\ncase $1 in -s) echo SunOS ;; -m) echo sparc64 ;; *) echo SunOS sparc64 ;; esac\n"), 0o755))
	s.ssh.Path = dir + ":" + os.Getenv("PATH")
}

func local(c *invoke.Call) int {
	_, _ = c.Stdout.Write([]byte("local\n"))
	return 0
}

func TestWrapWithoutHostIsTheLocalCall(t *testing.T) {
	s := newHostScene(t)
	h := s.hoster().Wrap("llm_x", Spec{PathFrom: 0}, local)
	out, errOut, rc := s.call(h, "a", "b")
	require.Equal(t, "local\n", out)
	require.Empty(t, errOut)
	require.Zero(t, rc)
	require.Zero(t, s.ssh.N())
	// --help anywhere stays local too.
	out, _, _ = s.call(h, "--host", "web", "--help")
	require.Equal(t, "local\n", out)
	require.Zero(t, s.ssh.N())
}

func TestWrapRefusesBeforeConnecting(t *testing.T) {
	s := newHostScene(t)
	h := s.hoster().Wrap("llm_x", Spec{PathFrom: 0, Values: []string{"-n"}}, local)
	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"--host"}, "missing_value/RETRY"},
		{[]string{"--host", "nope", "x"}, "unknown_host/STOP"},
		{[]string{"--host", "bad", "x"}, "unknown_host/STOP"},
		{[]string{"--host", "deploy@web.example", "x"}, "unknown_host/STOP"},
		{[]string{"--host", "web", "../x"}, "path_traversal_blocked/FATAL"},
		{[]string{"--host", "web", "/proc/1"}, "forbidden_path/FATAL"},
		{[]string{"--host", "web", "/etc/ssh/sshd_config"}, "forbidden_path/FATAL"},
		{[]string{"--host", "web", "/usr/bin"}, "forbidden_path/FATAL"},
		{[]string{"--host", "web", "~bob/x"}, "invalid_path/RETRY"},
	} {
		_, errOut, rc := s.call(h, tc.args...)
		require.Equal(t, 1, rc, tc.args)
		require.Equal(t, tc.code, code(t, errOut), tc.args)
	}
	require.Zero(t, s.ssh.N(), "nothing may connect before the rails")
	// A value of a value option is never a path nor --host.
	_, _, rc := s.call(h, "-n", "/proc", "--host", "web", "x")
	require.Zero(t, rc)
}

func code(t *testing.T, s string) string {
	t.Helper()
	var e struct{ Error, Action string }
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(s)), &e), s)
	return e.Error + "/" + e.Action
}

func TestWrapShipsOnceThenOneInvocation(t *testing.T) {
	if !shipsHere() {
		t.Skip("no binary is shipped from this platform")
	}
	s := newHostScene(t)
	h := s.hoster().Wrap("llm_x", Spec{PathFrom: 0}, local)
	out, errOut, rc := s.call(h, "--host", "web", "a", "b")
	require.Zero(t, rc, errOut)
	require.Equal(t, `{"host":"web","fn":"llm_x","argc":2}`+"\n", out)
	require.Equal(t, 2, s.ssh.N(), "first contact: dispatch, then ship and run")
	require.Equal(t, "deploy@web.example", s.ssh.Targets[0])
	exe, _ := os.ReadFile(s.exe)
	require.Equal(t, exe, s.ssh.Stdins[1], "the binary travels on stdin")
	b, ok := LocalBinary(func() (string, error) { return s.exe, nil })
	require.True(t, ok)
	cached := filepath.Join(s.rhome, b.Dir, "gash")
	st, err := os.Stat(cached)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), st.Mode().Perm())

	out, _, rc = s.call(h, "--host", "web", "c")
	require.Zero(t, rc)
	require.Equal(t, `{"host":"web","fn":"llm_x","argc":1}`+"\n", out)
	require.Equal(t, 3, s.ssh.N(), "cached: one invocation per call")
}

func shipsHere() bool {
	return platformCase(runtime.GOOS, runtime.GOARCH) != ""
}

func TestWrapFallbackOnAnotherPlatform(t *testing.T) {
	s := newHostScene(t)
	s.stubUname()
	spec := Spec{PathFrom: 0, Fallback: func(c *invoke.Call, nonce string, args []string) (*Plan, bool) {
		if len(args) > 0 && args[0] == "--needs-binary" {
			return nil, false
		}
		return &Plan{Script: `printf '%s\n' "fallback $(uname -s)"; exit 3`,
			Finish: func(h Host, out, errOut []byte, st int) ([]byte, []byte, int) {
				return append([]byte("finished "), out...), errOut, st - 3
			}}, true
	}}
	h := s.hoster().Wrap("llm_x", spec, local)
	out, errOut, rc := s.call(h, "--host", "web", "x")
	require.Zero(t, rc, errOut)
	require.Equal(t, "finished fallback SunOS\n", out)
	require.Equal(t, 1, s.ssh.N())

	_, errOut, rc = s.call(h, "--host", "web", "--needs-binary")
	require.Equal(t, 1, rc)
	require.Equal(t, "remote_binary_unavailable/STOP", code(t, errOut))
	require.Contains(t, errOut, "SunOS sparc64")
	require.Equal(t, 2, s.ssh.N())
}

func TestWrapPlanErrorRefusesLocally(t *testing.T) {
	s := newHostScene(t)
	spec := Spec{PathFrom: 0, Fallback: func(c *invoke.Call, nonce string, args []string) (*Plan, bool) {
		e := contract.New("missing_pattern", "", "")
		return &Plan{Err: &e}, true
	}}
	_, errOut, rc := s.call(s.hoster().Wrap("llm_x", spec, local), "--host", "web")
	require.Equal(t, 1, rc)
	require.Equal(t, "missing_pattern/RETRY", code(t, errOut))
	require.Zero(t, s.ssh.N())
}

func TestWrapUnreachable(t *testing.T) {
	s := newHostScene(t)
	s.ssh.Unreachable = true
	_, errOut, rc := s.call(s.hoster().Wrap("llm_x", Spec{PathFrom: 0}, local), "--host", "web")
	require.Equal(t, 1, rc)
	require.Equal(t, "host_unreachable/RETRY", code(t, errOut))
	require.Contains(t, errOut, "Connection refused")
	require.Equal(t, 1, s.ssh.N(), "no second probe")
}

func TestWrapRemoteStatusAndStderrPassThrough(t *testing.T) {
	s := newHostScene(t)
	require.NoError(t, os.WriteFile(s.exe, []byte("#!/bin/sh\necho '{\"error\":\"path_not_found\"}' >&2; exit 255\n"), 0o755))
	b, _ := LocalBinary(func() (string, error) { return s.exe, nil })
	require.NoError(t, os.MkdirAll(filepath.Join(s.rhome, b.Dir), 0o700))
	data, _ := os.ReadFile(s.exe)
	require.NoError(t, os.WriteFile(filepath.Join(s.rhome, b.Dir, "gash"), data, 0o700))
	out, errOut, rc := s.call(s.hoster().Wrap("llm_x", Spec{PathFrom: 0}, local), "--host", "web")
	require.Equal(t, 255, rc, "a remote 255 is the function's status, not a dead connection")
	require.Empty(t, out)
	require.Equal(t, `{"error":"path_not_found"}`+"\n", errOut)
	require.Equal(t, 1, s.ssh.N())
}

func TestWrapSeveralHosts(t *testing.T) {
	s := newHostScene(t)
	s.stubUname()
	spec := Spec{PathFrom: 0, Fallback: func(c *invoke.Call, nonce string, args []string) (*Plan, bool) {
		return &Plan{Script: `printf '{"n":1}\n'`}, true
	}}
	out, errOut, rc := s.call(s.hoster().Wrap("llm_x", spec, local), "--host", "web", "--host", "db")
	require.Zero(t, rc, errOut)
	require.Equal(t, `[{"host":"web","n":1},{"host":"db","n":1}]`+"\n", out)
	require.Equal(t, 2, s.ssh.N())
}

func TestAddHost(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`{"a":1}` + "\n", `{"host":"web","a":1}` + "\n"},
		{`{}`, `{"host":"web"}`},
		{"{\n  \"a\": 1\n}\n", "{\n  \"host\": \"web\",\n  \"a\": 1\n}\n"},
		{`[{"p":"x"},{"p":"y"}]` + "\n", `[{"host":"web","p":"x"},{"host":"web","p":"y"}]` + "\n"},
		{`[]`, `[]`},
		{`["a","b"]`, `["a","b"]`},
		{`[{"p":1},2]`, `[{"p":1},2]`},
		{"{\"a\":1}\n{\"a\":2}\n", "{\"host\":\"web\",\"a\":1}\n{\"host\":\"web\",\"a\":2}\n"},
		{"/srv/app/a.php\n/srv/app/b.php\n", "/srv/app/a.php\n/srv/app/b.php\n"},
		{"col\tn\nx\t1\n", "col\tn\nx\t1\n"},
		{`{"host":"web1","load":[1]}`, `{"host":"web1","load":[1]}`},
		{`42`, `42`},
		{``, ``},
	} {
		require.Equal(t, tc.want, string(AddHost([]byte(tc.in), "web")), tc.in)
	}
}

func TestCombineHosts(t *testing.T) {
	h := func(a, out string) HostOut { return HostOut{Host: Host{Alias: a}, Stdout: []byte(out)} }
	require.Equal(t, `[{"host":"a","x":1},{"host":"b","x":2}]`+"\n",
		string(CombineHosts([]HostOut{h("a", `{"x":1}`+"\n"), h("b", `{"x":2}`+"\n")}, false)))
	require.Equal(t, `[{"host":"a","p":1},{"host":"b","p":2},{"host":"b","p":3}]`+"\n",
		string(CombineHosts([]HostOut{h("a", `[{"p":1}]`), h("b", `[{"p":2},{"p":3}]`), h("c", `[]`)}, false)))
	require.Equal(t, "==> a <==\nx\n==> b <==\ny\n", string(CombineHosts([]HostOut{h("a", "x\n"), h("b", "y")}, false)))
	require.Equal(t, "==> a <==\n{\"x\":1}\n==> b <==\n{}\n", string(CombineHosts([]HostOut{h("a", "{\"x\":1}\n"), h("b", "{}\n")}, true)))
}

func TestCheckRails(t *testing.T) {
	for p, want := range map[string]string{
		"":                        "",
		"src":                     "",
		"~/app":                   "",
		"/srv/www/app":            "",
		"/etc/nginx/nginx.conf":   "",
		"/var/log/syslog":         "",
		"/etc/shadow":             "forbidden_path",
		"/root/x":                 "forbidden_path",
		"/etc/ssh":                "forbidden_path",
		"/usr/lib/x":              "forbidden_path",
		"//proc//1":               "forbidden_path",
		"a/../b":                  "path_traversal_blocked",
		"~alice":                  "invalid_path",
		"/srv/*.php":              "",
		"/home/u/.ssh/id_rsa.pub": "",
	} {
		got := ""
		if e := CheckRead(p); e != nil {
			got = e.Code
		}
		require.Equal(t, want, got, p)
	}
	for _, tc := range []struct {
		p     string
		force bool
		want  string
	}{
		{"/srv/app/index.php", false, ""},
		{"app/config.php", false, ""},
		{"/etc/nginx/sites-enabled/x", false, "sensitive_path"},
		{"/etc/nginx/sites-enabled/x", true, ""},
		{"/home/u/.ssh/authorized_keys", true, "secret_file_blocked"},
		{"/srv/git/.sshkeys/x", false, ""},
		{"/usr/bin/php", true, "write_path_denied"},
		{"/etc/passwd", true, "forbidden_path"},
		{"/srv/app/.env", true, "secret_file_blocked"},
		{"", false, "missing_file"},
	} {
		got := ""
		if e := CheckWrite(tc.p, tc.force); e != nil {
			got = e.Code
		}
		require.Equal(t, tc.want, got, tc.p)
	}
}

func TestShForbidden(t *testing.T) {
	script := ShForbidden() + `for p in "$@"; do set -- $p; if gash_forbidden "$1" "$2"; then echo "$1 $2 yes"; else echo "$1 $2 no"; fi; done`
	run := func(args ...string) string {
		out, err := exec.Command("sh", append([]string{"-c", script, "sh"}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
		return string(out)
	}
	got := run("/proc", "/proc/1", "/etc/shadow", "/srv/app", "/usr/bin/x", "/etc/ssh", "/ walk", "/etc walk", "/etc names", "/srv walk", "/var names")
	require.Equal(t, "/proc  yes\n/proc/1  yes\n/etc/shadow  yes\n/srv/app  no\n/usr/bin/x  yes\n/etc/ssh  yes\n"+
		"/ walk yes\n/etc walk yes\n/etc names no\n/srv walk no\n/var names no\n", got)
}

func TestCutTrailer(t *testing.T) {
	n := NewNonce()
	out, tag, st, detail := cutTrailer([]byte("warn\n\n"+n+" rc 3\n"), n)
	require.Equal(t, "warn\n", string(out))
	require.Equal(t, "rc", tag)
	require.Equal(t, 3, st)
	require.Empty(t, detail)
	out, tag, _, detail = cutTrailer([]byte("no newline\n"+n+" nobin 0 SunOS sparc64\n"), n)
	require.Equal(t, "no newline", string(out))
	require.Equal(t, "nobin", tag)
	require.Equal(t, "SunOS sparc64", detail)
	out, tag, _, _ = cutTrailer([]byte("ssh: refused\n"), n)
	require.Equal(t, "ssh: refused\n", string(out))
	require.Empty(t, tag)
}
