// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// matrixFunctions reads the function rows of go-impl/PORT/MATRIX.md.
func matrixFunctions(t *testing.T) map[string][]string {
	t.Helper()
	b, err := os.ReadFile("../../PORT/MATRIX.md")
	require.NoError(t, err)
	rows := map[string][]string{}
	for _, l := range strings.Split(string(b), "\n") {
		cells := strings.Split(l, " | ")
		if len(cells) < 7 || cells[1] != "function" {
			continue
		}
		name := strings.TrimPrefix(cells[0], "| ")
		if i := strings.Index(name, " ("); i > 0 {
			name = name[:i]
		}
		rows[name] = cells
	}
	return rows
}

// Every registry function has a MATRIX row and the reverse, with the same
// class and decision.
func TestRegistryMatchesMatrix(t *testing.T) {
	rows := matrixFunctions(t)
	require.Len(t, Functions(), len(rows))
	for _, f := range Functions() {
		row, ok := rows[f.Name]
		require.True(t, ok, "no MATRIX row for %s", f.Name)
		require.Equal(t, string(f.Class), row[2], f.Name)
		require.Equal(t, f.Decision, row[3], f.Name)
	}
	for name := range rows {
		_, ok := Lookup(name)
		require.True(t, ok, "MATRIX row %s is not in the registry", name)
	}
}

func TestFunctionsSortedAndUnique(t *testing.T) {
	fns := Functions()
	require.True(t, sort.SliceIsSorted(fns, func(i, j int) bool { return fns[i].Name < fns[j].Name }))
	for i := 1; i < len(fns); i++ {
		require.NotEqual(t, fns[i-1].Name, fns[i].Name)
	}
}

func TestLookupAndAliases(t *testing.T) {
	f, ok := Lookup("files_largest")
	require.True(t, ok)
	require.Equal(t, "files", f.Module)
	require.Equal(t, "files", f.Topic)
	require.Contains(t, f.Aliases, "flf")
	require.Equal(t, "lib/modules/files.sh", f.StubFile)

	g, ok := ByAlias("flf")
	require.True(t, ok)
	require.Equal(t, "files_largest", g.Name)

	_, ok = Lookup("no_such_function")
	require.False(t, ok)
}

func TestHelpBodiesAsRegistered(t *testing.T) {
	f, ok := Lookup("gash_username")
	require.True(t, ok)
	require.True(t, f.HasHelp)
	require.Equal(t, "Print the current user name", f.Help.Short)
	require.True(t, strings.HasPrefix(f.Help.Body, "USAGE\n  gash_username\n"))
	require.False(t, strings.HasSuffix(f.Help.Body, "\n"))

	all, ok := Lookup("all_colors")
	require.True(t, ok)
	require.Equal(t, &Usage{Program: "all_colors", Usage: "all_colors", Description: "Displays all available terminal colors (256 colors)."}, all.Fallback)
	require.Equal(t, "lib/core/utils.sh", all.StubFile)
}

func TestPartFilesMapToTheirLoader(t *testing.T) {
	for _, f := range Functions() {
		if strings.HasPrefix(f.StubFile, "lib/modules/") {
			require.NotContains(t, strings.TrimPrefix(f.StubFile, "lib/modules/"), "/", f.Name)
		}
	}
	f, _ := Lookup("llm_tree")
	require.Equal(t, "lib/modules/llm/fs.sh", f.File)
	require.Equal(t, "lib/modules/llm.sh", f.StubFile)
}

func TestStubVars(t *testing.T) {
	f, _ := Lookup("chrome_headless_update")
	vars := f.StubVars()
	require.Equal(t, CommonShellVars, vars[:len(CommonShellVars)])
	require.Contains(t, vars, "GASH_CHROME_ROOT")
	require.Contains(t, vars, "GASH_CHROME_LINK")
	for _, v := range vars {
		require.NotContains(t, []string{"GASH_LLM_EXEC", "GASH_LLM_EXEC_ALLOW", "GASH_LLM_SANDBOX"}, v)
	}
	db, _ := Lookup("llm_db_query")
	require.Equal(t, []string{"GASH_DB_TIMEOUT"}, db.Settings)
}

func TestInteractiveOnlyFunctions(t *testing.T) {
	var got []string
	for _, f := range Functions() {
		if !f.Headless {
			got = append(got, f.Name)
		}
	}
	require.Equal(t, []string{"gl", "gla", "glf", "glg", "glo", "gls", "help", "nvm"}, got)
}

func TestSettingsAndTopics(t *testing.T) {
	var names, headless []string
	for _, s := range Settings() {
		names = append(names, s.Name)
		if s.Headless {
			headless = append(headless, s.Name)
		}
		if s.FileOnly {
			require.False(t, s.Passed, s.Name)
		}
	}
	require.Len(t, names, 18)
	require.ElementsMatch(t, []string{"GASH_DB_TIMEOUT", "GASH_AI_CONFIRM", "GASH_AI_MODEL_CLAUDE", "GASH_AI_MODEL_GEMINI"}, headless)
	var topics []string
	for _, tp := range Topics() {
		topics = append(topics, tp.Name)
	}
	require.Equal(t, []string{"files", "system", "nav", "git", "docker", "remote", "dev", "llm", "ai", "gash"}, topics)
	require.Len(t, Packs(), 7)
}

func TestDelegated(t *testing.T) {
	var not []string
	for _, f := range Functions() {
		if !f.Delegated() {
			not = append(not, f.Name)
		}
	}
	require.Equal(t, []string{"gash", "help", "needs_confirm_prompt", "needs_help", "nvm", "path_add"}, not)
}

// help_spec.sh "Help System - Registry" against the static registry: an
// entry stores the help text, the short description, the aliases, the
// module and the see-also list, and the full load has more than 40.
func TestHelpRegistryAsHelpSpec(t *testing.T) {
	f, ok := Lookup("files_largest")
	require.True(t, ok)
	require.True(t, f.HasHelp)
	require.NotEmpty(t, f.Help.Body)
	require.NotEmpty(t, f.Help.Short)
	require.Contains(t, f.Help.Aliases, "flf")
	require.Equal(t, "files", f.Help.Module)
	require.NotEmpty(t, f.Help.SeeAlso)
	n := 0
	for _, f := range Functions() {
		if f.HasHelp {
			n++
		}
	}
	require.Greater(t, n, 40)
}

// LEDGER CORE-07: the help modules come in one fixed order, and every
// module with registered help is in it.
func TestLedger_CORE07(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range Functions() {
		if f.HasHelp && f.Help.Module != "" {
			seen[f.Help.Module] = true
			require.Contains(t, ModuleOrder, f.Help.Module, f.Name)
		}
	}
	for _, m := range ModuleOrder {
		require.True(t, seen[m], "ModuleOrder lists %s, which has no help", m)
	}
	require.Equal(t, []string{"files", "system", "git", "docker", "docker-compose", "ai", "sysinfo", "gash", "ssh",
		"llm", "utils", "node", "remote", "chrome", "php"}, ModuleOrder)
}
