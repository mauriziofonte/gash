// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

// TB is the part of testing.TB the Fake reports through.
type TB interface {
	Helper()
	Errorf(format string, args ...any)
	Error(args ...any)
	Cleanup(func())
}

// Call is one start as the Fake saw it.
type Call struct {
	// Use is the Bash use for RunBash, 0 for Run.
	Use BashUse
	// Argv is the whole command line, program first, as given (not looked
	// up); for RunBash the bash -c line RunBash builds.
	Argv []string
	// Env is the child environment as Exec would pass it.
	Env     []string
	Dir     string
	Timeout time.Duration
	// Stdin is everything the child could read: the whole Stdin reader;
	// nil when there was none.
	Stdin []byte
}

// Expect is one expected start and its scripted outcome.
type Expect struct {
	// Use is the expected Bash use: 0 for Run.
	Use BashUse
	// Argv must match exactly.
	Argv []string
	// Env, when not nil, must match exactly, order included.
	Env []string
	// EnvHas lists "NAME=value" entries the environment must hold, and
	// EnvLacks names it must not.
	EnvHas   []string
	EnvLacks []string
	// Stdin, when not nil, must match the whole input.
	Stdin *string
	// Dir, when not nil, must match.
	Dir *string

	// Stdout and Stderr are the scripted output, written to the Options
	// writers or captured into the Result.
	Stdout, Stderr string
	// Status is the exit status; Signal, when set, ends the process
	// instead (ExitCode 128+N).
	Status int
	Signal syscall.Signal
	// TimedOut is copied into the Result.
	TimedOut bool
	// Err is returned as the start error; a *StartError sets ExitCode to
	// its Status.
	Err error
}

// Text returns a pointer to s, for Expect.Stdin and Expect.Dir.
func Text(s string) *string { return &s }

// ErrUnexpected is returned by the Fake for a start that no expectation
// matches.
var ErrUnexpected = errors.New("runner: unexpected process start")

// Fake is the recording Runner of the unit tests: each start must match
// the next expectation, in order, and gets its scripted outcome. It
// applies the same refusals as Exec (no shell command strings in Run, the
// RunBash uses), so a test fails where production would.
type Fake struct {
	mu      sync.Mutex
	t       TB
	expects []Expect
	calls   []Call
}

var _ Runner = (*Fake)(nil)

// NewFake returns a Fake that reports mismatches through t and, when the
// test ends, expectations that were never met. t may be nil.
func NewFake(t TB, expects ...Expect) *Fake {
	f := &Fake{t: t, expects: expects}
	if t != nil {
		t.Cleanup(func() {
			if err := f.Verify(); err != nil {
				t.Error(err)
			}
		})
	}
	return f
}

// Expect appends expectations.
func (f *Fake) Expect(e ...Expect) *Fake {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.expects = append(f.expects, e...)
	return f
}

// Calls returns the starts seen so far.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// Verify reports expectations that no start met.
func (f *Fake) Verify() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.expects) == 0 {
		return nil
	}
	var argv []string
	for _, e := range f.expects {
		argv = append(argv, fmt.Sprintf("%q", e.Argv))
	}
	return fmt.Errorf("runner: %d expected starts never happened: %s", len(f.expects), strings.Join(argv, ", "))
}

// Run implements Runner.
func (f *Fake) Run(ctx context.Context, c Cmd) (Result, error) {
	argv := append([]string{c.Path}, c.Args...)
	if err := checkNoShell(argv); err != nil {
		f.fail("%v", err)
		return Result{ExitCode: 126}, err
	}
	return f.start(ctx, 0, argv, c.Options)
}

// RunBash implements Runner.
func (f *Fake) RunBash(ctx context.Context, b Bash) (Result, error) {
	argv, err := b.argv()
	if err != nil {
		f.fail("%v", err)
		return Result{ExitCode: 126}, err
	}
	return f.start(ctx, b.Use, argv, b.Options)
}

func (f *Fake) fail(format string, args ...any) {
	if f.t != nil {
		f.t.Helper()
		f.t.Errorf(format, args...)
	}
}

func (f *Fake) start(ctx context.Context, use BashUse, argv []string, o Options) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{ExitCode: 1}, err
	}
	call := Call{Use: use, Argv: argv, Env: normalizeEnv(o.Env), Dir: o.Dir, Timeout: o.Timeout}
	if o.Stdin != nil {
		in, err := io.ReadAll(o.Stdin)
		if err != nil {
			return Result{ExitCode: 1}, err
		}
		call.Stdin = in
	}

	f.mu.Lock()
	f.calls = append(f.calls, call)
	var e Expect
	have := len(f.expects) > 0
	if have {
		e = f.expects[0]
		f.expects = f.expects[1:]
	}
	f.mu.Unlock()

	if !have {
		f.fail("runner: unexpected start %q", argv)
		return Result{ExitCode: 127}, fmt.Errorf("%w: %q", ErrUnexpected, argv)
	}
	if why := e.mismatch(call); why != "" {
		f.fail("runner: start %q: %s", argv, why)
		return Result{ExitCode: 127}, fmt.Errorf("%w: %q: %s", ErrUnexpected, argv, why)
	}

	res := Result{ExitCode: e.Status, TimedOut: e.TimedOut}
	if e.Signal != 0 {
		res.Signal = e.Signal
		res.ExitCode = 128 + int(e.Signal)
	}
	var se *StartError
	if errors.As(e.Err, &se) {
		return Result{ExitCode: se.Status}, e.Err
	}
	if e.Err != nil {
		return res, e.Err
	}
	if err := emit(o.Stdout, e.Stdout, &res.Stdout); err != nil {
		return res, err
	}
	if err := emit(o.Stderr, e.Stderr, &res.Stderr); err != nil {
		return res, err
	}
	return res, nil
}

func emit(w io.Writer, s string, captured *[]byte) error {
	if w == nil {
		*captured = []byte(s)
		return nil
	}
	if s == "" {
		return nil
	}
	_, err := io.WriteString(w, s)
	return err
}

func (e Expect) mismatch(c Call) string {
	switch {
	case e.Use != c.Use:
		return fmt.Sprintf("use %s, want %s", c.Use, e.Use)
	case !slices.Equal(e.Argv, c.Argv):
		return fmt.Sprintf("want argv %q", e.Argv)
	case e.Env != nil && !slices.Equal(normalizeEnv(e.Env), c.Env):
		return fmt.Sprintf("env %q, want %q", c.Env, e.Env)
	case e.Stdin != nil && *e.Stdin != string(c.Stdin):
		return fmt.Sprintf("stdin %q, want %q", c.Stdin, *e.Stdin)
	case e.Dir != nil && *e.Dir != c.Dir:
		return fmt.Sprintf("dir %q, want %q", c.Dir, *e.Dir)
	}
	for _, kv := range e.EnvHas {
		if !slices.Contains(c.Env, kv) {
			return fmt.Sprintf("env %q lacks %q", c.Env, kv)
		}
	}
	for _, name := range e.EnvLacks {
		if _, ok := lookupEnv(c.Env, name); ok {
			return fmt.Sprintf("env %q has %s", c.Env, name)
		}
	}
	return ""
}
