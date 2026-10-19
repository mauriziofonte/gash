// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadCaller(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gash_env"), []byte("DB:default=mysql://u:p@h/d\n"), 0o644))
	other := filepath.Join(home, "other")
	require.NoError(t, os.WriteFile(other, []byte("AI:claude=sk\n"), 0o600))
	vars := map[string]string{"HOME": home}
	get := func(n string) string { return vars[n] }

	f := LoadCaller(get)
	require.Equal(t, "~/.gash_env", f.Shown)
	require.Len(t, f.DB, 1)
	require.Len(t, f.Warnings, 1, "mode 644 is warned outside headless mode")

	vars["GASH_HEADLESS"] = "1"
	require.Empty(t, LoadCaller(get).Warnings)
	vars["GASH_DEBUG"] = "1"
	require.Len(t, LoadCaller(get).Warnings, 1)

	vars["GASH_ENV_FILE"] = other
	f = LoadCaller(get)
	require.Equal(t, "~/other", f.Shown)
	require.Empty(t, f.DB)
	require.Len(t, f.AI, 1)
}
