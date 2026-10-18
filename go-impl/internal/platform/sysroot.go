// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package platform detects the operating system and distribution, resolves
// the XDG base directories from an environment snapshot, and maps every
// absolute system path through a Sysroot, so that code which reads system
// files runs unchanged against a fixture tree in tests.
package platform

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// maxSymlinkHops bounds symlink resolution inside a sysroot (Linux uses 40).
const maxSymlinkHops = 40

// ErrRelative is returned when a system path is not absolute.
var ErrRelative = errors.New("system path is not absolute")

// Sysroot maps absolute system paths ("/etc/hosts") to host paths. The zero
// value is the real root, where both are the same. A fixture sysroot maps
// "/etc/hosts" to "<root>/etc/hosts" and resolves symlinks inside the tree:
// an absolute link target is taken relative to the sysroot, never to the
// host, so a fixture can never make code read a host file.
type Sysroot struct {
	root string // "" for the real root, else an absolute cleaned directory
}

// Real returns the identity sysroot.
func Real() Sysroot { return Sysroot{} }

// NewSysroot returns a sysroot rooted at dir. "" and "/" give the real root.
func NewSysroot(dir string) (Sysroot, error) {
	if dir == "" || dir == "/" {
		return Sysroot{}, nil
	}
	if !filepath.IsAbs(dir) {
		return Sysroot{}, fmt.Errorf("sysroot %q: %w", dir, ErrRelative)
	}
	return Sysroot{root: filepath.Clean(dir)}, nil
}

// IsReal reports whether s is the identity mapping.
func (s Sysroot) IsReal() bool { return s.root == "" }

// Root returns the host directory of the sysroot ("/" for the real root).
func (s Sysroot) Root() string {
	if s.root == "" {
		return "/"
	}
	return s.root
}

// Path maps an absolute system path to its host path without resolving
// symlinks. It panics on a relative path: callers always join with a known
// absolute directory, and a relative path here is a programming error.
func (s Sysroot) Path(sys string) string {
	if !filepath.IsAbs(sys) {
		panic(fmt.Sprintf("platform: Sysroot.Path(%q): %v", sys, ErrRelative))
	}
	sys = filepath.Clean(sys)
	if s.root == "" {
		return sys
	}
	if sys == "/" {
		return s.root
	}
	return s.root + sys
}

// SystemPath is the inverse of Path: it maps a host path inside the sysroot
// back to the system path. ok is false when host is outside the sysroot.
func (s Sysroot) SystemPath(host string) (sys string, ok bool) {
	host = filepath.Clean(host)
	if s.root == "" {
		return host, filepath.IsAbs(host)
	}
	if host == s.root {
		return "/", true
	}
	if rest, found := strings.CutPrefix(host, s.root+"/"); found {
		return "/" + rest, true
	}
	return "", false
}

// Resolve returns the system path of sys with every symlink followed inside
// the sysroot (POSIX resolution, component by component). A missing
// component ends the resolution: the remaining components are appended
// unresolved and the error is nil, as for a path that does not exist yet.
func (s Sysroot) Resolve(sys string) (string, error) {
	return s.resolve(sys)
}

// resolveParent resolves every component except the last one, so that Lstat
// and Readlink act on the final symlink itself.
func (s Sysroot) resolveParent(sys string) (string, error) {
	if !filepath.IsAbs(sys) {
		return "", fmt.Errorf("%q: %w", sys, ErrRelative)
	}
	sys = filepath.Clean(sys)
	if sys == "/" {
		return "/", nil
	}
	dir, err := s.resolve(filepath.Dir(sys))
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filepath.Base(sys)), nil
}

func (s Sysroot) resolve(sys string) (string, error) {
	if !filepath.IsAbs(sys) {
		return "", fmt.Errorf("%q: %w", sys, ErrRelative)
	}
	pending := splitPath(sys)
	resolved := "/"
	hops := 0
	for len(pending) > 0 {
		comp := pending[0]
		pending = pending[1:]
		switch comp {
		case ".":
			continue
		case "..":
			resolved = filepath.Dir(resolved)
			continue
		}
		next := filepath.Join(resolved, comp)
		fi, err := os.Lstat(s.Path(next))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.ENOTDIR) {
				return filepath.Join(append([]string{next}, pending...)...), nil
			}
			return "", err
		}
		if fi.Mode()&fs.ModeSymlink == 0 {
			resolved = next
			continue
		}
		hops++
		if hops > maxSymlinkHops {
			return "", &fs.PathError{Op: "resolve", Path: sys, Err: errors.New("too many levels of symbolic links")}
		}
		target, err := os.Readlink(s.Path(next))
		if err != nil {
			return "", err
		}
		pending = append(splitPath(target), pending...)
		if filepath.IsAbs(target) {
			resolved = "/"
		}
	}
	return resolved, nil
}

func splitPath(p string) []string {
	var out []string
	for _, c := range strings.Split(p, "/") {
		if c != "" {
			out = append(out, c)
		}
	}
	return out
}

// host resolves sys and maps it to the host path.
func (s Sysroot) host(sys string) (string, error) {
	r, err := s.Resolve(sys)
	if err != nil {
		return "", err
	}
	return s.Path(r), nil
}

// ReadFile reads a system file, following symlinks inside the sysroot.
func (s Sysroot) ReadFile(sys string) ([]byte, error) {
	h, err := s.host(sys)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(h)
}

// Stat returns the file info of sys, following symlinks inside the sysroot.
func (s Sysroot) Stat(sys string) (fs.FileInfo, error) {
	h, err := s.host(sys)
	if err != nil {
		return nil, err
	}
	return os.Lstat(h)
}

// Lstat returns the file info of sys without following a final symlink.
func (s Sysroot) Lstat(sys string) (fs.FileInfo, error) {
	p, err := s.resolveParent(sys)
	if err != nil {
		return nil, err
	}
	return os.Lstat(s.Path(p))
}

// Readlink returns the target of the symlink sys, as written in the link.
func (s Sysroot) Readlink(sys string) (string, error) {
	p, err := s.resolveParent(sys)
	if err != nil {
		return "", err
	}
	return os.Readlink(s.Path(p))
}

// ReadDir lists a system directory, following symlinks inside the sysroot.
func (s Sysroot) ReadDir(sys string) ([]fs.DirEntry, error) {
	h, err := s.host(sys)
	if err != nil {
		return nil, err
	}
	return os.ReadDir(h)
}

// Exists reports whether sys exists (after following symlinks).
func (s Sysroot) Exists(sys string) bool {
	_, err := s.Stat(sys)
	return err == nil
}

// IsRegular reports whether sys is a regular file (after following symlinks).
func (s Sysroot) IsRegular(sys string) bool {
	fi, err := s.Stat(sys)
	return err == nil && fi.Mode().IsRegular()
}

// IsDir reports whether sys is a directory (after following symlinks).
func (s Sysroot) IsDir(sys string) bool {
	fi, err := s.Stat(sys)
	return err == nil && fi.IsDir()
}

// WalkFunc is called by Walk with system paths.
type WalkFunc func(sys string, d fs.DirEntry, err error) error

// Walk walks the system directory tree at sys in lexical order, as
// filepath.WalkDir does (symlinks are reported, not followed), calling fn
// with system paths. The root itself is resolved first.
func (s Sysroot) Walk(sys string, fn WalkFunc) error {
	r, err := s.Resolve(sys)
	if err != nil {
		return err
	}
	return filepath.WalkDir(s.Path(r), func(h string, d fs.DirEntry, err error) error {
		p, ok := s.SystemPath(h)
		if !ok {
			return fmt.Errorf("platform: walk left the sysroot at %q", h)
		}
		// Report paths under the name the caller used, also when sys
		// itself was a symlink.
		rel, _ := filepath.Rel(r, p)
		return fn(filepath.Join(sys, rel), d, err)
	})
}
