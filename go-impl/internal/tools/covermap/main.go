// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Command covermap is the M8 testscript-coverage gate. It checks that
// every public 2.x assertion has a black-box home before the Bash
// tests/ framework is removed at cutover (testing.md 2.1, 4, 8).
//
//	cd go-impl && go run ./internal/tools/covermap        # full report
//	cd go-impl && go run ./internal/tools/covermap -v     # list every uncovered row
//	cd go-impl && go run ./internal/tools/covermap -summary # one CI line
//
// The universe is every PARITY.md row whose Plan is `run`, `run*` or
// `split`: a public assertion that needs a testscript home. Each such
// row must be ACCOUNTED FOR by exactly one of:
//
//	(a) a testdata/script/*.txtar case declaring `# covers: <spec>:<line>`
//	    or `# covers-test: <exact test name>`;
//	(b) a tests/parity/bash-only.tsv row (an INTERNAL spec replaced by a
//	    Go unit test, reported as n/a);
//	(c) a LEDGER forced-divergence: a PARITY Go (rc.A) cell reading
//	    `FD <id>` for an id in LEDGER's Forced divergences table.
//
// It prints the totals, the coverage by source, and the uncovered rows
// grouped by spec, and exits non-zero when any row is uncovered or any
// txtar declaration is invalid. The convention the txtar follow is
// testdata/script/README.md. This is a standalone tool (package main);
// it is never run by `go test ./...`, so the default suite stays green
// while coverage is still being built up.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func main() {
	parity := flag.String("parity", "PORT/PARITY.md", "the parity table")
	ledger := flag.String("ledger", "PORT/LEDGER.md", "the ledger (forced-divergence ids)")
	bashOnly := flag.String("bash-only", "../tests/parity/bash-only.tsv", "the bash-only (n/a) map")
	script := flag.String("script", "testdata/script", "the directory of *.txtar cases")
	verbose := flag.Bool("v", false, "list every uncovered row, not only per-spec counts")
	summary := flag.Bool("summary", false, "print one line for CI and exit")
	flag.Parse()
	os.Exit(run(cfg{*parity, *ledger, *bashOnly, *script, *verbose, *summary}, os.Stdout, os.Stderr))
}

type cfg struct {
	parity, ledger, bashOnly, script string
	verbose, summary                 bool
}

func run(c cfg, stdout, stderr io.Writer) int {
	parSrc, err := os.ReadFile(c.parity)
	if err != nil {
		fmt.Fprintln(stderr, "covermap:", err)
		return 2
	}
	ledSrc, err := os.ReadFile(c.ledger)
	if err != nil {
		fmt.Fprintln(stderr, "covermap:", err)
		return 2
	}
	rows := parityRows(parSrc)
	fdIDs, allIDs := ledgerIDs(ledSrc)
	bashOnly, err := readBashOnly(c.bashOnly)
	if err != nil {
		fmt.Fprintln(stderr, "covermap:", err)
		return 2
	}
	cases, errs := readCases(c.script)

	rep := analyze(rows, fdIDs, allIDs, bashOnly, cases)
	rep.parseErrs = append(errs, rep.parseErrs...)

	if c.summary {
		fmt.Fprintf(stdout, "covermap: %d run/run*/split rows, %d covered, %d uncovered, %d errors\n",
			rep.total, rep.covered, rep.uncovered(), len(rep.parseErrs))
		return rep.status()
	}
	rep.print(stdout, c.verbose)
	return rep.status()
}

// ---- PARITY -------------------------------------------------------------

// Row is one PARITY table row under a `### <spec>` section.
type Row struct {
	Spec, Line, Name, Plan, Go string
}

func (r Row) key() string    { return r.Spec + "\x00" + r.Line }
func (r Row) universe() bool { return r.Plan == "run" || r.Plan == "run*" || r.Plan == "split" }

func parityRows(src []byte) []Row {
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
		out = append(out, Row{
			Spec: spec,
			Line: cells[0],
			Name: unescape(cells[1]),
			Go:   strings.TrimSpace(cells[4]),
			Plan: strings.Trim(strings.TrimSpace(cells[5]), "`"),
		})
	}
	return out
}

// tableRow splits a 7-column PARITY row whose first cell is a line number.
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

func unescape(s string) string { return strings.ReplaceAll(s, `\|`, "|") }

// ---- LEDGER -------------------------------------------------------------

var idRe = regexp.MustCompile(`\[([A-Z]+-[0-9]+)\]\(#`)

// ledgerIDs returns the forced-divergence ids (the ids listed in the
// `## Forced divergences` table) and the set of every id that appears
// anywhere as a `[ID](#anchor)` link (for `# ledger:` validation).
func ledgerIDs(src []byte) (fd, all map[string]bool) {
	fd, all = map[string]bool{}, map[string]bool{}
	section := ""
	for _, l := range strings.Split(string(src), "\n") {
		if s, ok := strings.CutPrefix(l, "## "); ok {
			section = strings.TrimSpace(s)
		}
		for _, m := range idRe.FindAllStringSubmatch(l, -1) {
			all[m[1]] = true
			if section == "Forced divergences" && strings.HasPrefix(l, "| [") {
				fd[m[1]] = true
			}
		}
	}
	return fd, all
}

// ---- bash-only.tsv ------------------------------------------------------

// readBashOnly returns the set of spec\x00line keys in the n/a map.
func readBashOnly(path string) (map[string]bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	out := map[string]bool{}
	for _, l := range strings.Split(string(b), "\n") {
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		f := strings.Split(l, "\t")
		if len(f) < 2 {
			continue
		}
		out[f[0]+"\x00"+f[1]] = true
	}
	return out, nil
}

// ---- txtar cases --------------------------------------------------------

// Cover is one `# covers:` / `# covers-test:` declaration.
type Cover struct {
	Spec, Line, Test string // Test set => a covers-test by name
}

// Case is the coverage declaration of one txtar file.
type Case struct {
	File       string
	Covers     []Cover
	FD         []string // forced-divergence ids
	Ledger     []string // reproduced-defect ledger ids (annotation)
	CoversNone bool
	Reason     string
	Declared   bool // any covers/covers-test/forced-divergence/covers-none line
}

// readCases parses the leading comment block of every *.txtar under dir,
// recursively (render fixtures are grouped in a render/ subdirectory).
func readCases(dir string) ([]Case, []string) {
	var paths []string
	var errs []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".txtar") {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		return nil, []string{"covermap: " + err.Error()}
	}
	sort.Strings(paths)
	var cases []Case
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			errs = append(errs, "covermap: "+err.Error())
			continue
		}
		rel, rerr := filepath.Rel(dir, p)
		if rerr != nil {
			rel = filepath.Base(p)
		}
		c, es := parseCase(rel, string(b))
		cases = append(cases, c)
		errs = append(errs, es...)
	}
	return cases, errs
}

// parseCase reads the directives from the leading comment block: the run
// of `#`-comment and blank lines before the first testscript command or
// `-- file --` marker.
func parseCase(file, body string) (Case, []string) {
	c := Case{File: file}
	var errs []string
	for _, l := range strings.Split(body, "\n") {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		if !strings.HasPrefix(t, "#") {
			break // first real line ends the header block
		}
		d := strings.TrimSpace(strings.TrimPrefix(t, "#"))
		switch {
		case strings.HasPrefix(d, "covers-test:"):
			v := strings.TrimSpace(strings.TrimPrefix(d, "covers-test:"))
			if v == "" {
				errs = append(errs, file+": empty covers-test")
				continue
			}
			c.Covers = append(c.Covers, Cover{Test: v})
			c.Declared = true
		case strings.HasPrefix(d, "covers:"):
			v := strings.TrimSpace(strings.TrimPrefix(d, "covers:"))
			c.Declared = true
			if strings.HasPrefix(v, "none") {
				c.CoversNone = true
				c.Reason = strings.TrimSpace(strings.TrimPrefix(v, "none"))
				continue
			}
			spec, line, ok := splitSpecLine(v)
			if !ok {
				errs = append(errs, file+": bad covers target "+v+" (want spec:line or none)")
				continue
			}
			c.Covers = append(c.Covers, Cover{Spec: spec, Line: line})
		case strings.HasPrefix(d, "forced-divergence:"):
			v := strings.TrimSpace(strings.TrimPrefix(d, "forced-divergence:"))
			if v == "" {
				errs = append(errs, file+": empty forced-divergence")
				continue
			}
			c.FD = append(c.FD, v)
			c.Declared = true
		case strings.HasPrefix(d, "ledger:"):
			v := strings.TrimSpace(strings.TrimPrefix(d, "ledger:"))
			if v != "" {
				c.Ledger = append(c.Ledger, v)
			}
		}
	}
	return c, errs
}

// splitSpecLine splits "spec_file.sh:123" on the last colon.
func splitSpecLine(v string) (spec, line string, ok bool) {
	i := strings.LastIndexByte(v, ':')
	if i <= 0 || i == len(v)-1 {
		return "", "", false
	}
	spec, line = v[:i], v[i+1:]
	if strings.Trim(line, "0123456789") != "" {
		return "", "", false
	}
	return spec, line, true
}

// ---- analysis -----------------------------------------------------------

type source int

const (
	srcTxtar source = iota
	srcBashOnly
	srcLedgerFD
)

type report struct {
	total     int
	byPlan    map[string]int
	covered   int
	bySource  map[source]int
	conflicts []string
	uncov     []Row // uncovered universe rows
	parseErrs []string
	fdCases   int
	fdIDs     []string
}

func (r report) uncovered() int { return r.total - r.covered }

func (r report) status() int {
	if r.uncovered() > 0 || len(r.parseErrs) > 0 {
		return 1
	}
	return 0
}

func analyze(rows []Row, fdIDs, allIDs, bashOnly map[string]bool, cases []Case) report {
	rep := report{byPlan: map[string]int{}, bySource: map[source]int{}}

	// Index PARITY rows.
	byKey := map[string]*Row{}
	byName := map[string][]*Row{}
	for i := range rows {
		r := &rows[i]
		byKey[r.key()] = r
		byName[r.Name] = append(byName[r.Name], r)
	}

	// Which universe keys each source accounts for.
	covered := map[string]map[source]bool{}
	mark := func(key string, s source) {
		if covered[key] == nil {
			covered[key] = map[source]bool{}
		}
		covered[key][s] = true
	}

	// Validate and apply txtar declarations.
	seenFD := map[string]bool{}
	for _, c := range cases {
		if !c.Declared {
			rep.parseErrs = append(rep.parseErrs, c.File+": no coverage declaration (add # covers:, # covers-test: or # forced-divergence:, or # covers: none)")
		}
		if len(c.FD) > 0 {
			rep.fdCases++
		}
		for _, id := range c.FD {
			if !fdIDs[id] {
				rep.parseErrs = append(rep.parseErrs, c.File+": unknown forced-divergence id "+id+" (not in LEDGER Forced divergences)")
				continue
			}
			if !seenFD[id] {
				seenFD[id] = true
				rep.fdIDs = append(rep.fdIDs, id)
			}
		}
		for _, id := range c.Ledger {
			if !allIDs[id] {
				rep.parseErrs = append(rep.parseErrs, c.File+": unknown ledger id "+id)
			}
		}
		for _, cv := range c.Covers {
			if cv.Test != "" {
				m := byName[cv.Test]
				switch {
				case len(m) == 0:
					rep.parseErrs = append(rep.parseErrs, c.File+": covers-test names no PARITY row: "+cv.Test)
				case len(m) > 1:
					rep.parseErrs = append(rep.parseErrs, c.File+": covers-test is ambiguous (matches "+plural(len(m))+"): "+cv.Test)
				case m[0].universe():
					mark(m[0].key(), srcTxtar)
				}
				continue
			}
			r := byKey[cv.Spec+"\x00"+cv.Line]
			if r == nil {
				rep.parseErrs = append(rep.parseErrs, c.File+": covers names no PARITY row: "+cv.Spec+":"+cv.Line)
				continue
			}
			if r.universe() {
				mark(r.key(), srcTxtar)
			}
		}
	}
	sort.Strings(rep.fdIDs)

	// bash-only and forced-divergence (Go column) sources.
	for i := range rows {
		r := &rows[i]
		if !r.universe() {
			continue
		}
		rep.total++
		rep.byPlan[r.Plan]++
		if bashOnly[r.key()] {
			mark(r.key(), srcBashOnly)
		}
		if id, ok := strings.CutPrefix(r.Go, "FD "); ok && fdIDs[strings.TrimSpace(id)] {
			mark(r.key(), srcLedgerFD)
		}
	}

	// Tally.
	for i := range rows {
		r := &rows[i]
		if !r.universe() {
			continue
		}
		srcs := covered[r.key()]
		if len(srcs) == 0 {
			rep.uncov = append(rep.uncov, *r)
			continue
		}
		rep.covered++
		for s := range srcs {
			rep.bySource[s]++
		}
		if len(srcs) > 1 {
			rep.conflicts = append(rep.conflicts, r.Spec+":"+r.Line+" ("+r.Name+")")
		}
	}
	return rep
}

func plural(n int) string { return fmt.Sprintf("%d rows", n) }

func (r report) print(w io.Writer, verbose bool) {
	fmt.Fprintf(w, "covermap: %d run/run*/split rows need a black-box home\n", r.total)
	fmt.Fprintf(w, "  by plan:   run=%d  run*=%d  split=%d\n", r.byPlan["run"], r.byPlan["run*"], r.byPlan["split"])
	fmt.Fprintf(w, "  covered:   %d  (txtar=%d  bash-only=%d  forced-divergence=%d)\n",
		r.covered, r.bySource[srcTxtar], r.bySource[srcBashOnly], r.bySource[srcLedgerFD])
	fmt.Fprintf(w, "  uncovered: %d\n", r.uncovered())
	if r.fdCases > 0 {
		fmt.Fprintf(w, "  forced-divergence cases: %d (ids: %s)\n", r.fdCases, strings.Join(r.fdIDs, ", "))
	}

	if len(r.parseErrs) > 0 {
		fmt.Fprintf(w, "\ncovermap: %d declaration error(s):\n", len(r.parseErrs))
		for _, e := range r.parseErrs {
			fmt.Fprintln(w, "  ERROR", e)
		}
	}
	if len(r.conflicts) > 0 {
		fmt.Fprintf(w, "\ncovermap: %d row(s) accounted by more than one source:\n", len(r.conflicts))
		for _, c := range r.conflicts {
			fmt.Fprintln(w, "  WARN", c)
		}
	}

	if r.uncovered() == 0 {
		fmt.Fprintln(w, "\ncovermap: every public assertion has a black-box home.")
		return
	}

	// Uncovered grouped by spec.
	bySpec := map[string][]Row{}
	var specs []string
	for _, row := range r.uncov {
		if _, ok := bySpec[row.Spec]; !ok {
			specs = append(specs, row.Spec)
		}
		bySpec[row.Spec] = append(bySpec[row.Spec], row)
	}
	sort.Strings(specs)
	fmt.Fprintf(w, "\ncovermap: uncovered by spec (%d specs):\n", len(specs))
	for _, s := range specs {
		fmt.Fprintf(w, "  %-40s %d\n", s, len(bySpec[s]))
		if verbose {
			for _, row := range bySpec[s] {
				fmt.Fprintf(w, "      %s:%s  [%s]  %s\n", row.Spec, row.Line, row.Plan, row.Name)
			}
		}
	}
	if !verbose {
		fmt.Fprintln(w, "\ncovermap: run with -v for the full uncovered list.")
	}
}
