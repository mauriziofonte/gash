// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package config

import "strings"

// FilePath is the configuration file 2.x reads: ${GASH_ENV_FILE:-$HOME/.gash_env}.
// An empty GASH_ENV_FILE counts as unset.
func FilePath(home, envFile string) string {
	if envFile != "" {
		return envFile
	}
	return home + "/.gash_env"
}

// ShownPath is how the messages name a path: a leading HOME, taken
// literally and followed by "/" or the end of the path, is shown as "~".
// A trailing "/" of HOME is ignored; an empty HOME or "/" shows the path
// as it is (3.0: CORE-14 fixed; 2.x used ${path/#$HOME/\~}, which took
// HOME as a glob pattern and needed no "/" after it, so /home/ab/x was
// ~b/x with HOME=/home/a).
func ShownPath(path, home string) string {
	home = strings.TrimRight(home, "/")
	if home == "" || !strings.HasPrefix(path, home) {
		return path
	}
	if rest := path[len(home):]; rest == "" || rest[0] == '/' {
		return "~" + rest
	}
	return path
}

// ExpandTilde is the SSH key path rule: "~" and a leading "~/" stand for
// HOME, used literally; any other path is returned as written. ok is false
// for "~user" paths, which name another user's home and are refused (3.0:
// CORE-15 fixed; 2.x replaced only the "~", so ~user/k was $HOME followed by
// user/k, and under Bash 5.2 an "&" in HOME stood for the matched "~").
func ExpandTilde(key, home string) (string, bool) {
	switch {
	case key == "~":
		return home, true
	case strings.HasPrefix(key, "~/"):
		return home + key[1:], true
	case strings.HasPrefix(key, "~"):
		return "", false
	}
	return key, true
}

// isSpace is [[:space:]] in the C locale: the only whitespace 3.x trims
// (LEDGER CORE-08).
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\v' || c == '\f' || c == '\r'
}
