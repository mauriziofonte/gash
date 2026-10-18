// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package compatgen

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "rewrite the golden copies in testdata/compat")

const (
	repoRoot = "../../.."
	golden   = "../../testdata/compat"
)

// The generated files are byte for byte the golden copies (go test
// ./internal/compatgen -update refreshes them).
func TestGolden(t *testing.T) {
	files, err := Generate(true)
	require.NoError(t, err)
	if *update {
		require.NoError(t, os.RemoveAll(golden))
		require.NoError(t, Write(files, golden))
	}
	require.Empty(t, Check(files, golden), "run: go test ./internal/compatgen -update")
	var want, got []string
	for _, f := range files {
		want = append(want, f.Path)
	}
	require.NoError(t, filepath.Walk(golden, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			rel, _ := filepath.Rel(golden, p)
			got = append(got, filepath.ToSlash(rel))
		}
		return err
	}))
	sort.Strings(got)
	require.Equal(t, want, got, "stale files in testdata/compat")
}

func TestCommittedSet(t *testing.T) {
	files, err := Generate(false)
	require.NoError(t, err)
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	require.Equal(t, []string{
		"bash_completion", "bin/gash", "gash.sh", "lib/compat/bootstrap.sh",
		"lib/core/config.sh", "lib/core/output.sh", "lib/core/utils.sh", "lib/core/validation.sh",
		"lib/modules/node.sh", "lib/modules/php.sh", "lib/modules/remote.sh",
	}, paths)
}

// tree writes the stage set with the repository's lint configuration.
func tree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files, err := Generate(true)
	require.NoError(t, err)
	require.NoError(t, Write(files, dir))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "VERSION"), []byte("2.1.0\n"), 0o644))
	for _, rc := range []string{".editorconfig", ".shellcheckrc"} {
		b, err := os.ReadFile(filepath.Join(repoRoot, rc))
		if err == nil {
			require.NoError(t, os.WriteFile(filepath.Join(dir, rc), b, 0o644))
		}
	}
	return dir
}

func bashFiles(dir string) []string {
	var out []string
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() && (strings.HasSuffix(p, ".sh") || filepath.Base(p) == "bash_completion") {
			out = append(out, p)
		}
		return err
	})
	sort.Strings(out)
	return out
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s %v:\n%s", name, args, out)
}

// bash -n, shellcheck -S warning and shfmt -d (the CI lint job) pass on
// every generated file; bin/gash is POSIX sh.
func TestLint(t *testing.T) {
	if testing.Short() {
		t.Skip("lint takes seconds: skipped with -short")
	}
	dir := tree(t)
	files := bashFiles(dir)
	require.NotEmpty(t, files)
	if _, err := exec.LookPath("bash"); err == nil {
		for _, f := range files {
			run(t, dir, "bash", "-n", f)
		}
	}
	run(t, dir, "sh", "-n", "bin/gash")
	if _, err := exec.LookPath("shellcheck"); err == nil {
		run(t, dir, "shellcheck", append([]string{"-s", "bash", "-S", "warning"}, files...)...)
		run(t, dir, "shellcheck", "-s", "sh", "-S", "warning", "bin/gash")
	} else {
		t.Log("shellcheck not installed: skipped")
	}
	if _, err := exec.LookPath("shfmt"); err == nil {
		run(t, dir, "shfmt", append([]string{"-d"}, files...)...)
	} else {
		t.Log("shfmt not installed: skipped")
	}
}

func needBash(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
}

func bash(t *testing.T, env []string, script string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", script)
	cmd.Env = append([]string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin", "LANG=C.UTF-8"}, env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Sourcing any generated file under set -euo pipefail prints nothing and
// returns 0, without a binary and in headless mode as a non-exported
// variable.
func TestSourcingIsQuiet(t *testing.T) {
	needBash(t)
	dir := tree(t)
	for _, f := range bashFiles(dir) {
		for _, mode := range []string{"GASH_HEADLESS=1", "GASH_HEADLESS="} {
			out, err := bash(t, nil, "set -euo pipefail; "+mode+"; source '"+f+"'; echo ok")
			require.NoError(t, err, f)
			require.Equal(t, "ok\n", out, "%s with %s", f, mode)
		}
	}
}

// Sourcing forks nothing (strace, when installed).
func TestSourcingForksNothing(t *testing.T) {
	needBash(t)
	if _, err := exec.LookPath("strace"); err != nil {
		t.Skip("strace not available")
	}
	dir := tree(t)
	for _, f := range bashFiles(dir) {
		log := filepath.Join(t.TempDir(), "trace")
		cmd := exec.Command("strace", "-f", "-qq", "-e", "trace=clone,clone3,fork,vfork", "-o", log,
			"bash", "--noprofile", "--norc", "-c", "GASH_HEADLESS=1; source '"+dir+"/gash.sh'; source '"+f+"'")
		cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin", "GASH_BIN=/bin/true"}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Skipf("strace cannot trace here: %v %s", err, out)
		}
		b, err := os.ReadFile(log)
		require.NoError(t, err)
		require.Empty(t, strings.TrimSpace(string(b)), f)
	}
}

// A call without a binary: the envelope in headless mode and for llm_*
// functions, Error: and Hint: lines otherwise; status 1.
func TestMissingBinary(t *testing.T) {
	needBash(t)
	dir := tree(t)
	env := []string{"GASH_RELEASE_URL=file:///nonexistent"}
	out, err := bash(t, env, "GASH_HEADLESS=1; source '"+dir+"/gash.sh'; gash_username")
	require.Error(t, err)
	require.Regexp(t, `^\{"error":"binary_not_found","details":"gash 2\.1\.0 for [a-z]+/[a-z0-9]+ is not installed and could not be downloaded","action":"STOP","recoverable":true,"hint":"Run: bash .*/install\.sh --repair"\}\n$`, out)

	out, err = bash(t, env, "source '"+dir+"/lib/core/output.sh'; source '"+dir+"/lib/core/utils.sh'; all_colors")
	require.Error(t, err)
	require.Regexp(t, `^Error: gash 2\.1\.0 for .* is not installed and could not be downloaded\nHint: Run: bash .*/install\.sh --repair\n$`, out)

	// The fallbacks of the shim helpers: needs_help's three lines, plain
	// table rows.
	out, err = bash(t, env, "GASH_HEADLESS=1; source '"+dir+"/gash.sh'; needs_help all_colors all_colors 'Displays all.' -h; printf 'A\\tB\\n1\\t2\\n' | __gash_table")
	require.NoError(t, err)
	require.Equal(t, "all_colors\nUsage: all_colors\nDisplays all.\nA\tB\n1\t2\n", out)
}

// The stub passes the caller's non-exported state, keeps set-empty and
// unset apart, lists the set settings and never passes the file-only ones.
func TestCallPassesState(t *testing.T) {
	needBash(t)
	dir := tree(t)
	fake := filepath.Join(t.TempDir(), "gash")
	require.NoError(t, os.WriteFile(fake, []byte("#!/bin/sh\nprintf '%s|' \"$@\"; echo; env | grep -E '^(GASH_|NO_COLOR|COLUMNS|GASH__)' | sort\n"), 0o755))
	out, err := bash(t, []string{"GASH_BIN=" + fake, "GASH_LLM_EXEC=1"},
		"GASH_HEADLESS=1; NO_COLOR=; COLUMNS=99; GASH_DB_TIMEOUT=7; source '"+dir+"/gash.sh'; llm_db_query 'a b' c")
	require.NoError(t, err, out)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Equal(t, "llm_db_query|a b|c|", lines[0])
	env := lines[1:]
	for _, kv := range []string{"GASH_HEADLESS=1", "NO_COLOR=", "COLUMNS=99", "GASH_DB_TIMEOUT=7", "GASH__SET=GASH_DB_TIMEOUT", "GASH__HELP=1"} {
		require.Contains(t, env, kv)
	}
	for _, kv := range env {
		require.False(t, strings.HasPrefix(kv, "GASH_LLM_EXEC") || strings.HasPrefix(kv, "GASH_NO_COLOR"), kv)
	}

	// The gash card: every word to the binary's root.
	out, err = bash(t, []string{"GASH_BIN=" + fake}, "GASH_HEADLESS=1; source '"+dir+"/gash.sh'; gash version --json")
	require.NoError(t, err, out)
	require.True(t, strings.HasPrefix(out, "version|--json|\n"), out)

	// A shim sourced alone: the help registry is not loaded.
	out, err = bash(t, []string{"GASH_BIN=" + fake}, "source '"+dir+"/lib/core/utils.sh'; all_colors -h")
	require.NoError(t, err, out)
	require.Contains(t, out, "GASH__HELP=0")
}

// Headless gash.sh defines the public functions and the module short
// aliases that the 2.x gash.sh of this checkout defines.
func TestHeadlessParityWith2x(t *testing.T) {
	needBash(t)
	if _, err := os.Stat(filepath.Join(repoRoot, "lib/core/help.sh")); err != nil {
		t.Skip("the 2.x sources are not in this tree")
	}
	root, err := filepath.Abs(repoRoot)
	require.NoError(t, err)
	dir := tree(t)
	list := `declare -F | cut -d' ' -f3 | grep -v '^_' | sort; alias | sort`
	want, err := bash(t, nil, "GASH_HEADLESS=1; source '"+root+"/gash.sh'; "+list)
	require.NoError(t, err)
	got, err := bash(t, nil, "GASH_HEADLESS=1; source '"+dir+"/gash.sh'; "+list)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

// Every compat helper that 2.x defined in the 4 core files is defined by
// the shims, under the same name.
func TestCoreHelpersKept(t *testing.T) {
	needBash(t)
	if _, err := os.Stat(filepath.Join(repoRoot, "lib/core/help.sh")); err != nil {
		t.Skip("the 2.x sources are not in this tree")
	}
	root, err := filepath.Abs(repoRoot)
	require.NoError(t, err)
	dir := tree(t)
	src := `for f in output utils validation config; do source "$1/lib/core/$f.sh"; done; declare -F | cut -d' ' -f3`
	want, err := bash(t, nil, "set -- '"+root+"'; "+src)
	require.NoError(t, err)
	got, err := bash(t, nil, "set -- '"+dir+"'; "+src)
	require.NoError(t, err)
	have := map[string]bool{}
	for _, n := range strings.Fields(got) {
		have[n] = true
	}
	for _, n := range strings.Fields(want) {
		require.True(t, have[n], "the shims do not define %s", n)
	}
}

// The stage holds the generated files instead of the 2.x implementation,
// the rest of the checkout, and the binary.
func TestStage(t *testing.T) {
	root := t.TempDir()
	for p, body := range map[string]string{
		"VERSION":               "9.9.9\n",
		"go-impl/go.mod":        "module x\n",
		"gash.sh":               "old\n",
		"lib/shell/prompt.sh":   "old\n",
		"lib/aliases/git.sh":    "old\n",
		"tests/specs/a_spec.sh": "spec\n",
		"quotes/list.txt":       "q\n",
		".git/HEAD":             "ref\n",
	} {
		p = filepath.Join(root, p)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	bin := filepath.Join(t.TempDir(), "gash")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755))
	found, err := FindRoot(filepath.Join(root, "tests", "specs"))
	require.NoError(t, err)
	require.Equal(t, root, found)

	out := filepath.Join(t.TempDir(), "stage")
	gashBin, err := Stage(root, out, bin)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(out, StageBin), gashBin)
	for _, p := range []string{"VERSION", "tests/specs/a_spec.sh", "quotes/list.txt", "lib/modules/llm.sh", "lib/core/output.sh", "bin/gash", StageBin} {
		require.FileExists(t, filepath.Join(out, p))
	}
	for _, p := range []string{"go-impl", ".git", "lib/shell", "lib/aliases"} {
		require.NoDirExists(t, filepath.Join(out, p))
	}
	b, err := os.ReadFile(filepath.Join(out, "gash.sh"))
	require.NoError(t, err)
	require.True(t, bytes.Contains(b, []byte("__gash_call")))

	_, err = Stage(root, out, bin)
	require.ErrorContains(t, err, "not empty")
	_, err = Stage(root, filepath.Join(root, "tests", "x"), bin)
	require.ErrorContains(t, err, "inside the checkout")
}
