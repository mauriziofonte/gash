// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package invoke

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnv(t *testing.T) {
	e := EnvFrom([]string{"A=1", "B=", "A=2", "junk", "C=x=y"})
	v, ok := e.Lookup("A")
	require.True(t, ok)
	require.Equal(t, "1", v)
	v, ok = e.Lookup("B")
	require.True(t, ok)
	require.Equal(t, "", v)
	_, ok = e.Lookup("junk")
	require.False(t, ok)
	require.Equal(t, "x=y", e.Get("C"))
	require.Equal(t, []string{"A=1", "B=", "C=x=y"}, e.Environ())
}

// A child gets the caller's exported environment: no protocol variables,
// no shell variable the stub exported for the call only.
func TestEnvChild(t *testing.T) {
	e := EnvFrom([]string{"PATH=/bin", "TOKEN=s3cret", "COLUMNS=80", "GASH__SET=", "GASH__HELP=1", "GASH__ADDED=TOKEN NO_COLOR", "NO_COLOR="})
	require.Equal(t, []string{"COLUMNS=80", "PATH=/bin"}, e.Child())
	require.Equal(t, []string{"A=1"}, EnvFrom([]string{"A=1"}).Child())
}

func TestTableHandleAndRun(t *testing.T) {
	tbl := NewTable()
	tbl.Handle("gash_username", func(c *Call) int {
		_, _ = c.Stdout.Write([]byte(c.Arg(0) + c.Arg(5)))
		require.Same(t, tbl, c.Handlers)
		return 3
	})
	require.Panics(t, func() { tbl.Handle("gash_username", func(*Call) int { return 0 }) })
	require.Panics(t, func() { tbl.Handle("no_such_function", func(*Call) int { return 0 }) })
	require.Equal(t, []string{"gash_username"}, tbl.Names())

	var out, errb bytes.Buffer
	rc := tbl.Run(&Call{Name: "gash_username", Args: []string{"a"}, Stdout: &out, Stderr: &errb})
	require.Equal(t, 3, rc)
	require.Equal(t, "a", out.String())

	rc = tbl.Run(&Call{Name: "llm_tree", Stdout: &out, Stderr: &errb})
	require.Equal(t, 1, rc)
	require.Equal(t, `{"error":"internal_error","details":"llm_tree is not implemented in this gash build","action":"STOP","recoverable":true,"hint":"Gash 3.0 development build: this function has no Go handler yet"}`+"\n", errb.String())
}

func TestCompatLookup(t *testing.T) {
	tbl := NewTable()
	h := func(*Call) int { return 0 }
	tbl.HandleCompat("help", h)
	tbl.HandleCompat("output table", h)
	require.Panics(t, func() { tbl.HandleCompat("help", h) })
	_, op, rest, ok := tbl.LookupCompat([]string{"output", "table", "--x"})
	require.True(t, ok)
	require.Equal(t, "output table", op)
	require.Equal(t, []string{"--x"}, rest)
	_, op, rest, ok = tbl.LookupCompat([]string{"help", "llm_tree"})
	require.True(t, ok)
	require.Equal(t, "help", op)
	require.Equal(t, []string{"llm_tree"}, rest)
	_, _, _, ok = tbl.LookupCompat([]string{"output"})
	require.False(t, ok)
	require.Equal(t, []string{"help", "output table"}, tbl.CompatOps())
}

func TestHelpLoaded(t *testing.T) {
	require.True(t, (&Call{}).HelpLoaded())
	require.True(t, (&Call{Env: EnvMap(map[string]string{"GASH__HELP": "1"})}).HelpLoaded())
	require.False(t, (&Call{Env: EnvMap(map[string]string{"GASH__HELP": "0"})}).HelpLoaded())
}
