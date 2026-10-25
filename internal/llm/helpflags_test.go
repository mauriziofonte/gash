// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/registry"
)

// llmOptions is every option spelling the llm_* parsers of this package
// accept (LEDGER LLM-31). Each is checked twice: the parser takes it (no
// unknown_option) and the function's registered help names it, so a flag
// cannot be undocumented.
var llmOptions = map[string][]string{
	"llm_tree":       {"--text", "--depth", "--max-depth", "--stats"},
	"llm_find":       {"--type", "--contains", "-n", "--limit"},
	"llm_grep":       {"--ext", "--context", "-C", "-n", "--limit"},
	"llm_config":     {"--max-bytes"},
	"llm_git_status": {"--summary", "--files", "--fetch", "--repos", "--timeout", "-j", "--format", "--max-lines", "--head", "--tail", "--width"},
	"llm_git_diff":   {"--staged", "--cached", "--files", "--numstat", "--summary", "--patch", "--format", "--max-lines", "--head", "--tail", "--width"},
	"llm_git_log":    {"-n", "--limit", "--since", "--until", "--grep", "--author", "--path", "--fields", "--format", "--max-lines", "--head", "--tail", "--width"},
	"llm_env":        {"--filter", "-f"},
	"llm_ports":      {"--listen", "-l", "--port"},
	"llm_procs":      {"--name", "--port"},
	"llm_project":    {},
	"llm_deps":       {"--dev"},

	// 3.4 text reads.

	// 3.4 data.

	// 3.4 git.
	"llm_git_overview": {"--log", "--files"},
	"llm_git_show":     {"--stat", "--files", "--patch", "--max-lines", "--head", "--tail", "--width"},

	// 3.4 network.
	"llm_http": {"--method", "--header", "--auth-env", "--resolve", "--follow", "--status-only", "--headers",
		"--json-path", "--grep", "--body-max", "--expect", "-k", "--insecure", "-j", "--jobs", "--timeout", "--format", "--width", "--max-lines", "--head", "--tail"},
	"llm_dns": {"--type", "--server", "-j", "--jobs", "--timeout", "--format", "--width", "--max-lines", "--head", "--tail"},
	"llm_tls": {"--sni", "-j", "--jobs", "--timeout", "--format", "--width", "--max-lines", "--head", "--tail"},
	"llm_tcp": {"-j", "--jobs", "--timeout", "--format", "--width", "--max-lines", "--head", "--tail"},

	// 3.4 processes, waiting and services.

	// 3.4 runtimes, lint and tests.

	// 3.4 remote.
}

// TestHelpListsEveryOption: every option above is accepted by its parser
// and appears in the function's help, and an option that is not in the
// list is unknown_option.
func TestHelpListsEveryOption(t *testing.T) {
	m := New()
	d := physTemp(t)
	env := baseEnv(physTemp(t), nil)
	for fn, opts := range llmOptions {
		f, ok := registry.Lookup(fn)
		require.True(t, ok, fn)
		for _, o := range opts {
			require.Regexp(t, `(^|[\s\[|,(])`+regexp.QuoteMeta(o)+`([\s\]|,=)]|$)`, f.Help.Body, "%s help lacks %s", fn, o)
			// A value after a boolean flag may itself be refused; the flag
			// must not be.
			r := runGo(t, m, d, env, fn, o, "1")
			require.NotContains(t, r.Stderr, `"error":"unknown_option","details":"`+o+`"`, "%s %s", fn, o)
		}
		r := runGo(t, m, d, env, fn, "--no-such-option")
		require.Contains(t, r.Stderr, `"error":"unknown_option","details":"--no-such-option"`, fn)
	}
}

// agentsSection3 is AGENTS.md section 3 (the function catalog).
func agentsSection3(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "AGENTS.md"))
	require.NoError(t, err)
	s := string(data)
	i := strings.Index(s, "\n## 3. ")
	require.GreaterOrEqual(t, i, 0)
	s = s[i+1:]
	if j := strings.Index(s, "\n## 4. "); j >= 0 {
		s = s[:j]
	}
	return s
}

var flagWord = regexp.MustCompile(`(?:^|[\s\[|(])(--?[A-Za-z][A-Za-z-]*)`)

// usageFlags are the option words of a signature or a help USAGE block.
func usageFlags(s string) map[string]bool {
	out := map[string]bool{}
	for _, m := range flagWord.FindAllStringSubmatch(s, -1) {
		out[m[1]] = true
	}
	return out
}

// helpUsage is the USAGE block of a help body.
func helpUsage(body string) string {
	i := strings.Index(body, "USAGE\n")
	if i < 0 {
		return ""
	}
	u := body[i+len("USAGE\n"):]
	if j := strings.Index(u, "\n\n"); j >= 0 {
		u = u[:j]
	}
	return u
}

// TestLedger_LLM30: AGENTS.md section 3 gives each llm_* function the
// options of its registered USAGE line, none it does not accept, and the
// grammar rules it states hold: no --flag=value, no --, a second PATH is
// unexpected_argument, a stray word for llm_ports/llm_procs is
// unknown_option, llm_env reads a bare word as its filter, -h as a flag's
// value prints the help.
func TestLedger_LLM30(t *testing.T) {
	sec := agentsSection3(t)
	row := regexp.MustCompile("(?m)^\\| `(llm_[a-z_]+)([^`]*)` \\|")
	seen := map[string]bool{}
	for _, m := range row.FindAllStringSubmatch(sec, -1) {
		fn, sig := m[1], strings.ReplaceAll(m[2], `\|`, "|")
		opts, ok := llmOptions[fn]
		if !ok {
			continue
		}
		seen[fn] = true
		accepted := map[string]bool{}
		for _, o := range opts {
			accepted[o] = true
		}
		f, _ := registry.Lookup(fn)
		got := usageFlags(sig)
		for o := range got {
			require.True(t, accepted[o], "AGENTS.md section 3: %s lists %s, which its parser refuses", fn, o)
		}
		for o := range usageFlags(helpUsage(f.Help.Body)) {
			require.True(t, got[o], "AGENTS.md section 3: %s lacks %s from its help USAGE", fn, o)
		}
	}
	for fn := range llmOptions {
		require.True(t, seen[fn], "AGENTS.md section 3 has no row for %s", fn)
	}

	m := New()
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "f"), "x\n")
	env := baseEnv(physTemp(t), nil)
	code := func(r result) string {
		var e struct{ Error string }
		_ = json.Unmarshal([]byte(r.Stderr), &e)
		return e.Error
	}
	for _, args := range [][]string{
		{"llm_tree", "--depth=2"}, {"llm_tree", "--"}, {"llm_grep", "x", "--context=1"},
		{"llm_grep", "--", "x"}, {"llm_find", "*", "--limit=1"}, {"llm_git_log", "--limit=1"},
		{"llm_env", "--filter=PATH"}, {"llm_ports", "x"}, {"llm_procs", "x"},
	} {
		require.Equal(t, "unknown_option", code(runGo(t, m, d, env, args[0], args[1:]...)), args)
	}
	for _, args := range [][]string{
		{"llm_tree", ".", "."}, {"llm_find", "*", ".", "."}, {"llm_grep", "x", ".", "."},
		{"llm_git_status", ".", "."}, {"llm_git_log", ".", "."}, {"llm_git_diff", ".", "."},
		{"llm_project", ".", "."}, {"llm_deps", ".", "."}, {"llm_config", "f", "f"},
	} {
		require.Equal(t, "unexpected_argument", code(runGo(t, m, d, env, args[0], args[1:]...)), args)
	}
	env["ZZ_LLM30"] = "1"
	r := runGo(t, m, d, env, "llm_env", "NOPE", "^ZZ_LLM30$")
	require.Equal(t, result{"{\n  \"ZZ_LLM30\": \"1\"\n}\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_grep", "x", "--ext", "-h")
	require.Equal(t, 0, r.RC)
	require.Contains(t, r.Stdout, "llm_grep PATTERN")
}
