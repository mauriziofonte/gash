// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package runner is the only package of Gash that starts processes.
//
// A process gets an explicit argv (never a shell string), an explicit
// environment (Env builds it from an allowlist; the binary's own
// environment never passes implicitly), its secrets and SQL on stdin, and
// an optional timeout. Its end is reported the way a 2.x caller saw it in
// $?: the exit status, 128+N when signal N ended it, 127 when the program
// was not found and 126 when it could not be executed.
//
// Run refuses a shell with a command string (sh -c and the like). The two
// sanctioned Bash uses, llm_exec's canonical script and the nvm bridge, go
// through RunBash, which names the use.
//
// Fake replaces the real runner in unit tests: it checks each start
// against an expected argv, environment and stdin, and returns scripted
// output and status.
package runner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Runner starts processes. Exec is the real one, Fake the one for tests.
type Runner interface {
	// Run starts c.Path with c.Args and waits for it.
	Run(ctx context.Context, c Cmd) (Result, error)
	// RunBash runs one of the sanctioned Bash scripts and waits for it.
	RunBash(ctx context.Context, b Bash) (Result, error)
}

// Options are the parts of a process start shared by Cmd and Bash.
type Options struct {
	// Env is the whole environment of the child, "NAME=value" entries
	// (see Env). nil or empty means an empty environment. Entries without
	// "=" are dropped; of duplicate names the first wins, as os.Getenv
	// reads them.
	Env []string
	// Dir is the working directory of the child; "" keeps the binary's.
	Dir string
	// Stdin is the child's standard input: secrets and SQL go here, never
	// into argv or Env. nil means /dev/null.
	Stdin io.Reader
	// Stdout and Stderr receive the child's output; nil captures it into
	// Result.Stdout and Result.Stderr. An *os.File (the terminal) is passed
	// to the child as it is.
	Stdout, Stderr io.Writer
	// Timeout, when positive, ends the child after this long (see Signal);
	// Result.TimedOut reports it.
	Timeout time.Duration
	// Signal is sent to the child when the timeout elapses or the context
	// is canceled. 0 means SIGTERM, as GNU timeout sends.
	Signal syscall.Signal
	// KillAfter is how long the child has to exit after Signal before it
	// gets SIGKILL, and how long its output pipes may stay open after it
	// exited (a background grandchild holding them). 0 means 5 seconds.
	KillAfter time.Duration
}

// Cmd is a program started with an explicit argv.
type Cmd struct {
	// Path is the program: a name without "/" is looked up in the PATH of
	// Env (an empty or relative entry is relative to Dir, as Bash reads
	// it); a name with "/" is used as it is. It is also the child's argv[0].
	Path string
	// Args are the arguments after the program name.
	Args []string
	Options
}

// Result is how a process ended.
type Result struct {
	// Stdout and Stderr hold the output when Options.Stdout and
	// Options.Stderr were nil.
	Stdout, Stderr []byte
	// ExitCode is the status as Bash reports it in $?: the exit status,
	// 128+N when signal N ended the process, 127 when the program was not
	// found and 126 when it could not be executed.
	ExitCode int
	// Signal is the signal that ended the process, 0 when it exited.
	Signal syscall.Signal
	// TimedOut reports that Options.Timeout elapsed and the process was
	// sent Options.Signal.
	TimedOut bool
}

// Success reports ExitCode 0.
func (r Result) Success() bool { return r.ExitCode == 0 }

// TimeoutStatus is the status GNU timeout would have returned for this
// run: 124 when the timeout elapsed (137 when SIGKILL ended the process),
// otherwise ExitCode.
func (r Result) TimeoutStatus() int {
	if !r.TimedOut {
		return r.ExitCode
	}
	if r.Signal == syscall.SIGKILL {
		return 128 + int(syscall.SIGKILL)
	}
	return 124
}

// StartError reports a program that could not be started. Status is the
// $? Bash gives in the same case: 127 when the program was not found, 126
// when it was found but could not be executed.
type StartError struct {
	Name   string
	Status int
	Err    error
}

func (e *StartError) Error() string {
	switch {
	case e.Status == 127 && !strings.Contains(e.Name, "/"):
		return e.Name + ": command not found"
	case e.Err != nil:
		return e.Name + ": " + reason(e.Err)
	}
	return e.Name + ": cannot execute"
}

func (e *StartError) Unwrap() error { return e.Err }

// reason is the strerror text Bash prints for the common start errors.
func reason(err error) string {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.ENOENT:
			return "No such file or directory"
		case syscall.EACCES:
			return "Permission denied"
		case syscall.EISDIR:
			return "Is a directory"
		case syscall.ENOEXEC:
			return "cannot execute binary file: Exec format error"
		}
	}
	return err.Error()
}

// ErrShellCommand is returned by Run for a shell started with a command
// string; such a script runs only through RunBash.
var ErrShellCommand = errors.New("runner: a shell command string needs RunBash")

// Exec is the Runner that starts real processes.
type Exec struct{}

var _ Runner = Exec{}

const defaultKillAfter = 5 * time.Second

// Run implements Runner.
func (Exec) Run(ctx context.Context, c Cmd) (Result, error) {
	argv := append([]string{c.Path}, c.Args...)
	if err := checkNoShell(argv); err != nil {
		return Result{ExitCode: 126}, err
	}
	return start(ctx, argv, c.Options)
}

// RunBash implements Runner.
func (Exec) RunBash(ctx context.Context, b Bash) (Result, error) {
	argv, err := b.argv()
	if err != nil {
		return Result{ExitCode: 126}, err
	}
	return start(ctx, argv, b.Options)
}

func start(ctx context.Context, argv []string, o Options) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{ExitCode: 1}, err // nothing was started
	}
	env := normalizeEnv(o.Env)
	path, serr := LookPath(argv[0], env, o.Dir)
	if serr != nil {
		return Result{ExitCode: serr.(*StartError).Status}, serr
	}

	runCtx, cancel := ctx, context.CancelFunc(func() {})
	if o.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, o.Timeout)
	}
	defer cancel()

	cmd := exec.CommandContext(runCtx, path, argv[1:]...)
	cmd.Args[0] = argv[0]
	cmd.Env = env
	cmd.Dir = o.Dir
	cmd.Stdin = o.Stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = o.Stdout, o.Stderr
	if o.Stdout == nil {
		cmd.Stdout = &stdout
	}
	if o.Stderr == nil {
		cmd.Stderr = &stderr
	}
	signaled := false
	sig := signalOf(o)
	cmd.Cancel = func() error {
		err := cmd.Process.Signal(sig)
		signaled = err == nil // not when the process had just exited
		return err
	}
	cmd.WaitDelay = o.KillAfter
	if cmd.WaitDelay <= 0 {
		cmd.WaitDelay = defaultKillAfter
	}

	if err := cmd.Start(); err != nil {
		return Result{ExitCode: startStatus(err)}, &StartError{Name: argv[0], Status: startStatus(err), Err: err}
	}
	werr := cmd.Wait()

	res := Result{}
	if o.Stdout == nil {
		res.Stdout = stdout.Bytes()
	}
	if o.Stderr == nil {
		res.Stderr = stderr.Bytes()
	}
	if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		res.Signal = ws.Signal()
		res.ExitCode = 128 + int(res.Signal)
	} else {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}
	res.TimedOut = signaled && ctx.Err() == nil

	if err := ctx.Err(); err != nil && signaled {
		return res, err
	}
	var exitErr *exec.ExitError
	if werr != nil && !errors.As(werr, &exitErr) && !errors.Is(werr, context.DeadlineExceeded) && !errors.Is(werr, context.Canceled) {
		return res, werr // an output writer failed, or pipes stayed open (exec.ErrWaitDelay)
	}
	return res, nil
}

func signalOf(o Options) syscall.Signal {
	if o.Signal != 0 {
		return o.Signal
	}
	return syscall.SIGTERM
}

// startStatus is the $? of an exec that failed after the lookup.
func startStatus(err error) int {
	if errors.Is(err, syscall.ENOENT) {
		return 127
	}
	return 126
}

// normalizeEnv returns the child environment: a new slice, entries
// without "=" or with NUL dropped, the first of duplicate names kept.
func normalizeEnv(env []string) []string {
	out := make([]string, 0, len(env))
	seen := make(map[string]bool, len(env))
	for _, kv := range env {
		i := strings.IndexByte(kv, '=')
		if i <= 0 || strings.IndexByte(kv, 0) >= 0 || seen[kv[:i]] {
			continue
		}
		seen[kv[:i]] = true
		out = append(out, kv)
	}
	return out
}

// CommandSubst is what $(...) makes of captured output: NUL bytes dropped
// (Bash warns and ignores them) and trailing newlines removed.
func CommandSubst(out []byte) string {
	s := string(out)
	if strings.IndexByte(s, 0) >= 0 {
		s = strings.ReplaceAll(s, "\x00", "")
	}
	return strings.TrimRight(s, "\n")
}

// lookupEnv is the value of name in a normalized environment.
func lookupEnv(env []string, name string) (string, bool) {
	for _, kv := range env {
		if len(kv) > len(name) && kv[len(name)] == '=' && kv[:len(name)] == name {
			return kv[len(name)+1:], true
		}
	}
	return "", false
}

// LookPath finds a program as Bash's command search does, in the PATH of
// env: a name with "/" is taken as it is; otherwise the first executable
// regular file NAME in the PATH directories wins (an empty or relative
// directory is relative to dir, or to the current directory when dir is
// ""). Without PATH in env nothing is found. The error is a *StartError
// with status 127 (not found) or 126 (found, but not executable).
func LookPath(name string, env []string, dir string) (string, error) {
	if name == "" {
		return "", &StartError{Name: name, Status: 127}
	}
	if strings.Contains(name, "/") {
		switch st, err := os.Stat(resolve(name, dir)); {
		case err != nil:
			return "", &StartError{Name: name, Status: startStatus(err), Err: err}
		case st.IsDir():
			return "", &StartError{Name: name, Status: 126, Err: syscall.EISDIR}
		case !executable(st):
			return "", &StartError{Name: name, Status: 126, Err: syscall.EACCES}
		}
		return name, nil
	}
	path, ok := lookupEnv(env, "PATH")
	if !ok {
		return "", &StartError{Name: name, Status: 127}
	}
	denied := ""
	for _, d := range strings.Split(path, ":") {
		if d == "" {
			d = "."
		}
		p := d + "/" + name
		st, err := os.Stat(resolve(p, dir))
		if err != nil || st.IsDir() {
			continue
		}
		if executable(st) {
			return p, nil
		}
		if denied == "" {
			denied = p
		}
	}
	if denied != "" {
		return "", &StartError{Name: name, Status: 126, Err: syscall.EACCES}
	}
	return "", &StartError{Name: name, Status: 127}
}

func resolve(p, dir string) string {
	if dir == "" || strings.HasPrefix(p, "/") {
		return p
	}
	return dir + "/" + p
}

// executable is Bash's test for an executable file: root needs any
// execute bit; another user the bit of the class (owner, group, other)
// it falls in.
func executable(st os.FileInfo) bool {
	mode := st.Mode().Perm()
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return mode&0o111 != 0
	}
	euid := os.Geteuid()
	switch {
	case euid == 0:
		return mode&0o111 != 0
	case int(sys.Uid) == euid:
		return mode&0o100 != 0
	case inGroup(int(sys.Gid)):
		return mode&0o010 != 0
	}
	return mode&0o001 != 0
}

func inGroup(gid int) bool {
	if gid == os.Getegid() {
		return true
	}
	groups, err := os.Getgroups()
	if err != nil {
		return false
	}
	for _, g := range groups {
		if g == gid {
			return true
		}
	}
	return false
}
