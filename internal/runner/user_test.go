// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runner

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestBashUserCommandArgv(t *testing.T) {
	argv, err := Bash{Use: BashUserCommand, Args: []string{"bash", "-c", "echo a | tr a b"}}.argv()
	if want := []string{"bash", "--noprofile", "--norc", "-c", UserCommandScript, "gash-run", "bash", "-c", "echo a | tr a b"}; err != nil || !slices.Equal(argv, want) {
		t.Errorf("user command argv = %q %v", argv, err)
	}
	for _, b := range []Bash{
		{Use: BashUserCommand},
		{Use: BashUserCommand, Args: []string{"x"}, Script: "rm -rf ~"},
		{Use: BashUserCommand, Args: []string{"x"}, Wrap: []string{"nice"}},
	} {
		if _, err := b.argv(); !errors.Is(err, ErrBashUse) {
			t.Errorf("%+v: %v, want a refusal", b, err)
		}
	}
	if BashUserCommand.String() != "user command" {
		t.Error("BashUserCommand.String")
	}
}

// RunUser starts a plain argv with Run and a shell with a command string
// through the fixed script, with the same output and status.
func TestRunUser(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	env := []string{"PATH=" + os.Getenv("PATH")}
	res, err := RunUser(context.Background(), Exec{}, []string{"printf", "%s", "x y"}, Options{Env: env})
	if err != nil || string(res.Stdout) != "x y" || res.ExitCode != 0 {
		t.Errorf("plain: %q %d %v", res.Stdout, res.ExitCode, err)
	}
	res, err = RunUser(context.Background(), Exec{}, []string{"bash", "-c", "echo $((1+2)) | tr 3 4; exit 7"}, Options{Env: env})
	if err != nil || strings.TrimSpace(string(res.Stdout)) != "4" || res.ExitCode != 7 {
		t.Errorf("shell: %q %d %v", res.Stdout, res.ExitCode, err)
	}
	res, err = RunUser(context.Background(), Exec{}, []string{"no-such-program-gash"}, Options{Env: env})
	var se *StartError
	if !errors.As(err, &se) || res.ExitCode != 127 {
		t.Errorf("missing: %d %v", res.ExitCode, err)
	}
	if _, err := RunUser(context.Background(), Exec{}, nil, Options{Env: env}); err == nil {
		t.Error("empty argv")
	}
}
