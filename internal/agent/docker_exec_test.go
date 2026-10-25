// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
)

// runDocker runs agent_docker_exec with a docker on PATH answered by
// respond through the fake runner.
func runDocker(t *testing.T, respond func(argv []string) runner.Expect, args ...string) (result, *runner.Fake) {
	t.Helper()
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nexit 9\n"), 0o755))
	m := New()
	f := runner.NewFake(t).Respond(func(c runner.Call) (runner.Expect, bool) {
		if filepath.Base(c.Argv[0]) != "docker" {
			return runner.Expect{}, false
		}
		return respond(c.Argv[1:]), true
	})
	m.run = f
	tbl := invoke.NewTable()
	m.Register(tbl)
	var out, errb bytes.Buffer
	env := invoke.EnvMap(map[string]string{"HOME": t.TempDir(), "PATH": bin, "GASH_HEADLESS": "1"})
	rc := tbl.Run(&invoke.Call{Ctx: context.Background(), Name: "agent_docker_exec", Args: args, Env: env,
		Stdout: &out, Stderr: &errb, Term: term.Fixed(env, false, false, false)})
	return result{out.String(), errb.String(), rc}, f
}

func TestDockerExecAgent(t *testing.T) {
	var argv []string
	ok := func(a []string) runner.Expect {
		argv = a
		return runner.Expect{Stdout: "Migrated: 2026_10_25_create_x\n", Stderr: "warn\n"}
	}
	// Any command, its words as they are, --user passed on.
	r, _ := runDocker(t, ok, "app", "--user", "www-data", "--", "php", "artisan", "migrate", "--force", "a b")
	require.Equal(t, result{`{"rc":0,"out":"Migrated: 2026_10_25_create_x\n","err":"warn\n"}` + "\n", "", 0}, r)
	require.Equal(t, []string{"exec", "--user=www-data", "app", "php", "artisan", "migrate", "--force", "a b"}, argv)

	// The command's own failure is rc, exit status 0; the budget applies.
	r, _ = runDocker(t, func([]string) runner.Expect {
		return runner.Expect{Status: 2, Stdout: "1\n2\n3\n", Stderr: "boom\n"}
	}, "app", "--head", "1", "--", "make", "test")
	require.Equal(t, result{`{"rc":2,"out":"1\n[... 2 lines omitted]\n","err":"boom\n","truncated":2}` + "\n", "", 0}, r)

	// --dry-run prints the argv and runs nothing.
	r, f := runDocker(t, ok, "app", "--dry-run", "-u", "0", "--", "chown", "www-data", "/var/www/storage")
	require.Equal(t, result{`{"argv":["docker","exec","--user=0","app","chown","www-data","/var/www/storage"],"dry_run":true,"written":false}` + "\n", "", 0}, r)
	require.Empty(t, f.Calls())

	// Container errors.
	for stderr, want := range map[string]string{
		"Error response from daemon: No such container: app":                  "container_not_found/RETRY",
		"Error response from daemon: container app is not running":            "container_not_running/CONTINUE",
		"Cannot connect to the Docker daemon at unix:///var/run/docker.sock.": "docker_unavailable/CONTINUE",
	} {
		r, _ := runDocker(t, func([]string) runner.Expect { return runner.Expect{Status: 1, Stderr: stderr + "\n"} }, "app", "--", "ls")
		require.Equal(t, 1, r.RC)
		require.Equal(t, want, code(t, r), stderr)
	}
}

func TestDockerExecAgentRefusals(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"app", "--", "rm", "-rf", "/"}, "dangerous_command_blocked/FATAL"},
		{[]string{"app", "--", "dd", "if=/dev/zero", "of=/dev/sda"}, "dangerous_command_blocked/FATAL"},
		{[]string{"app", "--dry-run", "--", "cat", "/app/.env"}, "secret_file_blocked/FATAL"},
		{[]string{"app", "--", "cp", "--target=/root/.ssh/id_ed25519", "x"}, "dangerous_command_blocked/FATAL"},
		{[]string{"app", "--", "php", "-r", "x", "--env-file=.env.production"}, "secret_file_blocked/FATAL"},
		{nil, "missing_container/RETRY"},
		{[]string{"app"}, "missing_command/RETRY"},
		{[]string{"app", "--"}, "missing_command/RETRY"},
		{[]string{"app", "ls"}, "unexpected_argument/RETRY"},
		{[]string{"app", "--user", "", "--", "ls"}, "invalid_user/RETRY"},
		{[]string{"app", "--privileged", "--", "ls"}, "unknown_option/RETRY"},
		{[]string{"app", "--head", "x", "--", "ls"}, "invalid_head/RETRY"},
	} {
		r, f := runDocker(t, func([]string) runner.Expect { return runner.Expect{} }, tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Empty(t, r.Stdout, tc.args)
		require.Equal(t, tc.code, code(t, r), tc.args)
		require.Empty(t, f.Calls(), tc.args)
	}

	// No docker on PATH.
	m := New()
	tbl := invoke.NewTable()
	m.Register(tbl)
	var out, errb bytes.Buffer
	env := invoke.EnvMap(map[string]string{"HOME": t.TempDir(), "PATH": t.TempDir()})
	rc := tbl.Run(&invoke.Call{Ctx: context.Background(), Name: "agent_docker_exec", Args: []string{"app", "--", "ls"}, Env: env,
		Stdout: &out, Stderr: &errb, Term: term.Fixed(env, false, false, false)})
	require.Equal(t, 1, rc)
	require.Equal(t, "docker_not_found/CONTINUE", code(t, result{out.String(), errb.String(), rc}))
}
