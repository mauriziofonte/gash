// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Command bashonlymap generates tests/parity/bash-only.tsv from PARITY.md.
//
//	cd go-impl && go run ./internal/tools/bashonlymap
//
// One row is written per PARITY test whose Plan is `unit`: the INTERNAL
// 2.x specs that a Go unit test replaces. gash-test.sh reads the file and
// reports those tests as n/a in Go mode, in a counter of their own
// (testing.md 2.1), so real skips stay visible.
//
// Columns (tab-separated, after a leading header comment line):
//
//	spec<TAB>line<TAB>test name<TAB>replacement
//
// where replacement is the PARITY "Replacement or note" cell (the Go
// test or symbol). With -check the file is not written and the status is
// 1 when it is missing or out of date.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	parity := flag.String("parity", "dev/port/archive/PARITY.md", "the parity table to read")
	out := flag.String("out", "tests/parity/bash-only.tsv", "the TSV to write")
	check := flag.Bool("check", false, "report only; status 1 when the file is missing or stale")
	flag.Parse()
	os.Exit(run(*parity, *out, *check, os.Stdout, os.Stderr))
}

func run(parityPath, outPath string, check bool, stdout, stderr io.Writer) int {
	src, err := os.ReadFile(parityPath)
	if err != nil {
		fmt.Fprintln(stderr, "bashonlymap:", err)
		return 2
	}
	want := generate(parityPath, src)
	fmt.Fprintf(stdout, "bashonlymap: %d unit rows\n", len(unitRows(src)))
	if check {
		got, err := os.ReadFile(outPath)
		if err != nil || !bytes.Equal(got, want) {
			fmt.Fprintln(stdout, "bashonlymap:", outPath, "is missing or out of date")
			return 1
		}
		return 0
	}
	if err := os.MkdirAll(dir(outPath), 0o755); err != nil {
		fmt.Fprintln(stderr, "bashonlymap:", err)
		return 2
	}
	if err := os.WriteFile(outPath, want, 0o644); err != nil {
		fmt.Fprintln(stderr, "bashonlymap:", err)
		return 2
	}
	return 0
}

// generate renders the TSV from the PARITY source.
func generate(parityPath string, src []byte) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# bash-only tests: the INTERNAL 2.x specs replaced by a Go unit test (Plan `unit`).\n")
	fmt.Fprintf(&b, "# Generated from %s by internal/tools/bashonlymap; do not edit by hand.\n", parityPath)
	fmt.Fprintf(&b, "# spec\tline\ttest name\treplacement\n")
	for _, r := range unitRows(src) {
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\n", r.Spec, r.Line, r.Name, r.Replacement)
	}
	return []byte(b.String())
}

// Row is one PARITY table row under a spec section.
type Row struct {
	Spec, Line, Name, Class, Bash, Go, Plan, Replacement string
}

// unitRows returns the rows whose Plan is `unit`, in file order.
func unitRows(src []byte) []Row {
	var out []Row
	spec := ""
	for _, l := range strings.Split(string(src), "\n") {
		if s, ok := strings.CutPrefix(l, "### "); ok {
			spec = strings.TrimSpace(s)
			continue
		}
		cells, ok := tableRow(l)
		if !ok || spec == "" {
			continue
		}
		if plan(cells[5]) != "unit" {
			continue
		}
		out = append(out, Row{
			Spec:        spec,
			Line:        cells[0],
			Name:        unescape(cells[1]),
			Class:       cells[2],
			Bash:        cells[3],
			Go:          cells[4],
			Plan:        plan(cells[5]),
			Replacement: unescape(cells[6]),
		})
	}
	return out
}

// tableRow splits a 7-column PARITY table row whose first cell is a line
// number (so the header and separator rows are skipped). A literal pipe
// inside a cell is written \| in the source and kept escaped here.
func tableRow(l string) ([]string, bool) {
	if !strings.HasPrefix(l, "| ") || !strings.HasSuffix(l, " |") {
		return nil, false
	}
	var cells []string
	var cur strings.Builder
	inner := l[2 : len(l)-2]
	for i := 0; i < len(inner); i++ {
		switch {
		case inner[i] == '\\' && i+1 < len(inner) && inner[i+1] == '|':
			cur.WriteString(`\|`)
			i++
		case strings.HasPrefix(inner[i:], " | "):
			cells = append(cells, cur.String())
			cur.Reset()
			i += 2
		default:
			cur.WriteByte(inner[i])
		}
	}
	cells = append(cells, cur.String())
	if len(cells) != 7 || cells[0] == "" || strings.Trim(cells[0], "0123456789") != "" {
		return nil, false
	}
	return cells, true
}

// plan strips the backticks the Plan cell is wrapped in.
func plan(s string) string { return strings.Trim(strings.TrimSpace(s), "`") }

func unescape(s string) string { return strings.ReplaceAll(s, `\|`, "|") }

// dir returns the directory part of a slash path.
func dir(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return "."
}
