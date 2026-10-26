// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
)

func TestLineScan(t *testing.T) {
	d := physTemp(t)
	p := filepath.Join(d, "app.log")
	appendTo := func(s string) {
		f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		require.NoError(t, err)
		_, err = f.WriteString(s)
		require.NoError(t, err)
		require.NoError(t, f.Close())
	}
	for _, re := range []string{`MARK`, `^ok MARK \d+$`, `(?i)mark \d`} {
		require.NoError(t, os.RemoveAll(p))
		s := newLineScan(p, regexp.MustCompile(re))
		// Missing file: nothing yet.
		_, found, changed := s.poll(nil)
		require.False(t, found)
		require.False(t, changed)
		appendTo("one\ntwo\n")
		_, found, changed = s.poll(nil)
		require.False(t, found, re)
		require.True(t, changed, re)
		require.Equal(t, int64(8), s.off)
		// A partial last line is matched before its newline comes.
		appendTo("ok MARK 7")
		line, found, _ := s.poll(nil)
		require.True(t, found, re)
		require.Equal(t, "ok MARK 7", line)
	}

	// Only appended bytes are read after the first scan; a truncated file
	// is read again from its start, a replaced one too; CRLF is one line.
	require.NoError(t, os.RemoveAll(p))
	s := newLineScan(p, regexp.MustCompile(`^done$`))
	appendTo("a\nb\n")
	_, found, _ := s.poll(nil)
	require.False(t, found)
	appendTo("c\r\ndone\r\n")
	line, found, _ := s.poll(nil)
	require.True(t, found)
	require.Equal(t, "done", line)
	require.NoError(t, os.WriteFile(p, []byte("x\n"), 0o644))
	_, found, _ = s.poll(nil)
	require.False(t, found)
	require.Equal(t, int64(2), s.off)
	require.NoError(t, os.WriteFile(p+".new", []byte("done\n"), 0o644))
	require.NoError(t, os.Rename(p+".new", p))
	_, found, changed := s.poll(nil)
	require.True(t, found)
	require.True(t, changed)

	// A long scan stops at stop() and goes on from its offset.
	big := strings.Repeat("filler line\n", 300000) + "needle\n"
	require.NoError(t, os.WriteFile(p, []byte(big), 0o644))
	s = newLineScan(p, regexp.MustCompile(`needle`))
	_, found, _ = s.poll(func() bool { return true })
	require.False(t, found)
	require.Equal(t, int64(scanChunk), s.off)
	line, found, _ = s.poll(nil)
	require.True(t, found)
	require.Equal(t, "needle", line)
}

func TestLineScanMatch(t *testing.T) {
	for _, tc := range []struct {
		re, block, want string
	}{
		{`b+c`, "abc\nxbbc\n", "abc"},
		{`^b+c`, "abc\nbbc\n", "bbc"},
		{`x$`, "x y\nyx\n", "yx"},
		{`zz`, "a\nb\n", ""},
		{`[0-9]{3}`, "12\n345\n", "345"},
		{`a.c`, "a\nc\nabc", "abc"},
	} {
		s := newLineScan("", regexp.MustCompile(tc.re))
		got, ok := s.match([]byte(tc.block))
		require.Equal(t, tc.want, got, tc.re)
		require.Equal(t, tc.want != "", ok, tc.re)
	}
}

func TestTailFile(t *testing.T) {
	d := physTemp(t)
	p := filepath.Join(d, "f")
	var b strings.Builder
	for i := 1; i <= 50000; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	require.NoError(t, os.WriteFile(p, []byte(b.String()), 0o644))
	require.Equal(t, []string{"line 49998", "line 49999", "line 50000"}, tailFile(p, 3))
	require.NoError(t, os.WriteFile(p, []byte("a\nb"), 0o644))
	require.Equal(t, []string{"a", "b"}, tailFile(p, 5))
	require.Nil(t, tailFile(filepath.Join(d, "none"), 2))
}

// waitJSON decodes the answer of llm_wait.
func waitJSON(t *testing.T, r result) map[string]any {
	t.Helper()
	var v map[string]any
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &v), r.Stdout+r.Stderr)
	return v
}

func TestWaitFileMatch(t *testing.T) {
	d := physTemp(t)
	env := baseEnv(physTemp(t), nil)
	log := filepath.Join(d, "app.log")
	mustWrite(t, log, "start\nold line\n")

	// Already there: met at once.
	r := runGo(t, New(), d, env, "llm_wait", "--timeout", "5", "--file-match", "app.log", "old")
	require.Equal(t, 0, r.RC, r.Stderr)
	v := waitJSON(t, r)
	require.Equal(t, true, v["met"])
	require.Equal(t, "old line", v["match"])
	require.Less(t, v["after_s"].(float64), 1.0)
	require.NotContains(t, v, "tail")

	// Appended later: met when it comes, with the tail and the width.
	go func() {
		time.Sleep(300 * time.Millisecond)
		f, _ := os.OpenFile(log, os.O_APPEND|os.O_WRONLY, 0o644)
		_, _ = f.WriteString("noise\nREADY " + strings.Repeat("y", 40) + "\n")
		_ = f.Close()
	}()
	start := time.Now()
	r = runGo(t, New(), d, env, "llm_wait", "--timeout", "10", "--file-match", log, "^READY", "--tail", "2", "--width", "10")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Less(t, time.Since(start), 3*time.Second)
	v = waitJSON(t, r)
	require.Equal(t, "READY yyy…", v["match"])
	require.Equal(t, []any{"noise", "READY yyy…"}, v["tail"])
	require.GreaterOrEqual(t, v["after_s"].(float64), 0.2)

	// Timeout: met:false, exit 1, nothing on stderr.
	r = runGo(t, New(), d, env, "llm_wait", "--timeout", "300ms", "--file-match", log, "NEVER", "--tail", "1")
	require.Equal(t, 1, r.RC)
	require.Empty(t, r.Stderr)
	require.Regexp(t, `^\{"met":false,"after_s":0\.[3-9]\d*,"tail":\["READY `, r.Stdout)

	// A file that does not exist yet is waited for.
	late := filepath.Join(d, "late.log")
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = os.WriteFile(late, []byte("hello\n"), 0o644)
	}()
	r = runGo(t, New(), d, env, "llm_wait", "--timeout", "5", "--file-match", late, "hel+o")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Equal(t, "hello", waitJSON(t, r)["match"])
}

func TestWaitExistsAndGone(t *testing.T) {
	d := physTemp(t)
	env := baseEnv(physTemp(t), nil)
	p := filepath.Join(d, "flag")
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = os.WriteFile(p, nil, 0o644)
	}()
	r := runGo(t, New(), d, env, "llm_wait", "--timeout", "5", "--exists", "flag")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Equal(t, true, waitJSON(t, r)["met"])
	r = runGo(t, New(), d, env, "llm_wait", "--timeout", "0.2", "--exists", "nothing")
	require.Equal(t, 1, r.RC)
	require.Empty(t, r.Stderr)
	require.Regexp(t, `^\{"met":false,"after_s":0\.[2-9][0-9]?\}\n$`, r.Stdout) // elapsed time, 0.2 or a little more

	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("no sleep")
	}
	marker := "1" + strconv.Itoa(os.Getpid()%1000)
	// The sleep is a grandchild (a child of this process would count as a
	// helper of the in-process call): bash starts it and prints its PID.
	sh := exec.Command("bash", "--noprofile", "--norc", "-c", "sleep 30."+marker+" & echo $!; wait")
	stdout, err := sh.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, sh.Start())
	t.Cleanup(func() {
		_ = sh.Process.Kill()
		_ = sh.Wait()
	})
	var pid int
	_, err = fmt.Fscan(stdout, &pid)
	require.NoError(t, err)
	r = runGo(t, New(), d, env, "llm_wait", "--timeout", "0.3", "--gone", strconv.Itoa(pid))
	require.Equal(t, 1, r.RC)
	r = runGo(t, New(), d, env, "llm_wait", "--timeout", "0.6", "--gone", `^sleep 30\.`+marker)
	require.Equal(t, 1, r.RC, r.Stderr)
	go func() {
		time.Sleep(300 * time.Millisecond)
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Kill()
		}
	}()
	r = runGo(t, New(), d, env, "llm_wait", "--timeout", "5", "--gone", strconv.Itoa(pid))
	require.Equal(t, 0, r.RC, r.Stdout+r.Stderr)
	r = runGo(t, New(), d, env, "llm_wait", "--timeout", "5", "--gone", `^sleep 30\.`+marker)
	require.Equal(t, 0, r.RC, r.Stdout+r.Stderr)
	// The caller's own command line never counts: this test binary's
	// name matches, and the wait is met at once.
	exe, _ := os.Executable()
	r = runGo(t, New(), d, env, "llm_wait", "--timeout", "5", "--gone", regexp.QuoteMeta(filepath.Base(exe)))
	require.Equal(t, 0, r.RC, r.Stdout+r.Stderr)
}

func TestWaitHTTP(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch k := n.Add(1); {
		case k < 2:
			w.WriteHeader(503)
		case k < 3:
			_, _ = w.Write([]byte(`{"status":"starting","n":1}`))
		default:
			_, _ = w.Write([]byte(`{"status":"ok","n":2.50,"ready":true}`))
		}
	}))
	defer srv.Close()
	d := physTemp(t)
	env := baseEnv(physTemp(t), map[string]string{"HTTP_PROXY": "http://127.0.0.1:1"})
	r := runGo(t, New(), d, env, "llm_wait", "--timeout", "5", "--interval", "50ms", "--http", srv.URL, "--json-path", "status", "--equals", "ok")
	require.Equal(t, 0, r.RC, r.Stdout+r.Stderr)
	v := waitJSON(t, r)
	require.Equal(t, float64(200), v["code"])
	require.NotContains(t, v, "last")
	for _, tc := range [][2]string{{"n", "2.5"}, {"ready", "true"}} {
		r = runGo(t, New(), d, env, "llm_wait", "--timeout", "1", "--http", srv.URL+"/x", "--json-path", tc[0], "--equals", tc[1])
		require.Equal(t, 0, r.RC, r.Stdout+r.Stderr)
	}
	r = runGo(t, New(), d, env, "llm_wait", "--timeout", "0.3", "--http", srv.URL, "--code", "404")
	require.Equal(t, 1, r.RC)
	v = waitJSON(t, r)
	require.Equal(t, "HTTP 200", v["last"])
	r = runGo(t, New(), d, env, "llm_wait", "--timeout", "0.3", "--http", srv.URL, "--json-path", "status", "--equals", "down")
	require.Equal(t, "status = ok", waitJSON(t, r)["last"])
	r = runGo(t, New(), d, env, "llm_wait", "--timeout", "0.3", "--http", srv.URL, "--json-path", "a.b", "--equals", "x")
	require.Equal(t, "a.b not found", waitJSON(t, r)["last"])
}

func TestWaitPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	d := physTemp(t)
	env := baseEnv(physTemp(t), nil)
	r := runGo(t, New(), d, env, "llm_wait", "--timeout", "2", "--port", addr)
	require.Equal(t, 0, r.RC, r.Stdout+r.Stderr)
	require.NoError(t, ln.Close())
	r = runGo(t, New(), d, env, "llm_wait", "--timeout", "0.3", "--port", addr)
	require.Equal(t, 1, r.RC)
	require.Equal(t, "connection refused", waitJSON(t, r)["last"])
}

func TestWaitCmdOK(t *testing.T) {
	d := physTemp(t)
	env := baseEnv(physTemp(t), nil)
	// Through the fake runner: the llm_exec script runs until it exits 0.
	m := New()
	runs := 0
	m.run = runner.NewFake(t).Respond(func(c runner.Call) (runner.Expect, bool) {
		if c.Use != runner.BashLLMExec {
			return runner.Expect{Status: 1}, true // the tool probes
		}
		runs++
		if runs < 3 {
			return runner.Expect{Stdout: "not yet\n", Status: 2}, true
		}
		return runner.Expect{Stdout: "a\nb\nready\n"}, true
	})
	r := runGo(t, m, d, env, "llm_wait", "--timeout", "5", "--interval", "10ms", "--cmd-ok", "ls x", "--tail", "2")
	require.Equal(t, 0, r.RC, r.Stdout+r.Stderr)
	v := waitJSON(t, r)
	require.Equal(t, float64(3), v["runs"])
	require.Equal(t, float64(0), v["rc"])
	require.Equal(t, []any{"b", "ready"}, v["tail"])

	// The llm_exec rails apply before anything runs.
	for _, tc := range [][2]string{{"rm -rf /tmp/x", "dangerous_command_blocked"}, {"touch x", "command_not_allowed"}, {"cat .env", "secret_file_blocked"}} {
		r = runGo(t, New(), d, env, "llm_wait", "--timeout", "5", "--cmd-ok", tc[0])
		require.Equal(t, 1, r.RC)
		require.Empty(t, r.Stdout)
		require.Contains(t, r.Stderr, `"error":"`+tc[1]+`"`, tc[0])
	}

	// For real: ls fails until the file appears.
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(d, "built"), nil, 0o644)
	}()
	r = runGo(t, New(), d, env, "llm_wait", "--timeout", "5", "--interval", "100ms", "--cmd-ok", "ls built")
	require.Equal(t, 0, r.RC, r.Stdout+r.Stderr)
	require.Greater(t, waitJSON(t, r)["runs"].(float64), float64(1))
}

func TestWaitErrors(t *testing.T) {
	d := physTemp(t)
	env := baseEnv(physTemp(t), nil)
	mustWrite(t, filepath.Join(d, ".env"), "A=1\n")
	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"--exists", "x"}, "missing_value"},
		{[]string{"--timeout"}, "missing_value"},
		{[]string{"--timeout", "1"}, "missing_condition"},
		{[]string{"--timeout", "0", "--exists", "x"}, "invalid_timeout"},
		{[]string{"--timeout", "2h", "--exists", "x"}, "invalid_timeout"},
		{[]string{"--timeout", "1", "--interval", "x", "--exists", "x"}, "invalid_interval"},
		{[]string{"--timeout", "1", "--exists", "x", "--gone", "1"}, "unexpected_argument"},
		{[]string{"--timeout", "1", "stray"}, "unexpected_argument"},
		{[]string{"--timeout", "1", "--bogus"}, "unknown_option"},
		{[]string{"--timeout", "1", "--file-match", "f"}, "missing_value"},
		{[]string{"--timeout", "1", "--file-match", "f", "("}, "invalid_regex"},
		{[]string{"--timeout", "1", "--file-match", ".env", "A"}, "secret_file_blocked"},
		{[]string{"--timeout", "1", "--file-match", "/etc/shadow", "A"}, "forbidden_path"},
		{[]string{"--timeout", "1", "--file-match", "../x", "A"}, "path_traversal_blocked"},
		{[]string{"--timeout", "1", "--file-match", ".", "A"}, "unexpected_argument"},
		{[]string{"--timeout", "1", "--tail", "x", "--exists", "a"}, "invalid_tail"},
		{[]string{"--timeout", "1", "--width", "0", "--exists", "a"}, "invalid_width"},
		{[]string{"--timeout", "1", "--http", "ftp://x"}, "invalid_url"},
		{[]string{"--timeout", "1", "--http", "http://x", "--code", "99"}, "invalid_code"},
		{[]string{"--timeout", "1", "--http", "http://x", "--json-path", "a"}, "unexpected_argument"},
		{[]string{"--timeout", "1", "--http", "http://x", "--json-path", "a[", "--equals", "1"}, "invalid_json_path"},
		{[]string{"--timeout", "1", "--exists", "a", "--code", "200"}, "unexpected_argument"},
		{[]string{"--timeout", "1", "--port", "nohost"}, "invalid_port"},
		{[]string{"--timeout", "1", "--port", "h:0"}, "invalid_port"},
		{[]string{"--timeout", "1", "--port", "h:http"}, "invalid_port"},
		{[]string{"--timeout", "1", "--gone", ""}, "invalid_pid"},
		{[]string{"--timeout", "1", "--gone", "12345678901"}, "invalid_pid"},
		{[]string{"--timeout", "1", "--gone", "a("}, "invalid_regex"},
		{[]string{"--timeout", "1", "--cmd-ok", ""}, "empty_command"},
	} {
		r := runGo(t, New(), d, env, "llm_wait", tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Empty(t, r.Stdout, tc.args)
		require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, tc.args)
	}
}

// TestWaitCanceled: the end of the call's context (Ctrl-C) ends the wait
// with met:false.
func TestWaitCanceled(t *testing.T) {
	d := physTemp(t)
	t.Chdir(d)
	tbl := invoke.NewTable()
	New().Register(tbl)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	var out, errb bytes.Buffer
	e := invoke.EnvMap(baseEnv(physTemp(t), nil))
	c := &invoke.Call{Ctx: ctx, Name: "llm_wait", Args: []string{"--timeout", "30", "--exists", "never"}, Env: e,
		Stdin: bytes.NewReader(nil), Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, false, false)}
	start := time.Now()
	require.Equal(t, 1, tbl.Run(c))
	require.Less(t, time.Since(start), 2*time.Second)
	require.Contains(t, out.String(), `"met":false`)
}
