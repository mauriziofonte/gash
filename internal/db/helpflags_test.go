// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/registry"
)

// dbOptions is every option spelling the llm_db_* parser accepts per
// function (LEDGER LLM-31): the parser takes each, the registered help
// names each.
var dbOptions = map[string][]string{
	"llm_db_query":   {"-c", "--connection", "-d", "--database", "-f", "--file", "-r", "--rows", "-n", "--limit"},
	"llm_db_tables":  {"-c", "--connection", "-d", "--database", "-f", "--file"},
	"llm_db_schema":  {"-c", "--connection", "-d", "--database", "-f", "--file"},
	"llm_db_sample":  {"-c", "--connection", "-d", "--database", "-f", "--file", "-n", "--limit"},
	"llm_db_explain": {"-c", "--connection", "-d", "--database", "-f", "--file", "--analyze"},
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
			r := runGo(t, m, d, env, fn, "x", o, "1")
			require.NotContains(t, r.Stderr, `"error":"unknown_option","details":"`+o+`"`, "%s %s", fn, o)
		}
		r := runGo(t, m, d, env, fn, "x", "--no-such-option")
		require.Contains(t, r.Stderr, `"error":"unknown_option","details":"--no-such-option"`, fn)
	}
}
