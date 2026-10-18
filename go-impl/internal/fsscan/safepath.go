// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import (
	"os"
	"path/filepath"
	"strings"
)

// scannerForbidden is __gash_fs_forbidden's list: the scanners' own, short
// forbidden set (not the full pathguard list). A resolved scan root under
// one of these is refused; the walk itself is not re-checked, so -L and
// --allow-root still descend into them (FS-02, FS-03).
var scannerForbidden = []string{"/proc", "/sys", "/dev", "/boot", "/root"}

// forbidden reports whether p is, or is under, a protected location, after
// collapsing "//" and "/./" and dropping a trailing "/." (files.sh:116-125).
func forbidden(p string) bool {
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	for strings.Contains(p, "/./") {
		p = strings.ReplaceAll(p, "/./", "/")
	}
	p = strings.TrimSuffix(p, "/.")
	for _, f := range scannerForbidden {
		if p == f || strings.HasPrefix(p, f+"/") {
			return true
		}
	}
	return false
}

// safePath resolves a user PATH and refuses dangerous targets
// (__gash_fs_safe_path). home and pwd are the caller's $HOME and $PWD; the
// relative path is resolved against pwd only, so CDPATH is ignored (FS-25).
func safePath(input, home, pwd string, allowRoot bool) (string, *fsError) {
	if input == "" {
		input = "."
	}
	if input == "~" {
		input = home
	} else if strings.HasPrefix(input, "~/") {
		input = home + "/" + input[2:]
	}

	// Protected locations are refused before any existence check.
	lexical := input
	if !strings.HasPrefix(lexical, "/") {
		lexical = pwd + "/" + lexical
	}
	if forbidden(lexical) {
		return "", errf("forbidden_path", lexical, "This path is protected and cannot be scanned")
	}

	info, err := os.Lstat(input)
	if err != nil {
		return "", errf("path_not_found", "Path '"+input+"' does not exist", "Provide an existing directory path")
	}
	// Follow a final symlink for the directory test, as `[[ -d ]]` does.
	st := info
	if info.Mode()&os.ModeSymlink != 0 {
		if s2, e2 := os.Stat(input); e2 == nil {
			st = s2
		}
	}
	if !st.IsDir() {
		return "", errf("not_a_directory", "Path '"+input+"' is not a directory", "Provide a directory path (not a file or special node)")
	}

	resolved := resolvePhysical(input, pwd)
	if resolved == "" {
		return "", errf("path_unreadable", "Path '"+input+"' is not readable", "Check directory permissions")
	}

	if resolved == "/" && !allowRoot {
		return "", errf("root_scan_blocked", "Scanning '/' is blocked", "Use --allow-root to override (slow, traverses virtual filesystems)")
	}
	if forbidden(resolved) {
		return "", errf("forbidden_path", resolved, "This path is protected and cannot be scanned")
	}
	return resolved, nil
}

// resolvePhysical is `cd -- "$input" && pwd -P`: the physical absolute path
// with every symlink component resolved. A relative input is resolved
// against the working directory (filepath.Abs, FS-25: CDPATH is not read);
// pwd is the fallback when the working directory cannot be read. "" when
// the directory cannot be resolved.
func resolvePhysical(input, pwd string) string {
	p := input
	if !strings.HasPrefix(p, "/") {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		} else {
			p = filepath.Join(pwd, p)
		}
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return ""
	}
	return resolved
}
