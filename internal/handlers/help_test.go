// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// run2xGash loads the 2.x gash.sh of the checkout in an interactive Bash
// (the palette set; stdout is not a terminal) with a temporary HOME, then
// runs script with GASH_VERSION=test: stdout, stderr and the status.
func run2xGash(t *testing.T, env []string, script string) (string, string, int) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repoRoot, "lib/modules/gash.sh")); err != nil {
		t.Skip("the 2.x sources are not in this tree")
	}
	root, err := filepath.Abs(repoRoot)
	require.NoError(t, err)
	return run2xIn(t, root, env, script)
}

// copy2x is a copy of the committed 2.x runtime files in a new temporary
// directory, for the self-management functions, which act on the tree
// gash.sh is sourced from (GASH_DIR): they never run on the checkout.
func copy2x(t *testing.T) string {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repoRoot, "lib/modules/gash.sh")); err != nil {
		t.Skip("the 2.x sources are not in this tree")
	}
	dir := filepath.Join(t.TempDir(), "gash")
	require.NoError(t, os.Mkdir(dir, 0o755))
	archive := exec.Command("git", "-C", repoRoot, "archive", "HEAD",
		"gash.sh", "bash_completion", "VERSION", ".gash_env.template", ".gashrc", "lib", "quotes")
	extract := exec.Command("tar", "-x", "-C", dir)
	pipe, err := archive.StdoutPipe()
	require.NoError(t, err)
	extract.Stdin = pipe
	require.NoError(t, extract.Start())
	require.NoError(t, archive.Run())
	require.NoError(t, extract.Wait())
	return dir
}

// run2xIn runs script in an interactive 2.x shell that sourced root/gash.sh.
func run2xIn(t *testing.T, root string, env []string, script string) (string, string, int) {
	t.Helper()
	cmd := exec.Command("bash", "--noprofile", "--norc", "-i", "-c",
		`source "$1/gash.sh" >/dev/null 2>&1; GASH_VERSION=test; `+script, "_", root)
	cmd.Env = append([]string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin", "TERM=dumb"}, env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	rc := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		require.True(t, errors.As(err, &ee), err)
		rc = ee.ExitCode()
	}
	return out.String(), errb.String(), rc
}

var helpCalls = [][]string{
	{}, {"--list"}, {"--search", "docker"}, {"--search", "flf"}, {"--search", "XYZnonexistent"}, {"--search"},
	{"--search", "Ä"}, {"--short", "files_largest"}, {"--short", "flf"}, {"--short", "nope"}, {"--short"},
	// 3.0: the full help of files_largest (flf), ai_query and
	// docker_stop_all (dsa) was rewritten with their fixes (FS-*, AI-*,
	// DOCK-22), so the full-help forms use functions whose body 3.0 kept.
	{"-h"}, {"--help"}, {"disk_usage"}, {"du2"}, {"gash_db_test"}, {"gash_help"}, {"nonexistent_func_xyz"},
	{"--list", "--no-color"}, {"--no-color", "dpa"}, {"all_colors"}, {"path_add"}, {"gash_username"},
}

// migrateListing is a gash_help listing line, from its name to the end of
// the line, of a function whose summary 3.0 rewrote: gash_migrate (the D-6
// pointer, SHELL-18) and archive_extract, ai_ask and llm_config (their
// fixed behaviour).
var migrateListing = regexp.MustCompile(`(?m)^(.*(?:gash_migrate|archive_extract|ai_ask|llm_config)\b).*$`)

// gash_help prints what 2.x prints: every form, colors off, on and
// shadowed by --no-color, at two widths.
func TestGashHelpMatches2x(t *testing.T) {
	for _, cols := range []string{"100", "60"} {
		for _, mode := range []string{"off", "on", "shadow"} {
			var script strings.Builder
			for _, args := range helpCalls {
				if mode == "shadow" {
					args = append([]string{"--no-color"}, args...)
				}
				script.WriteString("gash_help")
				for _, a := range args {
					script.WriteString(" " + shq(a))
				}
				script.WriteString(" 2>&1; printf '\\0%s\\0' $?\n")
			}
			gate := "__gash_use_color() { return 1; }; "
			if mode != "off" {
				gate = "__gash_use_color() { return 0; }; "
			}
			// stderr has only Bash's job-control notice of bash -i.
			out, _, _ := run2xGash(t, []string{"LANG=C.UTF-8", "COLUMNS=" + cols}, gate+script.String())
			fields := strings.Split(out, "\x00")
			for i, args := range helpCalls {
				if mode == "shadow" {
					args = append([]string{"--no-color"}, args...)
				}
				env := map[string]string{"LANG": "C.UTF-8", "COLUMNS": cols, "GASH_VERSION": "test"}
				o, e, rc := run(t, "gash_help", env, mode != "off", args...)
				// 3.0: SHELL-18 fixed: gash_migrate's description is the D-6
				// pointer, so its listing line is compared up to the name.
				require.Equal(t, migrateListing.ReplaceAllString(fields[2*i], "$1"), migrateListing.ReplaceAllString(o+e, "$1"), "%s %s %q", cols, mode, args)
				require.Equal(t, fields[2*i+1], strconv.Itoa(rc), "%s %s %q", cols, mode, args)
			}
		}
	}
}

func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// LEDGER CORE-07: gash_help --list (and gash_help with no argument) shows
// the modules in one fixed order, the 2.x list then node remote chrome php,
// never in a hash order.
func TestLedger_CORE07(t *testing.T) {
	want := []string{"FILES", "SYSTEM", "GIT", "DOCKER", "DOCKER-COMPOSE", "AI", "SYSINFO", "GASH", "SSH",
		"LLM", "UTILS", "NODE", "REMOTE", "CHROME", "PHP"}
	for _, args := range [][]string{{"--list"}, nil} {
		out, _, rc := run(t, "gash_help", map[string]string{"LANG": "C.UTF-8"}, false, args...)
		require.Equal(t, 0, rc)
		var got []string
		for _, l := range strings.Split(out, "\n") {
			if m := regexp.MustCompile(`^  ([A-Z][A-Z-]+)$`).FindStringSubmatch(l); m != nil {
				got = append(got, m[1])
			}
		}
		require.Equal(t, want, got, "%v", args)
	}
}
