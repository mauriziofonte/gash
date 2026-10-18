// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const table = `# Spec parity

### a_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 7 | first | BLACKBOX | pass | not run | ` + "`run`" + ` |  |
| 20 | moved | BLACKBOX | pass | not run | ` + "`run`" + ` | note |
| 30 | diverges | MIXED | pass | FD CORE-02 | ` + "`split`" + ` |  |
| 40 | replaced | INTERNAL | pass | n/a ui.Table | ` + "`unit`" + ` |  |
| 50 | skipped | BLACKBOX | pass | pass | ` + "`run`" + ` |  |
| 60 | untouched | BLACKBOX | pass | not run | ` + "`run`" + ` |  |

### b_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 7 | first | BLACKBOX | pass | not run | ` + "`run`" + ` |  |
`

const results = "a_spec.sh\t7\tfirst\tpass\t\n" +
	"a_spec.sh\t22\tmoved\tfail\texit 1\n" +
	"a_spec.sh\t30\tdiverges\tfail\texit 1\n" +
	"a_spec.sh\t40\treplaced\tpass\t\n" +
	"a_spec.sh\t50\tskipped\tskip\tjq not available\n" +
	"b_spec.sh\t7\tfirst\tfail\texit 2\n" +
	"c_spec.sh\t1\tnew test\tpass\t\n"

func TestUpdate(t *testing.T) {
	res, err := readResults(strings.NewReader(results))
	require.NoError(t, err)
	out, rep := update([]byte(table), res, false)
	got := string(out)
	require.Contains(t, got, "| 7 | first | BLACKBOX | pass | pass | `run` |  |\n")
	require.Contains(t, got, "| 20 | moved | BLACKBOX | pass | fail | `run` | note |\n")
	require.Contains(t, got, "| 30 | diverges | MIXED | pass | FD CORE-02 | `split` |  |\n")
	require.Contains(t, got, "| 40 | replaced | INTERNAL | pass | n/a ui.Table | `unit` |  |\n")
	require.Contains(t, got, "| 50 | skipped | BLACKBOX | pass | not run | `run` |  |\n")
	require.Contains(t, got, "| 60 | untouched | BLACKBOX | pass | not run | `run` |  |\n")
	require.Contains(t, got, "### b_spec.sh\n\n| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |\n|---|---|---|---|---|---|---|\n| 7 | first | BLACKBOX | pass | fail | `run` |  |\n")
	require.Equal(t, 4, rep.updated)
	require.Equal(t, 1, rep.moved)
	require.Equal(t, 2, rep.kept)
	require.Equal(t, []string{"c_spec.sh:1 new test"}, rep.unmatched)
	require.Equal(t, map[string]int{"pass": 3, "fail": 3, "not run": 1}, rep.totals)

	// Only the Go column changes.
	require.Equal(t, strings.Count(table, "\n"), strings.Count(got, "\n"))

	out, _ = update([]byte(table), res, true)
	require.Contains(t, string(out), "| 30 | diverges | MIXED | pass | fail | `split` |  |\n")
}

func TestRunWritesAndChecks(t *testing.T) {
	dir := t.TempDir()
	rp, pp := filepath.Join(dir, "r.tsv"), filepath.Join(dir, "PARITY.md")
	require.NoError(t, os.WriteFile(rp, []byte(results), 0o644))
	require.NoError(t, os.WriteFile(pp, []byte(table), 0o644))
	var out, errb bytes.Buffer
	require.Equal(t, 1, run(rp, pp, false, true, &out, &errb))
	b, _ := os.ReadFile(pp)
	require.Equal(t, table, string(b))
	require.Equal(t, 0, run(rp, pp, false, false, &out, &errb))
	require.Equal(t, 0, run(rp, pp, false, true, &out, &errb))
	require.Contains(t, out.String(), "parity: run totals: 3 pass, 3 fail, 1 not run")
}

func TestRealParityParses(t *testing.T) {
	b, err := os.ReadFile("../../../PORT/PARITY.md")
	if err != nil {
		t.Skip("no PARITY.md")
	}
	rows := 0
	for _, l := range strings.Split(string(b), "\n") {
		if _, ok := row(l); ok {
			rows++
		}
	}
	require.Equal(t, 1095, rows)
	out, rep := update(b, nil, false)
	require.Equal(t, string(b), string(out))
	require.Zero(t, rep.updated)
}
