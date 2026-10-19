// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// LEDGER SYS-46: wsl_compact's sudo sh -c line runs exactly as 2.x ran
// it, through a registered constant; any other shell string is refused.
func TestLedger_SYS46(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "argv")
	sudo := filepath.Join(dir, "sudo")
	if err := os.WriteFile(sudo, []byte("#!/bin/sh\nfor a in \"$0\" \"$@\"; do printf '%s\\n' \"$a\"; done >"+log+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := Exec{}.RunScript(context.Background(), Script{Name: ScriptWSLCompact, Options: Options{Env: []string{"PATH=" + dir}}})
	if err != nil || !res.Success() {
		t.Fatalf("RunScript: %+v %v", res, err)
	}
	got, _ := os.ReadFile(log)
	want := sudo + "\nsh\n-c\nsync && echo 3 > /proc/sys/vm/drop_caches && echo 1 > /proc/sys/vm/compact_memory\n"
	if string(got) != want {
		t.Errorf("argv = %q, want %q", got, want)
	}

	// A name that is not registered, and the same line through Run.
	if _, err := (Exec{}).RunScript(context.Background(), Script{Name: 99}); !errors.Is(err, ErrScript) {
		t.Errorf("unregistered script: %v", err)
	}
	argv := constScripts[ScriptWSLCompact]
	if _, err := (Exec{}).Run(context.Background(), Cmd{Path: argv[0], Args: argv[1:]}); !errors.Is(err, ErrShellCommand) {
		t.Errorf("Run of the same line: %v", err)
	}
	if err := (Exec{}).Start(context.Background(), Cmd{Path: "sh", Args: []string{"-c", "x"}}); !errors.Is(err, ErrShellCommand) {
		t.Errorf("Start of a shell string: %v", err)
	}
	if _, err := (Exec{}).StartPTY(context.Background(), Cmd{Path: "bash", Args: []string{"-c", "x"}}); !errors.Is(err, ErrShellCommand) {
		t.Errorf("StartPTY of a shell string: %v", err)
	}
	// A Script carries no text a caller could fill: its name and options.
	st := reflect.TypeOf(Script{})
	if st.NumField() != 2 || st.Field(0).Type != reflect.TypeOf(ConstScript(0)) || st.Field(1).Type != reflect.TypeOf(Options{}) {
		t.Errorf("Script has fields beyond Name and Options")
	}

	f := NewFake(t, Expect{Script: ScriptWSLCompact, Argv: argv, Status: 1})
	res, err = f.RunScript(context.Background(), Script{Name: ScriptWSLCompact})
	if err != nil || res.ExitCode != 1 || f.Calls()[0].Script != ScriptWSLCompact {
		t.Errorf("fake RunScript: %+v %v", res, err)
	}
}

// Start returns while the child still runs; the child does its work.
func TestStart(t *testing.T) {
	mark := filepath.Join(t.TempDir(), "mark")
	begin := time.Now()
	if err := (Exec{}).Start(context.Background(), helperCmd(t, "touch", mark)); err != nil {
		t.Fatal(err)
	}
	if time.Since(begin) > 150*time.Millisecond {
		t.Errorf("Start waited for the child")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if b, err := os.ReadFile(mark); err == nil && string(b) == "started" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the started child did not run")
		}
		time.Sleep(20 * time.Millisecond)
	}
	var se *StartError
	if err := (Exec{}).Start(context.Background(), Cmd{Path: "no-such-program-x", Options: Options{Env: []string{"PATH=/nonexistent"}}}); !errors.As(err, &se) || se.Status != 127 {
		t.Errorf("missing program: %v", err)
	}

	f := NewFake(t, Expect{Background: true, Argv: []string{"cmd.exe", "/C", "taskmgr"}, Dir: Text("/mnt/c")})
	if err := f.Start(context.Background(), Cmd{Path: "cmd.exe", Args: []string{"/C", "taskmgr"}, Options: Options{Dir: "/mnt/c"}}); err != nil {
		t.Error(err)
	}
	if c := f.Calls()[0]; !c.Background || c.PTY {
		t.Errorf("fake call %+v", c)
	}
	// A Start does not meet a Run expectation.
	g := NewFake(nil, Expect{Argv: []string{"x"}})
	if err := g.Start(context.Background(), Cmd{Path: "x"}); !errors.Is(err, ErrUnexpected) {
		t.Errorf("kind mismatch: %v", err)
	}
}

// A real PTY: the child's prompt is read, the answer typed, its status
// reported.
func TestStartPTY(t *testing.T) {
	p, err := Exec{}.StartPTY(context.Background(), helperCmd(t, "prompt"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	var out bytes.Buffer
	buf := make([]byte, 64)
	for !strings.Contains(out.String(), "Passphrase: ") {
		n, err := p.Read(buf)
		out.Write(buf[:n])
		if err != nil {
			t.Fatalf("read %q: %v", out.String(), err)
		}
	}
	if _, err := io.WriteString(p, "s3cret\n"); err != nil {
		t.Fatal(err)
	}
	rest, err := io.ReadAll(p)
	if err != nil {
		t.Fatalf("read to the end: %v", err)
	}
	if !strings.Contains(string(rest), "got s3cret") {
		t.Errorf("output %q", rest)
	}
	res, err := p.Wait()
	if err != nil || res.ExitCode != 3 {
		t.Errorf("Wait: %+v %v", res, err)
	}
}

// The Fake PTY plays its dialog and fails on input it did not expect.
func TestFakePTY(t *testing.T) {
	f := NewFake(t, Expect{PTY: true, Argv: []string{"ssh-add", "/k"}, Status: 0, Dialog: []DialogStep{
		{Out: "Enter passphrase for /k: "}, {In: "pass phrase\n"}, {Out: "Identity added: /k\n"},
	}})
	p, err := f.StartPTY(context.Background(), Cmd{Path: "ssh-add", Args: []string{"/k"}})
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 8)
	var out bytes.Buffer
	for out.String() != "Enter passphrase for /k: " {
		n, err := p.Read(b)
		if err != nil {
			t.Fatal(err)
		}
		out.Write(b[:n])
	}
	_, _ = io.WriteString(p, "pass ")
	_, _ = io.WriteString(p, "phrase\n")
	rest, err := io.ReadAll(p)
	if err != nil || string(rest) != "Identity added: /k\n" {
		t.Errorf("rest %q %v", rest, err)
	}
	if res, err := p.Wait(); err != nil || res.ExitCode != 0 {
		t.Errorf("Wait %+v %v", res, err)
	}

	rec := &recorder{}
	g := NewFake(rec, Expect{PTY: true, Argv: []string{"x"}, Dialog: []DialogStep{{In: "a\n"}}})
	q, _ := g.StartPTY(context.Background(), Cmd{Path: "x"})
	if _, err := io.WriteString(q, "b\n"); err == nil || len(rec.errs) == 0 {
		t.Errorf("wrong input accepted")
	}
}
