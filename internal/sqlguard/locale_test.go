// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sqlguard

import (
	"errors"
	"math/rand/v2"
	"strings"
	"testing"
)

// LEDGER DB-13 (forced divergence): the guard is the strictest of the C
// and the UTF-8 readings of 2.x. Each verdict below was checked against
// the 2.x function under LC_ALL=C and LC_ALL=C.UTF-8 (glibc 2.39), except
// the two marked stricter, which no 2.x locale refuses.
func TestLedger_DB13(t *testing.T) {
	sleep := "Statement contains 'SLEEP'"
	runVectors(t, []vector{
		// Case: towupper folds ı and ſ in UTF-8 locales.
		{name: "ſleep, mysql", driver: MySQL, query: "SELECT ſleep(1)", code: CodeWriteOperation, details: sleep},
		{name: "ſleep, pgsql", driver: PgSQL, query: "SELECT ſleep(1)", code: CodeWriteOperation, details: sleep},
		{name: "ſleep, sqlite", driver: SQLite, query: "SELECT ſleep(1)", code: CodeWriteOperation, details: sleep},
		{name: "ınto", driver: SQLite, query: "SELECT 1 ınto x", code: CodeWriteOperation, details: "Statement contains 'INTO'"},
		{name: "first word extended by ſ", driver: SQLite, query: "SELECTſ 1", code: CodeWriteOperation, details: "Statement starts with 'SELECTS'"},
		{name: "ſ then the Kelvin sign: towupper alone sees SLEEP", driver: SQLite, query: "SELECT ſLEEPK(1)", code: CodeWriteOperation, details: sleep},
		{name: "stricter: the Kelvin sign folds to K", driver: SQLite, query: "SELECT 1 FROM t LOCK", code: CodeWriteOperation, details: "Statement contains 'LOCK'"},
		// Blanks: iswspace takes U+3000, U+2028 and the like.
		{name: "LOAD U+3000 DATA", driver: MySQL, query: "SELECT 1 LOAD　DATA", code: CodeWriteOperation, details: "Statement contains 'LOAD DATA'"},
		{name: "stricter: LOAD NBSP DATA", driver: MySQL, query: "SELECT 1 LOAD DATA", code: CodeWriteOperation, details: "Statement contains 'LOAD DATA'"},
		{name: "dot line after U+3000", driver: SQLite, query: "SELECT 1\n　.shell id", code: CodeDotCommand, details: ".shell id"},
		{name: "go line before U+2028", driver: SQLite, query: "SELECT 1\ngo ", code: CodeMultipleStatements, details: "Line 'go' ends a statement in the database client"},
		// The mysql "-- " rule: U+3000 starts a comment in UTF-8 locales,
		// a no-break space does not.
		{name: "mysql --U+3000 comment changes the lexing", driver: MySQL, query: "SELECT 1 --　 '\n; DELETE '", code: CodeUnterminated},
		{name: "mysql --NBSP is code", driver: MySQL, query: "SELECT 1 --  ſleep(1)", code: CodeWriteOperation, details: sleep},
		// Both refuse: the C reading's error.
		{name: "U+3000 before the keyword", driver: SQLite, query: "　SELECT 1", code: CodeWriteOperation, details: "Statement starts with '?'"},
		{name: "ſelect", driver: SQLite, query: "ſelect 1 FROM t FOR UPDATE", code: CodeWriteOperation, details: "Statement starts with '?'"},
		// Both accept.
		{name: "ſ in a string", driver: SQLite, query: "SELECT 'ſleep(1)'", kind: "SELECT"},
		{name: "ſ in a comment", driver: PgSQL, query: "SELECT 1 -- ſleep", kind: "SELECT"},
		{name: "ı in an identifier", driver: SQLite, query: "SELECT ıd FROM t", kind: "SELECT"},
		{name: "U+3000 between words", driver: SQLite, query: "SELECT　1", kind: "SELECT"},
	})

	var e *Error
	if err := Precheck("SELECT ſleep(1)"); !errors.As(err, &e) || e.Details != sleep {
		t.Errorf("Precheck: %v", err)
	}
	if err := Precheck("SELECT ſleep"); err == nil {
		t.Error("Precheck accepted a query the UTF-8 reading refuses")
	}
}

// LEDGER DB-13, monotonicity: for random queries the guard refuses
// whenever one reading refuses, with the error of the first refusing
// reading in the order C, UTF-8, UTF-8 with case folding, and accepts
// only when every reading accepts, with the C reading's result. The same
// holds for Precheck.
func TestLedger_DB13_Monotonic(t *testing.T) {
	alpha := []string{
		"ſ", "ı", "K", "　", " ", " ", " ", " ", " ", "\u0085",
		" ", "é", "\xff", "SELECT ", "select", "WITH ", "SHOW", "DESC", "leep", "SLEEP", "LOAD",
		"DATA", "XML", "LOC", "K", "nto", "INTO", "go", "GO", "\n", "\n.", ".", "/", "--", "-- ", "#", "'",
		"\"", "`", "[", "]", "$$", "/*", "*/", ";", " ", "\t", "(", "e", "\\", "1", "x",
	}
	rng := rand.New(rand.NewPCG(13, 13))
	refusals, accepts, splits := 0, 0, 0
	for n := 0; n < 20000; n++ {
		var b strings.Builder
		for k := rng.IntN(14); k >= 0; k-- {
			b.WriteString(alpha[rng.IntN(len(alpha))])
		}
		q := b.String()
		for _, d := range []string{MySQL, PgSQL, SQLite, "other"} {
			var want error
			var wantRes Result
			refusedBy := 0
			for k, rd := range readings {
				res, err := check(d, q, rd)
				if k == 0 {
					wantRes = res
				}
				if err != nil {
					refusedBy++
					if want == nil {
						want = err
					}
				}
			}
			if refusedBy > 0 && refusedBy < len(readings) {
				splits++
			}
			res, err := Check(d, q)
			switch {
			case want == nil && (err != nil || res != wantRes):
				t.Fatalf("Check(%s, %q) = %+v %v, want the C result %+v", d, q, res, err, wantRes)
			case want != nil && (err == nil || *err.(*Error) != *want.(*Error)):
				t.Fatalf("Check(%s, %q) = %v, want the first refusal %v", d, q, err, want)
			case err == nil:
				accepts++
			default:
				refusals++
			}
		}
		var want error
		for _, rd := range readings {
			if err := precheck(q, rd); err != nil {
				want = err
				break
			}
		}
		if got := Precheck(q); (got == nil) != (want == nil) || got != nil && *got.(*Error) != *want.(*Error) {
			t.Fatalf("Precheck(%q) = %v, want %v", q, got, want)
		}
	}
	if splits == 0 || accepts == 0 || refusals == 0 {
		t.Errorf("weak sample: %d accepts, %d refusals, %d split verdicts", accepts, refusals, splits)
	}
}

// LEDGER CORE-16: the dot_command details are the first 24 characters of
// the line, counted as a UTF-8 locale counts them (an invalid byte is one
// character); LC_ALL=C would count bytes.
func TestLedger_CORE16(t *testing.T) {
	long := ".abcdefghijklmnopqrstuvwxyz0123"
	runVectors(t, []vector{
		{name: "24 characters", driver: SQLite, query: "SELECT 1\n" + long, code: CodeDotCommand, details: long[:24]},
		{name: "multibyte characters", driver: SQLite, query: "SELECT 1\n." + strings.Repeat("é", 30), code: CodeDotCommand, details: "." + strings.Repeat("é", 23)},
		{name: "invalid bytes count one each", driver: SQLite, query: "SELECT 1\n." + strings.Repeat("\xff", 30), code: CodeDotCommand, details: "." + strings.Repeat("\xff", 23)},
		{name: "after a Unicode blank", driver: SQLite, query: "SELECT 1\n　." + strings.Repeat("日", 30), code: CodeDotCommand, details: "." + strings.Repeat("日", 23)},
	})
}
