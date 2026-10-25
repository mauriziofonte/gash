// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runner

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

// Start starts c and returns once it runs, without waiting for it or
// capturing its output, as a 2.x `cmd &` did (wsl_taskmanager). The child
// gets Options.Stdin, Stdout and Stderr as they are (pass the terminal's
// *os.File to keep it; nil is /dev/null) and outlives the binary; Timeout,
// Signal and KillAfter do not apply. The context only gates the start.
func (Exec) Start(ctx context.Context, c Cmd) error {
	argv := append([]string{c.Path}, c.Args...)
	if err := checkNoShell(argv); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	env := normalizeEnv(c.Env)
	path, err := LookPath(argv[0], env, c.Dir)
	if err != nil {
		return err
	}
	cmd := exec.Command(path, argv[1:]...) //nolint:noctx // detached: the process outlives the call, ctx was checked above
	cmd.Args[0] = argv[0]
	cmd.Env, cmd.Dir = env, c.Dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = c.Stdin, c.Stdout, c.Stderr
	cmd.SysProcAttr = sysProcAttr(c.Options)
	if err := cmd.Start(); err != nil {
		return &StartError{Name: argv[0], Status: startStatus(err), Err: err}
	}
	go func() { _ = cmd.Wait() }() // reaped while the binary lives
	return nil
}

// PTY is a child running on a pseudo-terminal: reading gets what it
// prints (io.EOF once it has exited and its output is drained), writing
// types on its keyboard.
type PTY interface {
	io.ReadWriter
	// Wait waits for the child and reports how it ended.
	Wait() (Result, error)
	// Close closes the terminal (the child gets a hangup).
	Close() error
}

// StartPTY starts c with a new pseudo-terminal as its controlling
// terminal and its standard streams, in a new session (ssh-add asking for
// a passphrase). Options.Stdin, Stdout and Stderr are ignored: the
// terminal is all three. A canceled context or the Timeout sends Signal,
// then SIGKILL after KillAfter, as Run does.
func (Exec) StartPTY(ctx context.Context, c Cmd) (PTY, error) {
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
	p := &realPTY{cmd: cmd, cancel: cancel, ctx: ctx}
	sig := signalOf(c.Options)
	cmd.Cancel = func() error {
		err := cmd.Process.Signal(sig)
		p.signaled = err == nil
		return err
	}
	cmd.WaitDelay = c.KillAfter
	if cmd.WaitDelay <= 0 {
		cmd.WaitDelay = defaultKillAfter
	}
	f, err := pty.Start(cmd)
	if err != nil {
		cancel()
		return nil, &StartError{Name: argv[0], Status: startStatus(err), Err: err}
	}
	p.f = f
	return p, nil
}

type realPTY struct {
	f        *os.File
	cmd      *exec.Cmd
	ctx      context.Context
	cancel   context.CancelFunc
	signaled bool
	once     sync.Once
	res      Result
	err      error
}

// Read maps the EIO Linux returns once the child side is closed to io.EOF.
func (p *realPTY) Read(b []byte) (int, error) {
	n, err := p.f.Read(b)
	if err != nil && errors.Is(err, syscall.EIO) {
		err = io.EOF
	}
	return n, err
}

func (p *realPTY) Write(b []byte) (int, error) { return p.f.Write(b) }

func (p *realPTY) Wait() (Result, error) {
	p.once.Do(func() {
		defer p.cancel()
		werr := p.cmd.Wait()
		if ws, ok := p.cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			p.res.Signal = ws.Signal()
			p.res.ExitCode = 128 + int(p.res.Signal)
		} else {
			p.res.ExitCode = p.cmd.ProcessState.ExitCode()
		}
		p.res.TimedOut = p.signaled && p.ctx.Err() == nil
		if err := p.ctx.Err(); err != nil && p.signaled {
			p.err = err
			return
		}
		var exitErr *exec.ExitError
		if werr != nil && !errors.As(werr, &exitErr) && !errors.Is(werr, context.DeadlineExceeded) && !errors.Is(werr, context.Canceled) {
			p.err = werr
		}
	})
	return p.res, p.err
}

func (p *realPTY) Close() error { return p.f.Close() }
