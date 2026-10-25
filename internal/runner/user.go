// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runner

import (
	"context"
	"errors"
)

// RunUser runs a command line the caller wrote out word by word (gash run
// -- CMD..., llm_test -- CMD...): argv as Run starts it, and when argv is
// a shell with a command string (bash -c 'a | b'), which Run refuses,
// through BashUserCommand, that execs the same argv. The caller's words
// are never joined into a script.
func RunUser(ctx context.Context, r Runner, argv []string, o Options) (Result, error) {
	if len(argv) == 0 {
		return Result{ExitCode: 127}, &StartError{Status: 127}
	}
	res, err := r.Run(ctx, Cmd{Path: argv[0], Args: argv[1:], Options: o})
	if !errors.Is(err, ErrShellCommand) {
		return res, err
	}
	return r.RunBash(ctx, Bash{Use: BashUserCommand, Args: argv, Options: o})
}
