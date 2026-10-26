// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package testparse

import (
	"encoding/json"
	"strings"

	"github.com/mauriziofonte/gash/internal/lazyre"
)

var (
	// vitest: "      Tests  1 failed | 4 passed | 1 skipped (6)".
	reVitestTotals = lazyre.MustCompile(`^\s*Tests\s{2,}(.+?)\s+\((\d+)\)\s*$`)
	reVitestFail   = lazyre.MustCompile(`^\s*FAIL\s+(\S+?) > (.+?)\s*$`)
	reVitestAt     = lazyre.MustCompile(`^\s*❯ (.+?):(\d+):\d+\s*$`)
	vitestWords    = map[string]string{"passed": "passed", "failed": "failed", "skipped": "skipped", "todo": "skipped", "expected fail": "passed"}
)

func parseVitest(_ string, lines []string) (Summary, bool) {
	var s Summary
	found := false
	for _, l := range lines {
		m := reVitestTotals.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		c, ok := counts(m[1], "|", vitestWords)
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
		m := reVitestFail.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		f := Failure{Name: m[2], File: m[1]}
		stop := func(l string) bool {
			return reVitestFail.MatchString(l) || reVitestTotals.MatchString(l) || strings.Contains(l, "⎯⎯⎯")
		}
		f.Message, _ = firstText(lines, i+1, stop)
		for k := i + 1; k < len(lines) && !stop(lines[k]); k++ {
			if am := reVitestAt.FindStringSubmatch(lines[k]); am != nil {
				f.File, f.Line = am[1], atoi(am[2])
				break
			}
		}
		s.Failures = append(s.Failures, f)
	}
	return s, true
}

var (
	// jest: "Tests:       2 failed, 1 skipped, 5 passed, 8 total".
	reJestTotals = lazyre.MustCompile(`^Tests:\s+(.+), (\d+) total\s*$`)
	reJestSuite  = lazyre.MustCompile(`^\s*(?:FAIL|PASS)\s+(\S+)`)
	reJestBullet = lazyre.MustCompile(`^\s*● (.+?)\s*$`)
	reJestMarker = lazyre.MustCompile(`^\s*> (\d+) \|`)
	jestWords    = map[string]string{"passed": "passed", "failed": "failed", "skipped": "skipped", "todo": "skipped"}
)

func parseJest(text string, lines []string) (Summary, bool) {
	if s, ok := parseJestJSON(text); ok {
		return s, true
	}
	var s Summary
	found := false
	for _, l := range lines {
		m := reJestTotals.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		c, ok := counts(m[1], ",", jestWords)
		if !ok {
			continue
		}
		found = true
		s.Passed, s.Failed, s.Skipped = c["passed"], c["failed"], c["skipped"]
	}
	if !found {
		return Summary{}, false
	}
	suite := ""
	for i := 0; i < len(lines); i++ {
		if m := reJestSuite.FindStringSubmatch(lines[i]); m != nil {
			suite = m[1]
			continue
		}
		m := reJestBullet.FindStringSubmatch(lines[i])
		// "● Console" heads the console output of a suite (Jest 24 and
		// before), not a failure (3.4.2).
		if m == nil || m[1] == "Test suite failed to run" && suite == "" || m[1] == "Console" {
			continue
		}
		f := Failure{Name: m[1], File: suite}
		stop := func(l string) bool {
			return reJestBullet.MatchString(l) || reJestSuite.MatchString(l) || reJestTotals.MatchString(l) || strings.HasPrefix(l, "Test Suites:")
		}
		f.Message, _ = firstText(lines, i+1, stop)
		// The frame in the suite's own file, else the source marker line.
		for k := i + 1; k < len(lines) && !stop(lines[k]); k++ {
			if fm := reJSFrame.FindStringSubmatch(lines[k]); fm != nil && (suite == "" || strings.HasSuffix(fm[1], suite)) {
				f.File, f.Line = fm[1], atoi(fm[2])
				break
			}
		}
		if f.Line == 0 {
			for k := i + 1; k < len(lines) && !stop(lines[k]); k++ {
				if mm := reJestMarker.FindStringSubmatch(lines[k]); mm != nil {
					f.Line = atoi(mm[1])
					break
				}
			}
		}
		s.Failures = append(s.Failures, f)
	}
	return s, true
}

// parseJestJSON reads jest --json (vitest --reporter=json writes the same
// shape).
func parseJestJSON(text string) (Summary, bool) {
	t := strings.TrimSpace(text)
	i := strings.Index(t, `{"num`)
	if i < 0 || !strings.Contains(t, `"numTotalTests"`) {
		return Summary{}, false
	}
	var doc struct {
		NumFailedTests  *int `json:"numFailedTests"`
		NumPassedTests  int  `json:"numPassedTests"`
		NumPendingTests int  `json:"numPendingTests"`
		NumTodoTests    int  `json:"numTodoTests"`
		TestResults     []struct {
			Name             string `json:"name"`
			Message          string `json:"message"`
			Status           string `json:"status"`
			AssertionResults []struct {
				FullName        string   `json:"fullName"`
				Status          string   `json:"status"`
				FailureMessages []string `json:"failureMessages"`
				Location        *struct {
					Line int `json:"line"`
				} `json:"location"`
			} `json:"assertionResults"`
		} `json:"testResults"`
	}
	dec := json.NewDecoder(strings.NewReader(t[i:]))
	if dec.Decode(&doc) != nil || doc.NumFailedTests == nil {
		return Summary{}, false
	}
	s := Summary{Passed: doc.NumPassedTests, Failed: *doc.NumFailedTests, Skipped: doc.NumPendingTests + doc.NumTodoTests}
	for _, r := range doc.TestResults {
		failedHere := false
		for _, a := range r.AssertionResults {
			if a.Status != "failed" {
				continue
			}
			failedHere = true
			f := Failure{Name: a.FullName, File: r.Name}
			msg := strings.Join(a.FailureMessages, "\n")
			ml := strings.Split(msg, "\n")
			f.Message, _ = firstText(ml, 0, nil)
			if a.Location != nil {
				f.Line = a.Location.Line
			}
			if f.Line == 0 {
				for _, l := range ml {
					if fm := reJSFrame.FindStringSubmatch(l); fm != nil && fm[1] == r.Name {
						f.Line = atoi(fm[2])
						break
					}
				}
			}
			s.Failures = append(s.Failures, f)
		}
		// A suite that failed to run has no assertion results.
		if !failedHere && r.Status == "failed" && r.Message != "" {
			msg, _ := firstText(strings.Split(r.Message, "\n"), 0, nil)
			s.Failures = append(s.Failures, Failure{Name: "Test suite failed to run", File: r.Name, Message: msg})
		}
	}
	return s, true
}

var (
	reMochaPassing = lazyre.MustCompile(`^\s*(\d+) passing(?: \(.+\))?\s*$`)
	reMochaFailing = lazyre.MustCompile(`^\s*(\d+) failing\s*$`)
	reMochaPending = lazyre.MustCompile(`^\s*(\d+) pending\s*$`)
	reMochaItem    = lazyre.MustCompile(`^\s{1,4}(\d+)\) (.+?)\s*$`)
)

func parseMocha(_ string, lines []string) (Summary, bool) {
	var s Summary
	found, start := false, -1
	for i, l := range lines {
		switch {
		case reMochaPassing.MatchString(l):
			s.Passed, found = atoi(reMochaPassing.FindStringSubmatch(l)[1]), true
		case reMochaFailing.MatchString(l):
			s.Failed, found, start = atoi(reMochaFailing.FindStringSubmatch(l)[1]), true, i+1
		case reMochaPending.MatchString(l):
			s.Skipped = atoi(reMochaPending.FindStringSubmatch(l)[1])
		}
	}
	if !found {
		return Summary{}, false
	}
	if start < 0 {
		return s, true
	}
	for i := start; i < len(lines); i++ {
		m := reMochaItem.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		// The title runs over the following lines up to the one ending
		// in ":" (the suites, then the test).
		parts := []string{m[2]}
		j := i
		for !strings.HasSuffix(parts[len(parts)-1], ":") && j+1 < len(lines) && strings.TrimSpace(lines[j+1]) != "" {
			j++
			parts = append(parts, strings.TrimSpace(lines[j]))
		}
		f := Failure{Name: strings.TrimSuffix(strings.Join(parts, " "), ":")}
		stop := func(l string) bool { return reMochaItem.MatchString(l) }
		f.Message, _ = firstText(lines, j+1, stop)
		f.File, f.Line = jsFrame(lines, j+1, stop)
		s.Failures = append(s.Failures, f)
		i = j
	}
	return s, true
}

var (
	rePlaywrightTotal = lazyre.MustCompile(`^\s+(\d+) (passed|failed|skipped|flaky|did not run|interrupted)(?: \(.+\))?\s*$`)
	rePlaywrightItem  = lazyre.MustCompile(`^\s+\d+\) \[(.+?)\] › (.+?):(\d+):\d+ › (.+?)\s*─*\s*$`)
	rePlaywrightAt    = lazyre.MustCompile(`^\s*at (.+?):(\d+):\d+\s*$`)
)

func parsePlaywright(_ string, lines []string) (Summary, bool) {
	var s Summary
	found := false
	for _, l := range lines {
		m := rePlaywrightTotal.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		found = true
		n := atoi(m[1])
		switch m[2] {
		case "passed", "flaky":
			s.Passed += n
		case "failed", "interrupted":
			s.Failed += n
		default:
			s.Skipped += n
		}
	}
	if !found {
		return Summary{}, false
	}
	for i := 0; i < len(lines); i++ {
		m := rePlaywrightItem.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		f := Failure{Name: "[" + m[1] + "] " + m[4], File: m[2], Line: atoi(m[3])}
		stop := func(l string) bool { return rePlaywrightItem.MatchString(l) || rePlaywrightTotal.MatchString(l) }
		f.Message, _ = firstText(lines, i+1, stop)
		// The failing line of the spec, when the stack names it.
		for k := i + 1; k < len(lines) && !stop(lines[k]); k++ {
			if am := rePlaywrightAt.FindStringSubmatch(lines[k]); am != nil && strings.HasSuffix(am[1], m[2]) {
				f.Line = atoi(am[2])
				break
			}
		}
		s.Failures = append(s.Failures, f)
	}
	return s, true
}
