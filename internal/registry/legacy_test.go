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

func TestCanonicalCall(t *testing.T) {
	for in, want := range map[string]string{
		"process_find":                   "gash system ps",
		"archive_extract (tar archives)": "gash files extract (tar archives)",
		"hgrep (--pick)":                 "gash shell history-smart (--pick)",
		"gash_migrate":                   "gash migrate",
		"llm_procs --port":               "llm_procs --port",
		"gash_username":                  "gash_username",
		"gash files largest":             "gash files largest",
		"":                               "",
	} {
		require.Equal(t, want, CanonicalCall(in), in)
	}
}

// A user's own command under a former name is a definition, not a call;
// what the definition runs is still read. A word of a gash command is
// not a call either.
func TestScanLegacyDefinitions(t *testing.T) {
	text := "alias hgrep='gash shell history-smart'\n" +
		"alias hgrep=\"gash shell history-smart\" mkdir_cd='mkdir -p'\n" +
		"hgrep() { gash shell history-smart \"$@\"; }\n" +
		"sysinfo () { inxi -F; }\n" +
		"function git_sync { git pull; }\n" +
		"function  git_repair() { :; }\n" +
		"files_largest=1\n" +
		"alias big='files_largest ~'\n" +
		"myfunction git_sync\n" +
		"hgrep == x\n" +
		"alias si2='gash ai sysinfo'; gash llm sysinfo && gash help hgrep\n" +
		"x=$(gash system info); sysinfo | less\n" +
		"echo first, gash ai sysinfo asks\n"
	require.Equal(t, []LegacyHit{
		{Line: 8, Name: "files_largest", Command: "gash files largest", Shortcut: "flf"},
		{Line: 9, Name: "git_sync", Command: "gash git sync", Shortcut: "gsync"},
		{Line: 10, Name: "hgrep", Command: "gash shell history-smart"},
		{Line: 12, Name: "sysinfo", Command: "gash system info", Shortcut: "si"},
	}, ScanLegacy(text))
}
