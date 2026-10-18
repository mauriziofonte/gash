// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/handlers"
	"github.com/mauriziofonte/gash/internal/invoke"
)

// files returns stdout and stderr files and a reader of both.
func files(t *testing.T) (*os.File, *os.File, func() (string, string)) {
	t.Helper()
	dir := t.TempDir()
	o, err := os.Create(filepath.Join(dir, "out"))
	require.NoError(t, err)
	e, err := os.Create(filepath.Join(dir, "err"))
	require.NoError(t, err)
	return o, e, func() (string, string) {
		bo, _ := os.ReadFile(o.Name())
		be, _ := os.ReadFile(e.Name())
		return string(bo), string(be)
	}
}

func TestMainRunsTheCommandLine(t *testing.T) {
	o, e, read := files(t)
	rc := Main([]string{"gash_username"}, []string{"PATH=/usr/bin:/bin"}, nil, o, e)
	require.Equal(t, 0, rc)
	out, errOut := read()
	require.NotEmpty(t, strings.TrimSpace(out))
	require.Empty(t, errOut)
}

func TestPanicBecomesAnEnvelope(t *testing.T) {
	saved := newTable
	t.Cleanup(func() { newTable = saved })
	newTable = func() *invoke.Table {
		return handlers.Table(func(tb *invoke.Table) {
			tb.HandleCompat("boom", func(*invoke.Call) int { panic("kaboom") })
		})
	}
	for _, debug := range []bool{false, true} {
		o, e, read := files(t)
		env := []string{}
		if debug {
			env = append(env, "GASH_DEBUG=1")
		}
		rc := Main([]string{"__compat", "boom"}, env, nil, o, e)
		require.Equal(t, 1, rc)
		_, errOut := read()
		first, rest, _ := strings.Cut(errOut, "\n")
		require.Equal(t, `{"error":"internal_error","details":"kaboom","action":"STOP","recoverable":true}`, first)
		if debug {
			require.Contains(t, rest, "goroutine")
		} else {
			require.Empty(t, rest)
		}
	}
}
