// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

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

// TestLLMDockerCheckDelegates: llm_docker_check runs docker_compose_check
// PATH --json, PATH resolved by the path guard ("." when absent). 3.0:
// LLM-20 fixed, extra words are no longer ignored (2.x passed only
// "${1:-.}") and an option is unknown_option.
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
	require.NoError(t, os.MkdirAll(filepath.Join(d, "rel", "dir"), 0o755))
	for _, tc := range []struct {
		args []string
		path string
	}{
		{nil, d},
		{[]string{""}, "."},
		{[]string{"/opt/app"}, "/opt/app"},
		{[]string{"rel/dir"}, filepath.Join(d, "rel", "dir")},
	} {
		got := runGoFake(t, check, d, env, tc.args...)
		require.Equal(t, result{"[" + tc.path + "]\n[--json]\n", "checked\n", 3}, got, "%q", tc.args)
	}
	require.Equal(t, []string{"docker_compose_check"}, names[:1])
	got := runGoFake(t, check, d, env, "rel/dir", "extra")
	require.Equal(t, result{"", `{"error":"unexpected_argument","details":"extra","action":"RETRY","recoverable":true,"hint":"Pass at most one PATH"}` + "\n", 1}, got)
	got = runGoFake(t, check, d, env, "rel/dir", "--json")
	require.Equal(t, result{"", `{"error":"unknown_option","details":"--json","action":"RETRY","recoverable":true,"hint":"Pass the directory that holds the compose file"}` + "\n", 1}, got)
}

// TestLLMDockerCheckHelp: -h or --help anywhere prints the help (3.0:
// LLM-20 fixed, the llm_* grammar; 2.x looked at the first word only).
func TestLLMDockerCheckHelp(t *testing.T) {
	d := physTemp(t)
	env := baseEnv(d, nil)
	for _, args := range [][]string{{"-h"}, {"--help"}, {"--help", "x"}, {"x", "-h"}} {
		got := runGoFake(t, nil, d, env, args...)
		require.Equal(t, 0, got.RC)
		require.Empty(t, got.Stderr)
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

// TestLedger_LLM20_2x: 2.x applied no path guard and handed any PATH to
// docker_compose_check. 3.0: LLM-20 fixed, the same PATHs are refused.
func TestLedger_LLM20_2x(t *testing.T) {
	d := physTemp(t)
	env := baseEnv(d, nil)
	called := false
	check := func(c *invoke.Call) int { called = true; return 0 }
	for p, code := range map[string]string{
		"../x": "path_traversal_blocked", "/etc/shadow": "forbidden_path", "/proc/self": "forbidden_path",
	} {
		got := runGoFake(t, check, d, env, p)
		require.Equal(t, 1, got.RC, p)
		require.Empty(t, got.Stdout, p)
		require.Contains(t, got.Stderr, `"error":"`+code+`"`, p)
	}
	require.False(t, called)
}

// TestLedger_LLM20: llm_docker_check refuses ../x and a PATH/.env that
// resolves into a protected tree, and docker_compose_check runs with an
// environment allowlist, so an exported secret never reaches the image
// field (neither the compose interpolation nor docker compose config sees
// it); the shell variables of the call are kept.
func TestLedger_LLM20(t *testing.T) {
	d := physTemp(t)
	env := baseEnv(d, map[string]string{
		"ZSECRET": "s3cr3t-value", "GITHUB_TOKEN": "ghp_x", "DOCKER_HOST": "unix:///x.sock",
		"LC_ALL": "C.UTF-8", "GASH_HEADLESS": "1", "GASH__ADDED": "GASH_DB_TIMEOUT", "GASH_DB_TIMEOUT": "7",
	})
	var child []string
	var setting string
	check := func(c *invoke.Call) int {
		child = c.Env.Child()
		setting = c.Env.Get("GASH_DB_TIMEOUT")
		return 0
	}
	got := runGoFake(t, check, d, env, "../x")
	require.Equal(t, 1, got.RC)
	require.Contains(t, got.Stderr, `"error":"path_traversal_blocked"`)
	require.Nil(t, child)

	got = runGoFake(t, check, d, env)
	require.Equal(t, 0, got.RC, got.Stderr)
	joined := strings.Join(child, "\n")
	require.NotContains(t, joined, "s3cr3t-value")
	require.NotContains(t, joined, "GITHUB_TOKEN")
	require.NotContains(t, joined, "GASH_DB_TIMEOUT")
	for _, kv := range []string{"DOCKER_HOST=unix:///x.sock", "LC_ALL=C.UTF-8", "GASH_HEADLESS=1", "HOME=" + d} {
		require.Contains(t, child, kv)
	}
	require.Equal(t, "7", setting)

	// A .env that is a link into a protected tree is refused.
	p := filepath.Join(d, "proj")
	require.NoError(t, os.MkdirAll(p, 0o755))
	require.NoError(t, os.Symlink("/etc/shadow", filepath.Join(p, ".env")))
	child = nil
	got = runGoFake(t, check, d, env, p)
	require.Equal(t, result{"", `{"error":"forbidden_path","details":"` + p + `/.env","action":"FATAL","recoverable":false,"hint":"This path is protected and cannot be accessed"}` + "\n", 1}, got)
	require.Nil(t, child)
}
