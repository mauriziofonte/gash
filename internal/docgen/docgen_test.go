// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docgen

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/cli"
	"github.com/mauriziofonte/gash/internal/handlers"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/term"
)

var update = flag.Bool("update", false, "rewrite the generated documentation")

// docs is the docs/ folder of this checkout.
var docs = filepath.Join("..", "..", "docs")

// gashHelp runs `gash ARGS... --help` in process, as the binary does.
func gashHelp(t *testing.T) HelpFunc {
	tbl := handlers.Table(func(*invoke.Table) {})
	return func(args ...string) string {
		var out, errb bytes.Buffer
		env := invoke.EnvMap(map[string]string{"LANG": "C.UTF-8", "HOME": "/home/user"})
		rc := cli.Run(cli.Process{
			Args: append(append([]string{}, args...), "--help"), Env: env, Stdin: strings.NewReader(""),
			Stdout: &out, Stderr: &errb, Term: term.Fixed(env, false, false, false), Table: tbl,
			Executable: func() (string, error) { return "/opt/gash", nil },
		})
		require.Equal(t, 0, rc, "gash %v --help: %s", args, errb.String())
		return out.String()
	}
}

// generated compares (or, with -update, writes) a generated file.
func generated(t *testing.T, path, want string) {
	t.Helper()
	if *update {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(want), 0o644))
		return
	}
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(got), "%s is stale: go test ./internal/docgen -update", path)
}

// TestReference keeps docs/<area>/reference.md current.
func TestReference(t *testing.T) {
	help := gashHelp(t)
	for _, a := range Areas {
		generated(t, filepath.Join(docs, a.Key, "reference.md"), Reference(a, help))
	}
}

// TestEveryFunctionHasAnArea: each function with help is documented in
// exactly one area.
func TestEveryFunctionHasAnArea(t *testing.T) {
	known := map[string]bool{}
	for _, a := range Areas {
		known[a.Key] = true
	}
	for _, f := range registry.Functions() {
		if !f.HasHelp {
			continue
		}
		require.True(t, known[AreaOf(f)], "%s (topic %q) has no docs area", f.Name, f.Topic)
	}
	for _, al := range registry.Aliases() {
		if al.Pack != "" {
			require.True(t, known[packAreas[al.Pack]], "alias pack %q has no docs area", al.Pack)
		}
	}
}

// block checks (or, with -update, rewrites) a generated block of a
// hand-written page.
func block(t *testing.T, path, name, body string) {
	t.Helper()
	src, err := os.ReadFile(path)
	require.NoError(t, err)
	got, err := SetBlock(string(src), name, body)
	require.NoError(t, err, path)
	if *update {
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		return
	}
	require.Equal(t, got, string(src), "%s: the %s block is stale: go test ./internal/docgen -update", path, name)
}

// TestBlocks keeps the generated blocks of the guides current: the
// prerequisites of each area, the area index of docs/README.md, and the
// table of the systems CI tests (docs/install.md, README.md).
func TestBlocks(t *testing.T) {
	for _, a := range Areas {
		block(t, filepath.Join(docs, a.Key, "README.md"), "prerequisites", Prerequisites(a))
	}
	block(t, filepath.Join(docs, "README.md"), "areas", Index(""))
	if src, err := os.ReadFile(filepath.Join("..", "..", "README.md")); err == nil && strings.Contains(string(src), "BEGIN generated: areas") {
		block(t, filepath.Join("..", "..", "README.md"), "areas", Index("docs/"))
	}

	ci, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	require.NoError(t, err)
	hte, err := os.ReadFile(filepath.Join("..", "..", "testdata", "hte", "e2e", "run.sh"))
	require.NoError(t, err)
	systems, err := Systems(string(ci), string(hte))
	require.NoError(t, err)
	for _, page := range []string{filepath.Join(docs, "install.md"), filepath.Join("..", "..", "README.md")} {
		if src, err := os.ReadFile(page); err == nil && strings.Contains(string(src), "BEGIN generated: systems") {
			block(t, page, "systems", systems)
		}
	}
}

func TestExamplesParse(t *testing.T) {
	src := "intro\n<!-- example\nmkdir a\nmask: [0-9]+ms\n-->\n```console\n$ one \\\n  --flag\nout 1\n$ two\n```\n<!-- example -->\n```console\n$ three\nold\n```\n<!-- example -->\nnot a block\n"
	exs := Examples(src)
	require.Len(t, exs, 2)
	require.Equal(t, "mkdir a", exs[0].Setup)
	require.Len(t, exs[0].Masks, 1)
	require.Equal(t, []Step{{Command: "one \\\n  --flag", Output: "out 1\n"}, {Command: "two"}}, exs[0].Steps)
	require.Equal(t, 6, exs[0].Line)
	require.True(t, exs[0].Same("took 3ms\n", "took 41ms"))
	require.False(t, exs[0].Same("took 3ms", "took 41s"))
	got := WithOutputs(src, [][]string{{"OUT 1\n", "OUT 2\nmore\n"}, {""}})
	require.Equal(t, "intro\n<!-- example\nmkdir a\nmask: [0-9]+ms\n-->\n```console\n$ one \\\n  --flag\nOUT 1\n$ two\nOUT 2\nmore\n```\n<!-- example -->\n```console\n$ three\n```\n<!-- example -->\nnot a block\n", got)
}
