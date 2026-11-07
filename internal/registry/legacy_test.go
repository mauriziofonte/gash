// Gash 4.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLegacyNames(t *testing.T) {
	names := LegacyNames()
	require.Contains(t, names, "files_largest")
	require.Contains(t, names, "gash_migrate")
	require.Contains(t, names, "gash_help")
	require.NotContains(t, names, "gash_username") // Kept
	require.NotContains(t, names, "llm_read")      // permanent
	require.NotContains(t, names, "gl")            // its own shortcut
	require.NotContains(t, names, "path_add")      // script helper
	for _, n := range names {
		require.True(t, Legacy(n), n)
		cmd, _ := LegacyReplacement(n)
		require.NotEmpty(t, cmd, n)
	}
	require.False(t, Legacy("gash_username"))
	require.False(t, Legacy("llm_read"))
}

func TestScanLegacy(t *testing.T) {
	text := "# files_largest in a comment\n" +
		"alias big='files_largest ~ --limit 5'\n" +
		"my_files_largest; files_largest.sh; ./sysinfo; $sysinfo\n" +
		"git_sync && sysinfo cpu\n" +
		"llm_read x; gash_username\n"
	require.Equal(t, []LegacyHit{
		{Line: 2, Name: "files_largest", Command: "gash files largest", Shortcut: "flf"},
		{Line: 4, Name: "git_sync", Command: "gash git sync", Shortcut: "gsync"},
		{Line: 4, Name: "sysinfo", Command: "gash system info", Shortcut: "si"},
	}, ScanLegacy(text))
}
