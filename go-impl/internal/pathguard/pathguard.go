// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package pathguard

import (
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// FS is the minimal filesystem the guard needs: symlink resolution during
// realpath and the "-e || -L" existence test for secret-file targets.
type FS interface {
	Lstat(name string) (fs.FileInfo, error)
	Readlink(name string) (string, error)
}

// OSFS is the real filesystem.
type OSFS struct{}

func (OSFS) Lstat(name string) (fs.FileInfo, error) { return os.Lstat(name) }
func (OSFS) Readlink(name string) (string, error)   { return os.Readlink(name) }

// errSymlinkLoop is a path whose symlinks expand without end without ever
// repeating (a link to "itself/.."), where GNU realpath -m never returns;
// callers fall back to the input path, as 2.x does with
// `|| normalized="$path"`.
var errSymlinkLoop = errors.New("pathguard: symlink loop")

// maxSymlinkHops bounds the expansions of one Realpath. GNU realpath -m has
// no bound (a repeating loop ends on its own, below), so it is far above
// any real chain.
const maxSymlinkHops = 4096

// Guard holds the environment-dependent part of the 2.x rules: the current
// directory (for making relative paths absolute), $HOME (for the leading-~
// substitution), the effective forbidden list (the base list plus root's
// home when it is not /root) and the filesystem used to resolve symlinks.
type Guard struct {
	FS        FS
	Cwd       string
	Home      string
	Forbidden []string
}

// NewGuard builds a guard. rootHome is the value of ~root; when it is an
// absolute path other than / and /root (core.sh: == /?* && != /root) it is
// appended to the forbidden list, as core.sh does for macOS (/var/root).
func NewGuard(filesystem FS, cwd, home, rootHome string) *Guard {
	forbidden := append([]string{}, ForbiddenBasePaths...)
	if len(rootHome) > 1 && rootHome[0] == '/' && rootHome != "/root" {
		forbidden = append(forbidden, rootHome)
	}
	return &Guard{FS: filesystem, Cwd: cwd, Home: home, Forbidden: forbidden}
}

// OSGuard builds a guard bound to the real process: the OS filesystem, the
// current working directory, $HOME and root's home from the passwd database.
func OSGuard() *Guard {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	rootHome := lookupRootHome()
	return NewGuard(OSFS{}, cwd, home, rootHome)
}

// ValidatePath is __llm_validate_path: it refuses a ".." traversal and a
// forbidden path, and otherwise returns the realpath-normalized path. An
// empty path is ".".
func (g *Guard) ValidatePath(path string) (string, error) {
	if path == "" {
		return ".", nil
	}
	if strings.Contains("/"+path+"/", "/../") ||
		path == ".." || strings.HasPrefix(path, "../") || strings.HasSuffix(path, "/..") {
		return "", &Refusal{
			Code:   "path_traversal_blocked",
			Action: "FATAL",
			Hint:   "Path traversal (..) is not allowed for security reasons",
		}
	}
	// Textual leading-~ substitution (LLM-25: ~user is not expanded).
	if strings.HasPrefix(path, "~") {
		path = g.Home + path[1:]
	}
	normalized, err := g.Realpath(path, false)
	if err != nil {
		normalized = path
	}
	if g.PathForbidden(path, normalized) {
		return "", &Refusal{
			Code:    "forbidden_path",
			Details: normalized,
			Action:  "FATAL",
			Hint:    "This path is protected and cannot be accessed",
		}
	}
	return normalized, nil
}

// PathForbidden is __llm_path_forbidden: whether path is one of the forbidden
// entries or lives below one, tested over the realpath, the realpath with a
// leading /private stripped (macOS), and the lexical (symlink-kept) form.
// When normalized is non-empty the caller's realpath is reused.
func (g *Guard) PathForbidden(path, normalized string) bool {
	if normalized == "" {
		if n, err := g.Realpath(path, false); err == nil {
			normalized = n
		} else {
			normalized = path
		}
	}
	lexical, err := g.Realpath(path, true)
	if err != nil {
		lexical = path
	}
	for _, cand := range []string{normalized, strings.TrimPrefix(normalized, "/private"), lexical} {
		for _, f := range g.Forbidden {
			if cand == f || strings.HasPrefix(cand, f+"/") {
				return true
			}
		}
	}
	return false
}

// PathAboveForbidden is __llm_path_above_forbidden: whether a walk starting at
// path would enter a protected tree (path is "/" or a strict ancestor of a
// forbidden entry). mode "content" counts every forbidden entry; mode "names"
// skips the /etc/* entries (a listing cannot read /etc/shadow by name).
func (g *Guard) PathAboveForbidden(path, mode string) bool {
	normalized, err := g.Realpath(path, false)
	if err != nil {
		normalized = path
	}
	for _, cand := range []string{normalized, strings.TrimPrefix(normalized, "/private")} {
		cand = strings.TrimSuffix(cand, "/")
		if cand == "" {
			return true
		}
		for _, f := range g.Forbidden {
			if mode == "names" && strings.HasPrefix(f, "/etc/") {
				continue
			}
			if strings.HasPrefix(f, cand+"/") {
				return true
			}
		}
	}
	return false
}

// IsSecretFile is __llm_is_secret_file: the base name matches a secret
// pattern (exceptions excluded) or the path is under a secret directory,
// judged on the path as given and on a symlink's target.
func (g *Guard) IsSecretFile(file string) bool {
	if file == "" {
		return false
	}
	if SecretName(file) {
		return true
	}
	if g.existsOrSymlink(file) {
		if resolved, err := g.Realpath(file, false); err == nil && resolved != "" && SecretName(resolved) {
			return true
		}
	}
	return false
}

// Realpath is __llm_realpath: the canonical absolute form of path, what GNU
// `realpath -m` prints (2.x on Linux), on every platform. One component at
// a time, a symlink is replaced by its target; a missing component, a link
// that cannot be read and a link met again with the same rest of the path
// (a loop) stay as plain names, as GNU -m keeps them (so `ln -s loop loop;
// realpath -m loop/x` is DIR/loop/x). With lexical, symlinks are kept
// (realpath -m -s). The 2.x fallback walk for a realpath without -m (BSD,
// older macOS) failed after 40 links instead.
func (g *Guard) Realpath(path string, lexical bool) (string, error) {
	if path == "" {
		return "", errors.New("pathguard: empty path")
	}
	if !strings.HasPrefix(path, "/") {
		path = strings.TrimSuffix(g.Cwd, "/") + "/" + path
	}
	todo := strings.Split(path, "/")
	out := ""
	hops := 0
	seen := map[string]bool{}
	for i := 0; i < len(todo); i++ {
		comp := todo[i]
		switch comp {
		case "", ".":
			continue
		case "..":
			if j := strings.LastIndex(out, "/"); j >= 0 {
				out = out[:j]
			}
			continue
		}
		full := out + "/" + comp
		if !lexical {
			if info, err := g.FS.Lstat(full); err == nil && info.Mode()&fs.ModeSymlink != 0 {
				// GNU keys a loop on the link and the rest of the name from
				// this component on.
				key := linkID(full, info) + "\x00" + strings.Join(todo[i:], "/")
				link, err := g.FS.Readlink(full)
				if !seen[key] && err == nil {
					seen[key] = true
					if hops++; hops > maxSymlinkHops {
						return "", errSymlinkLoop
					}
					if strings.HasPrefix(link, "/") {
						out = ""
					}
					todo = append(strings.Split(link, "/"), todo[i+1:]...)
					i = -1
					continue
				}
			}
		}
		out = full
	}
	if out == "" {
		return "/", nil
	}
	return out, nil
}

// linkID names a symlink by device and inode when the file system gives
// them, else by its path.
func linkID(path string, info fs.FileInfo) string {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return strconv.FormatUint(uint64(st.Dev), 10) + ":" + strconv.FormatUint(st.Ino, 10)
	}
	return path
}

func (g *Guard) existsOrSymlink(name string) bool {
	_, err := g.FS.Lstat(name)
	return err == nil
}
