// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"os"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/pathguard"
)

// noiseGlobs are the --glob exclusions of llm_grep (llm_find --contains
// adds __pycache__).
var noiseGlobs = []string{"!node_modules", "!vendor", "!.git"}

// secretIglobs are the --iglob arguments of __llm_secret_excludes rg.
func secretIglobs() []string {
	var out []string
	ex := pathguard.SecretExcludes("rg")
	for i := 1; i < len(ex); i += 2 {
		out = append(out, ex[i])
	}
	return out
}

// splitExts is IFS=',' read -ra exts <<<"$extensions": the first line,
// split on commas, a final empty field dropped.
func splitExts(s string) []string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return nil
	}
	f := strings.Split(s, ",")
	if f[len(f)-1] == "" {
		f = f[:len(f)-1]
	}
	return f
}

// search builds the rg run of an llm function. Without one, rc is the
// exit status: a pattern that cannot be read as rg 14 read it fails with
// invalid_regex (LEDGER LLM-37); globs rg refused print nothing, as 2.x.
func (m *Module) search(c *invoke.Call, re string, globs []string, noIgnore bool) (s *rgSearch, rc int, ok bool) {
	r, err := compileRg(re)
	if err != nil {
		return nil, fail(c, "invalid_regex", re, "Patterns use ripgrep's regex syntax: "+err.Error()), false
	}
	cwd, err := m.getwd()
	if err != nil {
		cwd = c.Env.Get("PWD")
	}
	over, ok := newOverrides(cwd, globs, secretIglobs())
	if !ok {
		return nil, 0, false
	}
	userOver, _ := newOverrides(cwd, globs, nil)
	s = &rgSearch{re: r, over: over, userOver: userOver, cwd: cwd}
	s.rules.noIgnore = noIgnore
	s.rules.hidden = !noIgnore
	if !noIgnore {
		s.rules.global = globalExcludes(c.Env.Get("HOME"), c.Env.Get("XDG_CONFIG_HOME"))
	}
	return s, 0, true
}

// pathExists fails with path_not_found/RETRY when the search root does
// not exist (LLM-17: 2.x printed nothing and exited 0, as for no match).
func (m *Module) pathExists(c *invoke.Call, safe string) bool {
	if _, err := os.Stat(safe); err != nil {
		fail(c, "path_not_found", safe, "Provide an existing file or directory")
		return false
	}
	return true
}

// grep is llm_grep (fs.sh __llm_grep_impl), on the one search engine with
// rg semantics (LLM-03).
func (m *Module) grep(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	pattern, extensions, context, limit := "", "", "0", "100"
	havePattern := false
	var p pathTaker
	p.path = "."
	// 3.4 (plan F5, S1-S4): the shared output options and the search
	// modes; without them the output is the 3.0 output.
	o := llmfmt.New(llmfmt.FormatFlag | llmfmt.BudgetFlags | llmfmt.PathFlags)
	var x grepOpts
	args := c.Args
	for len(args) > 0 {
		n, ok := o.Take(c, args)
		if ok && n == 0 {
			n, ok = x.take(c, args)
		}
		if !ok {
			return 1
		}
		if n > 0 {
			args = args[n:]
			continue
		}
		a := args[0]
		switch a {
		case "--ext", "--context", "-C", "-n", "--limit":
			if len(args) < 2 {
				return fail(c, "missing_value", a, a+" needs a value")
			}
			switch a {
			case "--ext":
				extensions = args[1]
			case "--context", "-C":
				context = args[1]
			default:
				limit = args[1]
			}
			args = args[2:]
			continue
		}
		if strings.HasPrefix(a, "-") {
			return fail(c, "unknown_option", a, "Use --ext, --context, or --limit")
		}
		// The first positional is PATTERN, even when empty (LLM-18: 2.x
		// skipped an empty one and took the next word).
		if !havePattern {
			pattern, havePattern = a, true
		} else if !p.take(c, a) {
			return 1
		}
		args = args[1:]
	}
	if pattern == "" {
		return fail(c, "missing_pattern", "", "Provide a search pattern as first argument")
	}
	if !validUint(c, "limit", limit) {
		return 1
	}
	// LLM-01: 2.x evaluated the value as Bash arithmetic; it is a plain
	// integer here, refused otherwise.
	if !validUint(c, "context", context) {
		return 1
	}
	safe, ok := m.validatePath(c, p.path)
	if !ok {
		return 1
	}
	if !m.pathExists(c, safe) {
		return 1
	}
	if m.guard(c).IsSecretFile(safe) {
		return fail(c, "secret_file_blocked", safe, "This file contains secrets and cannot be searched")
	}
	forbidden, ok := m.walkRoot(c, safe, "content")
	if !ok {
		return 1
	}
	globs := append([]string{}, noiseGlobs...)
	for _, e := range splitExts(extensions) {
		globs = append(globs, "*."+e)
	}
	if x.any() || extActive(o) {
		return m.grepExt(c, grepQuery{safe: safe, limit: atoi(limit), context: atoi(context), o: o, x: x, forbidden: forbidden,
			search: func() (*rgSearch, int, bool) { return m.search(c, pattern, globs, false) }})
	}
	s, rc, ok := m.search(c, pattern, globs, false)
	if !ok {
		return rc
	}
	s.context = atoi(context)
	s.forbidden = forbidden
	s.run(safe, c.Stdout, atoi(limit))
	return 0
}
