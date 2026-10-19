// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Quote files with one quote each, so that the random pick is the same
// in 2.x and here.
var quoteFiles = map[string]string{
	"short":    "# Gash quotes: header\n\n# A talk, 2001\n\"Short.\" — Someone\n",
	"long":     "# header\n# Essay, 1999, https://example.test/a/very/long/path/that/goes/on/and/on/and/on\n\"" + strings.Repeat("word ", 40) + "end.\" — Author Name\n\n# trailing comment\n",
	"nosource": "# a source\n\n\"No source here.\" — X\n",
	"nospace":  "#tight source\n\"Tight.\" — Y",
	"comments": "# only\n# comments\n\n",
	"empty":    "",
	"newline":  "\n",
}

// gash_inspiring_quote prints what 2.x prints: each file, option, width
// and color mode.
func TestQuoteMatches2x(t *testing.T) {
	dir := t.TempDir()
	var names []string
	for name, text := range quoteFiles {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, name, "quotes"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name, "quotes/list.txt"), []byte(text), 0o644))
		names = append(names, name)
	}
	names = append(names, "missing")
	argSets := [][]string{{}, {"--source"}, {"foo"}, {"--bogus"}, {"-h"}, {"--help"}, {"--source", "extra"}}
	for _, mode := range []string{"off", "on"} {
		for _, cols := range []string{"", "40", "100", "abc", "0", "010"} {
			type call struct {
				name string
				args []string
			}
			var calls []call
			var script strings.Builder
			if mode == "on" {
				script.WriteString("__gash_use_color() { return 0; }; ")
			} else {
				script.WriteString("__gash_use_color() { return 1; }; ")
			}
			if cols == "" {
				script.WriteString("unset COLUMNS; ")
			} else {
				script.WriteString("COLUMNS=" + cols + "; ")
			}
			for _, name := range names {
				for _, args := range argSets {
					calls = append(calls, call{name, args})
					script.WriteString("GASH_DIR=" + shq(filepath.Join(dir, name)) + " gash_inspiring_quote")
					for _, a := range args {
						script.WriteString(" " + shq(a))
					}
					script.WriteString(" 2>&1; printf '\\0%s\\0' $?\n")
				}
			}
			out, _, _ := run2xGash(t, []string{"LANG=C.UTF-8"}, script.String())
			fields := strings.Split(out, "\x00")
			for i, cl := range calls {
				env := map[string]string{"LANG": "C.UTF-8", "GASH_DIR": filepath.Join(dir, cl.name)}
				if cols != "" {
					env["COLUMNS"] = cols
				}
				o, e, rc := run(t, "gash_inspiring_quote", env, mode == "on", cl.args...)
				require.Equal(t, fields[2*i], o+e, "%s %q %s %q", mode, cols, cl.name, cl.args)
				require.Equal(t, fields[2*i+1], strconv.Itoa(rc), "%s %q %s %q", mode, cols, cl.name, cl.args)
			}
		}
	}
}

// LEDGER SELF-13, reproduced in rc.A: a positional word is reported as
// an unknown option, with the usage, rc 1, as an unknown option is.
func TestLedger_SELF13_2x(t *testing.T) {
	for _, a := range []string{"foo", "--bogus"} {
		o, e, rc := run(t, "gash_inspiring_quote", nil, false, a)
		require.Empty(t, o)
		require.Equal(t, "Error: Unknown option: "+a+" (usage: gash_inspiring_quote [--source])\n", e)
		require.Equal(t, 1, rc)
	}
}
