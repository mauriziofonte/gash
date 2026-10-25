// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

var (
	archiveOnce sync.Once
	archiveDir  string
	archiveErr  error
)

// archive2x extracts the 2.x Bash tree once (git archive HEAD, never the
// worktree itself) so the reference functions run from a throwaway copy.
func archive2x(t *testing.T) string {
	t.Helper()
	archiveOnce.Do(func() {
		wd, err := os.Getwd()
		if err != nil {
			archiveErr = err
			return
		}
		root := wd
		for {
			if _, e := os.Stat(filepath.Join(root, "lib/modules/gash.sh")); e == nil {
				break
			}
			parent := filepath.Dir(root)
			if parent == root {
				archiveErr = os.ErrNotExist
				return
			}
			root = parent
		}
		dir, err := os.MkdirTemp("", "gash2x-db")
		if err != nil {
			archiveErr = err
			return
		}
		cmd := exec.Command("bash", "-c", `git -C "$1" archive HEAD | tar -x -C "$2"`, "bash", root, dir)
		if out, e := cmd.CombinedOutput(); e != nil {
			archiveErr = errors.New(e.Error() + ": " + string(out))
			return
		}
		archiveDir = dir
	})
	if archiveErr != nil {
		t.Skipf("cannot prepare the 2.x archive: %v", archiveErr)
	}
	return archiveDir
}

type result struct {
	Stdout, Stderr string
	RC             int
}

// baseEnv is the environment both implementations get.
func baseEnv(home string, extra map[string]string) map[string]string {
	env := map[string]string{"PATH": os.Getenv("PATH"), "HOME": home, "LC_ALL": "C", "GASH_HEADLESS": "1"}
	for k, v := range extra {
		env[k] = v
	}
	return env
}

// run2x runs the 2.x function fn in cwd with a frozen clock
// (EPOCHREALTIME pinned after the load: query_time 0µs, never slow).
func run2x(t *testing.T, cwd string, env map[string]string, fn string, args ...string) result {
	t.Helper()
	return run2xScript(t, `unset EPOCHREALTIME; EPOCHREALTIME=1792007131.734850; `, cwd, env, fn, args...)
}

func run2xScript(t *testing.T, clock, cwd string, env map[string]string, fn string, args ...string) result {
	t.Helper()
	dir := archive2x(t)
	script := `source "$1/gash.sh" >/dev/null 2>&1 || exit 98; ` + clock + `cd "$2" || exit 99; shift 2; "$@"`
	cmd := exec.Command("bash", append([]string{"--noprofile", "--norc", "-c", script, "bash", dir, cwd, fn}, args...)...)
	cmd.Env = []string{}
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

// frozen stops the module's clock: every call takes 0µs.
func frozen(m *Module) *Module {
	at := time.Unix(1792007131, 734850000)
	m.now = func() time.Time { return at }
	return m
}

// runGo runs the handler of fn with args in cwd.
func runGo(t *testing.T, m *Module, cwd string, env map[string]string, fn string, args ...string) result {
	t.Helper()
	t.Chdir(cwd)
	tbl := invoke.NewTable()
	m.Register(tbl)
	var out, errb bytes.Buffer
	e := invoke.EnvMap(env)
	c := &invoke.Call{Ctx: context.Background(), Name: fn, Args: args, Env: e, Stdin: bytes.NewReader(nil), Stdout: &out, Stderr: &errb,
		Term: term.Fixed(e, false, false, false)}
	rc := tbl.Run(c)
	return result{out.String(), errb.String(), rc}
}

// same runs fn in both implementations and requires identical bytes.
func same(t *testing.T, m *Module, cwd string, env map[string]string, fn string, args ...string) result {
	t.Helper()
	want := run2x(t, cwd, env, fn, args...)
	got := runGo(t, m, cwd, env, fn, args...)
	require.Equal(t, want, got, "%s %q in %s", fn, args, cwd)
	return got
}

func mustWrite(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), mode))
	require.NoError(t, os.Chmod(path, mode))
}

// physTemp is a temp dir by its physical path.
func physTemp(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return d
}

// sqliteFixture creates DIR/t.db with the sqlite3 shell.
func sqliteFixture(t *testing.T, dir, sqlText string) string {
	t.Helper()
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 is not installed")
	}
	db := filepath.Join(dir, "t.db")
	out, err := exec.Command("sqlite3", db, sqlText).CombinedOutput()
	require.NoError(t, err, string(out))
	return db
}
