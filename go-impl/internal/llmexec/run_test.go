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

var (
	bashOnce sync.Once
	bashPath string
)

// bash2x is $BASH in the shell run2x starts: the path 2.x runs its child
// with ("${BASH:-bash}"), which bash prints at the start of a syntax error.
func bash2x(t *testing.T) string {
	t.Helper()
	bashOnce.Do(func() {
		cmd := exec.Command("bash", "--noprofile", "--norc", "-c", `printf '%s' "$BASH"`)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
		if out, err := cmd.Output(); err == nil {
			bashPath = string(out)
		}
	})
	if bashPath == "" {
		t.Skip("cannot read $BASH")
	}
	return bashPath
}

// runMineCombined builds the plan for line and runs it, returning combined
// output and exit code.
func runMineCombined(t *testing.T, line, cwd, home string) (string, int, error) {
	t.Helper()
	env := testEnv(cwd)
	env.Home = home
	env.BashPath = bash2x(t)
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

// TestRun_DanglingOperators: a line that 2.x handed to bash with a dangling
// operator, an empty command between operators or only redirections runs the
// same way from the canonical script: the same syntax error on the same line
// with status 2 and no word run, or the same output (point 3 of the review).
// Output and status are compared byte for byte with 2.x llm_exec.
func TestRun_DanglingOperators(t *testing.T) {
	if !bash52(t) {
		t.Skip("line numbers follow the bash 5.2 declare -f layout")
	}
	cwd := t.TempDir()
	home := cwd
	mustWrite(t, filepath.Join(cwd, "list.txt"), "b\na\n")

	lines := []string{
		// The cases of the review.
		"ls |",
		"ls &&",
		"ls ||",
		"ls;",
		"2>/dev/null",
		// More dangling operators: no word of the line runs.
		"echo ran |",
		"echo ran &&",
		"echo ran ||",
		"ls 2>/dev/null |",
		"ls ||\n",
		"ls |  ",
		// Trailing separators: the line runs.
		"ls ;",
		"ls\n",
		"ls;\n",
		"ls ; ",
		// Redirection-only commands, alone and joined.
		">/dev/null",
		"&>/dev/null",
		"2>&1",
		">&2",
		"1>&2",
		"2>/dev/null >&2",
		"ls | 2>/dev/null",
		"ls && >/dev/null",
		"2>/dev/null; ls",
		// An operator alone on a later line: the earlier lines run first,
		// then bash stops on the same line with the same message.
		"ls\n;",
		"ls\n|",
		"ls\n&&",
		"ls\n;;",
		"ls\n; ;",
		"ls\n;&&",
		";",
		"|",
		";;",
		// Operators joined across a newline keep bash's grouping.
		"ls |\nls",
		"ls &&\nls",
		"ls ||\nls",
		// A line continuation between two operators merges them for bash
		// (|\<newline>| is ||), in 2.x and in the canonical script alike.
		"ls |\\\n| wc -l",
		// Line numbers survive continuations, comments and quoted newlines.
		"echo a\\\nb\n;",
		"echo a \\\n b\n;",
		"echo \"a\\\nb\"\n;",
		"echo 'a\nb'\n;",
		"echo a # note\n;",
		"echo a\necho b\n;",
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

// TestRun_SyntaxErrorEchoesCanonicalLine: when bash refuses a token on a line
// that also has words, it echoes that line; the canonical script shows the
// words single-quoted, so only that echoed line differs from 2.x. The error
// line, the token, the status and what ran are the same.
func TestRun_SyntaxErrorEchoesCanonicalLine(t *testing.T) {
	if !bash52(t) {
		t.Skip("line numbers follow the bash 5.2 declare -f layout")
	}
	cwd := t.TempDir()
	mustWrite(t, filepath.Join(cwd, "list.txt"), "b\na\n")
	for _, tc := range []struct{ line, echoed string }{
		{"| ls", "`| 'ls''"},
		{"; ls", "`; 'ls''"},
		{"ls ;; ls", "`'ls' ;; 'ls''"},
		{"ls | | ls", "`'ls' | | 'ls''"},
		{"ls\n; ls", "`; 'ls''"},
	} {
		mineOut, mineExit, err := runMineCombined(t, tc.line, cwd, cwd)
		if err != nil {
			t.Errorf("%q: Build refused: %v", tc.line, err)
			continue
		}
		refOut, refExit := run2x(t, tc.line, cwd, cwd)
		mine, ref := strings.Split(mineOut, "\n"), strings.Split(refOut, "\n")
		if mineExit != 2 || refExit != 2 || len(mine) != len(ref) || len(mine) < 3 {
			t.Errorf("%q:\n  mine  = %q (exit %d)\n  2.x   = %q (exit %d)", tc.line, mineOut, mineExit, refOut, refExit)
			continue
		}
		n := len(mine)
		// All but the echoed line (the last non-empty one) are identical.
		if strings.Join(mine[:n-2], "\n") != strings.Join(ref[:n-2], "\n") {
			t.Errorf("%q: output before the echoed line differs:\n  mine = %q\n  2.x  = %q", tc.line, mineOut, refOut)
		}
		if !strings.HasSuffix(mine[n-2], tc.echoed) {
			t.Errorf("%q: echoed line = %q, want suffix %q", tc.line, mine[n-2], tc.echoed)
		}
	}
}

// TestRun_TrailingBackslash pins a 2.x divergence that rc.A does not
// reproduce. The 2.x parser drops a backslash at the very end of the line,
// but bash keeps it as a literal character, so 2.x validated the word x and
// bash ran x\ (a different file, never checked). The canonical script runs
// only the validated word (D-1 A), so here the two outputs differ on purpose.
func TestRun_TrailingBackslash(t *testing.T) {
	cwd := t.TempDir()
	mustWrite(t, filepath.Join(cwd, "x"), "validated\n")
	mustWrite(t, filepath.Join(cwd, "x\\"), "unchecked\n")
	line := "cat x\\"

	refOut, refExit := run2x(t, line, cwd, cwd)
	if refOut != "unchecked\n" || refExit != 0 {
		t.Errorf("2.x: got %q (exit %d), want the unchecked file", refOut, refExit)
	}
	mineOut, mineExit, err := runMineCombined(t, line, cwd, cwd)
	if err != nil || mineOut != "validated\n" || mineExit != 0 {
		t.Errorf("mine: got %q (exit %d, err %v), want the validated word", mineOut, mineExit, err)
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
