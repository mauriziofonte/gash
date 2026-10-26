// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/term"
)

// TestValueFlagsAreValueFlags: --flag=value splits a flag the help lists
// with a value (3.4.2), so the help must be right about it. Each such
// flag, alone on the line, is missing_value from the function's own
// parser: the parser does take a value there.
func TestValueFlagsAreValueFlags(t *testing.T) {
	d, home := t.TempDir(), t.TempDir()
	t.Chdir(d)
	tbl := Table()
	env := invoke.EnvMap(map[string]string{"HOME": home, "PATH": "/usr/bin:/bin", "GASH_HEADLESS": "1"})
	checked := 0
	for _, f := range registry.Functions() {
		if !registry.EqualsForm(f.Name) || !f.HasHelp {
			continue
		}
		if _, ok := tbl.Lookup(f.Name); !ok {
			continue
		}
		for _, o := range registry.Options(f.Name) {
			if o.Values == 0 {
				continue
			}
			for _, name := range o.Names {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				var out, errb bytes.Buffer
				c := &invoke.Call{Ctx: ctx, Name: f.Name, Args: []string{name}, Env: env, Stdin: strings.NewReader(""),
					Stdout: &out, Stderr: &errb, Term: term.Fixed(env, false, false, false)}
				rc := tbl.Run(c)
				cancel()
				require.NotEqual(t, 0, rc, "%s %s", f.Name, name)
				require.Regexp(t, `missing_value|[Mm]issing value|needs a value|requires a value`, errb.String(), "%s %s: %s", f.Name, name, errb.String())
				checked++
			}
		}
	}
	t.Logf("%d value flags checked", checked)
	require.Greater(t, checked, 300)
}
