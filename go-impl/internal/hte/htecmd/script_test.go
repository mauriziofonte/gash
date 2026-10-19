// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package htecmd

// The black-box scripts of gash hte (design/testing.md 4):
// testdata/hte/script/*.txtar under testscript, with a stand-in `gash` on
// PATH that runs `gash hte ARGS...` through this package on the real
// process, as the binary will once the command line dispatches hte. They
// move to testdata/script when it does.
//
// The scripts run only what never reaches sudo or the system: help, usage
// errors, refusals before the engine acts, and commands that stop at a
// missing install in the script's own HOME. A script that differs for
// root says so with the [root] condition.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){"gash": standIn})
}

// standIn is `gash hte ARGS...` on the real process.
func standIn() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] != "hte" {
		fmt.Fprintln(os.Stderr, "the stand-in gash runs gash hte only")
		os.Exit(2)
	}
	env := invoke.EnvFrom(os.Environ())
	os.Exit(Run(&invoke.Call{
		Ctx: context.Background(), Name: "hte", Args: args[1:], Env: env,
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		Term: term.New(env, os.Stdin, os.Stdout, os.Stderr),
	}))
}

func TestScript(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:                 filepath.Join("..", "..", "..", "testdata", "hte", "script"),
		RequireExplicitExec: true,
		RequireUniqueNames:  true,
		Setup: func(env *testscript.Env) error {
			home := filepath.Join(env.WorkDir, "home")
			if err := os.Mkdir(home, 0o755); err != nil {
				return err
			}
			env.Setenv("HOME", home)
			env.Setenv("LANG", "C.UTF-8")
			env.Setenv("TERM", "dumb")
			return nil
		},
		Condition: func(cond string) (bool, error) {
			if cond == "root" {
				return os.Geteuid() == 0, nil
			}
			return false, fmt.Errorf("unknown condition %q", cond)
		},
	})
}
