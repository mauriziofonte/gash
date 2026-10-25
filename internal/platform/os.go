// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package platform

import (
	"bufio"
	"bytes"
	"slices"
	"strings"

	"github.com/mauriziofonte/gash/internal/lazyre"
)

// Family groups distributions that share package layouts.
type Family string

// The families gash knows. Unknown covers every other system.
const (
	FamilyDebian  Family = "debian" // Debian, Ubuntu and derivatives
	FamilyFedora  Family = "fedora" // Fedora, RHEL and derivatives
	FamilyArch    Family = "arch"   // Arch Linux and derivatives
	FamilyDarwin  Family = "darwin" // macOS
	FamilyUnknown Family = "unknown"
)

// OS describes the running system as read from the sysroot.
type OS struct {
	GOOS      string   // runtime.GOOS of the binary: "linux", "darwin"
	ID        string   // os-release ID ("ubuntu"), "macos" on darwin
	IDLike    []string // os-release ID_LIKE
	VersionID string   // os-release VERSION_ID, macOS ProductVersion
	Name      string   // os-release PRETTY_NAME, "macOS <version>" on darwin
	Family    Family
	WSL       bool // Linux under the Windows Subsystem for Linux
}

// Platform is the short platform name of receipts and reports:
// "<id>-<version>" ("ubuntu-24.04", "macos-14.5"), or the ID alone when
// there is no version (Arch is rolling).
func (o OS) Platform() string {
	id := o.ID
	if id == "" {
		id = o.GOOS
	}
	if o.VersionID == "" {
		return id
	}
	return id + "-" + o.VersionID
}

// DetectOS reads the distribution from the sysroot: /etc/os-release, else
// /usr/lib/os-release on Linux, SystemVersion.plist on darwin. It never
// fails: an unreadable or unknown system has FamilyUnknown.
func DetectOS(sr Sysroot, goos string) OS {
	o := OS{GOOS: goos, Family: FamilyUnknown}
	if goos == "darwin" {
		o.ID, o.Family = "macos", FamilyDarwin
		if b, err := sr.ReadFile("/System/Library/CoreServices/SystemVersion.plist"); err == nil {
			if m := plistVersion.FindSubmatch(b); m != nil {
				o.VersionID = string(m[1])
			}
		}
		o.Name = strings.TrimSpace("macOS " + o.VersionID)
		return o
	}
	var rel map[string]string
	for _, p := range []string{"/etc/os-release", "/usr/lib/os-release"} {
		if b, err := sr.ReadFile(p); err == nil {
			rel = ParseOSRelease(b)
			break
		}
	}
	o.ID = rel["ID"]
	o.IDLike = strings.Fields(rel["ID_LIKE"])
	o.VersionID = rel["VERSION_ID"]
	o.Name = rel["PRETTY_NAME"]
	o.Family = familyOf(o.ID, o.IDLike)
	o.WSL = detectWSL(sr)
	return o
}

var plistVersion = lazyre.MustCompile(`<key>ProductVersion</key>\s*<string>([0-9][0-9.]*)</string>`)

func familyOf(id string, like []string) Family {
	ids := append([]string{id}, like...)
	switch {
	case slices.ContainsFunc(ids, func(s string) bool { return s == "debian" || s == "ubuntu" }):
		return FamilyDebian
	case slices.ContainsFunc(ids, func(s string) bool { return s == "fedora" || s == "rhel" || s == "centos" }):
		return FamilyFedora
	case slices.Contains(ids, "arch"):
		return FamilyArch
	}
	return FamilyUnknown
}

func detectWSL(sr Sysroot) bool {
	if sr.Exists("/proc/sys/fs/binfmt_misc/WSLInterop") {
		return true
	}
	b, err := sr.ReadFile("/proc/version")
	return err == nil && bytes.Contains(bytes.ToLower(b), []byte("microsoft"))
}

// ParseOSRelease parses os-release(5): KEY=value lines, values optionally
// in double or single quotes; in double quotes a backslash escapes $, ", \
// and `. Comments, blank lines and malformed lines are skipped.
func ParseOSRelease(b []byte) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok || !osReleaseKey.MatchString(key) {
			continue
		}
		v, ok := unquoteOSRelease(val)
		if !ok {
			continue
		}
		out[key] = v
	}
	return out
}

var osReleaseKey = lazyre.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func unquoteOSRelease(v string) (string, bool) {
	if v == "" {
		return "", true
	}
	switch q := v[0]; q {
	case '\'':
		if len(v) < 2 || v[len(v)-1] != '\'' {
			return "", false
		}
		return v[1 : len(v)-1], true
	case '"':
		if len(v) < 2 || v[len(v)-1] != '"' {
			return "", false
		}
		inner := v[1 : len(v)-1]
		var sb strings.Builder
		for i := 0; i < len(inner); i++ {
			c := inner[i]
			if c == '\\' && i+1 < len(inner) && strings.IndexByte("$\"\\`", inner[i+1]) >= 0 {
				i++
				c = inner[i]
			}
			sb.WriteByte(c)
		}
		return sb.String(), true
	}
	return v, true
}
