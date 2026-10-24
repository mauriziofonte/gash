// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
)

// historyOff is the prefix of every remote command string.
const historyOff = "unset HISTFILE HISTSIZE; "

// exec is remote_exec TARGET "COMMAND" | TARGET --script FILE|- [--
// ARGS...]: ssh with the multiplexing options, -- and the target (LEDGER
// REM-02), then the probe that tells a 255 exit from a dead connection.
func (m *Module) exec(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	x := m.begin(c)
	input := c.Arg(0)
	if !x.requireArg(input, "host alias/target", `remote_exec <alias|user@host> "command"`) {
		return 1
	}
	args := c.Args[1:]
	target, ok := x.resolveTarget(input)
	if !ok {
		return 1
	}

	arg := func(i int) string {
		if i < len(args) {
			return args[i]
		}
		return ""
	}
	scriptFile := ""
	if arg(0) == "--script" {
		scriptFile = arg(1)
		if !x.requireArg(scriptFile, "script file", "remote_exec <alias> --script FILE|-") {
			return 1
		}
		args = args[2:]
	}

	var remoteCmd string
	var stdin io.Reader = c.Stdin
	if scriptFile != "" {
		content, ok := x.readScript(scriptFile)
		if !ok {
			return 1
		}
		if strings.Trim(content, " \t\n\v\f\r") == "" {
			x.p.Error("Script is empty: " + scriptFile)
			return 1
		}
		if arg(0) == "--" {
			args = args[1:]
		}
		remoteCmd = "bash -s --" + scriptArgs(args)
		stdin = strings.NewReader(content + "\n") // <<<"$script_content"
	} else {
		remoteCmd = arg(0)
		if !x.requireArg(remoteCmd, "remote command", `remote_exec <alias> "command"`) {
			return 1
		}
		if len(args) > 1 {
			x.p.Error("Pass the remote command as a SINGLE quoted string: remote_exec <alias> \"cmd arg1 arg2\". Refusing to silently drop extra args. For multi-line commands use --script.")
			return 1
		}
	}

	// A missing ~/.ssh (the multiplexing socket's directory) is created
	// with mode 0700, as OpenSSH expects; an existing one is left as it
	// is (LEDGER REM-07).
	if dir := x.home() + "/.ssh"; x.home() != "" {
		if _, err := os.Lstat(dir); errors.Is(err, fs.ErrNotExist) {
			_ = os.Mkdir(dir, 0o700)
		}
	}
	opts := sshOpts(x.home())

	// -- ends ssh's options: the target is never read as one (LEDGER
	// REM-02).
	res := x.run(stdin, c.Stdout, c.Stderr, "ssh", append(opts, "--", target, historyOff+remoteCmd)...)
	if res.ExitCode != 255 {
		return res.ExitCode
	}
	probe := append(append([]string{"-o", "ConnectTimeout=5"}, opts...), "--", target, "exit 0")
	if x.run(c.Stdin, c.Stdout, io.Discard, "ssh", probe...).Success() {
		x.p.Error("Remote command exited 255 (killed by a signal or explicit exit 255); the SSH connection is healthy.")
		return 255
	}
	x.p.Error("Failed to connect to " + target + ". Verify SSH connectivity and key configuration.")
	return 1
}

// readScript is the script of --script: FILE must be a regular file
// (else "Script file not readable: FILE"), or - for stdin, read whole as
// $(cat ...) reads it (NUL bytes and trailing newlines dropped). cat runs
// as in 2.x, so its own messages stay its own.
func (x *call) readScript(file string) (string, bool) {
	var res runner.Result
	if file == "-" {
		res = x.run(x.Stdin, nil, x.Stderr, "cat")
	} else {
		if st, err := os.Stat(file); err != nil || !st.Mode().IsRegular() {
			x.p.Error("Script file not readable: " + file)
			return "", false
		}
		res = x.run(x.Stdin, nil, x.Stderr, "cat", file)
	}
	return runner.CommandSubst(res.Stdout), true
}

// scriptArgs is the argument part of `bash -s --ARGS`: each argument
// quoted POSIX-style and preceded by a space, nothing when there is
// none. 2.x used Bash's printf %q, which is Bash-only and follows the
// locale (LEDGER REM-04).
func scriptArgs(args []string) string {
	var b strings.Builder
	for _, a := range args {
		b.WriteString(" '" + strings.ReplaceAll(a, "'", `'\''`) + "'")
	}
	return b.String()
}
