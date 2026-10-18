// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package state

import (
	"regexp"
	"slices"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
)

// Ports are the ports Apache listens on.
type Ports struct {
	HTTP  int `toml:"http"`
	HTTPS int `toml:"https"`
}

// StandardPorts and HighPorts are the two port sets of `install --ports`.
var (
	StandardPorts = Ports{HTTP: 80, HTTPS: 443}
	HighPorts     = Ports{HTTP: 8080, HTTPS: 8443}
)

// Loopback addresses Apache may listen on; never a wildcard.
const (
	LoopbackIPv4 = "127.0.0.1"
	LoopbackIPv6 = "::1"
)

// Autostart modes recorded by install (auto is resolved at install time).
var autostartModes = []string{"systemd", "launchd", "shell", "none"}

// Settings is hte.toml: ports, listen addresses, default PHP, enabled
// extra Apache modules, autostart mode (design/hte.md section 2).
type Settings struct {
	Ports      Ports    `toml:"ports"`
	Listen     []string `toml:"listen"`      // loopback addresses; ::1 is used only where IPv6 loopback exists
	DefaultPHP string   `toml:"default_php"` // "" = the highest installed version
	Modules    []string `toml:"modules"`     // extra modules enabled with `apache modules enable`
	Autostart  string   `toml:"autostart"`
}

// DefaultSettings returns the settings install writes for a port set.
func DefaultSettings(ports Ports, autostart string) Settings {
	return Settings{
		Ports:     ports,
		Listen:    []string{LoopbackIPv4, LoopbackIPv6},
		Modules:   []string{},
		Autostart: autostart,
	}
}

var moduleNameRE = regexp.MustCompile(`^[a-z0-9_]+$`)

// ModuleName reports an Apache module name as hte.toml takes it: the
// file mod_<name>.so without its prefix and suffix ("ssl", "proxy_fcgi").
func ModuleName(name string) bool { return moduleNameRE.MatchString(name) }

func invalidSettings(format string, a ...any) error {
	return hteerr.Newf(hteerr.InvalidState, "fix hte.toml or run gash hte install again", format, a...)
}

// Validate applies the strict rules to hte.toml.
func (s Settings) Validate() error {
	for _, p := range []struct {
		name string
		v    int
	}{{"ports.http", s.Ports.HTTP}, {"ports.https", s.Ports.HTTPS}} {
		if p.v < 1 || p.v > 65535 {
			return invalidSettings("%s = %d is not a TCP port", p.name, p.v)
		}
	}
	if s.Ports.HTTP == s.Ports.HTTPS {
		return invalidSettings("ports.http and ports.https are both %d", s.Ports.HTTP)
	}
	if !slices.Contains(s.Listen, LoopbackIPv4) {
		return invalidSettings("listen must contain %s", LoopbackIPv4)
	}
	seen := map[string]bool{}
	for _, a := range s.Listen {
		if a != LoopbackIPv4 && a != LoopbackIPv6 {
			return invalidSettings("listen address %q is not %s or %s", a, LoopbackIPv4, LoopbackIPv6)
		}
		if seen[a] {
			return invalidSettings("listen address %s is listed twice", a)
		}
		seen[a] = true
	}
	if s.DefaultPHP != "" && !phpVersionRE.MatchString(s.DefaultPHP) {
		return invalidSettings("default_php %q is not a PHP version", s.DefaultPHP)
	}
	mods := map[string]bool{}
	for _, m := range s.Modules {
		if !moduleNameRE.MatchString(m) {
			return invalidSettings("module %q is not an Apache module name", m)
		}
		if mods[m] {
			return invalidSettings("module %s is listed twice", m)
		}
		mods[m] = true
	}
	if !slices.Contains(autostartModes, s.Autostart) {
		return invalidSettings("autostart %q is not one of systemd, launchd, shell, none", s.Autostart)
	}
	return nil
}
