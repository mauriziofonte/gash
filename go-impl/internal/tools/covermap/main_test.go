// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

import "testing"

const parityFixture = "" +
	"### foo_spec.sh\n" +
	"\n" +
	"| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |\n" +
	"|---|---|---|---|---|---|---|\n" +
	"| 5 | public thing | BLACKBOX | pass | not run | `run` |  |\n" +
	"| 9 | diverging thing | BLACKBOX | pass | FD LLM-37 | `run` |  |\n" +
	"| 14 | internal thing | INTERNAL | pass | not run | `unit` | foo.Thing |\n" +
	"| 20 | seam thing | MIXED | pass | not run | `split` |  |\n" +
	"\n" +
	"### bar_spec.sh\n" +
	"\n" +
	"| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |\n" +
	"|---|---|---|---|---|---|---|\n" +
	"| 3 | stays bash | BLACKBOX | pass | not run | `shell` |  |\n"

const ledgerFixture = "" +
	"## Forced divergences\n" +
	"\n" +
	"| Id | Title |\n" +
	"|---|---|\n" +
	"| [LLM-37](#llm-37) | regex dialect |\n" +
	"\n" +
	"## Defects\n" +
	"\n" +
	"| [SELF-05](#self-05) | unload removes settings |\n"

func TestParityRows(t *testing.T) {
	rows := parityRows([]byte(parityFixture))
	if len(rows) != 5 {
		t.Fatalf("got %d rows, want 5", len(rows))
	}
	var universe int
	for _, r := range rows {
		if r.universe() {
			universe++
		}
	}
	if universe != 3 { // run, run (FD), split; unit and shell excluded
		t.Fatalf("got %d universe rows, want 3", universe)
	}
	if rows[1].Go != "FD LLM-37" {
		t.Fatalf("Go cell = %q, want %q", rows[1].Go, "FD LLM-37")
	}
}

func TestLedgerIDs(t *testing.T) {
	fd, all := ledgerIDs([]byte(ledgerFixture))
	if !fd["LLM-37"] {
		t.Fatal("LLM-37 should be a forced-divergence id")
	}
	if fd["SELF-05"] {
		t.Fatal("SELF-05 is a Defect, not a forced divergence")
	}
	if !all["SELF-05"] || !all["LLM-37"] {
		t.Fatal("both ids should be in the all-ids set")
	}
}

func TestParseCase(t *testing.T) {
	body := "# Gash 3.0. Author: X\n" +
		"#\n" +
		"# covers: foo_spec.sh:5\n" +
		"# covers-test: seam thing\n" +
		"# forced-divergence: LLM-37\n" +
		"# ledger: SELF-05\n" +
		"\n" +
		"env GASH_HEADLESS=1\n" +
		"# this later comment is not a directive\n" +
		"# covers: bar_spec.sh:3\n"
	c, errs := parseCase("x.txtar", body)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !c.Declared {
		t.Fatal("case should be declared")
	}
	if len(c.Covers) != 2 || c.Covers[0].Spec != "foo_spec.sh" || c.Covers[0].Line != "5" {
		t.Fatalf("covers = %+v", c.Covers)
	}
	if c.Covers[1].Test != "seam thing" {
		t.Fatalf("covers-test = %+v", c.Covers[1])
	}
	if len(c.FD) != 1 || c.FD[0] != "LLM-37" {
		t.Fatalf("fd = %v", c.FD)
	}
	if len(c.Ledger) != 1 || c.Ledger[0] != "SELF-05" {
		t.Fatalf("ledger = %v", c.Ledger)
	}
}

func TestParseCaseNone(t *testing.T) {
	c, errs := parseCase("n.txtar", "# covers: none (3.0 surface)\nexec gash x\n")
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !c.CoversNone || !c.Declared || c.Reason != "(3.0 surface)" {
		t.Fatalf("none case = %+v", c)
	}
}

func TestAnalyze(t *testing.T) {
	rows := parityRows([]byte(parityFixture))
	fd, all := ledgerIDs([]byte(ledgerFixture))
	bashOnly := map[string]bool{"foo_spec.sh\x0014": true} // a unit row, not in universe
	cases := []Case{
		{File: "a.txtar", Declared: true, Covers: []Cover{{Spec: "foo_spec.sh", Line: "5"}}},
		{File: "b.txtar", Declared: true, Covers: []Cover{{Test: "seam thing"}}},
		{File: "c.txtar", Declared: true, CoversNone: true},
	}
	rep := analyze(rows, fd, all, bashOnly, cases)
	if rep.total != 3 {
		t.Fatalf("total = %d, want 3", rep.total)
	}
	// foo:5 via txtar, seam via covers-test, foo:9 via Go-column FD => all 3.
	if rep.covered != 3 || rep.uncovered() != 0 {
		t.Fatalf("covered=%d uncovered=%d, want 3/0", rep.covered, rep.uncovered())
	}
	if rep.bySource[srcTxtar] != 2 || rep.bySource[srcLedgerFD] != 1 {
		t.Fatalf("bySource = %v", rep.bySource)
	}
	if len(rep.parseErrs) != 0 {
		t.Fatalf("parseErrs = %v", rep.parseErrs)
	}
}
