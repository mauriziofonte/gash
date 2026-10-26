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
	"github.com/mauriziofonte/gash/internal/prereq"
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
	if !x.requireArg(input, "host alias/target", `remote_exec <alias|user@host> "command"`, "missing_host") {
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
		if !x.requireArg(scriptFile, "script file", "remote_exec <alias> --script FILE|-", "missing_file") {
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
			x.fail("empty_command", "Script is empty: "+scriptFile, "Give a script with at least one command")
			return 1
		}
		if arg(0) == "--" {
			args = args[1:]
		}
		remoteCmd = "bash -s --" + scriptArgs(args)
		stdin = strings.NewReader(content + "\n") // <<<"$script_content"
	} else {
		remoteCmd = arg(0)
		if !x.requireArg(remoteCmd, "remote command", `remote_exec <alias> "command"`, "empty_command") {
			return 1
		}
		if len(args) > 1 {
			x.fail("unexpected_argument", "Pass the remote command as a SINGLE quoted string: remote_exec <alias> \"cmd arg1 arg2\". Refusing to silently drop extra args. For multi-line commands use --script.",
				`Quote the whole command: remote_exec <alias> "cmd arg1 arg2"`)
			return 1
		}
	}

	// ssh is looked up before anything is created or started: without
	// it, the prerequisite report, not a silent exit 127.
	if _, ok := prereq.Require(c, "ssh", "remote_exec", false); !ok {
		return 1
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
		// The command's own status: headless, no envelope (one marks a
		// connection that failed).
		if !x.headless() {
			x.p.Error("Remote command exited 255 (killed by a signal or explicit exit 255); the SSH connection is healthy.")
		}
		return 255
	}
	x.fail("host_unreachable", "Failed to connect to "+target+". Verify SSH connectivity and key configuration.",
		"Check the network and the SSH keys of the host, then retry")
	return 1
}

// readScript is the script of --script: FILE must be a regular file
// (else "Script file not readable: FILE"), or - for stdin, read whole as
// 2.x's $(cat ...) read it (NUL bytes and trailing newlines dropped). The
// binary reads it itself: no cat to miss (2.x then sent an empty script,
// "Script is empty"), and a file it cannot read is "not readable" too.
func (x *call) readScript(file string) (string, bool) {
	var data []byte
	var err error
	if file == "-" {
		if x.Stdin != nil {
			data, err = io.ReadAll(x.Stdin)
		}
	} else {
		if st, serr := os.Stat(file); serr != nil || !st.Mode().IsRegular() {
			x.fail("path_unreadable", "Script file not readable: "+file, "Give a readable regular file, or - for stdin")
			return "", false
		}
		data, err = os.ReadFile(file)
	}
	if err != nil {
		x.fail("path_unreadable", "Script file not readable: "+file, "Give a readable regular file, or - for stdin")
		return "", false
	}
	return runner.CommandSubst(data), true
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
