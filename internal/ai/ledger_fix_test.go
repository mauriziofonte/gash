// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/ui"
)

// ttyAnswer makes m's controlling terminal type answer once per open.
func ttyAnswer(t *testing.T, m *Module, answer string) {
	t.Helper()
	m.openTTY = func() (*os.File, error) {
		r, w, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		_, _ = w.WriteString(answer)
		_ = w.Close()
		return r, nil
	}
}

// writeFile writes body to root/rel, creating its directories.
func writeFile(t *testing.T, root, rel, body string) string {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestLedger_AI01_fix: a path named in an ai_sysinfo question passes the
// llm_* path guard (forbidden list, secret names, symlinks resolved) and
// the data collected for a question is confirmed per question.
func TestLedger_AI01_fix(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "etc/shadow", "root:MARKER-SHADOW:19000:0:99999:7:::\n")
	writeFile(t, root, "var/tmp/id_rsa.bak", "MARKER-KEY\n")
	writeFile(t, root, "etc/myapp.conf", "listen = 8080\n")
	if err := os.Symlink("/etc/shadow", filepath.Join(root, "var", "tmp", "innocent.txt")); err != nil {
		t.Fatal(err)
	}
	sr, err := platform.NewSysroot(root)
	if err != nil {
		t.Fatal(err)
	}
	m := New()
	m.sys = sr
	m.run = &fakeRunner{handle: func(name string, args []string) ([]byte, int) {
		if name == "ls" {
			return []byte("total 8\n-rw-r--r-- 1 root root 11 Jan  1 00:00 id_rsa.bak\n" +
				"lrwxrwxrwx 1 root root 11 Jan  1 00:00 innocent.txt -> /etc/shadow\n" +
				"-rw-r--r-- 1 root root  5 Jan  1 00:00 notes.txt\n"), 0
		}
		return nil, 1
	}}
	c, _, _ := newCall(map[string]string{}, nil, false)
	noSudo := func(string, ...string) []byte { return nil }

	for _, q := range []string{
		"what is in /etc/shadow?",
		"show /var/tmp/id_rsa.bak",
		"show /var/tmp/ID_RSA.BAK",
		"read /var/tmp/innocent.txt",
		"read /etc/../etc/shadow",
		"read /etc//shadow",
	} {
		got, kinds := m.questionContext(c, noSudo, q)
		if strings.Contains(got, "MARKER") {
			t.Errorf("%q: protected content collected:\n%s", q, got)
		}
		if !strings.Contains(got, "=== (skipped: protected path)") {
			t.Errorf("%q: no skipped line:\n%s", q, got)
		}
		if len(kinds) != 0 {
			t.Errorf("%q: kinds %v, want none", q, kinds)
		}
	}
	// An ordinary file is still read; a directory listing drops secrets.
	got, kinds := m.questionContext(c, noSudo, "read /etc/myapp.conf and list /var/tmp")
	if !strings.Contains(got, "listen = 8080") || !strings.Contains(got, "notes.txt") {
		t.Fatalf("ordinary data missing:\n%s", got)
	}
	if strings.Contains(got, "id_rsa.bak") || strings.Contains(got, "innocent.txt") {
		t.Fatalf("secret entries listed:\n%s", got)
	}
	if strings.Join(kinds, ",") != "files,directories" {
		t.Fatalf("kinds %v", kinds)
	}

	// The `?` action asks before sending collected data; "n" sends nothing.
	f := newFakeAPI(t)
	drilldownAPI(f)
	fm := testModule(f)
	fm.sys = sr
	fm.run = m.run
	ttyAnswer(t, fm, "n\n")
	cb, _, _ := newCall(map[string]string{}, nil, false)
	sess := fm.si.Begin(cb, false, false)
	defer sess.Release()
	in := strings.NewReader("?\nwhy does /etc/myapp.conf fail\nq\n")
	mc, _, errb := newCall(map[string]string{}, in, true)
	fm.drillMenu(mc, ui.StyleOf(mc.Term, false), sess, "claude", "tok", initialAnalysisJSON)
	if f.count() != 0 {
		t.Fatalf("declined question context was sent (%d requests)", f.count())
	}
	if !strings.Contains(errb.String(), "Send question context (files") {
		t.Fatalf("no per-question confirmation: %q", errb.String())
	}

	// No terminal: the existing refusal, nothing sent.
	fm.openTTY = func() (*os.File, error) { return nil, os.ErrNotExist }
	in = strings.NewReader("?\nwhy does /etc/myapp.conf fail\nq\n")
	mc, _, errb = newCall(map[string]string{}, in, true)
	fm.drillMenu(mc, ui.StyleOf(mc.Term, false), sess, "claude", "tok", initialAnalysisJSON)
	if f.count() != 0 || !strings.Contains(errb.String(), "Not sending question context (files)") {
		t.Fatalf("no-terminal refusal: count %d, stderr %q", f.count(), errb.String())
	}

	// GASH_AI_CONFIRM=0 sends without asking.
	in = strings.NewReader("?\nwhy does /etc/myapp.conf fail\nq\n")
	mc, _, _ = newCall(map[string]string{"GASH_AI_CONFIRM": "0"}, in, true)
	fm.drillMenu(mc, ui.StyleOf(mc.Term, false), sess, "claude", "tok", initialAnalysisJSON)
	if f.count() != 1 || !strings.Contains(string(f.lastBody()), "listen = 8080") {
		t.Fatalf("GASH_AI_CONFIRM=0: count %d", f.count())
	}
}

// TestLedger_AI03_fix: ai_ask answers one question per call; a typed exit,
// quit or q (any case, trimmed) returns 1 with no request and no message.
func TestLedger_AI03_fix(t *testing.T) {
	f := newFakeAPI(t)
	m := testModule(f)
	ttyAnswer(t, m, "")
	home := gashEnvHome(t, "AI:claude=t")
	for _, typed := range []string{"exit\n", "quit\n", " Q \n", "EXIT\n"} {
		c, out, errb := newCall(map[string]string{"HOME": home}, strings.NewReader(typed), true, "claude")
		if rc := m.aiAsk(c); rc != 1 || out.Len() != 0 || errb.Len() != 0 {
			t.Fatalf("%q: rc %d out %q err %q", typed, rc, out.String(), errb.String())
		}
	}
	if f.count() != 0 {
		t.Fatalf("exit/quit sent %d requests", f.count())
	}
	c, out, _ := newCall(map[string]string{"HOME": home}, strings.NewReader("how to exit vim\n"), true, "claude")
	if rc := m.aiAsk(c); rc != 0 || f.count() != 1 || !strings.Contains(out.String(), "Command:") {
		t.Fatalf("a real question: rc %d, %d requests", rc, f.count())
	}
}

// TestLedger_AI04_fix: ai_ask without a terminal (or headless) refuses
// before reading anything: rc 1, no request.
func TestLedger_AI04_fix(t *testing.T) {
	f := newFakeAPI(t)
	m := testModule(f) // openTTY fails: no controlling terminal
	home := gashEnvHome(t, "AI:claude=t")
	want := "Error: ai_ask needs a terminal: use ai_query \"question\" (stdin is read as piped input)\n"
	c, _, errb := newCall(map[string]string{"HOME": home}, strings.NewReader("how to list files\n"), false, "claude")
	if rc := m.aiAsk(c); rc != 1 || errb.String() != want {
		t.Fatalf("no tty: rc %d, err %q", rc, errb.String())
	}
	ttyAnswer(t, m, "how to list files\n")
	c, _, errb = newCall(map[string]string{"HOME": home, "GASH_HEADLESS": "1"}, strings.NewReader("x\n"), false, "claude")
	if rc := m.aiAsk(c); rc != 1 || errb.String() != want {
		t.Fatalf("headless: rc %d, err %q", rc, errb.String())
	}
	if f.count() != 0 {
		t.Fatalf("want no request, got %d", f.count())
	}
}

// TestLedger_AI05_fix: an open but idle non-terminal stdin is ignored after
// the wait, with a warning, instead of blocking until EOF; data that does
// arrive is read.
func TestLedger_AI05_fix(t *testing.T) {
	f := newFakeAPI(t)
	m := testModule(f)
	m.pipeWait = 50 * time.Millisecond
	home := gashEnvHome(t, "AI:claude=t")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()
	c, out, errb := newCall(map[string]string{"HOME": home}, nil, false, "what is ls")
	c.Stdin = r
	done := make(chan int, 1)
	go func() { done <- m.aiQuery(c) }()
	select {
	case rc := <-done:
		if rc != 0 || !strings.Contains(out.String(), "Explanation:") {
			t.Fatalf("rc %d out %q", rc, out.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ai_query blocked on an idle stdin")
	}
	if !strings.Contains(errb.String(), "Warning: stdin is open but sent nothing within 2s: ignored (redirect </dev/null to skip the wait)") {
		t.Fatalf("no warning: %q", errb.String())
	}
	if strings.Contains(claudeUser(t, f.lastBody()), "PIPE INPUT") {
		t.Fatal("idle stdin was treated as piped input")
	}

	// A pipe that delivers data within the wait is read.
	r2, w2, _ := os.Pipe()
	_, _ = w2.WriteString("PHP Fatal error\n")
	_ = w2.Close()
	c, _, errb = newCall(map[string]string{"HOME": home}, nil, false, "what is wrong")
	c.Stdin = r2
	if rc := m.aiQuery(c); rc != 0 || errb.Len() != 0 {
		t.Fatalf("piped: rc %d err %q", rc, errb.String())
	}
	if !strings.Contains(claudeUser(t, f.lastBody()), "PHP Fatal error") {
		t.Fatal("piped data not sent")
	}
}

// TestLedger_AI06_fix: the piped part and the question are redacted
// separately, so an unclosed PRIVATE KEY block in the pipe keeps the
// question and the closing "---" (ai_query and ai_ask).
func TestLedger_AI06_fix(t *testing.T) {
	f := newFakeAPI(t)
	m := testModule(f)
	home := gashEnvHome(t, "AI:claude=t")
	c, _, errb := newCall(map[string]string{"HOME": home},
		strings.NewReader("-----BEGIN RSA PRIVATE KEY-----\nabc\n"), false, "q1")
	if rc := m.aiQuery(c); rc != 0 {
		t.Fatalf("rc %d: %s", rc, errb.String())
	}
	u := claudeUser(t, f.lastBody())
	if strings.Contains(u, "abc") || !strings.Contains(u, "[REDACTED PRIVATE KEY]\n---\nUser question: q1") {
		t.Fatalf("body user:\n%s", u)
	}

	ttyAnswer(t, m, "q2\n")
	c, _, errb = newCall(map[string]string{"HOME": home},
		strings.NewReader("-----BEGIN OPENSSH PRIVATE KEY-----\nxyz\n"), false, "claude")
	if rc := m.aiAsk(c); rc != 0 {
		t.Fatalf("ai_ask rc %d: %s", rc, errb.String())
	}
	u = claudeUser(t, f.lastBody())
	if strings.Contains(u, "xyz") || !strings.Contains(u, "User question: q2") {
		t.Fatalf("ai_ask body user:\n%s", u)
	}
}
