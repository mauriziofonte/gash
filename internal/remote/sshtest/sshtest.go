// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package sshtest is the ssh of the agent-toolkit tests: a runner.Runner
// whose "ssh" runs the remote script locally with sh, in a directory that
// plays the remote home, and counts the invocations. It proves the
// protocol (one invocation per call, the trailer, the frames) without a
// network. Only tests import it.
package sshtest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/mauriziofonte/gash/internal/runner"
)

// SSH is the fake. Calls, Scripts, Targets and Stdins record every
// invocation.
type SSH struct {
	runner.Exec
	// Home is the remote $HOME and working directory.
	Home string
	// Path, when set, is the remote PATH (a stub uname first, say).
	Path string
	// Env are extra remote variables, NAME=value.
	Env []string
	// Unreachable makes every invocation fail as a dead connection: ssh's
	// 255 with its message, nothing run.
	Unreachable bool

	mu      sync.Mutex
	Calls   int
	Scripts []string
	Targets []string
	Stdins  [][]byte
}

// Run plays ssh: the argv after "--" is the target and the remote command.
// Anything but ssh is started for real.
func (f *SSH) Run(ctx context.Context, c runner.Cmd) (runner.Result, error) {
	if c.Path != "ssh" {
		return f.Exec.Run(ctx, c)
	}
	i := 0
	for i < len(c.Args) && c.Args[i] != "--" {
		i++
	}
	if i+2 >= len(c.Args) {
		return runner.Result{ExitCode: 255, Stderr: []byte("usage: ssh\n")}, nil
	}
	target, script := c.Args[i+1], c.Args[i+2]
	var in []byte
	if c.Stdin != nil {
		in, _ = io.ReadAll(c.Stdin)
	}
	f.mu.Lock()
	f.Calls++
	f.Scripts = append(f.Scripts, script)
	f.Targets = append(f.Targets, target)
	f.Stdins = append(f.Stdins, in)
	f.mu.Unlock()
	if f.Unreachable {
		return runner.Result{ExitCode: 255, Stderr: []byte("ssh: connect to host " + target + " port 22: Connection refused\n")}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	cmd.Dir = f.Home
	path := f.Path
	if path == "" {
		path = os.Getenv("PATH")
	}
	cmd.Env = append([]string{"HOME=" + f.Home, "PATH=" + path, "LC_ALL=C"}, f.Env...)
	cmd.Stdin = bytes.NewReader(in)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	rc := 0
	timedOut := false
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			rc = ee.ExitCode()
		} else {
			rc = 255
		}
		if ctx.Err() != nil {
			timedOut = true
			rc = 124
		}
	}
	return runner.Result{Stdout: out.Bytes(), Stderr: errb.Bytes(), ExitCode: rc, TimedOut: timedOut}, nil
}

// N is the number of invocations so far.
func (f *SSH) N() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Calls
}
