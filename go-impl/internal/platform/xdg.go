// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package platform

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// Dirs are the XDG base directories of one user.
//
// They are resolved from an environment snapshot with the rules of the XDG
// Base Directory specification (a variable is used when it holds an
// absolute path, else the default under $HOME), on every platform: macOS
// gets ~/.config, ~/.local/state and ~/.local/share too, not ~/Library
// (design/hte.md section 2). github.com/adrg/xdg is not called here: its
// macOS defaults are ~/Library/Application Support and it reads the process
// environment into package globals; the Linux results are checked against
// it in the tests.
type Dirs struct {
	Home       string
	ConfigHome string // $XDG_CONFIG_HOME, else $HOME/.config
	StateHome  string // $XDG_STATE_HOME, else $HOME/.local/state
	DataHome   string // $XDG_DATA_HOME, else $HOME/.local/share
	CacheHome  string // $XDG_CACHE_HOME, else $HOME/.cache
	RuntimeDir string // $XDG_RUNTIME_DIR, else "" (the spec has no default)
}

// ErrNoHome is returned when HOME is unset, empty or relative.
var ErrNoHome = errors.New("HOME is not set to an absolute path")

// ResolveDirs computes the XDG base directories from getenv.
func ResolveDirs(getenv func(string) string) (Dirs, error) {
	home := getenv("HOME")
	if home == "" || !filepath.IsAbs(home) {
		return Dirs{}, ErrNoHome
	}
	home = filepath.Clean(home)
	pick := func(name, def string) string {
		if v := getenv(name); v != "" && filepath.IsAbs(v) {
			return filepath.Clean(v)
		}
		return def
	}
	return Dirs{
		Home:       home,
		ConfigHome: pick("XDG_CONFIG_HOME", filepath.Join(home, ".config")),
		StateHome:  pick("XDG_STATE_HOME", filepath.Join(home, ".local", "state")),
		DataHome:   pick("XDG_DATA_HOME", filepath.Join(home, ".local", "share")),
		CacheHome:  pick("XDG_CACHE_HOME", filepath.Join(home, ".cache")),
		RuntimeDir: pick("XDG_RUNTIME_DIR", ""),
	}, nil
}

// RuntimeBase returns the directory under which a program keeps its runtime
// files: $XDG_RUNTIME_DIR/<name>, else /tmp/<name>-<uid>.
func (d Dirs) RuntimeBase(name string, uid int) string {
	if d.RuntimeDir != "" {
		return filepath.Join(d.RuntimeDir, name)
	}
	return filepath.Join("/tmp", name+"-"+strconv.Itoa(uid))
}

// ErrUnsafeDir is returned by EnsurePrivateDir for a path that is a symlink,
// not a directory, owned by another user or accessible to others.
var ErrUnsafeDir = errors.New("unsafe private directory")

// EnsurePrivateDir creates dir with mode 0700 when it does not exist, then
// checks that it is a real directory (not a symlink), owned by uid, with no
// group or other permission bits. The parent must exist.
func EnsurePrivateDir(dir string, uid int) error {
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		return fmt.Errorf("%s is a symlink: %w", dir, ErrUnsafeDir)
	case !fi.IsDir():
		return fmt.Errorf("%s is not a directory: %w", dir, ErrUnsafeDir)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != uid {
		return fmt.Errorf("%s is owned by uid %d, not %d: %w", dir, st.Uid, uid, ErrUnsafeDir)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s has mode %04o, want 0700: %w", dir, fi.Mode().Perm(), ErrUnsafeDir)
	}
	return nil
}
