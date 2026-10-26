// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import "strings"

// execAbs is __llm_exec_abs: the absolute form of a word as the child would
// see it — ~ and ~user expanded, a relative path joined to cwd (not
// resolved). Symlinks and ".." are left for the path guard to resolve.
func (lc *lineChecker) execAbs(word, cwd string) string {
	p := word
	switch {
	case p == "~" || strings.HasPrefix(p, "~/"):
		p = lc.env.Home + p[1:]
	case strings.HasPrefix(p, "~"):
		user := p
		if i := strings.IndexByte(user, '/'); i >= 0 {
			user = user[:i]
		}
		user = user[1:] // drop leading ~
		if user != "" && !strings.ContainsFunc(user, func(r rune) bool {
			return !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-')
		}) {
			if home, ok := lc.lookupUser(user); ok && strings.HasPrefix(home, "/") {
				p = home + p[len("~"+user):]
			}
		}
	}
	if !strings.HasPrefix(p, "/") {
		p = strings.TrimSuffix(cwd, "/") + "/" + p
	}
	return p
}

// checkPath is __llm_exec_check_path: refuse a path under a forbidden tree
// (/dev/null excepted). Words that do not look like paths are checked only
// when plain is true. details is the original word, as 2.x prints it.
func (lc *lineChecker) checkPath(word, cwd string, plain bool) error {
	if word == "" {
		return nil
	}
	if !plain && !strings.Contains(word, "/") && !strings.HasPrefix(word, "~") && word != "." && word != ".." {
		// A bare name may still be a PROTECT:all path of the user's
		// below the working directory (ls private).
		if len(lc.guard.Hidden) > 0 && lc.guard.Hides(lc.execAbs(word, cwd)) {
			return refuse("forbidden_path", word, "FATAL", "This path is protected and cannot be accessed")
		}
		return nil
	}
	p := lc.execAbs(word, cwd)
	if p == "/dev/null" {
		return nil
	}
	if lc.guard.PathForbidden(p, "") {
		return refuse("forbidden_path", word, "FATAL", "This path is protected and cannot be accessed")
	}
	return nil
}

// tildeExpand mimics bash tilde expansion for a rendered word: a leading ~
// that was typed unquoted (uq begins with ~) expands to $HOME or to ~user's
// home. Quoted tildes (uq does not begin with ~) stay literal.
func (lc *lineChecker) tildeExpand(lit, uq string) string {
	if !strings.HasPrefix(uq, "~") {
		return lit
	}
	// lit begins with the unquoted ~ as well.
	switch {
	case lit == "~" || strings.HasPrefix(lit, "~/"):
		return lc.env.Home + lit[1:]
	case strings.HasPrefix(lit, "~"):
		user := lit
		if i := strings.IndexByte(user, '/'); i >= 0 {
			user = user[:i]
		}
		user = user[1:]
		if user != "" {
			if home, ok := lc.lookupUser(user); ok && strings.HasPrefix(home, "/") {
				return home + lit[len("~"+user):]
			}
		}
		return lit
	}
	return lit
}

// updateCwd is the cd tracking at the end of __llm_exec_check_simple: a later
// command runs where this cd went. Unknown targets (cd -) keep the guess.
func (lc *lineChecker) updateCwd(words []pword, cwd string) string {
	if len(words) == 0 || words[0].lit != "cd" {
		return cwd
	}
	target := ""
	for _, w := range words[1:] {
		// [[ "$w" == -? ]] : a dash plus exactly one character is an option.
		if len(w.lit) == 2 && w.lit[0] == '-' {
			continue
		}
		target = w.lit
	}
	switch {
	case target == "":
		return lc.env.Home
	case target == "-":
		return cwd
	case target == "~" || strings.HasPrefix(target, "~/"):
		return lc.env.Home + target[1:]
	case strings.HasPrefix(target, "/"):
		return target
	default:
		return strings.TrimSuffix(cwd, "/") + "/" + target
	}
}
