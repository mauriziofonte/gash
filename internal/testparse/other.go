// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package testparse

import (
	"encoding/json"
	"strings"

	"github.com/mauriziofonte/gash/internal/lazyre"
)

var (
	// pytest: "==== 1 failed, 3 passed, 1 skipped, 1 error in 0.08s ====",
	// or the same without the rules under -q.
	rePytestTotals  = lazyre.MustCompile(`^=*\s*(\d+ [a-z]+(?:, \d+ [a-z]+)*) in ([\d.]+)s(?: \([^)]*\))?\s*=*$`)
	rePytestNoTests = lazyre.MustCompile(`^=*\s*no tests ran in [\d.]+s\s*=*$`)
	rePytestShort   = lazyre.MustCompile(`^(FAILED|ERROR) (\S+)(?: - (.*))?$`)
	rePytestHeader  = lazyre.MustCompile(`^_{3,} (.+?) _{3,}$`)
	rePytestLoc     = lazyre.MustCompile(`^(\S+\.py):(\d+):`)
	pytestWords     = map[string]string{
		"passed": "passed", "xpassed": "passed", "failed": "failed", "error": "failed", "errors": "failed",
		"skipped": "skipped", "xfailed": "skipped", "deselected": "", "warning": "", "warnings": "", "rerun": "",
	}
)

func parsePytest(_ string, lines []string) (Summary, bool) {
	var s Summary
	found := false
	for _, l := range lines {
		if rePytestNoTests.MatchString(l) {
			found = true
			continue
		}
		m := rePytestTotals.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		c, ok := counts(m[1], ",", pytestWords)
		if !ok {
			continue
		}
		found = true
		s.Passed, s.Failed, s.Skipped = c["passed"], c["failed"], c["skipped"]
	}
	if !found {
		return Summary{}, false
	}
	// The location of each test: its section of FAILURES or ERRORS, the
	// first traceback entry in the test's own file.
	locs := map[string][]string{} // section title -> its location lines
	title := ""
	for _, l := range lines {
		if m := rePytestHeader.FindStringSubmatch(l); m != nil {
			title = m[1]
			title = strings.TrimPrefix(title, "ERROR at setup of ")
			title = strings.TrimPrefix(title, "ERROR at teardown of ")
			continue
		}
		if strings.HasPrefix(l, "====") {
			title = ""
		}
		if title != "" && rePytestLoc.MatchString(l) {
			locs[title] = append(locs[title], l)
		}
	}
	for _, l := range lines {
		m := rePytestShort.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		file, name, _ := strings.Cut(m[2], "::")
		if name == "" {
			name = m[2]
		}
		f := Failure{Name: name, File: file, Message: m[3]}
		// pytest titles a class test "TestX.test_y", the node id
		// "TestX::test_y".
		for _, loc := range locs[strings.ReplaceAll(name, "::", ".")] {
			lm := rePytestLoc.FindStringSubmatch(loc)
			if lm[1] == file || f.Line == 0 {
				f.Line = atoi(lm[2])
				if lm[1] == file {
					break
				}
			}
		}
		s.Failures = append(s.Failures, f)
	}
	return s, true
}

var (
	reGoResult  = lazyre.MustCompile(`^(\s*)--- (PASS|FAIL|SKIP): (\S+) \(`)
	reGoPackage = lazyre.MustCompile(`^(ok|FAIL|\?)\s+(\S+)\s+`)
	reGoLoc     = lazyre.MustCompile(`^\s+(\S+\.go):(\d+): (.*)$`)
	reGoBuild   = lazyre.MustCompile(`^(\S+\.go):(\d+):(?:\d+:)? (.*)$`)
	// go test -v: the test the following lines of output belong to.
	reGoMarker = lazyre.MustCompile(`^=== (RUN|CONT|NAME|PAUSE)\s+(\S+)`)
)

func parseGo(_ string, lines []string) (Summary, bool) {
	if s, ok := parseGoJSON(lines); ok {
		return s, true
	}
	type result struct {
		status string
		name   string
		at     int // line index of the --- line
	}
	var results []result
	found, verbose := false, false
	// owner[i] is the test whose output line i is, under -v: go test
	// streams a test's log lines before its --- line, after its === RUN
	// (=== CONT, === NAME) marker.
	owner := make([]string, len(lines))
	cur := ""
	for i, l := range lines {
		if m := reGoMarker.FindStringSubmatch(l); m != nil {
			verbose = true
			cur = m[2]
			if m[1] == "PAUSE" {
				cur = ""
			}
			continue
		}
		if m := reGoResult.FindStringSubmatch(l); m != nil {
			results = append(results, result{m[2], m[3], i})
			found = true
			if m[2] != "FAIL" {
				verbose = true
			}
			cur = ""
			continue
		}
		if reGoPackage.MatchString(l) {
			found = true
			cur = ""
			continue
		}
		owner[i] = cur
	}
	if !found {
		return Summary{}, false
	}
	names := map[string]bool{}
	for _, r := range results {
		names[r.name] = true
	}
	leaf := func(n string) bool {
		for o := range names {
			if strings.HasPrefix(o, n+"/") {
				return false
			}
		}
		return true
	}
	var s Summary
	for _, r := range results {
		if !leaf(r.name) {
			continue
		}
		switch r.status {
		case "PASS":
			s.Passed++
		case "SKIP":
			s.Skipped++
		case "FAIL":
			s.Failed++
			f := Failure{Name: r.name}
			loc := func(l string) bool {
				if lm := reGoLoc.FindStringSubmatch(l); lm != nil {
					f.File, f.Line, f.Message = lm[1], atoi(lm[2]), lm[3]
					return true
				}
				return false
			}
			done := false
			if verbose {
				// The test's own lines, wherever -v streamed them.
				for k := 0; k < r.at && !done; k++ {
					done = owner[k] == r.name && loc(lines[k])
				}
			}
			// Without -v the t.Error lines follow the --- FAIL line,
			// indented under it; never past the next test's marker.
			for k := r.at + 1; k < len(lines) && !done; k++ {
				if reGoResult.MatchString(lines[k]) || reGoPackage.MatchString(lines[k]) || reGoMarker.MatchString(lines[k]) || lines[k] == "FAIL" {
					break
				}
				done = loc(lines[k])
			}
			s.Failures = append(s.Failures, f)
		}
	}
	// Without -v go test names no passing or skipped test: those counts
	// are unknown (not the number of ok packages).
	s.NoPassCounts = !verbose
	// A package that failed without a failing test: a build error or a
	// panic.
	for i, l := range lines {
		m := reGoPackage.FindStringSubmatch(l)
		if m == nil || m[1] != "FAIL" {
			continue
		}
		hasTest := false
		for k := i - 1; k >= 0 && !reGoPackage.MatchString(lines[k]); k-- {
			if rm := reGoResult.FindStringSubmatch(lines[k]); rm != nil && rm[2] == "FAIL" {
				hasTest = true
				break
			}
		}
		if hasTest {
			continue
		}
		f := Failure{Name: m[2]}
		for k := i - 1; k >= 0 && !reGoPackage.MatchString(lines[k]); k-- {
			t := strings.TrimSpace(lines[k])
			if strings.HasPrefix(t, "panic: ") {
				f.Message = t
			}
			if bm := reGoBuild.FindStringSubmatch(t); bm != nil {
				f.File, f.Line, f.Message = bm[1], atoi(bm[2]), bm[3]
			}
		}
		if f.Message == "" && strings.Contains(l, "[build failed]") {
			f.Message = "build failed"
		}
		s.Failed++
		s.Failures = append(s.Failures, f)
	}
	return s, true
}

// parseGoJSON reads go test -json: one event per line.
func parseGoJSON(lines []string) (Summary, bool) {
	type event struct {
		Action  string
		Package string
		Test    string
		Output  string
	}
	type key struct{ pkg, test string }
	var order []key
	status := map[key]string{}
	output := map[key][]string{}
	pkgFail := map[string]bool{}
	pkgHasFail := map[string]bool{}
	n := 0
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if !strings.HasPrefix(l, "{") || !strings.Contains(l, `"Action"`) {
			return Summary{}, false
		}
		var e event
		if json.Unmarshal([]byte(l), &e) != nil {
			return Summary{}, false
		}
		n++
		k := key{e.Package, e.Test}
		switch e.Action {
		case "output":
			output[k] = append(output[k], strings.TrimRight(e.Output, "\n"))
		case "pass", "fail", "skip":
			if e.Test == "" {
				if e.Action == "fail" {
					pkgFail[e.Package] = true
				}
				continue
			}
			if _, seen := status[k]; !seen {
				order = append(order, k)
			}
			status[k] = e.Action
			if e.Action == "fail" {
				pkgHasFail[e.Package] = true
			}
		}
	}
	if n == 0 {
		return Summary{}, false
	}
	var s Summary
	leaf := func(k key) bool {
		for o := range status {
			if o.pkg == k.pkg && strings.HasPrefix(o.test, k.test+"/") {
				return false
			}
		}
		return true
	}
	for _, k := range order {
		if !leaf(k) {
			continue
		}
		switch status[k] {
		case "pass":
			s.Passed++
		case "skip":
			s.Skipped++
		case "fail":
			s.Failed++
			f := Failure{Name: k.test}
			for _, o := range output[k] {
				if lm := reGoLoc.FindStringSubmatch(o); lm != nil {
					f.File, f.Line, f.Message = lm[1], atoi(lm[2]), lm[3]
					break
				}
			}
			s.Failures = append(s.Failures, f)
		}
	}
	for pkg := range pkgFail {
		if pkgHasFail[pkg] {
			continue
		}
		f := Failure{Name: pkg}
		for _, o := range output[key{pkg, ""}] {
			t := strings.TrimSpace(o)
			if strings.HasPrefix(t, "panic: ") || reGoBuild.MatchString(t) {
				f.Message = t
				break
			}
		}
		s.Failed++
		s.Failures = append(s.Failures, f)
	}
	return s, true
}

var (
	reCargoResult = lazyre.MustCompile(`^test result: (?:ok|FAILED)\. (\d+) passed; (\d+) failed; (\d+) ignored;`)
	reCargoStdout = lazyre.MustCompile(`^---- (\S+) stdout ----$`)
	// Rust 1.73+: "thread 'x' panicked at src/lib.rs:21:9:" then the
	// message; before: "thread 'x' panicked at 'msg', src/lib.rs:21:9".
	reCargoPanicNew = lazyre.MustCompile(`^thread '.*' panicked at (.+?):(\d+):\d+:$`)
	reCargoPanicOld = lazyre.MustCompile(`^thread '.*' panicked at '(.*)', (.+?):(\d+):\d+$`)
)

func parseCargo(_ string, lines []string) (Summary, bool) {
	var s Summary
	found := false
	for _, l := range lines {
		if m := reCargoResult.FindStringSubmatch(l); m != nil {
			found = true
			s.Passed += atoi(m[1])
			s.Failed += atoi(m[2])
			s.Skipped += atoi(m[3])
		}
	}
	if !found {
		return Summary{}, false
	}
	for i := 0; i < len(lines); i++ {
		m := reCargoStdout.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		f := Failure{Name: m[1]}
		for k := i + 1; k < len(lines) && !reCargoStdout.MatchString(lines[k]) && lines[k] != "failures:"; k++ {
			if pm := reCargoPanicNew.FindStringSubmatch(lines[k]); pm != nil {
				f.File, f.Line = pm[1], atoi(pm[2])
				f.Message, _ = firstText(lines, k+1, nil)
				break
			}
			if pm := reCargoPanicOld.FindStringSubmatch(lines[k]); pm != nil {
				f.Message, f.File, f.Line = pm[1], pm[2], atoi(pm[3])
				break
			}
		}
		s.Failures = append(s.Failures, f)
	}
	return s, true
}
