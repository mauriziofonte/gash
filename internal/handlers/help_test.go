// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/registry"
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

func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// LEDGER CORE-07: gash help --list shows the namespaces in one fixed
// order (the overview's), never in a hash order.
func TestLedger_CORE07(t *testing.T) {
	var want []string
	for _, ns := range registry.Namespaces() {
		if len(registry.CommandsUnder(ns.Name)) > 0 {
			want = append(want, ns.Name)
		}
	}
	out, _, rc := run(t, "gash_help", map[string]string{"LANG": "C.UTF-8"}, false, "--list")
	require.Equal(t, 0, rc)
	var got []string
	for _, l := range strings.Split(out, "\n") {
		if m := regexp.MustCompile(`^  gash ([a-z]+)  `).FindStringSubmatch(l); m != nil {
			got = append(got, m[1])
		}
	}
	require.Equal(t, want, got)
}
