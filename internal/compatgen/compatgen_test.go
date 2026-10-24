// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package compatgen

import (
	"bytes"
	"flag"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/compat"
	"github.com/mauriziofonte/gash/internal/registry"
)

var update = flag.Bool("update", false, "rewrite the golden copies in testdata/compat")

const (
	repoRoot = "../.."
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

// A "stub/NAME" template replaces the one-line stub of NAME everywhere it
// is defined; an override of a name without a stub is refused.
func TestStubOverride(t *testing.T) {
	overlay := func(stubs string) fstest.MapFS {
		m := fstest.MapFS{}
		entries, err := fs.ReadDir(compat.FS, ".")
		require.NoError(t, err)
		for _, e := range entries {
			b, err := fs.ReadFile(compat.FS, e.Name())
			require.NoError(t, err)
			m[e.Name()] = &fstest.MapFile{Data: b}
		}
		m["stubs.tmpl"] = &fstest.MapFile{Data: []byte(stubs)}
		return m
	}
	files, err := generate(overlay(`{{% define "stub/mkdir_cd" %}}
{{% .Name %}}() {
    __gash_call {{% .Name %}} "$@" && cd -- "$1"
}
{{% end %}}`), true)
	require.NoError(t, err)
	want := "mkdir_cd() {\n    __gash_call mkdir_cd \"$@\" && cd -- \"$1\"\n}\n"
	for _, f := range files {
		if f.Path == "gash.sh" || f.Path == "lib/modules/system.sh" {
			require.Contains(t, string(f.Data), want, f.Path)
			require.NotContains(t, string(f.Data), "mkdir_cd() { __gash_call", f.Path)
		}
	}
	_, err = generate(overlay(`{{% define "stub/needs_help" %}}x{{% end %}}`), true)
	require.ErrorContains(t, err, `"needs_help" has no stub`)
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
	require.NoError(t, os.WriteFile(fake, []byte("#!/bin/sh\n[ \"$1\" = __compat ] && exit 0\nprintf '%s|' \"$@\"; echo; env | grep -E '^(GASH_|NO_COLOR|COLUMNS|GASH__)' | sort\n"), 0o755))
	out, err := bash(t, []string{"GASH_BIN=" + fake, "GASH_LLM_EXEC=1"},
		"GASH_HEADLESS=1; NO_COLOR=; export COLUMNS=99; GASH_DB_TIMEOUT=7; source '"+dir+"/gash.sh'; llm_db_query 'a b' c")
	require.NoError(t, err, out)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Equal(t, "llm_db_query|a b|c|", lines[0])
	env := lines[1:]
	for _, kv := range []string{"GASH_HEADLESS=1", "NO_COLOR=", "COLUMNS=99", "GASH_DB_TIMEOUT=7", "GASH__SET=GASH_DB_TIMEOUT", "GASH__HELP=1"} {
		require.Contains(t, env, kv)
	}
	added := ""
	for _, kv := range env {
		require.False(t, strings.HasPrefix(kv, "GASH_LLM_EXEC") || strings.HasPrefix(kv, "GASH_NO_COLOR"), kv)
		if v, ok := strings.CutPrefix(kv, "GASH__ADDED="); ok {
			added = v
		}
	}
	// Exported by the stub only: everything but COLUMNS, which the caller
	// exported itself (GASH_DIR is a shell variable of gash.sh).
	require.ElementsMatch(t, []string{"GASH_HEADLESS", "NO_COLOR", "GASH_DIR", "GASH_DB_TIMEOUT"}, strings.Fields(added))

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
		"go.mod":                "module x\n",
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

// LEDGER SYS-34: __gash_section takes COLUMNS only when it is a run of
// digits; anything else is 80 (a 60-glyph rule) and is never evaluated.
func TestLedger_SYS34(t *testing.T) {
	needBash(t)
	dir := tree(t)
	mark := filepath.Join(t.TempDir(), "MARK")
	for cols, rule := range map[string]int{"a[$(touch " + mark + ")]": 60, "abc": 60, "": 60, "30": 30, "010": 8, "99": 60} {
		out, err := bash(t, []string{"LC_ALL=C"}, "source '"+dir+"/lib/core/output.sh'; COLUMNS='"+cols+"' __gash_section T")
		require.NoError(t, err)
		require.Equal(t, "\n"+strings.Repeat("-", rule)+"\nT\n\n", out, cols)
	}
	require.NoFileExists(t, mark)
}

// LEDGER CORE-18: the stub of each function whose 2.x body loads the
// settings file runs the shim's cached __gash_load_env first, and no other
// stub does; in a shell without gash.sh the warnings then come once, at
// the first such call, from the shell. After a headless gash.sh load the
// first load is quiet, unless GASH_DEBUG=1 loads at source time.
func TestLedger_CORE18(t *testing.T) {
	needBash(t)
	dir := tree(t)
	var names []string
	want := map[string]bool{}
	for _, f := range registry.Functions() {
		if f.HasStub() {
			names = append(names, f.Name)
			want[f.Name] = f.LoadsEnv
		}
	}
	require.True(t, want["gash_db_list"] && want["llm_exec"] && want["ai_query"] && !want["llm_tree"] && !want["all_colors"])
	out, err := bash(t, nil, "GASH_HEADLESS=1; source '"+dir+"/gash.sh'; for f in "+strings.Join(names, " ")+"; do declare -f \"$f\"; printf '\\0'; done")
	require.NoError(t, err, out)
	defs := strings.Split(out, "\x00")
	for i, n := range names {
		require.Equal(t, want[n], strings.Contains(defs[i], "__gash_load_env"), "%s: %s", n, defs[i])
	}

	fake := filepath.Join(t.TempDir(), "gash")
	require.NoError(t, os.WriteFile(fake, []byte("#!/bin/sh\nif [ \"$1 $2 $3\" = '__compat env load' ]; then\n"+
		"  echo 'Warning: Line 1 of ~/.gash_env: unrecognized entry' >&2; echo __GASH_ENV_LOADED=1; exit 0\nfi\necho \"call $1\"\n"), 0o755))
	out, err = bash(t, []string{"GASH_BIN=" + fake}, "for f in output utils config; do source '"+dir+"/lib/core/'$f.sh; done; "+
		"source '"+dir+"/lib/modules/llm.sh'; llm_tree; gash_db_list; llm_exec ls; gash_db_list")
	require.NoError(t, err, out)
	require.Equal(t, "call llm_tree\nWarning: Line 1 of ~/.gash_env: unrecognized entry\ncall gash_db_list\ncall llm_exec\ncall gash_db_list\n", out)
	// GASH_HEADLESS=1 without gash.sh: as loud as 2.x was.
	out, err = bash(t, []string{"GASH_BIN=" + fake, "GASH_HEADLESS=1"}, "for f in output utils config; do source '"+dir+"/lib/core/'$f.sh; done; "+
		"source '"+dir+"/lib/modules/llm.sh'; llm_exec ls")
	require.NoError(t, err, out)
	require.Equal(t, "Warning: Line 1 of ~/.gash_env: unrecognized entry\ncall llm_exec\n", out)

	// A headless gash.sh load parses quietly at the first call, as the 2.x
	// headless startup did; with GASH_DEBUG=1 it loads at source time,
	// warnings shown, and the calls then use the cache.
	out, err = bash(t, []string{"GASH_BIN=" + fake, "GASH_HEADLESS=1"}, "source '"+dir+"/gash.sh'; echo sourced; llm_exec ls; gash_db_list")
	require.NoError(t, err, out)
	require.Equal(t, "sourced\ncall llm_exec\ncall gash_db_list\n", out)
	out, err = bash(t, []string{"GASH_BIN=" + fake, "GASH_HEADLESS=1", "GASH_DEBUG=1"}, "source '"+dir+"/gash.sh'; echo sourced; llm_exec ls; gash_db_list")
	require.NoError(t, err, out)
	require.Equal(t, "Warning: Line 1 of ~/.gash_env: unrecognized entry\nsourced\ncall llm_exec\ncall gash_db_list\n", out)
}

// The gash stub passes the values the caller's shell gives the card's
// aliases (LEDGER SELF-12): one NAME<TAB>VALUE line per defined one,
// backslash and newline escaped; names the card does not list and
// undefined ones stay out, and the words reach the binary's root as they
// are.
func TestGashStubPassesCardAliases(t *testing.T) {
	needBash(t)
	dir := tree(t)
	fake := filepath.Join(t.TempDir(), "gash")
	require.NoError(t, os.WriteFile(fake, []byte("#!/bin/sh\n[ \"$1 $2 $3\" = '__compat env load' ] && exit 0\n"+
		"printf '[%s]' \"$@\"; printf '\\n%s' \"$GASH__CARD_ALIASES\"\n"), 0o755))
	out, err := bash(t, []string{"GASH_BIN=" + fake, "GASH_HEADLESS=1"}, "set -u; source '"+dir+"/gash.sh' >/dev/null 2>&1; unalias -a; "+
		`alias gst='git status --short' gd='a\b' ll=$'ls\n-l' mine=x ..='cd ..'; gash git --no-color; `+
		`gash; echo "${GASH__CARD_ALIASES-unset}"`)
	require.NoError(t, err, out)
	card := "..\tcd ..\ngd\ta\\\\b\ngst\tgit status --short\nll\tls\\n-l\n"
	require.Equal(t, "[git][--no-color]\n"+card+"[]\n"+card+"unset\n", out)
}

// bashInteractive runs script in an interactive bash without a binary
// (GASH_BIN=/bin/true: `gash init bash` emits nothing).
func bashInteractive(t *testing.T, script string) string {
	t.Helper()
	cmd := exec.Command("bash", "--noprofile", "--norc", "-i", "-c", script)
	cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin", "LANG=C.UTF-8", "TERM=dumb", "GASH_BIN=/bin/true"}
	cmd.Stdin = nil
	out, err := cmd.Output()
	require.NoError(t, err, string(out))
	return string(out)
}

// LEDGER SELF-03: gash_unload removes the completion specs Gash registered
// and restores the variables Gash sets or exports to their exact pre-load
// state: unset ones are unset (not left exported), set ones keep their
// value and export flag, and an unset PROMPT_COMMAND is unset again rather
// than left empty (3.0: SELF-03 fixed). The end-to-end proof is
// testdata/script/unload_exact.txtar.
func TestLedger_SELF03(t *testing.T) {
	needBash(t)
	dir := tree(t)
	out := bashInteractive(t, `complete -F _mine mine; LESS_TERMCAP_md=keep; unset PROMPT_COMMAND
source '`+dir+`/gash.sh' >/dev/null 2>&1; source '`+dir+`/bash_completion'
complete -p | grep -c __gash_complete >/dev/null || echo "no specs registered"
# What the shell layer does on a real load:
export LESSOPEN='| lesspipe %s' LS_COLORS='di=01' LESS_TERMCAP_mb=x LESS_TERMCAP_md=y PAGER=most
PROMPT_COMMAND=""
gash_unload
echo "specs=$(complete -p | grep -c __gash_complete)"
complete -p mine
for v in LESSOPEN LS_COLORS LESS_TERMCAP_mb PAGER PROMPT_COMMAND; do echo "$v=${!v-unset}"; done
declare -p LESS_TERMCAP_md`)
	require.Equal(t, "specs=0\ncomplete -F _mine mine\nLESSOPEN=unset\nLS_COLORS=unset\nLESS_TERMCAP_mb=unset\nPAGER=unset\n"+
		"PROMPT_COMMAND=unset\ndeclare -- LESS_TERMCAP_md=\"keep\"\n", out)
}

// LEDGER SELF-05: a GASH_* setting the user had before Gash loaded keeps its
// value and export flag after gash_unload; the GASH_* variables Gash set and
// the secret caches are gone (3.0: SELF-05 fixed; 2.x swept them all).
func TestLedger_SELF05(t *testing.T) {
	needBash(t)
	dir := tree(t)
	out := bashInteractive(t, `export GASH_BANNER=0; GASH_GIT_EXCLUDE=/srv/repo
source '`+dir+`/gash.sh' >/dev/null 2>&1
__GASH_ENV_DB_default=secret; GASH_BANNER=1
gash_unload
declare -p GASH_BANNER GASH_GIT_EXCLUDE
echo "GASH_VERSION=${GASH_VERSION-gone} GASH_DIR=${GASH_DIR-gone} cache=${__GASH_ENV_DB_default-gone}"`)
	require.Equal(t, "declare -x GASH_BANNER=\"0\"\ndeclare -- GASH_GIT_EXCLUDE=\"/srv/repo\"\n"+
		"GASH_VERSION=gone GASH_DIR=gone cache=gone\n", out)
}

// LEDGER SHELL-13: the headless branch of gash.sh returns before the
// interactive layer (`gash init bash`, which carries the 1.x reminder), so
// sourcing it headless with a 1.x ~/.gashrc prints nothing and writes no
// state (forced divergence; 2.x printed the reminder and wrote a notice).
func TestLedger_SHELL13(t *testing.T) {
	needBash(t)
	dir := tree(t)
	b, err := os.ReadFile(filepath.Join(dir, "gash.sh"))
	require.NoError(t, err)
	src := string(b)
	headless := strings.Index(src, "if [[ \"${GASH_HEADLESS-}\" == \"1\" ]]; then\n    if [[ \"${GASH_DEBUG-}\" == \"1\" ]]; then")
	require.Greater(t, headless, 0)
	ret := strings.Index(src[headless:], "\n    return 0\nfi\n")
	require.Greater(t, ret, 0)
	require.Greater(t, strings.Index(src, `" init bash)"`), headless+ret, "gash init bash runs after the headless return")

	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gashrc"), []byte("export GASH_BASH=\"$HOME/.gash/gash.sh\"\n"), 0o644))
	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", "GASH_HEADLESS=1; source '"+dir+"/gash.sh'; echo rc=$?")
	cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin", "LANG=C.UTF-8"}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)
	require.Equal(t, "rc=0\n", string(out))
	_, err = os.Stat(filepath.Join(home, ".local/state/gash"))
	require.True(t, os.IsNotExist(err), "no state written")
}

// LEDGER CORE-02: the generated helpers take a numeric operand only when
// it is a run of digits (with an optional minus); anything else is 0 and
// is never evaluated, so an array subscript holding a command
// substitution runs nothing. Integer operands give the 2.x output. The
// end-to-end proof is testdata/script/compat_helpers_arith.txtar.
func TestLedger_CORE02(t *testing.T) {
	needBash(t)
	dir := tree(t)
	tmp := t.TempDir()
	src := "GASH_HEADLESS=1; for f in output utils validation; do source '" + dir + "/lib/core/'$f.sh; done; "
	calls := map[string]string{
		"require_args_min": `__gash_require_args_min "$P" 1; echo "rc=$?"`,
		"bar_value":        `__gash_bar v "$P" 10 10; echo "[$v]"`,
		"bar_max":          `__gash_bar v 5 "$P" 10; echo "[$v]"`,
		"bar_width":        `__gash_bar v 5 10 "$P"; echo "[$v]"`,
		"want_colors":      `__gash_want_colors "$P"; echo "rc=$?"`,
		"wrap":             `__gash_wrap a "$P" 'hello world'; echo "${a[*]}"`,
		"choose":           `__gash_choose c "$P" x y </dev/null; echo "rc=$?"`,
		"progress":         `__gash_progress "$P" "$P" m; echo "rc=$?"`,
		"section":          `COLUMNS="$P" __gash_section T`,
	}
	for name, call := range calls {
		mark := filepath.Join(tmp, name)
		out, err := bash(t, []string{"LC_ALL=C"}, src+"P='a[$(touch "+mark+")]'; "+call)
		require.NoError(t, err, "%s: %s", name, out)
		require.NoFileExists(t, mark, "%s ran the substitution", name)
	}

	// A plain word is 0 (2.x read the unset variable of that name), and
	// integer vectors keep the 2.x results.
	out, err := bash(t, []string{"LC_ALL=C"}, src+`__gash_require_args_min abc 1; echo "rc=$?"; `+
		`__gash_require_args_min 2 1; echo "rc=$?"; __gash_require_args_min 0 1; echo "rc=$?"; `+
		`__gash_bar v 3 10 10; echo "[$v]"; __gash_bar v 1 100 10; echo "[$v]"; __gash_bar v 0 10 5; echo "[$v]"; `+
		`__gash_want_colors 1; echo "rc=$?"`)
	require.NoError(t, err, out)
	require.Equal(t, "Error: Expected at least 1 argument(s), got abc.\nrc=1\nrc=0\n"+
		"Error: Expected at least 1 argument(s), got 0.\nrc=1\n"+
		"[###\t.......]\n[#\t.........]\n[\t.....]\nrc=1\n", out)
}

// LEDGER CORE-14 in the generated Bash: __gash_shown (the settings-file
// permission warnings of the fallback loader, the gash_uninstall question)
// shows HOME as "~" only as a literal prefix followed by "/" or the end
// (3.0: CORE-14 fixed; 2.x used ${PATH/#$HOME/\~}, a glob).
func TestLedger_CORE14(t *testing.T) {
	needBash(t)
	dir := tree(t)
	out, err := bash(t, nil, `source '`+dir+`/lib/core/output.sh'; s() { HOME=$1; __gash_shown v "$2"; printf '%s|' "$v"; }; `+
		`s /home/a /home/a/x; s /home/a /home/ab/x; s /home/a /home/a; s /home/a/ /home/a/y; s "" /x; s '/home/*' /home/b/x; s '/h/[ab]' /h/a/x`)
	require.NoError(t, err, out)
	require.Equal(t, "~/x|/home/ab/x|~|~/y|/x|/home/b/x|/h/a/x|", out)
}

// LEDGER SHELL-03: TAB after "gash " offers all, the card topics and the
// binary's commands, all from the registry, and no longer "help" (an
// unknown topic: 3.0: SHELL-03 fixed). Every topic candidate is one the
// card accepts; testdata/script/completion_gash_topics.txtar runs them.
func TestLedger_SHELL03(t *testing.T) {
	needBash(t)
	dir := tree(t)
	out := bashInteractive(t, `source '`+dir+`/gash.sh' >/dev/null 2>&1; source '`+dir+`/bash_completion'; COMP_WORDS=(gash ""); COMP_CWORD=1; __gash_complete gash "" gash; printf '%s\n' "${COMPREPLY[@]}"`)
	got := strings.Fields(out)
	want := []string{"all"}
	for _, tp := range registry.Topics() {
		want = append(want, tp.Name)
	}
	want = append(want, registry.Commands()...)
	require.Equal(t, want, got)
	require.NotContains(t, got, "help")
	require.NotContains(t, got, "migrate")
}

// topLevel returns the lines of a generated Bash file outside function
// bodies (the generated files are shfmt-formatted: a function opens with
// "NAME() {" at column 0 and closes with "}" at column 0).
func topLevel(text string) []string {
	var out []string
	inFunc := false
	for _, l := range strings.Split(text, "\n") {
		switch {
		case !inFunc && funcOpen.MatchString(l):
			inFunc = true
		case inFunc && l == "}":
			inFunc = false
		case !inFunc:
			out = append(out, l)
		}
	}
	return out
}

var (
	funcOpen      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_:.-]*\(\) \{$`)
	hygieneBad    = regexp.MustCompile(`=~|(^|[;&|{(\s])readonly\s|declare\s+-[A-Za-z]*r`)
	gashAssign    = regexp.MustCompile(`(?:^|[^A-Za-z0-9_$])(__GASH_[A-Z0-9_]+)(?:\[[^]]*\])?\+?=`)
	gashDeclared  = regexp.MustCompile(`declare\s+-[A-Za-z]+\s+(__GASH_[A-Z0-9_]+)`)
	gashAllowedRE = regexp.MustCompile(`^__GASH_(COLOR|GLYPH|SYMBOL|BOOT|ENV|SPINNER|CALL|BOLD|SNAPSHOT)(_[A-Z0-9_]+)?$`)
)

// gashAllowed are the other __GASH_* names a generated file may set at
// load time: the palette and screen sequences, the load bookkeeping that
// gash_unload, gash_doctor --timing and completion read.
var gashAllowed = map[string]bool{
	"__GASH_AWK": true, "__GASH_RED": true, "__GASH_GREEN": true, "__GASH_YELLOW": true, "__GASH_BLUE": true,
	"__GASH_CYAN": true, "__GASH_WHITE": true, "__GASH_DIM": true, "__GASH_UNDERLINE": true,
	"__GASH_CLEAR_SCREEN": true, "__GASH_CURSOR_HIDE": true, "__GASH_CURSOR_SHOW": true, "__GASH_ERASE_EOL": true,
	"__GASH_FETCH_REASON": true, "__GASH_HELP_LOADED": true, "__GASH_LOAD_ERRORS": true, "__GASH_LOAD_TIMES": true,
	"__GASH_SETTINGS_KNOWN": true, "__GASH_T0": true, "__GASH_T1": true, "__GASH_TIMING": true,
	"__GASH_USER_ALIASES": true, "__GASH_PRE_SYMBOLS": true, "__GASH_POST_SYMBOLS": true,
	"__GASH_COMPLETE_NAMES": true, "__GASH_SNAPSHOT2": true,
}

// LEDGER SHELL-10: loading leaves no BASH_REMATCH and nothing readonly in
// the caller's shell: no generated file runs =~, readonly or declare -r at
// top level, and the __GASH_* names they set while loading are the
// documented bookkeeping of the allowlist. The init script is linted in
// internal/shellinit, the loaded shell in testdata/script/load_hygiene.txtar.
func TestLedger_SHELL10(t *testing.T) {
	dir := tree(t)
	files := append(bashFiles(dir), filepath.Join(dir, "bash_completion"))
	set := map[string]bool{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		for _, l := range topLevel(string(b)) {
			code := strings.TrimSpace(l)
			if strings.HasPrefix(code, "#") {
				continue
			}
			require.False(t, hygieneBad.MatchString(code), "%s: top-level %q", f, l)
			for _, m := range gashAssign.FindAllStringSubmatch(code, -1) {
				set[m[1]] = true
			}
			for _, m := range gashDeclared.FindAllStringSubmatch(code, -1) {
				set[m[1]] = true
			}
		}
	}
	require.NotEmpty(t, set)
	for name := range set {
		require.True(t, gashAllowed[name] || gashAllowedRE.MatchString(name), "%s is set at load time and not in the allowlist", name)
	}
}

// LEDGER SHELL-11: gash.sh sourced through a file link, a chain of two
// links (one relative) or a linked directory finds its tree: GASH_DIR is
// the real directory and GASH_VERSION the tree's version, where 2.x took
// the link's directory and loaded nothing (3.0: SHELL-11 fixed).
func TestLedger_SHELL11(t *testing.T) {
	needBash(t)
	dir := tree(t)
	links := t.TempDir()
	require.NoError(t, os.Symlink(filepath.Join(dir, "gash.sh"), filepath.Join(links, "one.sh")))
	require.NoError(t, os.Mkdir(filepath.Join(links, "sub"), 0o755))
	require.NoError(t, os.Symlink("../one.sh", filepath.Join(links, "sub", "two.sh")))
	require.NoError(t, os.Symlink(dir, filepath.Join(links, "tree")))
	for _, src := range []string{
		filepath.Join(links, "one.sh"),
		filepath.Join(links, "sub", "two.sh"),
	} {
		out, err := bash(t, []string{"GASH_HEADLESS=1"}, `source '`+src+`'; printf '%s|%s' "$GASH_DIR" "$GASH_VERSION"`)
		require.NoError(t, err, out)
		require.Equal(t, dir+"|2.1.0", out, src)
	}
	out, err := bash(t, []string{"GASH_HEADLESS=1"}, `source '`+links+`/tree/gash.sh'; printf '%s|%s' "$GASH_DIR" "$GASH_VERSION"`)
	require.NoError(t, err, out)
	require.Equal(t, links+"/tree|2.1.0", out)
}
