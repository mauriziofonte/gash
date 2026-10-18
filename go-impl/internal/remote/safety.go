// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"regexp"
	"strings"
)

// The 2.x path safety tables.
var (
	denyPrefixes = []string{
		"/etc/ssh", "/etc/sudoers", "/etc/sudoers.d",
		"/etc/passwd", "/etc/shadow", "/etc/shadow-", "/etc/gshadow", "/etc/gshadow-",
		"/etc/pam.d",
		"/boot", "/sys", "/proc", "/dev",
		"/bin", "/sbin", "/lib", "/lib64", "/lib32", "/libx32",
		"/usr/bin", "/usr/sbin", "/usr/lib", "/usr/lib64",
	}
	askPrefixes = []string{"/etc", "/var", "/usr/local", "/root"}
	// askComponents are sensitive wherever they appear: ASK, and never
	// safe for rsync --delete.
	askComponents      = []string{".ssh"}
	deleteSafePrefixes = []string{"/tmp", "/var/tmp"}
)

// normalizePath is __remote_normalize_path: "~" and "~/..." made
// relative, "~root" spelled /root and "~NAME" /home/NAME, repeated
// slashes collapsed, "." segments and trailing slashes dropped. As in
// 2.x, "/." (also "//.") comes out as "." (relative).
func normalizePath(p string) string {
	switch {
	case p == "~" || strings.HasPrefix(p, "~/"):
		p = strings.TrimPrefix(p[1:], "/")
		if p == "" {
			p = "."
		}
	case strings.HasPrefix(p, "~"):
		user, rest := p[1:], ""
		if i := strings.IndexByte(user, '/'); i >= 0 {
			user, rest = user[:i], "/"+user[i+1:]
		}
		if user == "root" {
			p = "/root" + rest
		} else {
			p = "/home/" + user + rest
		}
	}
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	for strings.Contains(p, "/./") {
		p = strings.ReplaceAll(p, "/./", "/")
	}
	p = strings.TrimPrefix(p, "./")
	if strings.HasSuffix(p, "/.") {
		p = p[:len(p)-2]
	}
	if p == "/." {
		p = "/"
	}
	for p != "/" && strings.HasSuffix(p, "/") {
		p = p[:len(p)-1]
	}
	if p == "" {
		return "."
	}
	return p
}

// sensitiveComponent is __remote_sensitive_component: the first
// askComponents entry that is a whole component of the normalized path,
// compared without case (ASCII letters; the entries are ASCII).
func sensitiveComponent(path string) (string, bool) {
	lowered := "/" + asciiLower(path) + "/"
	for _, comp := range askComponents {
		if strings.Contains(lowered, "/"+asciiLower(comp)+"/") {
			return comp, true
		}
	}
	return "", false
}

func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

// under is [[ P == PREFIX || P == PREFIX/* ]].
func under(p, prefix string) bool { return p == prefix || strings.HasPrefix(p, prefix+"/") }

// traversal is [[ "/P/" == */../* ]].
func traversal(p string) bool { return strings.Contains("/"+p+"/", "/../") }

// classifyPath is __remote_classify_path: "allow", "deny:<prefix>" or
// "ask:<prefix>" ("ask:.ssh" for a sensitive component), on the literal
// string: glob and expansion characters are not looked at (LEDGER
// REM-03).
func classifyPath(raw string) string {
	path := normalizePath(raw)
	if traversal(path) {
		return "deny:path_traversal"
	}
	abs := strings.HasPrefix(path, "/")
	if abs {
		if path == "/" {
			return "deny:/"
		}
		for _, prefix := range denyPrefixes {
			if under(path, prefix) {
				return "deny:" + prefix
			}
		}
	}
	if comp, ok := sensitiveComponent(path); ok {
		return "ask:" + comp
	}
	if !abs {
		return "allow"
	}
	for _, prefix := range deleteSafePrefixes {
		if under(path, prefix) {
			return "allow"
		}
	}
	for _, prefix := range askPrefixes {
		if under(path, prefix) {
			return "ask:" + prefix
		}
	}
	return "allow"
}

var (
	homeItself = regexp.MustCompile(`^/(home|Users)/[^/]+$`)
	belowHome  = regexp.MustCompile(`^/home/[^/]+/`)
)

// safeForDelete is __remote_is_safe_for_delete: below /tmp, /var/tmp,
// /home/<user>/ or the remote home (a relative path); never the home
// itself, a sensitive component or a path with "..".
func safeForDelete(raw string) bool {
	path := normalizePath(raw)
	if traversal(path) {
		return false
	}
	if _, ok := sensitiveComponent(path); ok {
		return false
	}
	if path == "." || path == "/root" || homeItself.MatchString(path) {
		return false
	}
	if !strings.HasPrefix(path, "/") {
		return true
	}
	for _, prefix := range deleteSafePrefixes {
		if under(path, prefix) {
			return true
		}
	}
	return belowHome.MatchString(path)
}

// checkSafety is __remote_check_safety PATH OPERATION FORCE: DENY always
// refused; ASK passes with --force, is refused headless, else asks y/N.
func (x *call) checkSafety(path, operation string, force bool) bool {
	verdict := classifyPath(path)
	kind, detail, _ := strings.Cut(verdict, ":")
	switch kind {
	case "deny":
		x.p.Error("Remote path '" + path + "' is in the DENY list (matched: " + detail + "). This is never allowed, not even with --force. Refusing " + operation + ".")
		return false
	case "ask":
		if force {
			return true
		}
		if x.headless() {
			x.p.Error("Remote path '" + path + "' is sensitive (matched: " + detail + "). " + operation + " refused in headless mode without --force.")
			return false
		}
		if x.s.Confirm(x.Term, x.Stderr, "Remote path '"+path+"' is sensitive (matched: "+detail+"). Proceed with "+operation+"?") {
			return true
		}
		x.p.Info(operation + " cancelled.")
		return false
	}
	return true
}

// checkDeleteSafety is __remote_check_delete_safety PATH FORCE, worded
// for the remote side also when PATH is remote_sync_pull's local
// destination (LEDGER REM-06).
func (x *call) checkDeleteSafety(path string, force bool) bool {
	if safeForDelete(path) || force {
		return true
	}
	if x.headless() {
		x.p.Error("rsync --delete targeting '" + path + "' is outside the safe set (below /tmp, /var/tmp, /home/<user>/ or the remote home; never the home itself or .ssh). Refused in headless mode without --force.")
		return false
	}
	if x.s.Confirm(x.Term, x.Stderr, "rsync --delete targeting '"+path+"' can remove files on the remote. Proceed?") {
		return true
	}
	x.p.Info("Operation cancelled.")
	return false
}
