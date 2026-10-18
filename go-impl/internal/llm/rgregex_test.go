// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// rgCorpus is the text testdata/rg_syntax.tsv was recorded on: ASCII,
// Unicode letters, digits, marks and spaces, and lines that are not UTF-8.
var rgCorpus = []string{
	"aaa", "a{,2}", "foo{bar}", "x{2}", "٣ digit", "word café.", "Ⓐ circled",
	"tab\there", "vt\x0bhere", "nbsp here", "caf\xe9 latin1", "x\xffy", "ab}", "\xff\xfe \xc3", "\xe2\x82 ab \xe2\x82\xac",
	"a_b \"q\" x/y @!,;:=`'", "Straße STRASSE", "KELVIN K k", "under_score",
	"αβγ greek", "日本語 text", "a‍b zwj", "x́y mark", "Ⅰ roman",
	"été", "_x_", "<tag>", "a-b", "a&b", "x　y", "end ", "12 34", "½ half",
	"� repl", "foo.bar", "A1 b2", "Café", "İIıi",
}

// rgLines is the corpus lines a pattern matches ("none"), or "refused".
func rgLines(pattern string) string {
	r, err := compileRg(pattern)
	if err != nil {
		return "refused"
	}
	var nums []string
	for i, l := range rgCorpus {
		if r.Match([]byte(l)) {
			nums = append(nums, strconv.Itoa(i+1))
		}
	}
	if nums == nil {
		return "none"
	}
	return strings.Join(nums, ",")
}

type rgEntry struct{ pattern, rg, rca string }

func rgTable(t *testing.T) []rgEntry {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "rg_syntax.tsv"))
	require.NoError(t, err)
	var out []rgEntry
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		require.Len(t, f, 3, line)
		p, err := strconv.Unquote(f[0])
		require.NoError(t, err, line)
		out = append(out, rgEntry{p, f[1], f[2]})
	}
	require.Greater(t, len(out), 140)
	return out
}

// TestLedger_LLM37: rg's syntax table, each pattern accepted with the
// lines rg matched, or refused; never read another way. A pattern rg
// refused is refused.
func TestLedger_LLM37(t *testing.T) {
	refusedRg, refusedHere := 0, 0
	for _, e := range rgTable(t) {
		got := rgLines(e.pattern)
		switch {
		case e.rca == "refused":
			require.Equal(t, "refused", got, "%q", e.pattern)
			if e.rg == "error" {
				refusedRg++
			} else {
				refusedHere++
			}
		default:
			require.NotEqual(t, "error", e.rg, "%q: rg refused it", e.pattern)
			require.Equal(t, e.rg, got, "%q", e.pattern)
		}
	}
	require.Positive(t, refusedRg)
	require.Positive(t, refusedHere)

	// Through llm_grep and llm_find --contains: invalid_regex, RETRY.
	m := New()
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "a.txt"), "word café.\ncaf\xe9 latin1\n")
	env := baseEnv(physTemp(t), map[string]string{"PATH": ""})
	for _, p := range []string{`\<word`, `foo{bar}`, `\Qa\E`, `[a&&b]`, `a**`, `\1`, `\x0a`} {
		for _, args := range [][]string{{"llm_grep", p, "."}, {"llm_find", "*", ".", "--contains", p}} {
			r := runGo(t, m, d, env, args[0], args[1:]...)
			require.Equal(t, 1, r.RC, "%q", args)
			require.Empty(t, r.Stdout)
			require.Contains(t, r.Stderr, `"error":"invalid_regex"`)
			require.Contains(t, r.Stderr, `"action":"RETRY"`)
		}
	}
	// Unicode word boundaries, and no class matching an invalid byte.
	require.Equal(t, result{"1:word café.\n", "", 0}, runGo(t, m, d, env, "llm_grep", `caf\w\b`, "a.txt"))
	require.Equal(t, result{"1:word café.\n", "", 0}, runGo(t, m, d, env, "llm_grep", `caf.`, "a.txt"))
	require.Equal(t, result{"2:caf\xe9 latin1\n", "", 0}, runGo(t, m, d, env, "llm_grep", `caf\b`, "a.txt"))
}

// The table still matches the rg installed here (ripgrep 14), and random
// patterns built from the constructs that differ are never misread.
func TestRgSyntaxMatchesRipgrep(t *testing.T) {
	rg, err := exec.LookPath("rg")
	if err != nil {
		t.Skip("no rg")
	}
	if v, _ := exec.Command(rg, "--version").Output(); !regexp.MustCompile(`^ripgrep 14\.`).Match(v) {
		t.Skip("not ripgrep 14")
	}
	f := filepath.Join(t.TempDir(), "c.txt")
	require.NoError(t, os.WriteFile(f, []byte(strings.Join(rgCorpus, "\n")+"\n"), 0o600))
	rgSays := func(p string) string {
		cmd := exec.Command(rg, "-n", "--no-heading", "--color=never", "-e", p, "--", f)
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		_ = cmd.Run()
		if errb.Len() > 0 {
			return "error"
		}
		var nums []string
		for _, l := range strings.Split(strings.TrimRight(out.String(), "\n"), "\n") {
			if n, _, ok := strings.Cut(l, ":"); ok {
				nums = append(nums, n)
			}
		}
		if nums == nil {
			return "none"
		}
		return strings.Join(nums, ",")
	}
	for _, e := range rgTable(t) {
		require.Equal(t, e.rg, rgSays(e.pattern), "%q", e.pattern)
	}
	toks := []string{"a", "é", `\w`, `\b`, ".", "[", "]", "^", "-", "{2}", "{", "}", "(", ")", "|", "*", "+", "?",
		`\d`, `\s`, `\W`, `\B`, "x", "(?i)", `\pL`, " ", ",", "[[:alpha:]]", `\S`, `\D`, "$", "e", "́", "_", `\.`,
		"k", "ß", `\x{e9}`, "[^", "1", "{1,3}", "(?:", `é`, "(?-i)", `\b*`, "$+", "^*", "(?s)", "(?U)",
		"(?i:", "K", "[a-z]", `[^\w]`, "{,2}", "{ 2 }", "{02}", `\x0a`, "(?m)", `\B`}
	seed := uint64(37)
	rnd := func(n int) int {
		seed = seed*6364136223846793005 + 1442695040888963407
		return int((seed >> 33) % uint64(n))
	}
	for range 1500 {
		var b strings.Builder
		for j := 1 + rnd(6); j > 0; j-- {
			b.WriteString(toks[rnd(len(toks))])
		}
		p := b.String()
		if got := rgLines(p); got != "refused" {
			require.Equal(t, rgSays(p), got, "%q", p)
		}
	}
}
