// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Command parity writes the results of a Go-mode run of the 2.x specs into
// the "Go (rc.A)" column of go-impl/PORT/PARITY.md.
//
//	bash tests/run.sh --impl go             # writes tests/.results/go.tsv
//	cd go-impl && go run ./internal/tools/parity
//
// A row takes "pass" or "fail" from the run, "not run" for a skip. Rows
// classified as a forced divergence ("FD <id>") or replaced by a Go test
// ("n/a <test>") keep their value unless -force is given. Rows the run did
// not reach keep theirs. Results are matched by spec and line, then by
// spec and name when the line moved. With -check the file is not written
// and the status is 1 when it would change.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	results := flag.String("results", "../tests/.results/go.tsv", "per-test results of tests/run.sh --impl go")
	parity := flag.String("parity", "PORT/PARITY.md", "the parity table to update")
	force := flag.Bool("force", false, "also overwrite FD and n/a rows")
	check := flag.Bool("check", false, "report only; status 1 when the file would change")
	flag.Parse()
	os.Exit(run(*results, *parity, *force, *check, os.Stdout, os.Stderr))
}

func run(resultsPath, parityPath string, force, check bool, stdout, stderr io.Writer) int {
	rf, err := os.Open(resultsPath)
	if err != nil {
		fmt.Fprintln(stderr, "parity:", err)
		return 2
	}
	res, err := readResults(rf)
	_ = rf.Close()
	if err != nil {
		fmt.Fprintln(stderr, "parity:", err)
		return 2
	}
	src, err := os.ReadFile(parityPath)
	if err != nil {
		fmt.Fprintln(stderr, "parity:", err)
		return 2
	}
	out, rep := update(src, res, force)
	rep.print(stdout)
	if bytes.Equal(out, src) {
		return 0
	}
	if check {
		fmt.Fprintln(stdout, "parity: the Go column is out of date")
		return 1
	}
	if err := os.WriteFile(parityPath, out, 0o644); err != nil {
		fmt.Fprintln(stderr, "parity:", err)
		return 2
	}
	return 0
}

// Result is one line of the results file.
type Result struct {
	Spec, Line, Name, Status, Reason string
}

// readResults reads "spec\tline\tname\tstatus\treason" lines; a spec
// that ran twice keeps its last result.
func readResults(r io.Reader) ([]Result, error) {
	var out []Result
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	n := 0
	for sc.Scan() {
		n++
		if sc.Text() == "" {
			continue
		}
		f := strings.Split(sc.Text(), "\t")
		if len(f) < 4 {
			return nil, fmt.Errorf("results line %d: want 4 or 5 tab-separated fields", n)
		}
		for len(f) < 5 {
			f = append(f, "")
		}
		out = append(out, Result{Spec: f[0], Line: f[1], Name: f[2], Status: f[3], Reason: f[4]})
	}
	return out, sc.Err()
}

// cell is the Go column value of a run status.
func cell(status string) string {
	switch status {
	case "pass", "fail":
		return status
	}
	return "not run"
}

type report struct {
	updated, kept, moved int
	totals               map[string]int
	unmatched            []string
}

func (r report) print(w io.Writer) {
	fmt.Fprintf(w, "parity: %d rows updated (%d matched by name after a line change), %d FD/n/a rows kept\n", r.updated, r.moved, r.kept)
	fmt.Fprintf(w, "parity: run totals: %d pass, %d fail, %d not run\n", r.totals["pass"], r.totals["fail"], r.totals["not run"])
	for _, u := range r.unmatched {
		fmt.Fprintln(w, "parity: no PARITY row for", u)
	}
}

// update rewrites the Go column of every table row of src that has a
// result.
func update(src []byte, results []Result, force bool) ([]byte, report) {
	rep := report{totals: map[string]int{}}
	byLine := map[string]*Result{}
	byName := map[string][]*Result{}
	used := map[*Result]bool{}
	for i := range results {
		r := &results[i]
		byLine[r.Spec+"\x00"+r.Line] = r
		byName[r.Spec+"\x00"+r.Name] = append(byName[r.Spec+"\x00"+r.Name], r)
		rep.totals[cell(r.Status)]++
	}
	lines := strings.SplitAfter(string(src), "\n")
	spec := ""
	for i, l := range lines {
		body := strings.TrimRight(l, "\n")
		if s, ok := strings.CutPrefix(body, "### "); ok {
			spec = strings.TrimSpace(s)
			continue
		}
		cells, ok := row(body)
		if !ok || spec == "" {
			continue
		}
		line, name := cells[0], unescape(cells[1])
		r := byLine[spec+"\x00"+line]
		if r != nil && r.Name != name {
			r = nil
		}
		if r == nil {
			if c := byName[spec+"\x00"+name]; len(c) == 1 {
				r = c[0]
				rep.moved++
			}
		}
		if r == nil {
			continue
		}
		used[r] = true
		cur := cells[4]
		if !force && (strings.HasPrefix(cur, "FD ") || strings.HasPrefix(cur, "n/a ")) {
			rep.kept++
			continue
		}
		cells[4] = cell(r.Status)
		if cells[4] != cur {
			rep.updated++
		}
		lines[i] = "| " + strings.Join(cells, " | ") + " |" + l[len(body):]
	}
	for i := range results {
		if r := &results[i]; !used[r] {
			rep.unmatched = append(rep.unmatched, r.Spec+":"+r.Line+" "+r.Name)
		}
	}
	return []byte(strings.Join(lines, "")), rep
}

// row splits a 7-column table row whose first cell is a line number.
func row(l string) ([]string, bool) {
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

func unescape(s string) string { return strings.ReplaceAll(s, `\|`, "|") }
