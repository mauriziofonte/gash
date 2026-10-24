// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
)

// A command run for a function sees the caller's exported environment
// only, never a shell variable the stub exported for the call; one that
// does not start outputs nothing.
func TestOutputChildEnv(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "show"),
		[]byte("#!/bin/sh\necho \"${KEPT-}|${TOKEN-unset}|${GASH__SET-unset}\"\necho\n"), 0o755))
	c := &invoke.Call{Ctx: context.Background(), Env: invoke.EnvFrom([]string{
		"PATH=" + dir, "KEPT=1", "TOKEN=s3cret", "GASH__ADDED=TOKEN", "GASH__SET=",
	})}
	m := &Module{run: runner.Exec{}}
	require.Equal(t, "1|unset|unset", m.output(c, "show"))
	require.Equal(t, "", m.output(c, "nonexistent-program"))
	p, ok := m.lookPath(c, "show")
	require.True(t, ok)
	require.Equal(t, dir+"/show", p)
	_, ok = m.lookPath(c, "sh")
	require.False(t, ok)
}
