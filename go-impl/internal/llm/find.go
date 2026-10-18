// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"os"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/pathguard"
)

// findPruned are the -name ... -prune tests of the llm_find name search.
var findPruned = map[string]bool{"node_modules": true, "vendor": true, ".git": true, "__pycache__": true}

// find is llm_find (fs.sh __llm_find_impl).
func (m *Module) find(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	pattern, fileType, contains, limit := "", "f", "", "100"
	var p pathTaker
	p.path = "."
	args := c.Args
	for len(args) > 0 {
		a := args[0]
		switch a {
		case "--type", "--contains", "-n", "--limit":
			if len(args) < 2 {
				return fail(c, "missing_value", a, a+" needs a value")
			}
			switch a {
			case "--type":
				fileType = args[1]
			case "--contains":
				contains = args[1]
			default:
				limit = args[1]
			}
			args = args[2:]
			continue
		}
		if strings.HasPrefix(a, "-") {
			return fail(c, "unknown_option", a, "Use --type, --contains, or --limit")
		}
		if pattern == "" {
			pattern = a
		} else if !p.take(c, a) {
			return 1
		}
		args = args[1:]
	}
	if pattern == "" {
		return fail(c, "missing_pattern", "", "Provide a name pattern as first argument, e.g. '*.php' or '*'")
	}
	if fileType != "f" && fileType != "d" {
		return fail(c, "invalid_type", fileType, "Use --type f (files) or --type d (directories)")
	}
	if contains != "" && fileType == "d" {
		return fail(c, "invalid_type", "--type d with --contains", "--contains searches file contents; drop --type d")
	}
	if !validUint(c, "limit", limit) {
		return 1
	}
	safe, ok := m.validatePath(c, p.path)
	if !ok {
		return 1
	}
	n := atoi(limit)
	if contains != "" {
		if m.guard(c).IsSecretFile(safe) {
			return fail(c, "secret_file_blocked", safe, "This file contains secrets and cannot be searched")
		}
		if st, err := os.Stat(safe); err == nil && st.Mode().IsRegular() {
			if !pathguard.MatchGlobSegment(pattern, safe[strings.LastIndexByte(safe, '/')+1:]) {
				return 0
			}
		}
		globs := append([]string{pattern}, noiseGlobs...)
		globs = append(globs, "!__pycache__")
		s, ok := m.search(c, contains, globs, true)
		if !ok {
			return 0
		}
		s.filesOnly = true
		s.run(safe, c.Stdout, n)
		return 0
	}
	if n == 0 {
		return 0
	}
	out := &lineLimiter{w: c.Stdout, left: n}
	w := findWalker{maxDepth: -1, withStart: true}
	w.visit = func(e findEntry) bool {
		name := startName(e.path)
		if e.depth > 0 {
			name = e.path[strings.LastIndexByte(e.path, '/')+1:]
		}
		if findPruned[name] {
			return false
		}
		if e.typeLetter() != fileType[0] {
			return true
		}
		if !pathguard.MatchGlobSegment(pattern, name) || strings.Contains(e.path, "\n") {
			return true
		}
		if out.line([]byte(e.path+"\n")) != nil {
			w.stop = true
		}
		return true
	}
	w.walk(safe)
	return 0
}
