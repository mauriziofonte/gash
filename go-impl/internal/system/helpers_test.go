// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
)

// testSelf is the PID the tests give the binary.
const testSelf = 4242

// fakeHandoff is a hand-off descriptor that records what was sent.
type fakeHandoff struct {
	bytes.Buffer
	closed bool
}

func (f *fakeHandoff) Close() error { f.closed = true; return nil }

// kills records the signals of a test module.
type kills struct {
	sent []string
	// alive answers the 0-signal probe per PID; refuse makes every signal
	// fail with that error.
	alive  map[int]bool
	refuse error
}

func (k *kills) kill(pid int, sig syscall.Signal) error {
	k.sent = append(k.sent, sigName(sig)+" "+strconv.Itoa(pid))
	if k.refuse != nil {
		return k.refuse
	}
	if sig == 0 && !k.alive[pid] {
		return syscall.ESRCH
	}
	if sig == syscall.SIGKILL {
		delete(k.alive, pid)
	}
	return nil
}

func sigName(s syscall.Signal) string {
	switch s {
	case 0:
		return "0"
	case syscall.SIGTERM:
		return "TERM"
	case syscall.SIGKILL:
		return "KILL"
	}
	return s.String()
}

// tm is a test module with the fake runner f.
func tm(t *testing.T, f *runner.Fake) (*Module, *kills, *fakeHandoff) {
	t.Helper()
	k := &kills{alive: map[int]bool{}}
	h := &fakeHandoff{}
	m := &Module{
		run:      f,
		sys:      platform.Real(),
		kill:     k.kill,
		sleep:    func(time.Duration) {},
		self:     testSelf,
		loc:      time.UTC,
		ipURL:    "https://ipinfo.io/ip",
		open:     func() (io.WriteCloser, error) { return h, nil },
		readable: func(p string) bool { _, err := os.Stat(p); return err == nil },
	}
	return m, k, h
}

// result is one call's output.
type result struct {
	out, err string
	rc       int
}

// call runs NAME on m with env, the term tt (a plain one when nil) and
// stdin.
func call(m *Module, name string, env map[string]string, tt *term.Term, stdin string, args ...string) result {
	if env == nil {
		env = map[string]string{}
	}
	if _, ok := env["LANG"]; !ok {
		env["LANG"] = "C.UTF-8"
	}
	e := invoke.EnvMap(env)
	if tt == nil {
		tt = term.Fixed(e, false, false, false)
	}
	var out, errb bytes.Buffer
	c := &invoke.Call{Name: name, Args: args, Env: e, Stdin: strings.NewReader(stdin), Stdout: &out, Stderr: &errb, Term: tt}
	tbl := invoke.NewTable()
	m.Register(tbl)
	rc := tbl.Run(c)
	return result{out.String(), errb.String(), rc}
}

// binDir makes a PATH directory holding executables of the given names.
func binDir(t *testing.T, names ...string) string {
	t.Helper()
	d := t.TempDir()
	for _, n := range names {
		require.NoError(t, os.WriteFile(filepath.Join(d, n), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	}
	return d
}

// ttyTerm is a color terminal on stdout and stderr whose keyboard types
// input (no controlling terminal when input is "-").
func ttyTerm(env map[string]string, input string) *term.Term {
	if env == nil {
		env = map[string]string{"LANG": "C.UTF-8"}
	}
	e := invoke.EnvMap(env)
	t := term.Fixed(e, true, true, true)
	if input == "-" {
		return t.WithoutTTY()
	}
	return t.WithTTY(input)
}
