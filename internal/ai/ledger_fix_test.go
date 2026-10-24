// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
