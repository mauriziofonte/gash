// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/registry"
)

// dbOptions is every option spelling the llm_db_* parser accepts per
// function (LEDGER LLM-31): the parser takes each, the registered help
// names each.
var dbOptions = map[string][]string{
	"llm_db_query":     {"-c", "--connection", "-d", "--database", "-f", "--file", "-r", "--rows", "-n", "--limit", "--format", "--scalar"},
	"llm_db_tables":    {"-c", "--connection", "-d", "--database", "-f", "--file", "--counts", "--format", "--scalar"},
	"llm_db_schema":    {"-c", "--connection", "-d", "--database", "-f", "--file", "--all", "--format", "--scalar"},
	"llm_db_sample":    {"-c", "--connection", "-d", "--database", "-f", "--file", "-n", "--limit", "--format", "--scalar"},
	"llm_db_explain":   {"-c", "--connection", "-d", "--database", "-f", "--file", "--analyze", "--format", "--scalar"},
	"llm_db_databases": {"-c", "--connection", "-d", "--database", "-f", "--file", "--format", "--scalar"},
}

func TestHelpListsEveryOption(t *testing.T) {
	d := physTemp(t)
	m := frozen(New())
	env := baseEnv(d, nil)
	for fn, opts := range dbOptions {
		f, ok := registry.Lookup(fn)
		require.True(t, ok, fn)
		for _, o := range opts {
			require.Regexp(t, `(^|[\s\[|,(])`+regexp.QuoteMeta(o)+`([\s\]|,=)]|$)`, f.Help.Body, "%s help lacks %s", fn, o)
			_, listed := registry.OptionOf(fn, o)
			require.True(t, listed, "%s: no OPTIONS line spells %s (3.4.2: --flag=value reads them)", fn, o)
			r := runGo(t, m, d, env, fn, "x", o, "1")
			require.NotContains(t, r.Stderr, `"error":"unknown_option","details":"`+o+`"`, "%s %s", fn, o)
		}
		r := runGo(t, m, d, env, fn, "x", "--no-such-option")
		require.Contains(t, r.Stderr, `"error":"unknown_option","details":"--no-such-option"`, fn)
	}
}

var flagWord = regexp.MustCompile(`(?:^|[\s\[|(])(--?[A-Za-z][A-Za-z-]*)`)

func usageFlags(s string) map[string]bool {
	out := map[string]bool{}
	for _, m := range flagWord.FindAllStringSubmatch(s, -1) {
		out[m[1]] = true
	}
	return out
}

// TestLedger_LLM30_db: the llm_db_* signatures of AGENTS.md section 3.3
// list the options of the registered USAGE lines and none the parser
// refuses.
func TestLedger_LLM30_db(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "AGENTS.md"))
	require.NoError(t, err)
	s := string(data)
	s = s[strings.Index(s, "\n### 3.3 "):]
	s = s[:strings.Index(s, "\n### 3.4 ")]
	line := regexp.MustCompile(`(?m)^(llm_db_[a-z]+) (.*)$`)
	seen := map[string]bool{}
	for _, m := range line.FindAllStringSubmatch(s, -1) {
		fn, sig := m[1], m[2]
		if i := strings.Index(sig, "#"); i >= 0 {
			sig = sig[:i]
		}
		seen[fn] = true
		accepted := map[string]bool{}
		for _, o := range dbOptions[fn] {
			accepted[o] = true
		}
		got := usageFlags(sig)
		for o := range got {
			require.True(t, accepted[o], "AGENTS.md 3.3: %s lists %s, which its parser refuses", fn, o)
		}
		f, _ := registry.Lookup(fn)
		u := f.Help.Body[strings.Index(f.Help.Body, "USAGE\n")+6:]
		u = u[:strings.Index(u, "\n\n")]
		for o := range usageFlags(u) {
			require.True(t, got[o], "AGENTS.md 3.3: %s lacks %s from its help USAGE", fn, o)
		}
	}
	for fn := range dbOptions {
		require.True(t, seen[fn], "AGENTS.md 3.3 has no line for %s", fn)
	}
}
