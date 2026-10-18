// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// fakeCheck stands for docker_compose_check: it prints its arguments and
// a line on stderr, and returns 3.
const fakeCheck = `docker_compose_check() { printf '[%s]\n' "$@"; printf 'checked\n' >&2; return 3; }`

// run2xFake runs the 2.x llm_docker_check with docker_compose_check
// replaced by fakeCheck.
func run2xFake(t *testing.T, cwd string, env map[string]string, args ...string) result {
	t.Helper()
	dir := archive2x(t)
	script := `source "$1/gash.sh" >/dev/null 2>&1 || exit 98; ` + fakeCheck + `; cd "$2" || exit 99; shift 2; llm_docker_check "$@"`
	cmd := exec.Command("bash", append([]string{"--noprofile", "--norc", "-c", script, "bash", dir, cwd}, args...)...)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	rc := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		require.ErrorAs(t, err, &ee)
		rc = ee.ExitCode()
	}
	return result{out.String(), errb.String(), rc}
}

// runGoFake runs the llm_docker_check handler with a docker_compose_check
// handler that does what fakeCheck does (nil: none registered).
func runGoFake(t *testing.T, check invoke.Handler, cwd string, env map[string]string, args ...string) result {
	t.Helper()
	t.Chdir(cwd)
	tbl := invoke.NewTable()
	New().Register(tbl)
	if check != nil {
		tbl.Handle("docker_compose_check", check)
	}
	var out, errb bytes.Buffer
	e := invoke.EnvMap(env)
	c := &invoke.Call{Ctx: context.Background(), Name: "llm_docker_check", Args: args, Env: e, Stdin: bytes.NewReader(nil),
		Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, false, false)}
	rc := tbl.Run(c)
	return result{out.String(), errb.String(), rc}
}

func TestLLMDockerCheckDelegates(t *testing.T) {
	d := physTemp(t)
	env := baseEnv(d, nil)
	var names []string
	check := func(c *invoke.Call) int {
		names = append(names, c.Name)
		for _, a := range c.Args {
			_, _ = fmt.Fprintf(c.Stdout, "[%s]\n", a)
		}
		_, _ = fmt.Fprintln(c.Stderr, "checked")
		return 3
	}
	for _, args := range [][]string{
		nil,
		{""},
		{"/opt/app"},
		{"rel/dir", "extra", "--json"},
		{"x", "-h"},
		{"../../etc"},
	} {
		want := run2xFake(t, d, env, args...)
		got := runGoFake(t, check, d, env, args...)
		require.Equal(t, want, got, "%q", args)
	}
	require.Equal(t, []string{"docker_compose_check"}, names[:1])
}

func TestLLMDockerCheckHelp(t *testing.T) {
	d := physTemp(t)
	env := baseEnv(d, nil)
	for _, args := range [][]string{{"-h"}, {"--help"}, {"--help", "x"}} {
		want := run2xFake(t, d, env, args...)
		got := runGoFake(t, nil, d, env, args...)
		require.Equal(t, want, got, "%q", args)
		require.Contains(t, got.Stdout, "llm_docker_check")
	}
}

// Without a docker_compose_check handler (a development build) the error
// names the function that is missing.
func TestLLMDockerCheckWithoutCheck(t *testing.T) {
	d := physTemp(t)
	got := runGoFake(t, nil, d, baseEnv(d, nil))
	require.Equal(t, 1, got.RC)
	require.Contains(t, got.Stderr, `"details":"docker_compose_check is not implemented in this gash build"`)
}
