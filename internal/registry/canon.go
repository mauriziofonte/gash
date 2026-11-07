// Gash 3.5. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

import (
	"regexp"
	"strings"
	"sync"
)

// The help texts of the catalog were written for the flat names
// (files_largest, gsync). CanonicalHelp rewrites them for the pages of
// 3.5: the USAGE lines and the example commands name the canonical
// command, and the prose names the canonical command of every former flat
// name. The catalog keeps the text as written (the options of a function
// are parsed from it).

var (
	canonOnce sync.Once
	// formerRe matches a former flat name (one with an underscore, so
	// that prose words never match) or a permanent agent name, as a word.
	formerRe *regexp.Regexp
	canonOf  map[string]string // former flat name, shortcut or function -> canonical
	agentOf  map[string]string // llm_* / agent_* -> canonical
	helpMemo sync.Map          // name -> Help
)

func buildCanon() {
	canonOnce.Do(func() {
		canonOf = map[string]string{}
		agentOf = map[string]string{}
		var former []string
		for i := range tree {
			c := &tree[i]
			if c.Hidden {
				continue
			}
			for _, n := range c.FormerNames() {
				canonOf[n] = c.Name()
				if strings.Contains(n, "_") {
					former = append(former, regexp.QuoteMeta(n))
				}
			}
			if c.Permanent() {
				agentOf[c.Handler] = c.Name()
				canonOf[c.Handler] = c.Name()
			}
			if c.Handler != "" {
				canonOf[c.Handler] = c.Name()
			}
			if c.Shortcut != "" && c.Shortcut != c.Handler {
				canonOf[c.Shortcut] = c.Name()
			}
		}
		// Longest first, so files_largest never matches as files_large...
		sortByLenDesc(former)
		formerRe = regexp.MustCompile(`(^|[^A-Za-z0-9_-])(` + strings.Join(former, "|") + `)($|[^A-Za-z0-9_])`)
	})
}

func sortByLenDesc(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && len(s[j]) > len(s[j-1]); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// Canonical returns the canonical name of a word that names a command at
// the start of a shell command (a flat function name, a permanent agent
// name or a shortcut); ok is false for any other word.
func Canonical(word string) (string, bool) {
	buildCanon()
	c, ok := canonOf[word]
	return c, ok
}

// CanonicalText rewrites prose: each former flat name becomes its
// canonical name; the permanent agent names stay.
func CanonicalText(s string) string {
	buildCanon()
	if !strings.Contains(s, "_") {
		return s
	}
	// The regexp consumes the delimiters: run it until it stops changing.
	for range 4 {
		next := formerRe.ReplaceAllStringFunc(s, func(m string) string {
			sub := formerRe.FindStringSubmatch(m)
			return sub[1] + canonOf[sub[2]] + sub[3]
		})
		if next == s {
			break
		}
		s = next
	}
	return s
}

// commandWord rewrites the shell command at the start of line (after its
// indentation) when its first word names a command: the line, the
// indentation and the growth of the line's first word.
func commandWord(line string) (string, int) {
	trimmed := strings.TrimLeft(line, " ")
	indent := line[:len(line)-len(trimmed)]
	word, rest, _ := strings.Cut(trimmed, " ")
	canon, ok := Canonical(word)
	if !ok {
		return line, 0
	}
	if rest != "" || strings.HasSuffix(trimmed, " ") {
		return indent + canon + " " + rest, len(canon) - len(word)
	}
	return indent + canon, len(canon) - len(word)
}

// pipeline rewrites each command of an example line: the first one and
// those after |, &&, ||, ; and $(.
func pipeline(line string) string {
	line, _ = commandWord(line)
	for _, sep := range []string{" | ", " && ", " || ", "; ", "$("} {
		parts := strings.Split(line, sep)
		for i := 1; i < len(parts); i++ {
			parts[i], _ = commandWord(parts[i])
		}
		line = strings.Join(parts, sep)
	}
	return line
}

// CanonicalHelp returns the help of f as the 3.5 pages show it.
func CanonicalHelp(f *Function) Help {
	if v, ok := helpMemo.Load(f.Name); ok {
		return v.(Help)
	}
	h := f.Help
	var out []string
	section := ""
	grow := 0 // the growth of the last USAGE line's first word
	for _, line := range strings.Split(h.Body, "\n") {
		if line != "" && line[0] != ' ' && strings.ToUpper(line) == line {
			section = line
			out = append(out, line)
			continue
		}
		trimmed := strings.TrimLeft(line, " ")
		switch {
		case trimmed == "":
		case strings.HasPrefix(trimmed, "#"):
			line = CanonicalText(line)
		case section == "USAGE":
			var g int
			if next, d := commandWord(line); d != 0 || next != line {
				line, grow = next, d
			} else if g = grow; g > 0 && len(line)-len(trimmed) > 2 {
				line = strings.Repeat(" ", g) + line // a continuation, kept aligned
			} else if g < 0 && len(line)-len(trimmed) > 2-g {
				line = line[-g:]
			}
			line = pipelineRest(line)
		case section == "EXAMPLES":
			line = pipeline(line)
			if !strings.HasPrefix(strings.TrimLeft(line, " "), "gash help ") {
				line = CanonicalText(line) // the words gash help is given stay as typed
			}
		default:
			line = CanonicalText(line)
		}
		out = append(out, line)
	}
	h.Body = strings.Join(out, "\n")
	var see []string
	for _, s := range strings.Fields(h.SeeAlso) {
		if c, ok := Canonical(s); ok {
			s = c
		}
		see = append(see, s)
	}
	h.SeeAlso = strings.Join(see, "\n")
	helpMemo.Store(f.Name, h)
	return h
}

// pipelineRest rewrites the commands after a pipe on a USAGE line
// (`command | ai_query "..."`).
func pipelineRest(line string) string {
	parts := strings.Split(line, " | ")
	for i := 1; i < len(parts); i++ {
		parts[i], _ = commandWord(parts[i])
	}
	return strings.Join(parts, " | ")
}
