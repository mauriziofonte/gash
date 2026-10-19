// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"slices"
	"sync"
	"syscall"
)

// Credential is the user a child runs as (Options.Credential): user ID,
// group ID and supplementary groups. Only a root process may give another
// user's.
type Credential struct {
	UID, GID uint32
	Groups   []uint32
}

// sysProcAttr is the process attributes of Options.Setsid and
// Options.Credential; nil when neither is set, as before they existed.
func sysProcAttr(o Options) *syscall.SysProcAttr {
	if !o.Setsid && o.Credential == nil {
		return nil
	}
	a := &syscall.SysProcAttr{Setsid: o.Setsid}
	if c := o.Credential; c != nil {
		a.Credential = &syscall.Credential{Uid: c.UID, Gid: c.GID, Groups: slices.Clone(c.Groups)}
	}
	return a
}

// captureFile is a temporary file that receives one output stream of a
// child (Options.FileCapture), read back once the child has exited.
type captureFile struct{ f *os.File }

func newCaptureFile() (*captureFile, error) {
	f, err := os.CreateTemp("", "gash-runner-*")
	if err != nil {
		return nil, err
	}
	// Unlinked at once: the open descriptor is all the child and the
	// read-back need, and nothing is left behind on a crash.
	_ = os.Remove(f.Name())
	return &captureFile{f: f}, nil
}

func (c *captureFile) bytes() []byte {
	if _, err := c.f.Seek(0, io.SeekStart); err != nil {
		return nil
	}
	b, _ := io.ReadAll(c.f)
	return b
}

func (c *captureFile) close() { _ = c.f.Close() }

// Session is a child whose standard input and output the caller drives
// while it runs (gash hte's root session behind one sudo).
type Session interface {
	// Stdin is the child's standard input; closing it sends end of file.
	Stdin() io.WriteCloser
	// Stdout is the child's standard output.
	Stdout() io.Reader
	// Wait waits for the child and reports how it ended. Result.Stderr
	// holds what it printed on standard error when Options.Stderr was nil.
	// It may be called more than once.
	Wait() (Result, error)
}

// SessionStarter starts sessions. Exec and Fake implement it; it is not
// part of Runner, so that other Runner implementations stay valid.
type SessionStarter interface {
	StartSession(ctx context.Context, c Cmd) (Session, error)
}

var (
	_ SessionStarter = Exec{}
	_ SessionStarter = (*Fake)(nil)
)

// StartSession starts c with pipes as its standard input and output and
// returns once it runs. Options.Stdin and Options.Stdout are ignored (the
// pipes replace them); Options.Stderr receives standard error (the
// terminal, for a sudo prompt), else it is captured. Timeout bounds the
// whole session: Signal, then SIGKILL after KillAfter, as Run does.
func (Exec) StartSession(ctx context.Context, c Cmd) (Session, error) {
	argv := append([]string{c.Path}, c.Args...)
	if err := checkNoShell(argv); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	env := normalizeEnv(c.Env)
	path, err := LookPath(argv[0], env, c.Dir)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := ctx, context.CancelFunc(func() {})
	if c.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, c.Timeout)
	}
	cmd := exec.CommandContext(runCtx, path, argv[1:]...)
	cmd.Args[0] = argv[0]
	cmd.Env, cmd.Dir = env, c.Dir
	cmd.SysProcAttr = sysProcAttr(c.Options)
	s := &execSession{cmd: cmd, ctx: ctx, cancel: cancel, captured: c.Stderr == nil}
	if s.captured {
		cmd.Stderr = &s.stderr
	} else {
		cmd.Stderr = c.Stderr
	}
	sig := signalOf(c.Options)
	cmd.Cancel = func() error {
		err := cmd.Process.Signal(sig)
		s.signaled = err == nil
		return err
	}
	cmd.WaitDelay = c.KillAfter
	if cmd.WaitDelay <= 0 {
		cmd.WaitDelay = defaultKillAfter
	}
	if s.stdin, err = cmd.StdinPipe(); err != nil {
		cancel()
		return nil, err
	}
	if s.stdout, err = cmd.StdoutPipe(); err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, &StartError{Name: argv[0], Status: startStatus(err), Err: err}
	}
	return s, nil
}

type execSession struct {
	cmd      *exec.Cmd
	ctx      context.Context
	cancel   context.CancelFunc
	stdin    io.WriteCloser
	stdout   io.Reader
	stderr   bytes.Buffer
	captured bool
	signaled bool
	once     sync.Once
	res      Result
	err      error
}

func (s *execSession) Stdin() io.WriteCloser { return s.stdin }
func (s *execSession) Stdout() io.Reader     { return s.stdout }

func (s *execSession) Wait() (Result, error) {
	s.once.Do(func() {
		defer s.cancel()
		werr := s.cmd.Wait()
		if ws, ok := s.cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			s.res.Signal = ws.Signal()
			s.res.ExitCode = 128 + int(s.res.Signal)
		} else {
			s.res.ExitCode = s.cmd.ProcessState.ExitCode()
		}
		if s.captured {
			s.res.Stderr = s.stderr.Bytes()
		}
		s.res.TimedOut = s.signaled && s.ctx.Err() == nil
		if err := s.ctx.Err(); err != nil && s.signaled {
			s.err = err
			return
		}
		var exitErr *exec.ExitError
		if werr != nil && !errors.As(werr, &exitErr) && !errors.Is(werr, context.DeadlineExceeded) && !errors.Is(werr, context.Canceled) {
			s.err = werr
		}
	})
	return s.res, s.err
}

// fakeSession is a Fake child of StartSession: Expect.Serve plays it over
// kernel pipes (buffered, as a real child's are).
type fakeSession struct {
	stdin  *os.File
	stdout *os.File
	done   chan struct{}
	res    Result
	err    error
}

func (s *fakeSession) Stdin() io.WriteCloser { return s.stdin }
func (s *fakeSession) Stdout() io.Reader     { return s.stdout }

func (s *fakeSession) Wait() (Result, error) {
	<-s.done
	return s.res, s.err
}

func startFakeSession(e Expect, res Result) (Session, error) {
	inR, inW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		inR.Close()
		inW.Close()
		return nil, err
	}
	s := &fakeSession{stdin: inW, stdout: outR, done: make(chan struct{}), res: res}
	go func() {
		defer close(s.done)
		var serr error
		if e.Serve != nil {
			serr = e.Serve(inR, outW)
		}
		outW.Close()
		_, _ = io.Copy(io.Discard, inR) // the child reads until the caller closes
		inR.Close()
		if serr != nil {
			s.res.ExitCode = 1
			s.res.Stderr = []byte(serr.Error())
		}
	}()
	return s, nil
}
