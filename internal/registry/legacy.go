// Gash 4.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

import (
	"strings"
	"sync"
)

// LegacyHit is one former flat name found in a file (gash doctor
// --legacy): the line, the name and the command that replaces it.
type LegacyHit struct {
	Line     int    `json:"line"`
	Name     string `json:"name"`
	Command  string `json:"command"`
	Shortcut string `json:"shortcut,omitempty"`
}

var (
	legacyOnce sync.Once
	legacySet  map[string]bool
)

// LegacyReplacement is the command and shortcut that replace a legacy
// name: gash files largest and flf for files_largest, gash migrate for
// gash_migrate.
func LegacyReplacement(name string) (command, shortcut string) {
	if name == "gash_migrate" {
		return "gash migrate", ""
	}
	if c, ok := CommandOf(name); ok {
		return c.Name(), c.Shortcut
	}
	return "", ""
}

// ScanLegacy returns the legacy names text calls, line by line: a name
// as a whole word (files_largest, not my_files_largest or
// files_largest.sh), outside a comment line. It reads text, not Bash: a
// name in a string or a heredoc is reported too.
func ScanLegacy(text string) []LegacyHit {
	legacyOnce.Do(func() {
		legacySet = map[string]bool{}
		for _, n := range LegacyNames() {
			legacySet[n] = true
		}
	})
	var hits []LegacyHit
	for i, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		for start := 0; start < len(line); {
			if !wordByte(line[start]) {
				start++
				continue
			}
			end := start
			for end < len(line) && wordByte(line[end]) {
				end++
			}
			w := line[start:end]
			before := start == 0 || !strings.ContainsRune("./-$", rune(line[start-1]))
			after := end == len(line) || !strings.ContainsRune(".-", rune(line[end]))
			if legacySet[w] && before && after {
				cmd, sc := LegacyReplacement(w)
				hits = append(hits, LegacyHit{Line: i + 1, Name: w, Command: cmd, Shortcut: sc})
			}
			start = end
		}
	}
	return hits
}

func wordByte(b byte) bool {
	return b == '_' || 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z' || '0' <= b && b <= '9'
}
