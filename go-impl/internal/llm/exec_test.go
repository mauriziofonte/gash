// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/runner"
)

// execFixture is a working directory with files to read and a secret.
func execFixture(t *testing.T) (dir, home string) {
	t.Helper()
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "a.txt"), "alpha\nbeta\ngamma\n")
	mustWrite(t, filepath.Join(d, "src", "b.go"), "package b\n")
	mustWrite(t, filepath.Join(d, ".env"), "SECRET=1\n")
	home = filepath.Join(d, "home")
	mustMkdir(t, home)
	return d, home
}

func TestExecMatches2x(t *testing.T) {
	d, home := execFixture(t)
	m := New()
	env := baseEnv(home, map[string]string{"PWD": d, "MY_TOKEN": "s3cret", "TERM": "dumb"})
	for _, args := range [][]string{
		{"cat a.txt"},
		{"head -n 2 a.txt | wc -l"},
		{"grep -n beta a.txt"},
		{"ls src"},
		{"cd src && ls"},
		{"echo $HOME"},
		{"printf '%s\\n' \"$PWD\""},
		{"env | sort"},
		{"cat .env"},
		{"cat /etc/shadow"},
		{"rm a.txt"},
		{"curl http://example.com"},
		{"cat a.txt > out.txt"},
		{"echo 'unterminated"},
		{"false"},
		{"exit 3"},
		{"cat missing.txt"},
		{"seq 3"},
		{""},
		{"   "},
		{"ls", "extra"},
		{"--help"},
		{"-h"},
		{"ls", "--help"},
	} {
		same(t, m, d, env, "llm_exec", args...)
	}
	require.NoFileExists(t, filepath.Join(d, "out.txt"))
	require.FileExists(t, filepath.Join(d, "a.txt"))
}

// TestExecSettingsFromFile: GASH_LLM_EXEC_ALLOW, GASH_LLM_EXEC and
// GASH_LLM_SANDBOX come from ~/.gash_env only.
func TestExecSettingsFromFile(t *testing.T) {
	d, home := execFixture(t)
	m := New()
	env := baseEnv(home, map[string]string{"PWD": d, "GASH_LLM_EXEC": "unrestricted", "GASH_LLM_EXEC_ALLOW": "touch"})
	// From the environment: ignored.
	same(t, m, d, env, "llm_exec", "touch x")
	require.NoFileExists(t, filepath.Join(d, "x"))
	cfg := filepath.Join(home, ".gash_env")
	mustWrite(t, cfg, "GASH_LLM_EXEC_ALLOW=make,uname:seq\n")
	require.NoError(t, os.Chmod(cfg, 0o600))
	for _, line := range []string{"uname -s", "seq 2", "make --version | head -c 0", "touch x"} {
		same(t, m, d, env, "llm_exec", line)
	}
	mustWrite(t, cfg, "GASH_LLM_EXEC=unrestricted\n")
	same(t, m, d, env, "llm_exec", "echo $MY_UNSET; env | grep -c GASH_LLM_EXEC")
	r := same(t, m, d, env, "llm_exec", "touch made && ls made")
	require.Equal(t, "made\n", r.Stdout)
	mustWrite(t, cfg, "GASH_LLM_SANDBOX=1\n")
	same(t, m, d, env, "llm_exec", "ls") // sandbox_unavailable without bwrap
}

// TestExecBashPath: the caller's $BASH runs the line (2.x ${BASH:-bash}),
// and bash from the caller's PATH without it.
func TestExecBashPath(t *testing.T) {
	d, home := execFixture(t)
	m := New()
	rec := &recorder{status: 4, out: "ran\n"}
	m.run = rec
	m.lookPathFn = func(name string) (string, bool) {
		if name == "bash" {
			return "/opt/bash", true
		}
		return "", false
	}
	env := baseEnv(home, map[string]string{"PWD": d, "BASH": "/custom/bash", "GASH__ADDED": "BASH", "MY_TOKEN": "x"})
	r := runGo(t, m, d, env, "llm_exec", "true")
	require.Equal(t, result{"ran\n", "", 4}, r)
	delete(env, "BASH")
	r = runGo(t, m, d, env, "llm_exec", "true")
	require.Equal(t, 4, r.RC)
	require.Len(t, rec.calls, 2)
	b := rec.calls[0]
	require.Equal(t, runner.BashLLMExec, b.Use)
	require.Equal(t, "/custom/bash", b.Bash)
	require.Nil(t, b.Wrap)
	require.True(t, strings.HasSuffix(b.Script, "\n'true'"), b.Script)
	require.Contains(t, b.Env, "PWD="+d)
	for _, kv := range b.Env {
		require.False(t, strings.HasPrefix(kv, "BASH=") || strings.HasPrefix(kv, "MY_TOKEN="), kv)
	}
	require.Equal(t, "/opt/bash", rec.calls[1].Bash)
}

// recorder is a Runner that records RunBash calls and answers them (the
// other methods are not used).
type recorder struct {
	runner.Runner
	calls  []runner.Bash
	status int
	out    string
}

func (r *recorder) Run(context.Context, runner.Cmd) (runner.Result, error) {
	return runner.Result{ExitCode: 127}, nil
}

func (r *recorder) RunBash(_ context.Context, b runner.Bash) (runner.Result, error) {
	r.calls = append(r.calls, b)
	_, _ = io.WriteString(b.Stdout, r.out)
	return runner.Result{ExitCode: r.status}, nil
}
