// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// rgEnv is the environment of a comparison with the 2.x rg path: rg reads
// RIPGREP_CONFIG_PATH (LLM-16), which makes it sort by path, the order the
// native engine always uses; 2.x itself had rg's nondeterministic order.
func rgEnv(t *testing.T, home string) map[string]string {
	t.Helper()
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg is not installed: no 2.x reference")
	}
	cfg := filepath.Join(physTemp(t), "rgrc")
	mustWrite(t, cfg, "--sort=path\n")
	return baseEnv(home, map[string]string{"RIPGREP_CONFIG_PATH": cfg})
}

// searchFixture is a tree with every rule of the rg walk.
func searchFixture(t *testing.T) string {
	t.Helper()
	d := physTemp(t)
	w := func(rel, content string) { mustWrite(t, filepath.Join(d, rel), content) }
	w("src/a.php", "<?php\nneedle one\nfoo\nbar\nbaz\nneedle two\nqux\nquux\ncorge\ngrault\nneedle three\n")
	w("src/b.js", "x\nneedle js\n")
	w("src/c.md", "needle md")
	w("src/crlf.txt", "needle\r\nother\r\n")
	w("src/Upper.PHP", "needle upper\n")
	w("src/sub/deep.txt", "needle deep\n")
	w("src/sub/build/out.txt", "needle build\n")
	w(".hidden/x.php", "needle hidden\n")
	w(".hiddenfile.php", "needle hiddenfile\n")
	w("ignored.php", "needle ignored\n")
	w("dotignored.txt", "needle dotignored\n")
	w("rgignored.txt", "needle rgignored\n")
	w("top.txt", "needle top\n")
	w("src/top.txt", "needle src top\n")
	w("a.log", "needle log\n")
	w("keep.log", "needle keep\n")
	w("node_modules/y.php", "needle nm\n")
	w("vendor/v.php", "needle vendor\n")
	w("__pycache__/z.php", "needle pycache\n")
	w(".env", "needle=secret\n")
	w("app.key", "needle key\n")
	w("credentials", "needle creds\n")
	w("prod.ENV", "needle upper env\n")
	w(".ssh/id_x", "needle ssh\n")
	w("u/.config/gcloud/cfg", "needle gcloud\n")
	w("sub/.env.example", "needle example\n")
	w("bin.dat", "needle before\n\x00needle after\n")
	w("bom.txt", "\xef\xbb\xbfneedle bom\n")
	w("u16.txt", "\xff\xfen\x00e\x00e\x00d\x00l\x00e\x00\n\x00")
	w("nonl.txt", "a\nneedle nonl")
	w("ctl.txt", "needle \x01\x1b ctl\n")
	w(".git/HEAD", "ref: refs/heads/main\n")
	w(".git/info/exclude", "excluded.txt\n")
	w("excluded.txt", "needle excluded\n")
	w(".gitignore", "ignored.php\n/top.txt\nbuild/\n*.log\n!keep.log\n")
	w(".ignore", "dotignored.txt\n")
	w(".rgignore", "rgignored.txt\n")
	require.NoError(t, os.Symlink("src/b.js", filepath.Join(d, "link.js")))
	return d
}

func TestGrepMatches2x(t *testing.T) {
	d := searchFixture(t)
	home := physTemp(t)
	env := rgEnv(t, home)
	m := New()
	for _, args := range [][]string{
		{"needle"},
		{"needle", "."},
		{"needle", d},
		{"needle", "src"},
		{"needle", "src/a.php"},
		{"needle", "--ext", "php"},
		{"needle", "--ext", "php,js"},
		{"needle", "--ext", "php,,js,"},
		{"needle", "--ext", ","},
		{"needle", "--ext", "{php,md}"},
		{"needle", "--ext", "env"},
		{"needle", "--ext", "log"},
		{"needle", "--ext", "txt"},
		{"needle", "--ext", "php\njs"},
		{"needle", "--ext", "["},
		{"needle", "-C", "1"},
		{"needle", "--context", "2", "src"},
		{"needle", "-C", "1", "src/a.php"},
		{"needle", "-C", "0"},
		{"needle", "-C", "007", "--limit", "9"},
		{"needle", "--limit", "3"},
		{"needle", "-n", "0"},
		{"needle", "-n", "1", "src/a.php"},
		{"needle", "bin.dat"},
		{"after", "bin.dat"},
		{"needle", "-C", "1", "bin.dat"},
		{"after", "-C", "1", "bin.dat"},
		{"nothing", "bin.dat"},
		{"needle", "bom.txt"},
		{"needle", "u16.txt"},
		{"needle", "sub/.env.example"},
		{"needle", "link.js"},
		{"needle$", "src"},
		{"^", "src/c.md"},
		{"(?i)NEEDLE", "src"},
		{"ne+dle (one|two)", "src"},
		{"a{3}", "src"},
		{"needle", "nonexistent"},
		{"needle", "/nonexistent"},
		// refusals and argument errors, in the 2.x order
		{"needle", ".env"},
		{"needle", "app.key"},
		{"needle", ".ssh"},
		{"needle", "../x"},
		{"needle", "/proc/1"},
		{},
		{""},
		{"", "needle"},
		{"needle", "--ext"},
		{"needle", "-C"},
		{"needle", "--limit"},
		{"needle", "--bogus"},
		{"needle", ".", "src"},
		{"needle", "--limit", "abc"},
		{"needle", "--limit", "1234567890"},
		{"needle", "--limit", "x", "-C", "y"},
		{"needle", "-h"},
		{"--ext", "--help"},
	} {
		same(t, m, d, env, "llm_grep", args...)
	}
}

// TestGrepGlobalExcludes: the global excludes file of git (core.excludesFile
// or ~/.config/git/ignore) applies in a repository only.
func TestGrepGlobalExcludes(t *testing.T) {
	m := New()
	home := physTemp(t)
	env := rgEnv(t, home)
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "repo", ".git", "HEAD"), "x\n")
	mustWrite(t, filepath.Join(d, "repo", "g.txt"), "needle\n")
	mustWrite(t, filepath.Join(d, "repo", "h.txt"), "needle\n")
	mustWrite(t, filepath.Join(d, "plain", "g.txt"), "needle\n")
	mustWrite(t, filepath.Join(home, ".config", "git", "ignore"), "g.txt\n")
	same(t, m, d, env, "llm_grep", "needle", "repo")
	same(t, m, d, env, "llm_grep", "needle", "plain")
	mustWrite(t, filepath.Join(home, ".gitconfig"), "[core]\n\texcludesFile = ~/my-ignore\n")
	mustWrite(t, filepath.Join(home, "my-ignore"), "h.txt\n")
	same(t, m, d, env, "llm_grep", "needle", "repo")
	// A .gitignore above the search root applies; one above the repository
	// root does not.
	mustWrite(t, filepath.Join(d, ".gitignore"), "h.txt\n")
	mustWrite(t, filepath.Join(d, "repo", ".gitignore"), "sub/x.txt\n")
	mustWrite(t, filepath.Join(d, "repo", "sub", "x.txt"), "needle\n")
	mustWrite(t, filepath.Join(d, "repo", "sub", "y.txt"), "needle\n")
	same(t, m, d, env, "llm_grep", "needle", "repo/sub")
	mustWrite(t, filepath.Join(d, ".ignore"), "y.txt\n")
	same(t, m, d, env, "llm_grep", "needle", "repo/sub")
}

// TestLedger_LLM01: --context is a 1-9 digit integer; anything else is
// invalid_context (RETRY) and nothing is evaluated or searched.
func TestLedger_LLM01(t *testing.T) {
	m := New()
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "f"), "x\n")
	env := baseEnv(physTemp(t), nil)
	marker := filepath.Join(d, "marker")
	for _, v := range []string{"abc", "a[$(touch " + marker + ")]", "1+1", "-1", "", "1234567890", " 1"} {
		r := runGo(t, m, d, env, "llm_grep", "x", "--context", v)
		require.Equal(t, result{"", `{"error":"invalid_context","details":"Value '` + v +
			`' is not a positive integer below 1000000000","action":"RETRY","recoverable":true,"hint":"Provide a plain integer for context"}` + "\n", 1}, r, v)
	}
	require.NoFileExists(t, marker)
	r := runGo(t, m, d, env, "llm_grep", "x", "-C", "1")
	require.Equal(t, result{d + "/f:1:x\n", "", 0}, r)
	// The limit is checked first, as the other integers of the grammar.
	r = runGo(t, m, d, env, "llm_grep", "x", "-C", "z", "-n", "y")
	require.Contains(t, r.Stderr, `"invalid_limit"`)
}

// TestLedger_LLM03: one engine with rg semantics, whatever is installed:
// the same output with an empty PATH.
func TestLedger_LLM03(t *testing.T) {
	m := New()
	d := searchFixture(t)
	home := physTemp(t)
	env := baseEnv(home, nil)
	want := runGo(t, m, d, env, "llm_grep", "needle|FIXME")
	env["PATH"] = ""
	got := runGo(t, m, d, env, "llm_grep", "needle|FIXME")
	require.Equal(t, want, got)
	require.Contains(t, got.Stdout, d+"/src/a.php:2:needle one\n")
	require.NotContains(t, got.Stdout, "bin.dat")
	require.NotContains(t, got.Stdout, ".hidden")
	r := runGo(t, m, d, env, "llm_grep", "fo+", "src")
	require.Equal(t, d+"/src/a.php:3:foo\n", r.Stdout)
	r = runGo(t, m, d, env, "llm_grep", "u{2}", "src")
	require.Equal(t, d+"/src/a.php:8:quux\n", r.Stdout)
}

// TestLedger_LLM07_2x: a file PATH prints line:content, a directory
// path:line:content.
func TestLedger_LLM07_2x(t *testing.T) {
	m := New()
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "f.txt"), "hit\n")
	env := rgEnv(t, physTemp(t))
	require.Equal(t, "1:hit\n", same(t, m, d, env, "llm_grep", "hit", "f.txt").Stdout)
	require.Equal(t, d+"/f.txt:1:hit\n", same(t, m, d, env, "llm_grep", "hit", ".").Stdout)
}

// TestLedger_LLM15_2x: a walk leaves out secret-exception files too, an
// explicit exception file is searched.
func TestLedger_LLM15_2x(t *testing.T) {
	m := New()
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, ".env.example"), "hit\n")
	env := rgEnv(t, physTemp(t))
	require.Equal(t, "", same(t, m, d, env, "llm_grep", "hit", ".").Stdout)
	require.Equal(t, "1:hit\n", same(t, m, d, env, "llm_grep", "hit", ".env.example").Stdout)
}

// TestLedger_LLM16: RIPGREP_CONFIG_PATH changes nothing.
func TestLedger_LLM16(t *testing.T) {
	m := New()
	d := searchFixture(t)
	home := physTemp(t)
	env := baseEnv(home, nil)
	want := runGo(t, m, d, env, "llm_grep", "needle")
	cfg := filepath.Join(home, "rgrc")
	mustWrite(t, cfg, "--hidden\n--no-ignore\n--files-with-matches\n")
	env["RIPGREP_CONFIG_PATH"] = cfg
	require.Equal(t, want, runGo(t, m, d, env, "llm_grep", "needle"))
}

// TestLedger_LLM17_2x: a missing PATH is a silent success. (An invalid
// regex, silent in 2.x too, fails with invalid_regex: LLM-37.)
func TestLedger_LLM17_2x(t *testing.T) {
	m := New()
	d := physTemp(t)
	env := rgEnv(t, physTemp(t))
	require.Equal(t, result{"", "", 0}, same(t, m, d, env, "llm_grep", "x", "/nonexistent"))
	require.Equal(t, result{"", "", 0}, same(t, m, d, env, "llm_find", "*", "/nonexistent"))
	require.Equal(t, result{"", "", 0}, run2x(t, d, env, "llm_grep", "(", "."))
	require.Equal(t, 1, runGo(t, m, d, env, "llm_grep", "(", ".").RC)
}

// TestLedger_LLM26: binary files are skipped in a walk.
func TestLedger_LLM26(t *testing.T) {
	m := New()
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "b"), "hit\x00\n")
	mustWrite(t, filepath.Join(d, "t"), "hit\n")
	env := baseEnv(physTemp(t), map[string]string{"PATH": ""})
	require.Equal(t, d+"/t:1:hit\n", runGo(t, m, d, env, "llm_grep", "hit").Stdout)
}

func TestSearchTreeSize(t *testing.T) {
	m := New()
	d := physTemp(t)
	for _, n := range []string{"b", "a", "a.txt", "a-b", "B", "_x"} {
		mustWrite(t, filepath.Join(d, n, "f"), "hit\n")
		mustWrite(t, filepath.Join(d, n+"f"), "hit\n")
	}
	env := rgEnv(t, physTemp(t))
	r := same(t, m, d, env, "llm_grep", "hit")
	require.Equal(t, 12, strings.Count(r.Stdout, "\n"))
}

// TestLedger_LLM38: a file with a NUL anywhere (its first byte, the middle,
// past rg's first 64 KiB buffer) is binary and skipped whole, by llm_grep
// and llm_find --contains alike.
func TestLedger_LLM38(t *testing.T) {
	archive2x(t) // before runGo changes the directory
	m := New()
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "start"), "\x00needle\n")
	mustWrite(t, filepath.Join(d, "middle"), "needle\n\x00\nneedle\n")
	mustWrite(t, filepath.Join(d, "end"), "needle\n"+strings.Repeat("pad\n", 40000)+"\x00")
	mustWrite(t, filepath.Join(d, "text"), "needle\n")
	env := baseEnv(physTemp(t), map[string]string{"PATH": ""})
	require.Equal(t, result{d + "/text:1:needle\n", "", 0}, runGo(t, m, d, env, "llm_grep", "needle", d))
	require.Equal(t, result{d + "/text\n", "", 0}, runGo(t, m, d, env, "llm_find", "*", d, "--contains", "needle"))
	if _, err := exec.LookPath("rg"); err == nil {
		// 2.x: rg reported the match before a NUL past its first buffer.
		require.Contains(t, run2x(t, d, rgEnv(t, physTemp(t)), "llm_grep", "needle", d).Stdout, d+"/end:1:needle")
	}
}
