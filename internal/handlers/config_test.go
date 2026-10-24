// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const repoRoot = "../.."

// run2xConfig runs script in a Bash with the 2.x lib/core files sourced
// as gash.sh orders them, the settings already loaded (their warnings
// are the stub's, LEDGER CORE-18) and __gash_use_color answering color:
// stdout and stderr.
func run2xConfig(t *testing.T, env []string, color bool, script string) (string, string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repoRoot, "lib/modules/gash.sh")); err != nil {
		t.Skip("the 2.x sources are not in this tree")
	}
	root, err := filepath.Abs(repoRoot)
	require.NoError(t, err)
	gate := "__gash_use_color() { return 1; }; "
	if color {
		gate = "__gash_use_color() { return 0; }; "
	}
	cmd := exec.Command("bash", "--noprofile", "--norc", "-c",
		`for f in output help utils validation config; do source "$1/lib/core/$f.sh"; done; `+
			`__gash_load_env 2>/dev/null; `+gate+script, "_", root)
	cmd.Env = append([]string{"PATH=/usr/bin:/bin", "TERM=dumb"}, env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	require.NoError(t, cmd.Run(), errb.String())
	return out.String(), errb.String()
}

var configFixtures = map[string]string{
	"none":  "",
	"empty": "# nothing\n",
	"many": "DB:default=mysql://u:p@ss@h/d\nDB:default=pgsql://:SECRET@h2:5432/e\nDB:a-very-long-connection-name=mariadb://root:x@db\n" +
		"DB:ünï=mysql://ü:pä@h/d\nDB:nopw=mysql://u@h/d\nDB:tab=mysql://u:p@h/d\tx\n" +
		"AI:claude=sk-ant-1234567890\nAI:gemini=short\nAI:x=é€abcdefghij\nAI:averyveryverylongname=k\n",
}

// The listings print what 2.x prints, in every color mode and locale.
func TestConfigListsMatch2x(t *testing.T) {
	for name, content := range configFixtures {
		home := t.TempDir()
		if name != "none" {
			require.NoError(t, os.WriteFile(filepath.Join(home, ".gash_env"), []byte(content), 0o600))
		}
		for _, fn := range []string{"gash_db_list", "gash_ai_list"} {
			for _, lang := range []string{"C.UTF-8", "C"} {
				for _, color := range []bool{false, true} {
					env := []string{"HOME=" + home, "LANG=" + lang}
					wantOut, wantErr := run2xConfig(t, env, color, fn)
					out, errOut, rc := run(t, fn, map[string]string{"HOME": home, "LANG": lang}, color)
					require.Equal(t, 0, rc)
					require.Equal(t, wantOut, out, "%s %s %s %v", name, fn, lang, color)
					require.Equal(t, wantErr, errOut, "%s %s %s %v", name, fn, lang, color)
				}
			}
		}
	}
}

func TestConfigListsHelp(t *testing.T) {
	out, _, rc := run(t, "gash_db_list", map[string]string{"GASH__HELP": "0"}, false, "--help")
	require.Equal(t, 0, rc)
	require.Equal(t, "gash_db_list\nUsage: gash_db_list\nList available database connections from ~/.gash_env\n", out)
	out, _, rc = run(t, "gash_ai_list", map[string]string{"GASH__HELP": "0"}, false, "-h")
	require.Equal(t, 0, rc)
	require.Equal(t, "gash_ai_list\nUsage: gash_ai_list\nList available AI providers from ~/.gash_env\n", out)
}

// listOf runs a listing on a settings file with content.
func listOf(t *testing.T, fn, content string, env map[string]string) (string, string) {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, ".gash_env")
	if env["GASH_ENV_FILE"] != "" {
		path = env["GASH_ENV_FILE"]
	}
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	e := map[string]string{"HOME": home}
	for k, v := range env {
		e[k] = v
	}
	out, errOut, rc := run(t, fn, e, false)
	require.Equal(t, 0, rc)
	return out, errOut
}

// LEDGER CORE-03: an empty user name no longer leaves the password visible
// (3.0: CORE-03 fixed).
func TestLedger_CORE03(t *testing.T) {
	out, _ := listOf(t, "gash_db_list", "DB:default=mysql://:SECRETPW@h/d\nDB:u=mysql://u:p@h/d\n", nil)
	require.Equal(t, "\n  default         mysql://:****@h/d\n  u               mysql://u:****@h/d\n", out)
}

// LEDGER CORE-04 (reproduced in rc.A): every duplicate name is listed.
func TestLedger_CORE04_2x(t *testing.T) {
	out, _ := listOf(t, "gash_db_list", "DB:default=mysql://a:p1@h1/d\nDB:default=pgsql://b:p2@h2/e\n", nil)
	require.Equal(t, "\n  default         mysql://a:****@h1/d\n  default         pgsql://b:****@h2/e\n", out)
}

// LEDGER CORE-06 (reproduced in rc.A): the messages name ~/.gash_env
// when GASH_ENV_FILE names another file.
func TestLedger_CORE06_2x(t *testing.T) {
	other := filepath.Join(t.TempDir(), "other")
	env := map[string]string{"GASH_ENV_FILE": other}
	_, errOut := listOf(t, "gash_db_list", "", env)
	require.Equal(t, "Info: No database connections configured in ~/.gash_env\nInfo: Run 'gash_env_init' to create a template configuration\n", errOut)
	_, errOut = listOf(t, "gash_ai_list", "", env)
	require.Equal(t, "Info: No AI providers configured in ~/.gash_env\nInfo: Add: AI:claude=YOUR_API_KEY or AI:gemini=YOUR_API_KEY\n", errOut)
	// The file itself is the one read.
	out, _ := listOf(t, "gash_ai_list", "AI:claude=sk-123456789\n", env)
	require.True(t, strings.Contains(out, "claude     sk-12345..."), out)
}
