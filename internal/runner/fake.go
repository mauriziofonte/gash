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
	// Use is the Bash use for RunBash, 0 otherwise.
	Use BashUse
	// Script is the constant script of RunScript, 0 otherwise.
	Script ConstScript
	// Background is a Start, PTY a StartPTY, Session a StartSession.
	Background, PTY, Session bool
	// Argv is the whole command line, program first, as given (not looked
	// up); for RunBash the bash -c line RunBash builds.
	Argv []string
	// Env is the child environment as Exec would pass it.
	Env     []string
	Dir     string
	Timeout time.Duration
	// Stdin is everything the child could read: the whole Stdin reader;
	// nil when there was none (and for a StartSession, whose input the
	// Serve of its expectation reads).
	Stdin []byte
	// Setsid and Credential are the Options of the same names.
	Setsid     bool
	Credential *Credential
}

// Expect is one expected start and its scripted outcome.
type Expect struct {
	// Use is the expected Bash use: 0 for Run.
	Use BashUse
	// Script is the expected constant script of RunScript.
	Script ConstScript
	// Background expects a Start, PTY a StartPTY, Session a StartSession.
	Background, PTY, Session bool
	// Serve plays the child of a StartSession: it reads what the code
	// writes to the session's Stdin and writes what the code reads from
	// its Stdout, until it returns. Then the session ends with Status (and
	// Stderr); an error ends it with status 1 and the error text as Stderr.
	Serve func(stdin io.Reader, stdout io.Writer) error
	// Dialog is the conversation of a StartPTY child, in order: what it
	// prints (reads of the PTY return it) and what the code must type
	// (writes must match it). After the last step reads get io.EOF.
	Dialog []DialogStep
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
	respond func(Call) (Expect, bool)
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

// Respond sets a function that answers, in any order, the starts it
// accepts (ok true), before the expectations are consulted: for code
// whose starts follow from what it discovers rather than a fixed order.
// Only the outcome fields of its Expect are used (Stdout, Stderr, Status,
// Signal, TimedOut, Err, Dialog, Serve); a start it declines goes to the
// expectations. The refusals of Exec still apply.
func (f *Fake) Respond(answer func(c Call) (Expect, bool)) *Fake {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.respond = answer
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
	return f.start(ctx, Call{Argv: argv}, c.Options)
}

// RunBash implements Runner.
func (f *Fake) RunBash(ctx context.Context, b Bash) (Result, error) {
	argv, err := b.argv()
	if err != nil {
		f.fail("%v", err)
		return Result{ExitCode: 126}, err
	}
	return f.start(ctx, Call{Use: b.Use, Argv: argv}, b.Options)
}

// RunScript implements Runner.
func (f *Fake) RunScript(ctx context.Context, sc Script) (Result, error) {
	argv, err := sc.argv()
	if err != nil {
		f.fail("%v", err)
		return Result{ExitCode: 126}, err
	}
	return f.start(ctx, Call{Script: sc.Name, Argv: argv}, sc.Options)
}

// Start implements Runner: the expectation's Err is the start error; its
// output and status are not used.
func (f *Fake) Start(ctx context.Context, c Cmd) error {
	argv := append([]string{c.Path}, c.Args...)
	if err := checkNoShell(argv); err != nil {
		f.fail("%v", err)
		return err
	}
	_, err := f.start(ctx, Call{Background: true, Argv: argv}, Options{Env: c.Env, Dir: c.Dir, Stdin: c.Stdin, Setsid: c.Setsid, Credential: c.Credential})
	return err
}

// StartSession implements SessionStarter: the expectation's Serve plays
// the child over kernel pipes; Err is the start error.
func (f *Fake) StartSession(ctx context.Context, c Cmd) (Session, error) {
	argv := append([]string{c.Path}, c.Args...)
	if err := checkNoShell(argv); err != nil {
		f.fail("%v", err)
		return nil, err
	}
	o := c.Options
	o.Stdin = nil // the session's input goes to Serve
	e, err := f.next(ctx, Call{Session: true, Argv: argv}, o)
	if err != nil {
		return nil, err
	}
	if e.Err != nil {
		return nil, e.Err
	}
	res := Result{ExitCode: e.Status, TimedOut: e.TimedOut, Stderr: []byte(e.Stderr)}
	if e.Signal != 0 {
		res.Signal, res.ExitCode = e.Signal, 128+int(e.Signal)
	}
	if c.Stderr != nil {
		if err := emit(c.Stderr, e.Stderr, &res.Stderr); err != nil {
			return nil, err
		}
		res.Stderr = nil
	}
	return startFakeSession(e, res)
}

// StartPTY implements Runner: the child follows the expectation's Dialog,
// then ends with its Status or Signal.
func (f *Fake) StartPTY(ctx context.Context, c Cmd) (PTY, error) {
	argv := append([]string{c.Path}, c.Args...)
	if err := checkNoShell(argv); err != nil {
		f.fail("%v", err)
		return nil, err
	}
	e, err := f.next(ctx, Call{PTY: true, Argv: argv}, Options{Env: c.Env, Dir: c.Dir, Timeout: c.Timeout})
	if err != nil {
		return nil, err
	}
	res := Result{ExitCode: e.Status, TimedOut: e.TimedOut}
	if e.Signal != 0 {
		res.Signal, res.ExitCode = e.Signal, 128+int(e.Signal)
	}
	return &fakePTY{f: f, argv: argv, steps: slices.Clone(e.Dialog), res: res}, nil
}

func (f *Fake) fail(format string, args ...any) {
	if f.t != nil {
		f.t.Helper()
		f.t.Errorf(format, args...)
	}
}

// next records call and matches it against the next expectation.
func (f *Fake) next(ctx context.Context, call Call, o Options) (Expect, error) {
	if err := ctx.Err(); err != nil {
		return Expect{}, err
	}
	argv := call.Argv
	call.Env, call.Dir, call.Timeout = normalizeEnv(o.Env), o.Dir, o.Timeout
	call.Setsid, call.Credential = o.Setsid, o.Credential
	if o.Stdin != nil {
		in, err := io.ReadAll(o.Stdin)
		if err != nil {
			return Expect{}, err
		}
		call.Stdin = in
	}

	f.mu.Lock()
	f.calls = append(f.calls, call)
	respond := f.respond
	f.mu.Unlock()
	if respond != nil {
		if e, ok := respond(call); ok {
			return answered(e, call)
		}
	}

	f.mu.Lock()
	var e Expect
	have := len(f.expects) > 0
	if have {
		e = f.expects[0]
		f.expects = f.expects[1:]
	}
	f.mu.Unlock()

	if !have {
		f.fail("runner: unexpected start %q", argv)
		return Expect{}, fmt.Errorf("%w: %q", ErrUnexpected, argv)
	}
	if why := e.mismatch(call); why != "" {
		f.fail("runner: start %q: %s", argv, why)
		return Expect{}, fmt.Errorf("%w: %q: %s", ErrUnexpected, argv, why)
	}
	return answered(e, call)
}

// answered is the start error an expectation gives: a *StartError, or
// any Err of a start that has no Result (Start, StartPTY, StartSession).
func answered(e Expect, call Call) (Expect, error) {
	var se *StartError
	if errors.As(e.Err, &se) || (e.Err != nil && (call.Background || call.PTY || call.Session)) {
		return e, e.Err
	}
	return e, nil
}

func (f *Fake) start(ctx context.Context, call Call, o Options) (Result, error) {
	e, err := f.next(ctx, call, o)
	if err != nil {
		var se *StartError
		switch {
		case errors.As(err, &se):
			return Result{ExitCode: se.Status}, err
		case errors.Is(err, ErrUnexpected):
			return Result{ExitCode: 127}, err
		}
		return Result{ExitCode: 1}, err
	}

	res := Result{ExitCode: e.Status, TimedOut: e.TimedOut}
	if e.Signal != 0 {
		res.Signal = e.Signal
		res.ExitCode = 128 + int(e.Signal)
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
	case e.Script != c.Script:
		return fmt.Sprintf("script %s, want %s", c.Script, e.Script)
	case e.Background != c.Background || e.PTY != c.PTY:
		return fmt.Sprintf("kind background=%t pty=%t, want background=%t pty=%t", c.Background, c.PTY, e.Background, e.PTY)
	case e.Session != c.Session:
		return fmt.Sprintf("kind session=%t, want session=%t", c.Session, e.Session)
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

// DialogStep is one turn of a Fake PTY's conversation: Out is printed by
// the child, In must be typed by the code; exactly one is set.
type DialogStep struct {
	Out, In string
}

// fakePTY plays a Dialog.
type fakePTY struct {
	mu    sync.Mutex
	f     *Fake
	argv  []string
	steps []DialogStep
	typed string // the part of the current In step typed so far
	res   Result
}

// Read returns the output of the current Out step; at the end of the
// dialog io.EOF. A read while the child waits for input fails the test.
func (p *fakePTY) Read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.steps) == 0 {
		return 0, io.EOF
	}
	st := &p.steps[0]
	if st.Out == "" {
		p.f.fail("runner: PTY %q: read while the child waits for %q", p.argv, st.In)
		return 0, fmt.Errorf("%w: read while the child waits for input", ErrUnexpected)
	}
	n := copy(b, st.Out)
	st.Out = st.Out[n:]
	if st.Out == "" {
		p.steps = p.steps[1:]
	}
	return n, nil
}

// Write must type the current In step (in one or more writes).
func (p *fakePTY) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.steps) == 0 || p.steps[0].In == "" {
		p.f.fail("runner: PTY %q: unexpected input %q", p.argv, b)
		return 0, fmt.Errorf("%w: unexpected input", ErrUnexpected)
	}
	typed := p.typed + string(b)
	want := p.steps[0].In
	if !strings.HasPrefix(want, typed) {
		p.f.fail("runner: PTY %q: typed %q, want %q", p.argv, typed, want)
		return 0, fmt.Errorf("%w: unexpected input", ErrUnexpected)
	}
	p.typed = typed
	if typed == want {
		p.typed, p.steps = "", p.steps[1:]
	}
	return len(b), nil
}

// Wait reports the scripted end; a dialog left unfinished fails the test.
func (p *fakePTY) Wait() (Result, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.steps) > 0 {
		p.f.fail("runner: PTY %q ended with %d dialog steps left", p.argv, len(p.steps))
	}
	return p.res, nil
}

func (p *fakePTY) Close() error { return nil }
