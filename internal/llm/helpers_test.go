// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

var (
	archiveOnce sync.Once
	archiveDir  string
	archiveErr  error
)

// archive2x extracts the 2.x Bash tree once (git archive HEAD, WORKER-RULES
// rule 4: never source the worktree itself) so the reference functions run
// from a throwaway copy.
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
		dir, err := os.MkdirTemp("", "gash2x-llm")
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

// result is what one call printed.
type result struct {
	Stdout, Stderr string
	RC             int
}

// baseEnv is the environment both implementations get: the test's PATH, a
// private HOME and the C locale.
func baseEnv(home string, extra map[string]string) map[string]string {
	env := map[string]string{"PATH": os.Getenv("PATH"), "HOME": home, "LC_ALL": "C", "GASH_HEADLESS": "1"}
	for k, v := range extra {
		env[k] = v
	}
	return env
}

// run2x runs the 2.x function fn with args in cwd.
func run2x(t *testing.T, cwd string, env map[string]string, fn string, args ...string) result {
	t.Helper()
	dir := archive2x(t)
	script := `source "$1/gash.sh" >/dev/null 2>&1 || exit 98; cd "$2" || exit 99; shift 2; "$@"`
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

// runGo runs the handler of fn with args in cwd (the test's working
// directory is changed for the call, as the binary inherits the caller's).
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

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(path, 0o755))
}

// rawNamesOK reports whether dir's filesystem can hold a filename whose bytes
// are not valid UTF-8. Linux (ext4/tmpfs) stores arbitrary bytes; macOS APFS
// rejects the name with "illegal byte sequence". The 2.x find/awk pipeline
// and the Go port both pass such names through raw, but only where the
// filesystem can store them.
func rawNamesOK(t *testing.T, dir string) bool {
	t.Helper()
	p := filepath.Join(dir, "probe\xff\xfe")
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		return false
	}
	_ = os.Remove(p)
	return true
}

// physTemp is a temp dir by its physical path (realpath output).
func physTemp(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return d
}
