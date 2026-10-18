// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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
	{"-h"}, {"--help"}, {"files_largest"}, {"flf"}, {"ai_query"}, {"gash_help"}, {"nonexistent_func_xyz"},
	{"--list", "--no-color"}, {"--no-color", "dsa"}, {"all_colors"}, {"path_add"}, {"gash_username"},
}

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
				require.Equal(t, fields[2*i], o+e, "%s %s %q", cols, mode, args)
				require.Equal(t, fields[2*i+1], strconv.Itoa(rc), "%s %s %q", cols, mode, args)
			}
		}
	}
}

func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
