// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
)

// fakeRunner answers Run from a handler keyed on the program and arguments;
// the other Runner methods are inert.
type fakeRunner struct {
	handle func(name string, args []string) ([]byte, int)
	calls  [][]string
}

func (f *fakeRunner) Run(_ context.Context, c runner.Cmd) (runner.Result, error) {
	f.calls = append(f.calls, append([]string{c.Path}, c.Args...))
	if f.handle == nil {
		return runner.Result{}, nil
	}
	out, code := f.handle(c.Path, c.Args)
	return runner.Result{Stdout: out, ExitCode: code}, nil
}

func (f *fakeRunner) RunBash(context.Context, runner.Bash) (runner.Result, error) {
	return runner.Result{}, nil
}
func (f *fakeRunner) RunScript(context.Context, runner.Script) (runner.Result, error) {
	return runner.Result{}, nil
}
func (f *fakeRunner) Start(context.Context, runner.Cmd) error                  { return nil }
func (f *fakeRunner) StartPTY(context.Context, runner.Cmd) (runner.PTY, error) { return nil, nil }

// TestStripTrailingPunct: LEDGER AI-18, the whole trailing punctuation run
// is stripped, interior dots kept.
func TestStripTrailingPunct(t *testing.T) {
	cases := map[string]string{
		"/etc/nginx.conf.":   "/etc/nginx.conf",
		"/etc/nginx.conf?":   "/etc/nginx.conf",
		"/etc/nginx.conf":    "/etc/nginx.conf",
		"/etc/a.conf.,:!":    "/etc/a.conf",
		"/var/log/x.log":     "/var/log/x.log",
		"/etc/foo.d/bar.ini": "/etc/foo.d/bar.ini",
	}
	for in, want := range cases {
		if got := stripTrailingPunct(in); got != want {
			t.Errorf("stripTrailingPunct(%q)=%q want %q", in, got, want)
		}
	}
}

// TestLedger_AI08_portRegex: the bare ":" alternative makes the port regex
// take clock times, not just ports (the reproduced defect).
func TestLedger_AI08_portRegex(t *testing.T) {
	q := "why is 12:30 slow on port 3306 and listening on :8080?"
	var got []string
	for _, mm := range rePortEntity.FindAllStringSubmatch(q, -1) {
		got = append(got, mm[2])
	}
	want := []string{"30", "3306", "8080"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("ports %v want %v", got, want)
	}
}

// TestLedger_AI08_portClockOnly: a lone clock time still yields a "port".
func TestLedger_AI08_portClockOnly(t *testing.T) {
	m := strings.Join(func() []string {
		var r []string
		for _, mm := range rePortEntity.FindAllStringSubmatch("backup runs at 03:15 daily", -1) {
			r = append(r, mm[2])
		}
		return r
	}(), ",")
	if m != "15" {
		t.Fatalf("clock time -> port %q, want 15", m)
	}
}

// TestCollectQuestionContext wires the collector to a fake runner, a temp
// sysroot and a fake sudo, and checks the per-entity blocks and the AI-18
// path strip.
func TestCollectQuestionContext(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "etc", "myapp.conf"),
		[]byte("key = value\n# comment\nother = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sr, err := platform.NewSysroot(root)
	if err != nil {
		t.Fatal(err)
	}
	fr := &fakeRunner{handle: func(name string, args []string) ([]byte, int) {
		switch {
		case name == "systemctl" && len(args) > 0 && args[0] == "cat":
			return []byte("[Unit]\nDescription=My app\n"), 0
		case name == "systemctl" && len(args) > 0 && args[0] == "status":
			return []byte("active (running)\n"), 0
		case name == "ss":
			return []byte("LISTEN 0 128 0.0.0.0:3306 users\nLISTEN 0 128 0.0.0.0:80 users\n"), 0
		}
		return nil, 0
	}}
	m := New()
	m.sys = sr
	m.run = fr
	sudo := func(name string, args ...string) []byte { return []byte("journal: started\n") }

	c, _, _ := newCall(map[string]string{}, nil, false)
	got := m.collectQuestionContext(c, sudo,
		"restart myapp.service, read /etc/myapp.conf. on port 3306")

	for _, want := range []string{
		"=== SYSTEMD UNIT: myapp.service ===",
		"Description=My app",
		"journal: started",
		"=== FILE: /etc/myapp.conf ===", // trailing '.' stripped (AI-18)
		"key = value",
		"=== PORT: 3306 ===",
		":3306",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("context missing %q\n---\n%s", want, got)
		}
	}
	// The port block grep -F keeps only the matching ss line.
	if strings.Contains(got[strings.Index(got, "=== PORT:"):], ":80 ") {
		t.Errorf("port block should not contain the :80 line:\n%s", got)
	}
}

// TestCollectQuestionContext_Dedup: a port named twice yields one block.
func TestCollectQuestionContext_Dedup(t *testing.T) {
	m := New()
	root := t.TempDir()
	sr, _ := platform.NewSysroot(root)
	m.sys = sr
	m.run = &fakeRunner{handle: func(string, []string) ([]byte, int) { return []byte("x\n"), 0 }}
	c, _, _ := newCall(map[string]string{}, nil, false)
	got := m.collectQuestionContext(c, func(string, ...string) []byte { return nil },
		"port 3306 is slow, really port 3306 is slow")
	if n := strings.Count(got, "=== PORT: 3306 ==="); n != 1 {
		t.Fatalf("want one PORT block, got %d:\n%s", n, got)
	}
}

// TestCollectQuestionContext_ServiceKeyword: a keyword with no detected
// unit/path/port still collects its service block.
func TestCollectQuestionContext_ServiceKeyword(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "etc", "nginx"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "etc", "nginx", "nginx.conf"),
		[]byte("# main\nworker_processes auto;\n\nevents {}\n"), 0o644)
	sr, _ := platform.NewSysroot(root)
	m := New()
	m.sys = sr
	m.run = &fakeRunner{handle: func(name string, args []string) ([]byte, int) {
		return []byte("active\n"), 0
	}}
	c, _, _ := newCall(map[string]string{}, nil, false)
	got := m.collectQuestionContext(c, func(string, ...string) []byte { return nil },
		"nginx keeps crashing")
	if !strings.Contains(got, "=== SERVICE: Nginx ===") {
		t.Fatalf("no nginx block:\n%s", got)
	}
	if !strings.Contains(got, "worker_processes auto;") {
		t.Fatalf("nginx.conf key line missing:\n%s", got)
	}
	if strings.Contains(got, "# main") {
		t.Fatalf("grep -Ev '^(#|$)' should drop comment lines:\n%s", got)
	}
}
