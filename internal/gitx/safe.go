// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"context"
	"io"

	"github.com/mauriziofonte/gash/internal/pathguard"
	"github.com/mauriziofonte/gash/internal/runner"
)

// Safe runs the hardened git of the llm_* callers, the 2.x __llm_git
// wrapper (lib/modules/llm/core.sh):
//
//	GIT_OPTIONAL_LOCKS=0 command git \
//	  -c core.fsmonitor=false -c core.hooksPath=/dev/null \
//	  -c diff.external= -c core.pager=cat ARGS...
//
// so a repository's own configuration cannot name a program to run or pull
// the index into a write, while the rest of the caller's environment (HOME,
// PATH, the user's global git config) applies as in 2.x. GIT_OPTIONAL_LOCKS
// is prepended so it overrides any inherited value, as the var assignment
// in front of the command does. This is the only hardened git path; the
// git module functions in this package deliberately run without it
// (LEDGER GIT-10).
//
// env is the caller's exported environment (invoke.Env.Child()). stdin and
// the stdout/stderr writers follow runner.Options: a nil writer captures
// into the result, io.Discard is the 2.x >/dev/null or 2>/dev/null.
func Safe(ctx context.Context, run runner.Runner, env []string, stdin io.Reader, stdout, stderr io.Writer, args ...string) runner.Result {
	argv := append(append([]string(nil), pathguard.GitSafeConfig...), args...)
	e := append([]string{"GIT_OPTIONAL_LOCKS=0"}, env...)
	res, _ := run.Run(ctx, runner.Cmd{Path: "git", Args: argv, Options: runner.Options{
		Env: e, Stdin: stdin, Stdout: stdout, Stderr: stderr,
	}})
	return res
}
