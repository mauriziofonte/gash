// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

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

// envFile is a private GASH_ENV_FILE holding lines.
func envFile(t *testing.T, lines string) string {
	t.Helper()
	p := filepath.Join(physTemp(t), "gash_env")
	mustWrite(t, p, lines)
	require.NoError(t, os.Chmod(p, 0o600))
	return p
}

// writeLines change a file: a command outside the allowlist, a redirect,
// a delete.
var writeLines = []struct {
	line string
	argv []string
}{
	{"touch made", []string{"touch", "made"}},
	{"echo x > made", []string{"sh", "-c", "echo x > made"}},
	{"rm -f made", []string{"rm", "-f", "made"}},
}

// TestUnrestrictedIsLLMExecOnly: GASH_LLM_EXEC=unrestricted lifts the
// checks of llm_exec alone. llm_remote_run, llm_docker_exec, llm_wait
// --cmd-ok and gash run --safe refuse a write exactly as without it, and
// nothing runs (3.4 before: with it they ran any command, on remote hosts
// too).
func TestUnrestrictedIsLLMExecOnly(t *testing.T) {
	const hosts = "HOST:web=deploy@web.example\n"
	for _, w := range writeLines {
		t.Run(w.line, func(t *testing.T) {
			checked := envFile(t, hosts)
			open := envFile(t, hosts+"GASH_LLM_EXEC=unrestricted\n")

			// llm_remote_run: refused before ssh.
			f := newRunFixture(t)
			f.env["GASH_ENV_FILE"] = checked
			want := f.run(t, "web", "--", w.line)
			require.Equal(t, 1, want.RC, want.Stderr)
			f.env["GASH_ENV_FILE"] = open
			require.Equal(t, want, f.run(t, "web", "--", w.line))
			require.Zero(t, f.ssh.N(), "nothing connected")

			// llm_docker_exec: refused before docker.
			d := physTemp(t)
			ran := false
			m, _ := dockerModule(t, func([]string) runner.Expect { ran = true; return runner.Expect{} })
			env := baseEnv(physTemp(t), map[string]string{"GASH_ENV_FILE": checked})
			args := append([]string{"ctr", "--"}, w.argv...)
			want = runGo(t, m, d, env, "llm_docker_exec", args...)
			require.Equal(t, 1, want.RC, want.Stderr)
			env["GASH_ENV_FILE"] = open
			require.Equal(t, want, runGo(t, m, d, env, "llm_docker_exec", args...))
			require.False(t, ran, "docker ran")

			// llm_wait --cmd-ok: refused before the first run.
			env["GASH_ENV_FILE"] = checked
			want = runGo(t, New(), d, env, "llm_wait", "--timeout", "2", "--cmd-ok", w.line)
			require.Equal(t, 1, want.RC, want.Stderr)
			env["GASH_ENV_FILE"] = open
			require.Equal(t, want, runGo(t, New(), d, env, "llm_wait", "--timeout", "2", "--cmd-ok", w.line))

			// gash run --safe: refused, nothing run.
			safe := func(file string) (int, bool, string) {
				e := invoke.EnvMap(baseEnv(physTemp(t), map[string]string{"GASH_ENV_FILE": file}))
				var errb bytes.Buffer
				c := &invoke.Call{Ctx: context.Background(), Name: "gash", Env: e, Stdin: bytes.NewReader(nil),
					Stdout: &bytes.Buffer{}, Stderr: &errb, Term: term.Fixed(e, false, false, false)}
				t.Chdir(d)
				rc, refused := SafeExec(c, w.argv, bytes.NewReader(nil), &bytes.Buffer{}, &errb)
				return rc, refused, errb.String()
			}
			rc, refused, stderr := safe(checked)
			require.True(t, refused, stderr)
			require.Equal(t, 1, rc)
			rc2, refused2, stderr2 := safe(open)
			require.Equal(t, []any{rc, refused, stderr}, []any{rc2, refused2, stderr2})
			require.NoFileExists(t, filepath.Join(d, "made"))
		})
	}

	// llm_exec still honours it.
	d := physTemp(t)
	env := baseEnv(physTemp(t), map[string]string{"PWD": d, "GASH_ENV_FILE": envFile(t, "GASH_LLM_EXEC=unrestricted\n")})
	r := runGo(t, New(), d, env, "llm_exec", "touch made")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.FileExists(t, filepath.Join(d, "made"))
}

// TestExecAllowStillApplies: GASH_LLM_EXEC_ALLOW, which names each
// command, keeps applying to llm_docker_exec and llm_wait --cmd-ok.
func TestExecAllowStillApplies(t *testing.T) {
	d := physTemp(t)
	var argv []string
	m, _ := dockerModule(t, func(a []string) runner.Expect { argv = a; return runner.Expect{Stdout: "ok"} })
	env := baseEnv(physTemp(t), map[string]string{"GASH_ENV_FILE": envFile(t, "GASH_LLM_EXEC_ALLOW=touch\n")})
	r := runGo(t, m, d, env, "llm_docker_exec", "ctr", "--", "touch", "made")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Equal(t, []string{"exec", "ctr", "touch", "made"}, argv)
	r = runGo(t, New(), d, env, "llm_wait", "--timeout", "5", "--cmd-ok", "touch made")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.FileExists(t, filepath.Join(d, "made"))
}
