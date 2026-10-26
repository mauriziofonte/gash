// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package wsl

import (
	"errors"
	"path"
	"strings"
)

// Path forms of ToWindows.
const (
	// FormWindows is C:\Users\me, \\wsl.localhost\Ubuntu\home\me.
	FormWindows = "windows"
	// FormMixed is C:/Users/me, //wsl.localhost/Ubuntu/home/me: Windows
	// paths with forward slashes, for tools that take both.
	FormMixed = "mixed"
)

// ErrNoDistro is a Linux path outside the Windows drives when the
// distribution name is unknown (WSL_DISTRO_NAME unset): its UNC form needs
// it.
var ErrNoDistro = errors.New("the distribution name is unknown (WSL_DISTRO_NAME is not set)")

// ErrNotMapped is a Windows path with no Linux form: a network share, a
// drive that is not mounted, another distribution.
var ErrNotMapped = errors.New("the path has no Linux form here")

// UNCHost is the host of the UNC paths of the distributions (Windows 11
// and WSL from the Store; \\wsl$ is the older alias, still accepted).
const UNCHost = "wsl.localhost"

// ToWindows translates the absolute Linux path p (cleaned, symlinks kept)
// to its Windows form: a path on a drive mount becomes X:\..., any other
// path the UNC path of the distribution.
func (f Facts) ToWindows(p, form string) (string, error) {
	if !strings.HasPrefix(p, "/") {
		return "", errors.New("not an absolute path: " + p)
	}
	p = path.Clean(p)
	sep := `\`
	if form == FormMixed {
		sep = "/"
	}
	if d, m, ok := f.mountOf(p); ok {
		out := d.Letter + ":" + strings.ReplaceAll(m.Dir, `\`, sep)
		if rest := strings.TrimPrefix(strings.TrimPrefix(p, m.Path), "/"); rest != "" {
			if !strings.HasSuffix(out, sep) {
				out += sep
			}
			out += strings.ReplaceAll(rest, "/", sep)
		}
		return out, nil
	}
	if f.Distro == "" {
		return "", ErrNoDistro
	}
	out := sep + sep + UNCHost + sep + f.Distro
	if p != "/" {
		out += strings.ReplaceAll(p, "/", sep)
	} else {
		out += sep
	}
	return out, nil
}

// ToLinux translates a Windows path to the Linux one: X:\... to the mount
// of drive X that shows it (the preferred mount of the whole drive, or a
// mount of a directory that holds the path; the automount root plus the
// lower-case letter when none does, as wslpath does), \\wsl.localhost\<this
// distro>\... (or \\wsl$\...) to /..., and a relative path to the same path
// with forward slashes. A share, or another distribution, is ErrNotMapped.
func (f Facts) ToLinux(w string) (string, error) {
	s := strings.ReplaceAll(w, `\`, "/")
	switch {
	case isLetterColon(s) && (len(s) == 2 || s[2] == '/'):
		letter := strings.ToUpper(s[:1])
		mount := strings.TrimSuffix(f.AutomountRoot, "/") + "/" + strings.ToLower(letter)
		rest := strings.Trim(path.Clean("/"+strings.TrimLeft(s[2:], "/")), "/")
		if d, ok := f.Drive(letter); ok {
			mount, rest = d.linuxOf(mount, rest)
		}
		if rest == "" {
			return mount, nil
		}
		return path.Clean(mount + "/" + rest), nil
	case strings.HasPrefix(s, "//"):
		host, rest, _ := strings.Cut(s[2:], "/")
		if !strings.EqualFold(host, UNCHost) && !strings.EqualFold(host, "wsl$") {
			return "", ErrNotMapped
		}
		distro, rest, _ := strings.Cut(rest, "/")
		if f.Distro == "" || !strings.EqualFold(distro, f.Distro) {
			return "", ErrNotMapped
		}
		return path.Clean("/" + rest), nil
	case len(s) > 0 && s[0] == '/':
		// A rooted path without a drive is relative to the current drive in
		// Windows; it has no single Linux form.
		return "", ErrNotMapped
	}
	return s, nil
}

// linuxOf is the mount and the path below it of rest, a path of the
// drive ("Users/me", "" for its root): the preferred mount of the whole
// drive, else the mount of the deepest directory that holds rest, else
// fallback (the automount root's) with rest as it is.
func (d Drive) linuxOf(fallback, rest string) (string, string) {
	best, mount, below := -1, fallback, rest
	for _, m := range d.mounts() {
		if m.whole() {
			return m.Path, rest
		}
		dir := strings.Trim(strings.ReplaceAll(m.Dir, `\`, "/"), "/")
		under := strings.EqualFold(rest, dir) ||
			len(rest) > len(dir) && rest[len(dir)] == '/' && strings.EqualFold(rest[:len(dir)], dir)
		if under && len(dir) > best {
			best, mount, below = len(dir), m.Path, strings.TrimLeft(rest[len(dir):], "/")
		}
	}
	return mount, below
}

// IsWindowsPath reports a string that reads as a Windows path: a drive
// letter, a UNC prefix or a backslash.
func IsWindowsPath(s string) bool {
	return isLetterColon(s) || strings.HasPrefix(s, `\\`) || strings.Contains(s, `\`)
}
