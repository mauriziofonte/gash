// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package testparse reads the output of a test runner or a static
// analyzer into counts and failures (plan L2): agent_test and
// `gash run --test-summary` share it. The runners are pest, phpunit,
// phpstan, psalm, vitest, jest, mocha, pytest, go test, cargo test and
// playwright, in their default reporters and, where the runner has one
// that is commonly used, their JSON reporter (phpstan --error-format=json,
// jest/vitest --json, go test -json).
//
// A parser recognizes its runner by the summary it prints (PHPUnit's
// "Tests: N, Assertions: M", pytest's "N passed in Xs", ...): without
// that summary the output is not parsed, and the caller shows the last
// lines instead. A static analyzer's findings are failures (failed is
// their number, passed is 0).
package testparse

import (
	"bytes"
	"strings"

	"github.com/mauriziofonte/gash/internal/lazyre"
)

// Failure is one failing test or analyzer finding. File is as the runner
// printed it; Line is 0 when the runner gave none.
type Failure struct {
	Name    string `json:"name"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// Summary is what a parser read.
type Summary struct {
	Runner                  string
	Passed, Failed, Skipped int
	Failures                []Failure
}

// parser reads one runner's output (lines without ANSI escapes); ok is
// false when the output does not carry that runner's summary.
type parser func(text string, lines []string) (Summary, bool)

// Runners are the runner names --runner takes, in detection order.
var Runners = []string{"phpunit", "pest", "phpstan", "psalm", "vitest", "jest", "mocha", "pytest", "go", "cargo", "playwright"}

var parsers = map[string]parser{
	"phpunit":    parsePHPUnit,
	"pest":       parsePest,
	"phpstan":    parsePHPStan,
	"psalm":      parsePsalm,
	"vitest":     parseVitest,
	"jest":       parseJest,
	"mocha":      parseMocha,
	"pytest":     parsePytest,
	"go":         parseGo,
	"cargo":      parseCargo,
	"playwright": parsePlaywright,
}

// Known reports whether name is a runner --runner takes.
func Known(name string) bool {
	_, ok := parsers[name]
	return ok
}

// Parse reads out, the merged stdout and stderr of a test command. runner
// is a name of Runners, or "" / "auto" to detect it: the runner argv
// names is tried first (Hint), then every parser in Runners order. ok is
// false when no parser recognized the output.
func Parse(out []byte, runner string, argv []string) (Summary, bool) {
	text := Clean(out)
	lines := strings.Split(text, "\n")
	if runner != "" && runner != "auto" {
		p, ok := parsers[runner]
		if !ok {
			return Summary{}, false
		}
		s, ok := p(text, lines)
		s.Runner = runner
		return s, ok
	}
	order := Runners
	if h := Hint(argv); h != "" {
		order = append([]string{h}, Runners...)
	}
	for _, name := range order {
		if s, ok := parsers[name](text, lines); ok {
			if s.Runner == "" {
				s.Runner = name
			}
			// jest's JSON is vitest's too: the command names which.
			if name == "jest" && Hint(argv) == "vitest" {
				s.Runner = "vitest"
			}
			return s, true
		}
	}
	return Summary{}, false
}

// Hint is the runner a command line names: vendor/bin/pest, npx vitest
// run, python -m pytest, go test ./..., cargo test, php artisan test (the
// Collision printer, pest's format); "" when it names none.
func Hint(argv []string) string {
	for i, a := range argv {
		base := a[strings.LastIndexByte(a, '/')+1:]
		base = strings.TrimSuffix(base, ".phar")
		switch base {
		case "pest", "phpunit", "phpstan", "psalm", "vitest", "jest", "mocha", "pytest", "playwright":
			return base
		case "artisan":
			if i+1 < len(argv) && argv[i+1] == "test" {
				return "pest"
			}
		case "go", "cargo":
			if i+1 < len(argv) && argv[i+1] == "test" {
				return base
			}
		}
	}
	return ""
}

var reANSI = lazyre.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07]*\x07`)

// Clean is out as text: ANSI escapes removed, CRLF as LF, and of a line
// that a progress display rewrote with carriage returns the last state.
func Clean(out []byte) string {
	if bytes.IndexByte(out, 0x1b) >= 0 {
		out = reANSI.ReplaceAll(out, nil)
	}
	s := strings.ReplaceAll(string(out), "\r\n", "\n")
	if !strings.Contains(s, "\r") {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if j := strings.LastIndexByte(strings.TrimRight(l, "\r"), '\r'); j >= 0 {
			lines[i] = l[j+1:]
		}
	}
	return strings.Join(lines, "\n")
}

// Tail is the last n non-blank lines of out, cleaned as Clean does.
func Tail(out []byte, n int) []string {
	lines := strings.Split(Clean(out), "\n")
	var keep []string
	for i := len(lines) - 1; i >= 0 && len(keep) < n; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			keep = append(keep, strings.TrimRight(lines[i], " \t"))
		}
	}
	for i, j := 0, len(keep)-1; i < j; i, j = i+1, j-1 {
		keep[i], keep[j] = keep[j], keep[i]
	}
	if keep == nil {
		keep = []string{}
	}
	return keep
}

// --- shared helpers ---

// atoi is the value of a digit string (callers pass regexp digit runs).
func atoi(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			break
		}
		n = n*10 + int(s[i]-'0')
		if n > 1<<40 {
			break
		}
	}
	return n
}

// firstText is the first non-blank line of lines from i, trimmed, and its
// index (-1 when there is none before stop returns true).
func firstText(lines []string, i int, stop func(string) bool) (string, int) {
	for ; i < len(lines); i++ {
		if stop != nil && stop(lines[i]) {
			return "", -1
		}
		if t := strings.TrimSpace(lines[i]); t != "" {
			return t, i
		}
	}
	return "", -1
}

// counts reads "N word" parts (separated by sep) into a map; ok is false
// when a part is not "N word" with a word of known.
func counts(s, sep string, known map[string]string) (map[string]int, bool) {
	out := map[string]int{}
	for _, part := range strings.Split(s, sep) {
		part = strings.TrimSpace(part)
		num, word, found := strings.Cut(part, " ")
		if !found || num == "" || strings.Trim(num, "0123456789") != "" {
			return nil, false
		}
		key, ok := known[strings.TrimSpace(word)]
		if !ok {
			return nil, false
		}
		out[key] += atoi(num)
	}
	return out, len(out) > 0
}

// reJSFrame is a JavaScript stack frame: "at fn (file:line:col)" or
// "at file:line:col".
var reJSFrame = lazyre.MustCompile(`^\s*at (?:.*? \()?(.+?):(\d+):\d+\)?$`)

// jsFrame is the first stack frame from lines[i] on that is not inside
// node itself or node_modules, before stop.
func jsFrame(lines []string, i int, stop func(string) bool) (string, int) {
	for ; i < len(lines); i++ {
		if stop != nil && stop(lines[i]) {
			break
		}
		m := reJSFrame.FindStringSubmatch(lines[i])
		if m == nil || strings.HasPrefix(m[1], "node:") || strings.Contains(m[1], "node_modules") {
			continue
		}
		return m[1], atoi(m[2])
	}
	return "", 0
}
