// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package proc is the only place the HTE packages start processes: an
// absolute binary, an explicit argument vector, an explicit environment
// (never the caller's: nothing secret reaches a child), no shell. It stands
// in for internal/runner until that package exists. Tests use Fake.
package proc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// DefaultTimeout bounds a command that sets no Timeout.
const DefaultTimeout = 60 * time.Second

// Credential runs a child as another user (the root side only).
type Credential struct {
	UID, GID uint32
	Groups   []uint32
}

// Cmd is one command.
type Cmd struct {
	Path string   // absolute path of the binary
	Args []string // arguments, without argv[0]
	// Env is the whole environment of the child; nil means BaseEnv("").
	Env   []string
	Dir   string // working directory; "" means /
	Stdin []byte // standard input; nil means /dev/null
	// StdinFile is an open file given as standard input instead of Stdin
	// (the root step hands a key file to openssl this way, so that the
	// key never passes through this process).
	StdinFile *os.File
	// Timeout bounds the command; 0 means DefaultTimeout.
	Timeout time.Duration
	// Setsid starts the child in a new session, without a controlling
	// terminal, so that nothing it runs can prompt on /dev/tty.
	Setsid bool
	// Credential runs the child as another user.
	Credential *Credential
	// Terminal connects the child's standard streams to this process's
	// (an interactive sudo prompt); Stdout and Stderr are not captured.
	Terminal bool
}

// String is the command line for messages: the arguments are quoted when
// they hold blanks or quotes. It never holds a secret: none is ever in
// argv.
func (c Cmd) String() string {
	parts := []string{c.Path}
	for _, a := range c.Args {
		if a == "" || strings.ContainsAny(a, " \t\n'\"\\$") {
			a = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

// Result is what a command left.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int // -1 when killed by a signal or the timeout
}

// OK reports a zero exit status.
func (r Result) OK() bool { return r.ExitCode == 0 }

// Output is stderr, else stdout, trimmed: the text for an error message.
func (r Result) Output() string {
	if s := strings.TrimSpace(string(r.Stderr)); s != "" {
		return s
	}
	return strings.TrimSpace(string(r.Stdout))
}

// Runner runs commands. A non-zero exit status is a Result, not an error;
// the error is for a command that could not start or timed out.
type Runner interface {
	Run(ctx context.Context, c Cmd) (Result, error)
}

// Session is a started command whose standard input and output the caller
// drives (the root session behind one sudo).
type Session interface {
	Stdin() io.WriteCloser
	Stdout() io.Reader
	// Wait waits for the command to exit; Stderr holds what it printed.
	Wait() (Result, error)
}

// Starter starts sessions.
type Starter interface {
	Start(ctx context.Context, c Cmd) (Session, error)
}

// BasePath is the PATH of every child.
const BasePath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

// BaseEnv is the environment of a child: a fixed PATH, the C locale, and
// HOME when given. Nothing else of the caller's environment is passed.
func BaseEnv(home string) []string {
	env := []string{"PATH=" + BasePath, "LC_ALL=C", "LANG=C"}
	if home != "" {
		env = append(env, "HOME="+home)
	}
	return env
}

// OS is the real runner.
type OS struct{}

func (c Cmd) check() error {
	if !filepath.IsAbs(c.Path) {
		return fmt.Errorf("proc: %q is not an absolute path", c.Path)
	}
	return nil
}

func (c Cmd) command(ctx context.Context) *exec.Cmd {
	cmd := exec.CommandContext(ctx, c.Path, c.Args...)
	cmd.Env = c.Env
	if cmd.Env == nil {
		cmd.Env = BaseEnv("")
	}
	cmd.Dir = c.Dir
	if cmd.Dir == "" {
		cmd.Dir = "/"
	}
	attr := &syscall.SysProcAttr{Setsid: c.Setsid}
	if c.Credential != nil {
		attr.Credential = &syscall.Credential{Uid: c.Credential.UID, Gid: c.Credential.GID, Groups: c.Credential.Groups}
	}
	cmd.SysProcAttr = attr
	cmd.Cancel = func() error { return cmd.Process.Kill() }
	return cmd
}

func timeout(c Cmd) time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return DefaultTimeout
}

// Run runs c. Standard output and error go to temporary files rather than
// pipes: a daemon that forks and keeps them open (php-fpm, httpd -k start)
// would otherwise hold Run until it exits.
func (OS) Run(ctx context.Context, c Cmd) (Result, error) {
	if err := c.check(); err != nil {
		return Result{ExitCode: -1}, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout(c))
	defer cancel()
	cmd := c.command(ctx)
	var outF, errF *os.File
	switch {
	case c.Terminal:
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	default:
		var err error
		if outF, err = os.CreateTemp("", "gash-hte-out-"); err != nil {
			return Result{ExitCode: -1}, err
		}
		defer closeRemove(outF)
		if errF, err = os.CreateTemp("", "gash-hte-err-"); err != nil {
			return Result{ExitCode: -1}, err
		}
		defer closeRemove(errF)
		cmd.Stdout, cmd.Stderr = outF, errF
		switch {
		case c.StdinFile != nil:
			cmd.Stdin = c.StdinFile
		case c.Stdin != nil:
			cmd.Stdin = bytes.NewReader(c.Stdin)
		}
	}
	err := cmd.Run()
	res := Result{ExitCode: exitCode(cmd, err)}
	if outF != nil {
		res.Stdout, _ = readBack(outF)
		res.Stderr, _ = readBack(errF)
	}
	if ctx.Err() != nil {
		return res, fmt.Errorf("%s: timed out after %s", c.Path, timeout(c))
	}
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return res, err
	}
	return res, nil
}

func exitCode(cmd *exec.Cmd, err error) int {
	if cmd.ProcessState == nil {
		return -1
	}
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return -1
		}
	}
	return cmd.ProcessState.ExitCode()
}

func closeRemove(f *os.File) {
	f.Close()
	os.Remove(f.Name())
}

func readBack(f *os.File) ([]byte, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return io.ReadAll(io.LimitReader(f, 4<<20))
}

type osSession struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.Reader
	stderr *bytes.Buffer
	cancel context.CancelFunc
}

func (s *osSession) Stdin() io.WriteCloser { return s.stdin }
func (s *osSession) Stdout() io.Reader     { return s.stdout }

func (s *osSession) Wait() (Result, error) {
	defer s.cancel()
	err := s.cmd.Wait()
	res := Result{ExitCode: exitCode(s.cmd, err), Stderr: s.stderr.Bytes()}
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return res, err
	}
	return res, nil
}

// Start starts c with pipes for standard input and output; standard error
// goes to the terminal when Terminal is set (sudo's prompt and messages),
// else it is captured. Timeout bounds the whole session.
func (OS) Start(ctx context.Context, c Cmd) (Session, error) {
	if err := c.check(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout(c))
	cmd := c.command(ctx)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	s := &osSession{cmd: cmd, stdin: stdin, stdout: stdout, stderr: &bytes.Buffer{}, cancel: cancel}
	if c.Terminal {
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stderr = s.stderr
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	return s, nil
}

// Fake answers commands from a handler and records them, for tests. A nil
// Handler answers every command with exit status 0 and no output.
type Fake struct {
	Handler func(c Cmd) (Result, error)
	mu      sync.Mutex
	calls   []Cmd
}

// Run records c and returns the handler's answer.
func (f *Fake) Run(_ context.Context, c Cmd) (Result, error) {
	if err := c.check(); err != nil {
		return Result{ExitCode: -1}, err
	}
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
	if f.Handler == nil {
		return Result{}, nil
	}
	return f.Handler(c)
}

// Calls returns the recorded commands.
func (f *Fake) Calls() []Cmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Cmd(nil), f.calls...)
}

// Lines returns the recorded command lines.
func (f *Fake) Lines() []string {
	var out []string
	for _, c := range f.Calls() {
		out = append(out, c.String())
	}
	return out
}

// Find returns an executable named name in the system directories of
// BasePath, as seen through exists; "" when there is none.
func Find(name string, exists func(string) bool) string {
	for _, dir := range strings.Split(BasePath, ":") {
		if p := filepath.Join(dir, name); exists(p) {
			return p
		}
	}
	return ""
}
