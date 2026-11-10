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

// CanonicalCall is call, a function name and what follows it ("process_find",
// "archive_extract (tar archives)", "git_sync [-m MESSAGE]"), as a 4.x shell
// takes it: a legacy first word becomes its command ("gash system ps",
// "gash files extract (tar archives)"). Any other call stays: a permanent
// name (llm_procs --port), a kept one (gash_username), a command.
func CanonicalCall(call string) string {
	word, rest, found := strings.Cut(call, " ")
	if !Legacy(word) {
		return call
	}
	cmd, _ := LegacyReplacement(word)
	if found {
		return cmd + " " + rest
	}
	return cmd
}

// ScanLegacy returns the legacy names text calls, line by line: a name
// as a whole word (files_largest, not my_files_largest or
// files_largest.sh), outside a comment line. It reads text, not Bash: a
// name in a string or a heredoc is reported too. A name the line defines
// (alias hgrep=..., hgrep() {, function hgrep) is not a call: the user's
// own command under a former name is not reported. Nor is a word of a
// gash command (gash ai sysinfo, gash help hgrep).
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
			if legacySet[w] && before && after && !defines(line, start, end) && !gashWord(line, start) {
				cmd, sc := LegacyReplacement(w)
				hits = append(hits, LegacyHit{Line: i + 1, Name: w, Command: cmd, Shortcut: sc})
			}
			start = end
		}
	}
	return hits
}

// defines reports the word line[start:end] as the name a definition
// gives: followed by = (an alias or a variable), followed by () (a
// function), or after the function keyword.
func defines(line string, start, end int) bool {
	if strings.HasPrefix(line[end:], "=") || strings.HasPrefix(strings.TrimLeft(line[end:], " \t"), "()") {
		return true
	}
	before := strings.TrimRight(line[:start], " \t")
	if len(before) == start || !strings.HasSuffix(before, "function") {
		return false
	}
	kw := len(before) - len("function")
	return kw == 0 || !wordByte(before[kw-1])
}

// gashWord reports the word at start as a word of a gash command: the
// simple command it is in (after the last ; | & ( { or quote) starts with
// gash, or gash is one of the two words before it (prose naming gash ai
// sysinfo).
func gashWord(line string, start int) bool {
	seg := line[:start]
	if i := strings.LastIndexAny(seg, ";|&({'\"`"); i >= 0 {
		seg = seg[i+1:]
	}
	f := strings.Fields(seg)
	n := len(f)
	return n > 0 && (f[0] == "gash" || f[n-1] == "gash" || (n > 1 && f[n-2] == "gash"))
}

func wordByte(b byte) bool {
	return b == '_' || 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z' || '0' <= b && b <= '9'
}
