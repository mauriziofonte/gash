// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"regexp"
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
	"llm_git_status": {},
	"llm_git_diff":   {"--staged", "--cached"},
	"llm_git_log":    {"-n", "--limit"},
	"llm_env":        {"--filter", "-f"},
	"llm_ports":      {"--listen", "-l"},
	"llm_procs":      {"--name", "--port"},
	"llm_project":    {},
	"llm_deps":       {"--dev"},
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
