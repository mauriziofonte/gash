// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package testparse

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/mauriziofonte/gash/internal/lazyre"
)

var (
	// PHPUnit 9-11 totals: "OK (4 tests, 9 assertions)" or
	// "Tests: 6, Assertions: 7, Errors: 1, Failures: 1, Skipped: 1."
	rePHPUnitOK     = lazyre.MustCompile(`^OK \((\d+) tests?, (\d+) assertions?\)$`)
	rePHPUnitTotals = lazyre.MustCompile(`^Tests: (\d+), Assertions: (\d+)((?:, [A-Za-z ]+: \d+)*)\.$`)
	rePHPUnitPart   = lazyre.MustCompile(`, ([A-Za-z ]+): (\d+)`)
	rePHPUnitHead   = lazyre.MustCompile(`^There (?:was|were) \d+ (failures?|errors?|[a-z ]+?)(?: in \d+ tests?)?:$`)
	rePHPUnitItem   = lazyre.MustCompile(`^(\d+)\) (.+)$`)
	rePHPUnitFrame  = lazyre.MustCompile(`^((?:/|[A-Za-z]:\\|\.{0,2}/?[\w.-]+/).*):(\d+)$`)
)

func parsePHPUnit(_ string, lines []string) (Summary, bool) {
	var s Summary
	found := false
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if m := rePHPUnitOK.FindStringSubmatch(l); m != nil {
			s.Passed, found = atoi(m[1]), true
			continue
		}
		m := rePHPUnitTotals.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		found = true
		total, failed, skipped := atoi(m[1]), 0, 0
		for _, p := range rePHPUnitPart.FindAllStringSubmatch(m[3], -1) {
			switch p[1] {
			case "Errors", "Failures":
				failed += atoi(p[2])
			case "Skipped", "Incomplete":
				skipped += atoi(p[2])
			}
		}
		s.Failed, s.Skipped, s.Passed = failed, skipped, max(total-failed-skipped, 0)
	}
	if !found {
		return Summary{}, false
	}
	s.Failures = phpunitFailures(lines)
	return s, true
}

// phpunitFailures reads the "There was 1 failure:" and "There were 2
// errors:" sections: "N) Class::test", the message, then the stack
// frames (path:line), of which the test file's is the location.
func phpunitFailures(lines []string) []Failure {
	var out []Failure
	inSection := false
	for i := 0; i < len(lines); i++ {
		l := strings.TrimRight(lines[i], " \t")
		if m := rePHPUnitHead.FindStringSubmatch(l); m != nil {
			k := m[1]
			inSection = strings.HasPrefix(k, "failure") || strings.HasPrefix(k, "error")
			continue
		}
		if l == "FAILURES!" || l == "ERRORS!" || strings.HasPrefix(l, "Tests: ") || strings.HasPrefix(l, "OK") {
			inSection = false
		}
		if !inSection {
			continue
		}
		m := rePHPUnitItem.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		f := Failure{Name: m[2]}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if rePHPUnitItem.MatchString(lines[j]) || strings.TrimSpace(lines[j]) == "--" || rePHPUnitHead.MatchString(lines[j]) {
				end = j
				break
			}
		}
		block := lines[i+1 : end]
		var frames [][2]string
		for _, b := range block {
			b = strings.TrimSpace(b)
			if fm := rePHPUnitFrame.FindStringSubmatch(b); fm != nil {
				frames = append(frames, [2]string{fm[1], fm[2]})
			} else if f.Message == "" && b != "" {
				f.Message = b
			}
		}
		if fr := pickFrame(frames); fr[0] != "" {
			f.File, f.Line = fr[0], atoi(fr[1])
		}
		out = append(out, f)
		i = end - 1
	}
	return out
}

// pickFrame is the test file's frame (a path under tests/ or ending in
// Test.php), else the first frame outside vendor/, else the first.
func pickFrame(frames [][2]string) [2]string {
	for _, f := range frames {
		if strings.HasSuffix(f[0], "Test.php") || strings.Contains(f[0], "/tests/") || strings.HasPrefix(f[0], "tests/") {
			return f
		}
	}
	for _, f := range frames {
		if !strings.Contains(f[0], "/vendor/") {
			return f
		}
	}
	if len(frames) > 0 {
		return frames[0]
	}
	return [2]string{}
}

var (
	// Pest and Laravel's Collision printer: "Tests:    2 failed, 1
	// skipped, 2 passed (5 assertions)".
	rePestTotals = lazyre.MustCompile(`^\s*Tests:\s+(.+?)(?:\s+\((\d+) assertions?\))?\s*$`)
	rePestFailed = lazyre.MustCompile(`^\s*FAILED\s+(.+?) > (.+?)(?:\s{2,}(\S+))?\s*$`)
	rePestAt     = lazyre.MustCompile(`^\s*at (.+):(\d+)$`)
	pestWords    = map[string]string{
		"passed": "passed", "failed": "failed", "skipped": "skipped", "todo": "skipped", "todos": "skipped",
		"incomplete": "skipped", "risky": "passed", "warning": "passed", "warnings": "passed",
		"deprecated": "passed", "notice": "passed", "notices": "passed",
	}
)

func parsePest(_ string, lines []string) (Summary, bool) {
	var s Summary
	found := false
	for _, l := range lines {
		m := rePestTotals.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		c, ok := counts(m[1], ",", pestWords)
		if !ok {
			continue
		}
		found = true
		s.Passed, s.Failed, s.Skipped = c["passed"], c["failed"], c["skipped"]
	}
	if !found {
		return Summary{}, false
	}
	for i := 0; i < len(lines); i++ {
		m := rePestFailed.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		f := Failure{Name: m[1] + " > " + m[2]}
		stop := func(l string) bool { return rePestFailed.MatchString(l) || rePestTotals.MatchString(l) }
		msg, j := firstText(lines, i+1, stop)
		if j >= 0 && !rePestAt.MatchString(lines[j]) {
			f.Message = msg
		}
		if m[3] != "" {
			f.Message = strings.TrimSuffix(m[3]+": "+f.Message, ": ")
		}
		for k := i + 1; k < len(lines) && !stop(lines[k]); k++ {
			if am := rePestAt.FindStringSubmatch(lines[k]); am != nil {
				f.File, f.Line = am[1], atoi(am[2])
				break
			}
		}
		s.Failures = append(s.Failures, f)
	}
	return s, true
}

var (
	rePHPStanFound = lazyre.MustCompile(`^\s*\[ERROR\] Found (\d+) errors?`)
	rePHPStanOK    = lazyre.MustCompile(`^\s*\[OK\] No errors`)
	rePHPStanHead  = lazyre.MustCompile(`^\s+Line\s+(\S.*?)\s*$`)
	rePHPStanRow   = lazyre.MustCompile(`^\s+:?(\d+)\s{2,}(\S.*?)\s*$`)
	rePHPStanCont  = lazyre.MustCompile(`^\s{4,}(\S.*?)\s*$`)
	rePHPStanRule  = lazyre.MustCompile(`^\s*-{3,}(\s+-{3,})*\s*$`)
)

// phpstanIDMarks start the identifier and tip lines under a message.
var phpstanIDMarks = []string{"🪪", "💡", "✏️"}

func parsePHPStan(text string, lines []string) (Summary, bool) {
	if s, ok := parsePHPStanJSON(text); ok {
		return s, true
	}
	var s Summary
	found := false
	for _, l := range lines {
		if m := rePHPStanFound.FindStringSubmatch(l); m != nil {
			s.Failed, found = atoi(m[1]), true
		} else if rePHPStanOK.MatchString(l) {
			found = true
		}
	}
	if !found {
		return Summary{}, false
	}
	file := ""
	var cur *Failure
	for _, l := range lines {
		if rePHPStanRule.MatchString(l) {
			cur = nil
			continue
		}
		if m := rePHPStanHead.FindStringSubmatch(l); m != nil {
			file, cur = m[1], nil
			continue
		}
		if file == "" {
			continue
		}
		if m := rePHPStanRow.FindStringSubmatch(l); m != nil {
			s.Failures = append(s.Failures, Failure{File: file, Line: atoi(m[1]), Message: m[2]})
			cur = &s.Failures[len(s.Failures)-1]
			continue
		}
		if m := rePHPStanCont.FindStringSubmatch(l); m != nil && cur != nil {
			t := m[1]
			mark := false
			for _, p := range phpstanIDMarks {
				if strings.HasPrefix(t, p) {
					mark = true
					if p == "🪪" && cur.Name == "" {
						cur.Name = strings.TrimSpace(strings.TrimPrefix(t, p))
					}
				}
			}
			if !mark {
				cur.Message += " " + t
			}
		}
	}
	return s, true
}

// parsePHPStanJSON reads --error-format=json: findings sorted by file
// and line (the JSON object does not keep phpstan's order).
func parsePHPStanJSON(text string) (Summary, bool) {
	t := strings.TrimSpace(text)
	if !strings.HasPrefix(t, "{") || !strings.Contains(t, `"totals"`) {
		return Summary{}, false
	}
	var doc struct {
		Totals *struct {
			Errors     int `json:"errors"`
			FileErrors int `json:"file_errors"`
		} `json:"totals"`
		Files map[string]struct {
			Messages []struct {
				Message    string `json:"message"`
				Line       int    `json:"line"`
				Identifier string `json:"identifier"`
			} `json:"messages"`
		} `json:"files"`
		Errors []string `json:"errors"`
	}
	if json.Unmarshal([]byte(t), &doc) != nil || doc.Totals == nil {
		return Summary{}, false
	}
	s := Summary{Failed: doc.Totals.Errors + doc.Totals.FileErrors}
	files := make([]string, 0, len(doc.Files))
	for f := range doc.Files {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		for _, m := range doc.Files[f].Messages {
			s.Failures = append(s.Failures, Failure{Name: m.Identifier, File: f, Line: m.Line, Message: m.Message})
		}
	}
	for _, e := range doc.Errors {
		s.Failures = append(s.Failures, Failure{Message: e})
	}
	sort.SliceStable(s.Failures, func(i, j int) bool {
		a, b := s.Failures[i], s.Failures[j]
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
	return s, true
}

var (
	rePsalmIssue = lazyre.MustCompile(`^ERROR: (\S+) - (.+?):(\d+):\d+ - (.*?)(?: \(see https?://\S+\))?\s*$`)
	rePsalmCount = lazyre.MustCompile(`^\s*(\d+) errors? found\s*$`)
	rePsalmNone  = lazyre.MustCompile(`^\s*No errors found!?\s*$`)
)

func parsePsalm(_ string, lines []string) (Summary, bool) {
	var s Summary
	found := false
	for _, l := range lines {
		if m := rePsalmCount.FindStringSubmatch(l); m != nil {
			s.Failed, found = atoi(m[1]), true
		} else if rePsalmNone.MatchString(l) {
			found = true
		} else if m := rePsalmIssue.FindStringSubmatch(l); m != nil {
			s.Failures = append(s.Failures, Failure{Name: m[1], File: m[2], Line: atoi(m[3]), Message: m[4]})
		}
	}
	if !found {
		return Summary{}, false
	}
	return s, true
}
