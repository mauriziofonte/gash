// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package discover finds the system binaries the userland stack reuses
// (design/hte.md sections 1 and 4): the platform's Apache, its module
// directory and built-in modules, and the installed PHP-FPM versions. Every
// path is a system path read through a platform.Sysroot, so tests run on
// the fixture trees under testdata/hte/sysroot. Running a binary (httpd -l,
// php-fpm -v) goes through an injected Probe; this package never starts a
// process itself.
package discover

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/render"
	"github.com/mauriziofonte/gash/internal/platform"
)

// Probe runs the system binary bin with fixed arguments and returns its
// standard output. The real probe runs it with an empty environment and
// a timeout; tests pass a fake keyed by bin.
type Probe func(bin string, args ...string) (string, error)

// HomebrewPrefixes are probed in order: Apple silicon, then Intel.
var HomebrewPrefixes = []string{"/opt/homebrew", "/usr/local"}

// Apache are the facts of the platform's Apache.
type Apache struct {
	Binary    string // system path: /usr/sbin/apache2, /usr/sbin/httpd, ...
	ModuleDir string // system path of the module directory
	MimeTypes string // the platform's mime.types
	Prefix    string // Homebrew prefix on macOS, else ""
}

type apacheLayout struct {
	binary, moduleDir, mimeTypes string
}

var apacheLayouts = map[platform.Family]apacheLayout{
	platform.FamilyDebian: {"/usr/sbin/apache2", "/usr/lib/apache2/modules", "/etc/mime.types"},
	platform.FamilyFedora: {"/usr/sbin/httpd", "/usr/lib64/httpd/modules", "/etc/mime.types"},
	platform.FamilyArch:   {"/usr/bin/httpd", "/usr/lib/httpd/modules", "/etc/httpd/conf/mime.types"},
	// Darwin: relative to the Homebrew prefix.
	platform.FamilyDarwin: {"bin/httpd", "lib/httpd/modules", "etc/httpd/mime.types"},
}

// ApacheInstallHint is the command that installs the platform's Apache
// with the modules the userland list needs.
func ApacheInstallHint(f platform.Family) string {
	switch f {
	case platform.FamilyDebian:
		return "sudo apt install apache2"
	case platform.FamilyFedora:
		return "sudo dnf install httpd mod_ssl mod_http2"
	case platform.FamilyArch:
		return "sudo pacman -S apache"
	case platform.FamilyDarwin:
		return "brew install httpd"
	}
	return ""
}

// HomebrewPrefix returns the first Homebrew prefix holding bin/brew.
func HomebrewPrefix(sr platform.Sysroot) (string, bool) {
	for _, p := range HomebrewPrefixes {
		if isExecutable(sr, path.Join(p, "bin/brew")) {
			return p, true
		}
	}
	return "", false
}

func isExecutable(sr platform.Sysroot, sys string) bool {
	fi, err := sr.Stat(sys)
	return err == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&0o111 != 0
}

// FindApache locates the platform's Apache binary, module directory and
// mime.types. An unknown platform is platform_unsupported; a missing piece
// is apache_not_found with the install command as hint.
func FindApache(sr platform.Sysroot, o platform.OS) (Apache, error) {
	l, ok := apacheLayouts[o.Family]
	if !ok {
		return Apache{}, hteerr.Newf(hteerr.PlatformUnsupported,
			"gash hte supports Debian, Ubuntu, Fedora, Arch and macOS with Homebrew",
			"%s is not a supported platform", o.Platform())
	}
	hint := ApacheInstallHint(o.Family)
	var a Apache
	if o.Family == platform.FamilyDarwin {
		prefix, ok := HomebrewPrefix(sr)
		if !ok {
			return Apache{}, hteerr.Newf(hteerr.ApacheNotFound, "install Homebrew, then: "+hint,
				"no Homebrew installation under %s", strings.Join(HomebrewPrefixes, " or "))
		}
		a = Apache{
			Binary:    path.Join(prefix, l.binary),
			ModuleDir: path.Join(prefix, l.moduleDir),
			MimeTypes: path.Join(prefix, l.mimeTypes),
			Prefix:    prefix,
		}
	} else {
		a = Apache{Binary: l.binary, ModuleDir: l.moduleDir, MimeTypes: l.mimeTypes}
	}
	switch {
	case !isExecutable(sr, a.Binary):
		return Apache{}, hteerr.Newf(hteerr.ApacheNotFound, hint, "no Apache binary at %s", a.Binary)
	case !sr.IsDir(a.ModuleDir):
		return Apache{}, hteerr.Newf(hteerr.ApacheNotFound, hint, "no Apache module directory at %s", a.ModuleDir)
	case !sr.IsRegular(a.MimeTypes):
		return Apache{}, hteerr.Newf(hteerr.ApacheNotFound, hint, "no mime.types at %s", a.MimeTypes)
	}
	return a, nil
}

// mpmSources maps the source file name `httpd -l` prints for a
// statically built MPM to its module name.
var mpmSources = map[string]string{"event.c": "mpm_event", "worker.c": "mpm_worker", "prefork.c": "mpm_prefork"}

// ParseBuiltinModules reads the output of `httpd -l` ("Compiled in
// modules:" then one source file per line) and returns the module names:
// mod_log_config.c is log_config, event.c is mpm_event; core.c and
// http_core.c are core and http_core. Any other shape is an error.
func ParseBuiltinModules(out string) ([]string, error) {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "Compiled in modules:" {
		return nil, fmt.Errorf("discover: httpd -l output does not start with \"Compiled in modules:\"")
	}
	var mods []string
	for _, l := range lines[1:] {
		src := strings.TrimSpace(l)
		if mpm, ok := mpmSources[src]; ok {
			mods = append(mods, mpm)
			continue
		}
		name, ok := strings.CutSuffix(src, ".c")
		if !ok || name == "" || strings.ContainsAny(name, " /") {
			return nil, fmt.Errorf("discover: httpd -l line %q is not a module source file", l)
		}
		mods = append(mods, strings.TrimPrefix(name, "mod_"))
	}
	return mods, nil
}

// BuiltinModules runs `httpd -l` through probe.
func BuiltinModules(a Apache, probe Probe) ([]string, error) {
	out, err := probe(a.Binary, "-l")
	if err != nil {
		return nil, hteerr.Newf(hteerr.ApacheNotFound, "", "%s -l failed: %v", a.Binary, err)
	}
	mods, err := ParseBuiltinModules(out)
	if err != nil {
		return nil, hteerr.Newf(hteerr.ApacheNotFound, "", "%s: %v", a.Binary, err)
	}
	return mods, nil
}

// ModuleFiles lists the modules of the module directory: the names of
// its mod_<name>.so files, sorted. `apache modules enable NAME` accepts
// only these.
func ModuleFiles(sr platform.Sysroot, a Apache) ([]string, error) {
	ents, err := sr.ReadDir(a.ModuleDir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		name, ok := strings.CutPrefix(e.Name(), "mod_")
		if !ok {
			continue
		}
		name, ok = strings.CutSuffix(name, ".so")
		if !ok || name == "" {
			continue
		}
		if sr.IsRegular(path.Join(a.ModuleDir, e.Name())) {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out, nil
}

// Modules is the resolved LoadModule list of httpd.conf.
type Modules struct {
	Load     []render.Module // modules to load, in render.BaseModules order then the extra modules
	Builtin  []string        // wanted modules built into the binary (no LoadModule)
	Missing  []string        // wanted modules with no file in the module directory
	Warnings []string
}

// ResolveModules turns the userland list plus the extra modules of
// hte.toml into LoadModule lines: built-in modules are left out, each
// other one must exist as <module dir>/mod_<name>.so. A statically built
// MPM replaces mpm_event (two MPMs cannot be loaded); one other than event
// is a warning, since HTTP/2 needs a threaded MPM.
func ResolveModules(sr platform.Sysroot, a Apache, builtin, extra []string) Modules {
	var m Modules
	staticMPM := ""
	for _, b := range builtin {
		if strings.HasPrefix(b, "mpm_") {
			staticMPM = b
		}
	}
	if staticMPM != "" && staticMPM != "mpm_event" {
		m.Warnings = append(m.Warnings, fmt.Sprintf("%s is built with %s; HTTP/2 needs mpm_event", a.Binary, staticMPM))
	}
	seen := map[string]bool{}
	for _, name := range append(slices.Clone(render.BaseModules), extra...) {
		if seen[name] {
			continue
		}
		seen[name] = true
		switch {
		case slices.Contains(builtin, name):
			m.Builtin = append(m.Builtin, name)
			continue
		case name == "mpm_event" && staticMPM != "":
			continue
		}
		file := path.Join(a.ModuleDir, "mod_"+name+".so")
		if !sr.IsRegular(file) {
			m.Missing = append(m.Missing, name)
			continue
		}
		// The path is kept as found, not resolved: Homebrew's
		// lib/httpd/modules symlink follows a brew upgrade, a Cellar path
		// would not.
		m.Load = append(m.Load, render.Module{Name: name, Path: file})
	}
	return m
}

// MissingError is apache_module_missing (STOP) for modules the userland
// list needs and the platform lacks, or nil when none is missing. The hint
// names the platform's install command for them.
func (m Modules) MissingError(a Apache, f platform.Family) error {
	if len(m.Missing) == 0 {
		return nil
	}
	return hteerr.Newf(hteerr.ApacheModuleMissing, ModuleInstallHint(f, m.Missing),
		"%s lacks the modules %s", a.ModuleDir, strings.Join(m.Missing, ", "))
}

// ModuleInstallHint is the command that installs the Apache modules
// missing on the platform. Debian, Arch and Homebrew ship every module of
// the userland list in the server package; Fedora splits mod_ssl and
// mod_http2 out of httpd.
func ModuleInstallHint(f platform.Family, missing []string) string {
	switch f {
	case platform.FamilyDebian:
		return "sudo apt install --reinstall apache2"
	case platform.FamilyFedora:
		pkgs := []string{}
		for _, m := range missing {
			p := "httpd"
			if m == "ssl" || m == "http2" {
				p = "mod_" + m
			}
			if !slices.Contains(pkgs, p) {
				pkgs = append(pkgs, p)
			}
		}
		return "sudo dnf install " + strings.Join(pkgs, " ")
	case platform.FamilyArch:
		return "sudo pacman -S apache"
	case platform.FamilyDarwin:
		return "brew reinstall httpd"
	}
	return ""
}
