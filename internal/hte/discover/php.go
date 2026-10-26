// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package discover

import (
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"syscall"

	"github.com/mauriziofonte/gash/internal/hte/state"
	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/platform"
)

// Layout names where a PHP-FPM binary was found.
type Layout string

// The probed layouts of design/hte.md section 4.
const (
	LayoutDebian     Layout = "debian"      // /usr/sbin/php-fpmX.Y + /etc/php/X.Y/fpm/php.ini (distro, sury/ondrej)
	LayoutFedora     Layout = "fedora"      // /usr/sbin/php-fpm + /etc/php.ini
	LayoutRemi       Layout = "remi"        // /opt/remi/phpXY/root/usr/sbin/php-fpm + /etc/opt/remi/phpXY/php.ini
	LayoutArch       Layout = "arch"        // /usr/bin/php-fpm + /etc/php/php.ini
	LayoutArchLegacy Layout = "arch-legacy" // /usr/bin/php-fpm-legacy + /etc/php-legacy/php.ini
	LayoutAUR        Layout = "aur"         // /usr/bin/php-fpmXY + /etc/phpXY/php.ini (inferred)
	LayoutHomebrew   Layout = "homebrew"    // <prefix>/opt/php@X.Y/sbin/php-fpm or <prefix>/opt/php/sbin/php-fpm + <prefix>/etc/php/X.Y/php.ini
)

// PHP is one usable PHP-FPM version.
type PHP struct {
	Version string `json:"version"` // "8.3"
	Full    string `json:"full"`    // "8.3.6", as -v reports it
	FPM     string `json:"fpm"`     // system path of the FPM binary
	INI     string `json:"ini"`     // system path of its php.ini
	Layout  Layout `json:"layout"`
	// SessionPath is the effective session.save_path of php.ini ("" means
	// the system temporary directory).
	SessionPath string `json:"session_path"`
	// NeedsSessionDir is true when SessionPath is set and not writable by
	// the user: the pools then get a per-version directory (LEDGER HTE-41).
	NeedsSessionDir bool `json:"needs_session_dir"`
	// ScanDir is the system directory of additional .ini files the binary
	// was built with ("" for none): the userland master gets it back in
	// PHP_INI_SCAN_DIR, ahead of the global overlay directory.
	ScanDir string `json:"scan_dir"`
}

// PHPEnv is what PHP discovery reads.
type PHPEnv struct {
	Sysroot platform.Sysroot
	OS      platform.OS
	Probe   Probe
	Cache   *Cache // optional: probe results by binary path, size and mtime
	// Writable reports whether the user can write the system directory
	// sys; nil means access(2) on the sysroot path.
	Writable func(sys string) bool
}

// PHPResult lists the usable versions, ascending, and every candidate
// binary that did not qualify, with the reason (doctor prints them).
type PHPResult struct {
	Versions []PHP    `json:"versions"`
	Problems []string `json:"problems"`
}

// Installed returns the version numbers, ascending.
func (r PHPResult) Installed() []string {
	out := make([]string, 0, len(r.Versions))
	for _, p := range r.Versions {
		out = append(out, p.Version)
	}
	return out
}

// Find returns the entry of version v.
func (r PHPResult) Find(v string) (PHP, bool) {
	for _, p := range r.Versions {
		if p.Version == v {
			return p, true
		}
	}
	return PHP{}, false
}

// FPMBinaries are the PHP-FPM binaries DiscoverPHP would probe here, in
// its order: the installed ones of the platform's layouts, without running
// them.
func FPMBinaries(sr platform.Sysroot, o platform.OS) []string {
	var out []string
	for _, c := range candidates(sr, o) {
		if isExecutable(sr, c.fpm) {
			out = append(out, c.fpm)
		}
	}
	return out
}

type candidate struct {
	layout  Layout
	fpm     string
	ini     string // versionToken stands for the version -v reports
	version string // "" for an unversioned binary
	scan    string // the layout's scan directory, used when -i does not name one
}

const versionToken = "{version}"

func nodot(v string) string { return strings.ReplaceAll(v, ".", "") }

func candidates(sr platform.Sysroot, o platform.OS) []candidate {
	var out []candidate
	switch o.Family {
	case platform.FamilyDebian:
		for _, v := range state.SupportedPHP {
			out = append(out, candidate{LayoutDebian, "/usr/sbin/php-fpm" + v, "/etc/php/" + v + "/fpm/php.ini", v, "/etc/php/" + v + "/fpm/conf.d"})
		}
	case platform.FamilyFedora:
		out = append(out, candidate{LayoutFedora, "/usr/sbin/php-fpm", "/etc/php.ini", "", "/etc/php.d"})
		for _, v := range state.SupportedPHP {
			xy := nodot(v)
			out = append(out, candidate{LayoutRemi, "/opt/remi/php" + xy + "/root/usr/sbin/php-fpm", "/etc/opt/remi/php" + xy + "/php.ini", v,
				"/etc/opt/remi/php" + xy + "/php.d"})
		}
	case platform.FamilyArch:
		out = append(out,
			candidate{LayoutArch, "/usr/bin/php-fpm", "/etc/php/php.ini", "", "/etc/php/conf.d"},
			candidate{LayoutArchLegacy, "/usr/bin/php-fpm-legacy", "/etc/php-legacy/php.ini", "", "/etc/php-legacy/conf.d"})
		for _, v := range state.SupportedPHP {
			xy := nodot(v)
			out = append(out, candidate{LayoutAUR, "/usr/bin/php-fpm" + xy, "/etc/php" + xy + "/php.ini", v, "/etc/php" + xy + "/conf.d"})
		}
	case platform.FamilyDarwin:
		prefix, ok := HomebrewPrefix(sr)
		if !ok {
			return nil
		}
		out = append(out, candidate{LayoutHomebrew, prefix + "/opt/php/sbin/php-fpm", prefix + "/etc/php/" + versionToken + "/php.ini", "",
			prefix + "/etc/php/" + versionToken + "/conf.d"})
		for _, v := range state.SupportedPHP {
			out = append(out, candidate{LayoutHomebrew, prefix + "/opt/php@" + v + "/sbin/php-fpm", prefix + "/etc/php/" + v + "/php.ini", v,
				prefix + "/etc/php/" + v + "/conf.d"})
		}
	}
	return out
}

// fpmVersionRE matches the first line of `php-fpm -v`:
// "PHP 8.3.6 (fpm-fcgi) (built: ...)". The SAPI must be fpm-fcgi: a CLI
// binary under an FPM name does not count.
var fpmVersionRE = lazyre.MustCompile(`^PHP ([0-9]+)\.([0-9]+)\.([0-9]+)[^ ]* \(fpm-fcgi\)`)

// ParseFPMVersion returns "X.Y" and the full version from `php-fpm -v`.
func ParseFPMVersion(out string) (short, full string, ok bool) {
	first, _, _ := strings.Cut(out, "\n")
	m := fpmVersionRE.FindStringSubmatch(first)
	if m == nil {
		return "", "", false
	}
	return m[1] + "." + m[2], m[1] + "." + m[2] + "." + m[3], true
}

// sessionDefaults are the compiled-in session.save_path of each layout
// when php.ini does not set it. Debian builds PHP with
// /var/lib/php/sessions (1733, writable); the design states that Fedora's
// default is /var/lib/php/session, group apache.
var sessionDefaults = map[Layout]string{
	LayoutDebian: "/var/lib/php/sessions",
	LayoutFedora: "/var/lib/php/session",
	LayoutRemi:   "/var/lib/php/session",
}

// iniSessionPath returns the last uncommented session.save_path of a
// php.ini, with the "N;MODE;" prefix of the files handler removed.
func iniSessionPath(ini []byte) (string, bool) {
	val, found := "", false
	for _, line := range strings.Split(string(ini), "\n") {
		line = strings.TrimSpace(line)
		key, v, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "session.save_path" {
			continue
		}
		v = strings.TrimSpace(v)
		if rest, ok := strings.CutPrefix(v, `"`); ok {
			v, _, _ = strings.Cut(rest, `"`)
		} else {
			v, _, _ = strings.Cut(v, ";")
			v = strings.TrimSpace(v)
		}
		if i := strings.LastIndex(v, ";"); i >= 0 {
			v = v[i+1:]
		}
		val, found = v, true
	}
	return val, found
}

func accessWritable(sr platform.Sysroot) func(string) bool {
	return func(sys string) bool {
		const wOK = 0x2
		return syscall.Access(sr.Path(sys), wOK) == nil
	}
}

// DiscoverPHP probes every layout of the platform. A version counts only
// when its FPM binary is an executable file, `-v` answers that version with
// the fpm-fcgi SAPI, and its php.ini exists (LEDGER HTE-22). When two
// layouts give the same version (Fedora distro and Remi), the first one in
// probe order is used and the other is reported, unless both names lead
// to the same file.
func DiscoverPHP(env PHPEnv) PHPResult {
	sr := env.Sysroot
	writable := env.Writable
	if writable == nil {
		writable = accessWritable(sr)
	}
	var res PHPResult
	seen := map[string]string{}
	var used []fs.FileInfo // the FPM binaries of res.Versions
	for _, c := range candidates(sr, env.OS) {
		fi, err := sr.Stat(c.fpm)
		if err != nil {
			continue // not installed: not a problem
		}
		if !fi.Mode().IsRegular() || fi.Mode().Perm()&0o111 == 0 {
			res.Problems = append(res.Problems, c.fpm+" is not an executable file")
			continue
		}
		// Two names of one binary (Homebrew links opt/php and
		// opt/php@X.Y to the same keg) are one installation.
		if slices.ContainsFunc(used, func(u fs.FileInfo) bool { return os.SameFile(u, fi) }) {
			continue
		}
		entry, cached := env.Cache.lookup(c.fpm, fi)
		if !cached {
			out, err := env.Probe(c.fpm, "-v")
			if err != nil {
				res.Problems = append(res.Problems, fmt.Sprintf("%s -v failed: %v", c.fpm, err))
				continue
			}
			entry = CacheEntry{Output: firstLine(out)}
		}
		out := entry.Output
		short, full, ok := ParseFPMVersion(out)
		if ok && !entry.Info {
			// -i names the compiled-in scan directory; a binary that
			// cannot answer keeps the layout's default.
			entry.Info = true
			entry.ScanDir = strings.ReplaceAll(c.scan, versionToken, short)
			if info, err := env.Probe(c.fpm, "-i"); err == nil {
				if dir, found := ParseScanDir(info); found {
					entry.ScanDir = dir
				}
			}
			cached = false
		}
		if !cached {
			env.Cache.store(c.fpm, fi, entry)
		}
		switch {
		case !ok:
			res.Problems = append(res.Problems, fmt.Sprintf("%s -v does not report a PHP-FPM version: %q", c.fpm, firstLine(out)))
			continue
		case c.version != "" && short != c.version:
			res.Problems = append(res.Problems, fmt.Sprintf("%s reports PHP %s, not %s", c.fpm, short, c.version))
			continue
		case !slices.Contains(state.SupportedPHP, short):
			res.Problems = append(res.Problems, fmt.Sprintf("%s reports PHP %s, which gash hte does not support", c.fpm, short))
			continue
		}
		ini := strings.ReplaceAll(c.ini, versionToken, short)
		data, err := sr.ReadFile(ini)
		if err != nil || !sr.IsRegular(ini) {
			res.Problems = append(res.Problems, fmt.Sprintf("%s has no php.ini at %s", c.fpm, ini))
			continue
		}
		if prev, dup := seen[short]; dup {
			res.Problems = append(res.Problems, fmt.Sprintf("PHP %s is also at %s; using %s", short, c.fpm, prev))
			continue
		}
		seen[short] = c.fpm
		used = append(used, fi)
		sess, set := iniSessionPath(data)
		if !set {
			sess = sessionDefaults[c.layout]
		}
		res.Versions = append(res.Versions, PHP{
			Version: short, Full: full, FPM: c.fpm, INI: ini, Layout: c.layout,
			SessionPath:     sess,
			NeedsSessionDir: sess != "" && !writable(sess),
			ScanDir:         entry.ScanDir,
		})
	}
	slices.SortFunc(res.Versions, func(a, b PHP) int { return state.ComparePHP(a.Version, b.Version) })
	return res
}

// ParseScanDir returns the "Scan this dir for additional .ini files" value
// of `php-fpm -i`: "" for (none), found false when the line is absent.
func ParseScanDir(info string) (dir string, found bool) {
	for _, line := range strings.Split(info, "\n") {
		rest, ok := strings.CutPrefix(line, "Scan this dir for additional .ini files => ")
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		if rest == "(none)" {
			return "", true
		}
		return rest, true
	}
	return "", false
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}

// PHPInstallHint is the command that installs PHP-FPM version v on the
// platform, the distribution's own version when v is "" (doctor and
// install print it when no version is installed; 3.4: it was unused).
func PHPInstallHint(f platform.Family, v string) string {
	if v == "" {
		switch f {
		case platform.FamilyDebian:
			return "sudo apt install php-fpm"
		case platform.FamilyFedora:
			return "sudo dnf install php-fpm"
		case platform.FamilyArch:
			return "sudo pacman -S php-fpm"
		case platform.FamilyDarwin:
			return "brew install php"
		}
		return ""
	}
	switch f {
	case platform.FamilyDebian:
		return "sudo apt install php" + v + "-fpm"
	case platform.FamilyFedora:
		return "sudo dnf install php-fpm (distro version) or php" + nodot(v) + "-php-fpm (Remi)"
	case platform.FamilyArch:
		return "sudo pacman -S php-fpm (current) or php-legacy-fpm (legacy)"
	case platform.FamilyDarwin:
		return "brew install php@" + v
	}
	return ""
}
