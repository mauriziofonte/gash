// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package prereq

import (
	"context"
	"time"

	"github.com/mauriziofonte/gash/internal/runner"
)

// probeTimeout bounds a probe: a program that hangs on its probe does not
// satisfy the tool.
const probeTimeout = 5 * time.Second

// runProbe runs PATH ARGS with stdin from /dev/null and reports a zero
// exit status.
func runProbe(path string, args []string, env []string) bool {
	res, err := runner.Exec{}.Run(context.Background(), runner.Cmd{Path: path, Args: args,
		Options: runner.Options{Env: env, Timeout: probeTimeout, KillAfter: time.Second}})
	return err == nil && res.ExitCode == 0
}

// runVersion is the output of PATH --version (stdout and stderr), "" when
// it does not run.
func runVersion(path string, env []string) string {
	res, err := runner.Exec{}.Run(context.Background(), runner.Cmd{Path: path, Args: []string{"--version"},
		Options: runner.Options{Env: env, Timeout: probeTimeout, KillAfter: time.Second}})
	if err != nil {
		return ""
	}
	return string(res.Stdout) + string(res.Stderr)
}
