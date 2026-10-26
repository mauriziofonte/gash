// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docgen

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/app"
	"github.com/mauriziofonte/gash/internal/compatgen"
)

// TestMain lets this test binary run as gash (testscript's re-exec), the
// binary the verified examples load.
func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){"gash": func() {
		os.Exit(app.Main(os.Args[1:], os.Environ(), os.Stdin, os.Stdout, os.Stderr))
	}})
}

// TestExamples runs the verified examples of every page (Examples) and
// checks that each shows the output Gash prints; -update writes it back.
// Linux only: the outputs are the Linux ones.
func TestExamples(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the verified examples show Linux output")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	var pages []string
	for _, p := range markdownFiles(t, root) {
		src, err := os.ReadFile(p)
		require.NoError(t, err)
		if len(Examples(string(src))) > 0 {
			pages = append(pages, p)
		}
	}
	if len(pages) == 0 {
		t.Skip("no verified examples")
	}
	exe, err := os.Executable()
	require.NoError(t, err)
	stage := filepath.Join(t.TempDir(), "stage")
	bin, err := compatgen.Stage(root, stage, exe)
	require.NoError(t, err)

	for _, page := range pages {
		rel, _ := filepath.Rel(root, page)
		t.Run(rel, func(t *testing.T) {
			src, err := os.ReadFile(page)
			require.NoError(t, err)
			exs := Examples(string(src))
			outputs := make([][]string, len(exs))
			for n, ex := range exs {
				outputs[n] = runExample(t, stage, bin, ex)
				for s, st := range ex.Steps {
					if ex.Same(st.Output, outputs[n][s]) {
						// Only masked parts differ: the page stays.
						outputs[n][s] = st.Output
					}
				}
			}
			got := WithOutputs(string(src), outputs)
			if *update {
				require.NoError(t, os.WriteFile(page, []byte(got), 0o644))
				return
			}
			require.Equal(t, got, string(src), "%s: an example's output differs from what Gash prints: go test ./internal/docgen -update", rel)
		})
	}
}

// runExample runs an example in a fresh sandbox (HOME and working
// directory): the setup, then its steps in one Bash with Gash loaded, each
// with stdout and stderr together. The sandbox path shows as ~.
func runExample(t *testing.T, stage, bin string, ex Example) []string {
	t.Helper()
	home := t.TempDir()
	env := []string{
		"HOME=" + home, "PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8", "LC_ALL=C.UTF-8", "TZ=UTC",
		"TERM=dumb", "COLUMNS=100", "GASH_BIN=" + bin, "GASH_SH=" + filepath.Join(stage, "gash.sh"),
	}
	if ex.Setup != "" {
		cmd := exec.Command("bash", "--norc", "--noprofile", "-e", "-c", ex.Setup)
		cmd.Dir, cmd.Env = home, env
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "line %d: setup: %s", ex.Line, out)
	}
	const mark = "@@gash-docs-step@@"
	var script strings.Builder
	script.WriteString("source \"$GASH_SH\" >/dev/null 2>&1\n")
	for _, st := range ex.Steps {
		fmt.Fprintf(&script, "printf '%%s\\n' %q\n{\n%s\n} 2>&1\n", mark, st.Command)
	}
	cmd := exec.Command("bash", "--norc", "--noprofile", "-i", "-c", script.String())
	cmd.Dir, cmd.Env = home, env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run()
	parts := strings.Split(out.String(), mark+"\n")
	require.Len(t, parts, len(ex.Steps)+1, "line %d: output: %s", ex.Line, out.String())
	res := make([]string, len(ex.Steps))
	for i := range ex.Steps {
		res[i] = strings.ReplaceAll(parts[i+1], home, "~")
	}
	return res
}
