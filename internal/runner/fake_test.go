// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runner

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"syscall"
	"testing"
)

// recorder is a TB that keeps the reports instead of failing the test.
type recorder struct {
	errs     []string
	cleanups []func()
}

func (r *recorder) Helper() {}
func (r *recorder) Errorf(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}
func (r *recorder) Error(args ...any) { r.errs = append(r.errs, fmt.Sprint(args...)) }
func (r *recorder) Cleanup(f func())  { r.cleanups = append(r.cleanups, f) }
func (r *recorder) end() {
	for _, f := range r.cleanups {
		f()
	}
}

func TestFakeScriptsAndRecords(t *testing.T) {
	f := NewFake(t,
		Expect{
			Argv:     []string{"mysql", "--batch", "--database=app"},
			EnvHas:   []string{"LANG=C"},
			EnvLacks: []string{"MYSQL_PWD"},
			Stdin:    Text("SET SESSION TRANSACTION READ ONLY;\nSELECT 1\n;\n"),
			Stdout:   "1\n",
			Stderr:   "warning\n",
		},
		Expect{Argv: []string{"git", "status"}, Env: []string{"HOME=/h"}, Dir: Text("/repo"), Status: 128, Stdout: "out"},
		Expect{Argv: []string{"sleep", "9"}, Signal: syscall.SIGTERM, TimedOut: true},
		Expect{Argv: []string{"nope"}, Err: &StartError{Name: "nope", Status: 127}},
	)
	ctx := context.Background()

	res, err := f.Run(ctx, Cmd{Path: "mysql", Args: []string{"--batch", "--database=app"}, Options: Options{
		Env:   []string{"LANG=C", "HOME=/h"},
		Stdin: strings.NewReader("SET SESSION TRANSACTION READ ONLY;\nSELECT 1\n;\n"),
	}})
	if err != nil || string(res.Stdout) != "1\n" || string(res.Stderr) != "warning\n" || res.ExitCode != 0 {
		t.Errorf("first start: %+v %v", res, err)
	}

	var out strings.Builder
	res, err = f.Run(ctx, Cmd{Path: "git", Args: []string{"status"}, Options: Options{Env: []string{"HOME=/h"}, Dir: "/repo", Stdout: &out}})
	if err != nil || res.ExitCode != 128 || out.String() != "out" || res.Stdout != nil {
		t.Errorf("second start: %+v %q %v", res, out.String(), err)
	}

	res, err = f.Run(ctx, Cmd{Path: "sleep", Args: []string{"9"}})
	if err != nil || res.ExitCode != 143 || res.Signal != syscall.SIGTERM || res.TimeoutStatus() != 124 {
		t.Errorf("signal: %+v %v", res, err)
	}

	res, err = f.Run(ctx, Cmd{Path: "nope"})
	var se *StartError
	if !errors.As(err, &se) || res.ExitCode != 127 || err.Error() != "nope: command not found" {
		t.Errorf("start error: %+v %v", res, err)
	}

	calls := f.Calls()
	if len(calls) != 4 || string(calls[0].Stdin) != "SET SESSION TRANSACTION READ ONLY;\nSELECT 1\n;\n" || calls[1].Dir != "/repo" || calls[2].Stdin != nil {
		t.Errorf("calls = %+v", calls)
	}
	if !slices.Equal(calls[0].Env, []string{"LANG=C", "HOME=/h"}) {
		t.Errorf("recorded env = %q", calls[0].Env)
	}
}

func TestFakeMismatches(t *testing.T) {
	tests := []struct {
		name   string
		expect Expect
		cmd    Cmd
		want   string
	}{
		{"argv", Expect{Argv: []string{"a", "b"}}, Cmd{Path: "a", Args: []string{"c"}}, `want argv ["a" "b"]`},
		{"env", Expect{Argv: []string{"a"}, Env: []string{"X=1"}}, Cmd{Path: "a", Options: Options{Env: []string{"X=2"}}}, `env ["X=2"], want ["X=1"]`},
		{"env has", Expect{Argv: []string{"a"}, EnvHas: []string{"X=1"}}, Cmd{Path: "a"}, `lacks "X=1"`},
		{"env lacks", Expect{Argv: []string{"a"}, EnvLacks: []string{"PGPASSWORD"}}, Cmd{Path: "a", Options: Options{Env: []string{"PGPASSWORD=pw"}}}, "has PGPASSWORD"},
		{"stdin", Expect{Argv: []string{"a"}, Stdin: Text("x")}, Cmd{Path: "a"}, `stdin "", want "x"`},
		{"dir", Expect{Argv: []string{"a"}, Dir: Text("/d")}, Cmd{Path: "a"}, `dir "", want "/d"`},
		{"use", Expect{Argv: []string{"a"}, Use: BashLLMExec}, Cmd{Path: "a"}, "use BashUse(0), want llm_exec"},
	}
	for _, tt := range tests {
		r := &recorder{}
		f := NewFake(r, tt.expect)
		_, err := f.Run(context.Background(), tt.cmd)
		if !errors.Is(err, ErrUnexpected) || len(r.errs) != 1 || !strings.Contains(r.errs[0], tt.want) {
			t.Errorf("%s: %v, reports %q, want %q", tt.name, err, r.errs, tt.want)
		}
	}

	// No expectation left.
	r := &recorder{}
	f := NewFake(r)
	if _, err := f.Run(context.Background(), Cmd{Path: "x"}); !errors.Is(err, ErrUnexpected) || len(r.errs) != 1 {
		t.Errorf("unexpected start: %v %q", err, r.errs)
	}

	// Expectations never met are reported at the end of the test.
	r = &recorder{}
	f = NewFake(r, Expect{Argv: []string{"never"}})
	r.end()
	if len(r.errs) != 1 || !strings.Contains(r.errs[0], `["never"]`) || f.Verify() == nil {
		t.Errorf("leftover expectations: %q", r.errs)
	}
	if err := NewFake(nil).Expect(Expect{Argv: []string{"a"}}).Verify(); err == nil {
		t.Error("Verify with a leftover expectation")
	}
}

// The Fake refuses what Exec refuses.
func TestFakeRefusals(t *testing.T) {
	r := &recorder{}
	f := NewFake(r)
	if _, err := f.Run(context.Background(), Cmd{Path: "sudo", Args: []string{"sh", "-c", "sync"}}); !errors.Is(err, ErrShellCommand) {
		t.Errorf("sh -c through the fake: %v", err)
	}
	if _, err := f.RunBash(context.Background(), Bash{Use: BashNVMBridge, Script: "other"}); !errors.Is(err, ErrBashUse) {
		t.Errorf("nvm bridge with another script: %v", err)
	}
	if len(r.errs) != 2 || len(f.Calls()) != 0 {
		t.Errorf("reports %q, calls %d", r.errs, len(f.Calls()))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewFake(nil).Run(ctx, Cmd{Path: "a"}); !errors.Is(err, context.Canceled) {
		t.Errorf("done context: %v", err)
	}
}

func TestFakeRunBash(t *testing.T) {
	f := NewFake(t, Expect{
		Use:    BashNVMBridge,
		Argv:   []string{"bash", "--noprofile", "--norc", "-c", NVMBridgeScript, "nvm", "current"},
		Env:    []string{"NVM_DIR=/n"},
		Stdout: "v20.1.0\n",
	})
	b := Bash{Use: BashNVMBridge, Args: []string{"current"}}
	b.Env = []string{"NVM_DIR=/n"}
	res, err := f.RunBash(context.Background(), b)
	if err != nil || CommandSubst(res.Stdout) != "v20.1.0" {
		t.Errorf("RunBash: %+v %v", res, err)
	}
	if c := f.Calls(); len(c) != 1 || c[0].Use != BashNVMBridge {
		t.Errorf("calls = %+v", c)
	}
}
