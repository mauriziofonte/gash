// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	archiveOnce sync.Once
	archiveDir  string
	archiveErr  error
)

// archive2x extracts the 2.x bash tree once (git archive HEAD, WORKER-RULES
// rule 4: never source the worktree directly) so the reference llm_exec can
// run from a throwaway copy.
func archive2x(t *testing.T) string {
	t.Helper()
	archiveOnce.Do(func() {
		dir, err := os.MkdirTemp("", "gash2x")
		if err != nil {
			archiveErr = err
			return
		}
		wt, err := os.Getwd()
		if err != nil {
			archiveErr = err
			return
		}
		// Walk up to the git worktree root (the module lives in go-impl/).
		root := wt
		for {
			if _, e := os.Stat(filepath.Join(root, "gash.sh")); e == nil {
				break
			}
			parent := filepath.Dir(root)
			if parent == root {
				archiveErr = os.ErrNotExist
				return
			}
			root = parent
		}
		cmd := exec.Command("bash", "-c", "git -C "+root+" archive HEAD | tar -x -C "+dir)
		if out, e := cmd.CombinedOutput(); e != nil {
			archiveErr = &execError{string(out), e}
			return
		}
		archiveDir = dir
	})
	if archiveErr != nil {
		t.Skipf("cannot prepare 2.x archive: %v", archiveErr)
	}
	return archiveDir
}

type execError struct {
	out string
	err error
}

func (e *execError) Error() string { return e.err.Error() + ": " + e.out }

// run2x runs the reference 2.x llm_exec in cwd with HOME=home and returns the
// combined output and exit code.
func run2x(t *testing.T, line, cwd, home string) (string, int) {
	t.Helper()
	dir := archive2x(t)
	script := "GASH_HEADLESS=1; source '" + dir + "/gash.sh'; cd '" + cwd + "'; llm_exec \"$GASH_LINE\""
	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", script)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "GASH_LINE=" + line}
	out, err := cmd.CombinedOutput()
	return string(out), exitOf(err)
}

// runMineCombined builds the plan for line and runs it, returning combined
// output and exit code.
func runMineCombined(t *testing.T, line, cwd, home string) (string, int, error) {
	t.Helper()
	env := testEnv(cwd)
	env.Home = home
	env.Vars = map[string]string{"PATH": os.Getenv("PATH"), "HOME": home}
	plan, err := Build(line, Settings{}, env, permissiveProbe())
	if err != nil {
		return "", 0, err
	}
	cmd := exec.Command(plan.Argv[0], plan.Argv[1:]...)
	cmd.Env = plan.Env
	cmd.Dir = cwd
	out, rerr := cmd.CombinedOutput()
	return string(out), exitOf(rerr), nil
}

func exitOf(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}

// TestRun_Parity runs allowed command lines through the canonical script and
// through the reference 2.x llm_exec in a temp dir and compares output and
// exit code byte for byte (PRD 7 vectors, D-1 A rendering).
func TestRun_Parity(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	cwd := t.TempDir()
	home := cwd
	mustWrite(t, filepath.Join(cwd, "list.txt"), "b\na\n")
	mustWrite(t, filepath.Join(cwd, "notes.txt"), "see /proc/cpuinfo\n")

	lines := []string{
		"echo hello",
		"sort list.txt | head -n 1",
		"ls nothing-here 2>/dev/null || echo missing",
		`echo 'a b' "c d" e\;f`,
		"echo one\necho two # trailing > comment",
		"echo err >&2",
		"cat /dev/null && echo ok",
		"echo /proc/x",
		"grep /proc/ notes.txt",
		"cat ./notes.txt",
		"echo '*'",
		"grep -q zzz list.txt",
		"ls ./*",
		"printf '%s-%s\\n' a b",
	}
	for _, line := range lines {
		mineOut, mineExit, err := runMineCombined(t, line, cwd, home)
		if err != nil {
			t.Errorf("%q: Build refused: %v", line, err)
			continue
		}
		refOut, refExit := run2x(t, line, cwd, home)
		if mineOut != refOut || mineExit != refExit {
			t.Errorf("%q:\n  mine  = %q (exit %d)\n  2.x   = %q (exit %d)", line, mineOut, mineExit, refOut, refExit)
		}
	}
}

// TestRun_CallerUntouched: cd and variables in the line never reach the
// caller, exit status passes through (llm_exec_security_spec.sh:8).
func TestRun_CallerUntouched(t *testing.T) {
	cwd := t.TempDir()
	mustWrite(t, filepath.Join(cwd, "list.txt"), "b\na\n")
	_, exit, err := runMineCombined(t, "cd /", cwd, cwd)
	if err != nil || exit != 0 {
		t.Errorf("cd /: err=%v exit=%d", err, exit)
	}
	_, exit, err = runMineCombined(t, "grep -q zzz list.txt", cwd, cwd)
	if err != nil || exit != 1 {
		t.Errorf("grep -q zzz: err=%v exit=%d (want exit 1)", err, exit)
	}
}

// TestRun_SedAwkSandboxed: sed and awk run sandboxed; writes are refused
// (llm_exec_security_spec.sh:91).
func TestRun_SedAwkSandboxed(t *testing.T) {
	if !(OSProbe{}).HasSandboxedSed() || !(OSProbe{}).HasSandboxedGawk() {
		t.Skip("sed --sandbox / gawk --sandbox required")
	}
	cwd := t.TempDir()
	mustWrite(t, filepath.Join(cwd, "in.txt"), "a b\n")
	env := testEnv(cwd)
	env.Vars = map[string]string{"PATH": os.Getenv("PATH"), "HOME": cwd}

	for _, tc := range []struct{ line, want string }{
		{"sed -n 1p in.txt", "a b\n"},
		{`awk '{print $2}' in.txt`, "b\n"},
	} {
		plan, err := Build(tc.line, Settings{}, env, OSProbe{})
		if err != nil {
			t.Fatalf("%q: %v", tc.line, err)
		}
		out, _ := runPlan(t, plan, cwd)
		if out != tc.want {
			t.Errorf("%q: got %q want %q", tc.line, out, tc.want)
		}
	}
	// Sandbox-violating programs fail at runtime (no file written).
	for _, line := range []string{`sed -n '1w out' in.txt`, `awk 'BEGIN{system("touch pwned")}'`} {
		plan, err := Build(line, Settings{}, env, OSProbe{})
		if err != nil {
			t.Fatalf("%q build: %v", line, err)
		}
		runPlan(t, plan, cwd)
	}
	for _, n := range []string{"out", "pwned"} {
		if _, err := os.Stat(filepath.Join(cwd, n)); err == nil {
			t.Errorf("%s must not exist", n)
		}
	}
}

// TestRun_ExportedFunctionIgnored: an exported function shadowing echo is
// dropped in the child (llm_exec_security_spec.sh:107).
func TestRun_ExportedFunctionIgnored(t *testing.T) {
	cwd := t.TempDir()
	env := testEnv(cwd)
	env.Vars = map[string]string{
		"PATH":             os.Getenv("PATH"),
		"HOME":             cwd,
		"BASH_FUNC_echo%%": "() { builtin echo pwned; }",
	}
	plan, err := Build("echo safe", Settings{}, env, permissiveProbe())
	if err != nil {
		t.Fatal(err)
	}
	// The child env is the allowlist; the exported function is not in it.
	for _, kv := range plan.Env {
		if strings.HasPrefix(kv, "BASH_FUNC_") {
			t.Fatalf("exported function leaked into child env: %q", kv)
		}
	}
	out, _ := runPlan(t, plan, cwd)
	if out != "safe\n" {
		t.Errorf("got %q want %q", out, "safe\n")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
