// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package state is the declarative state of gash localweb: one TOML file per
// site, the localweb.toml settings, their strict validation, atomic load and
// save, the mutation lock, and the file layout of design/hte.md section 2.
package state

import (
	"path/filepath"

	"github.com/mauriziofonte/gash/internal/platform"
)

// Names of the userland layout. Gash 4.0 and older used the legacy ones
// (gash localweb was gash hte); LegacyPaths reads such a tree until the
// engine moves it.
const (
	Name               = "localweb"
	SettingsName       = "localweb.toml"
	LegacyName         = "hte"
	LegacySettingsName = "hte.toml"
)

// Paths is the userland file layout of design/hte.md section 2.
type Paths struct {
	ConfigDir  string // $XDG_CONFIG_HOME/gash/localweb
	StateDir   string // $XDG_STATE_HOME/gash/localweb
	DataDir    string // $XDG_DATA_HOME/gash/localweb
	RuntimeDir string // $XDG_RUNTIME_DIR/gash-localweb, else /tmp/gash-localweb-<uid>
	// Legacy marks the layout of Gash 4.0 and older (gash/hte, hte.toml).
	Legacy bool
}

// NewPaths derives the layout from the XDG directories of uid.
func NewPaths(d platform.Dirs, uid int) Paths { return paths(d, uid, Name, false) }

// LegacyPaths is the layout Gash 4.0 and older wrote: gash/hte, gash-hte
// and hte.toml.
func LegacyPaths(d platform.Dirs, uid int) Paths { return paths(d, uid, LegacyName, true) }

func paths(d platform.Dirs, uid int, name string, legacy bool) Paths {
	return Paths{
		ConfigDir:  filepath.Join(d.ConfigHome, "gash", name),
		StateDir:   filepath.Join(d.StateHome, "gash", name),
		DataDir:    filepath.Join(d.DataHome, "gash", name),
		RuntimeDir: d.RuntimeBase("gash-"+name, uid),
		Legacy:     legacy,
	}
}

// Settings is localweb.toml (hte.toml in the legacy layout).
func (p Paths) Settings() string {
	if p.Legacy {
		return filepath.Join(p.ConfigDir, LegacySettingsName)
	}
	return filepath.Join(p.ConfigDir, SettingsName)
}

// SitesDir holds the site state files and their Apache overlays.
func (p Paths) SitesDir() string { return filepath.Join(p.ConfigDir, "sites") }

// SiteFile is the state of one site.
func (p Paths) SiteFile(domain string) string {
	return filepath.Join(p.SitesDir(), domain+".toml")
}

// SiteOverlay is the optional Apache overlay included in every vhost of a site.
func (p Paths) SiteOverlay(domain string) string {
	return filepath.Join(p.SitesDir(), domain+".apache.conf")
}

// ApacheOverlay is the optional server-level Apache overlay.
func (p Paths) ApacheOverlay() string { return filepath.Join(p.ConfigDir, "apache.conf") }

// PHPOverlayDir is the global PHP overlay directory of one PHP version:
// its *.ini files are read by that version's master after the system's
// (PHP_INI_SCAN_DIR), never rewritten by the renderer.
func (p Paths) PHPOverlayDir(version string) string {
	return filepath.Join(p.ConfigDir, "php", version)
}

// PHPOverlay is the file of the overlay directory that `gash localweb php
// global set|unset` edits.
func (p Paths) PHPOverlay(version string) string {
	return filepath.Join(p.PHPOverlayDir(version), "global.ini")
}

// HostsState names the extra hosts names of the managed block (the
// legacy "# hte-cli" names migrate carried over).
func (p Paths) HostsState() string { return filepath.Join(p.ConfigDir, "hosts.toml") }

// RenderDir holds the rendered generations and the current symlink.
func (p Paths) RenderDir() string { return filepath.Join(p.StateDir, "render") }

// LogsDir holds every Apache and PHP log.
func (p Paths) LogsDir() string { return filepath.Join(p.StateDir, "logs") }

// SiteLogDir holds the logs of one site.
func (p Paths) SiteLogDir(domain string) string { return filepath.Join(p.LogsDir(), domain) }

// CertsDir holds one directory per site.
func (p Paths) CertsDir() string { return filepath.Join(p.DataDir, "certs") }

// CertDir is the 0700 certificate directory of a site.
func (p Paths) CertDir(domain string) string { return filepath.Join(p.CertsDir(), domain) }

// CertFile is a site's certificate.
func (p Paths) CertFile(domain string) string { return filepath.Join(p.CertDir(domain), "cert.pem") }

// KeyFile is a site's private key (0600).
func (p Paths) KeyFile(domain string) string { return filepath.Join(p.CertDir(domain), "key.pem") }

// SessionDir is the per-version PHP session directory used where the
// platform default is not writable (LEDGER HTE-41).
func (p Paths) SessionDir(version string) string {
	return filepath.Join(p.StateDir, "php", version, "sessions")
}

// Receipt is the user copy of the install receipt.
func (p Paths) Receipt() string { return filepath.Join(p.StateDir, "receipt.json") }

// Audit is the append-only audit log.
func (p Paths) Audit() string { return filepath.Join(p.StateDir, "audit.jsonl") }

// Lock is the mutation lock file.
func (p Paths) Lock() string { return filepath.Join(p.StateDir, "lock") }

// UnitLease names the command that holds the lock and waits for a job of
// the systemd user unit (LEDGER HTE-73).
func (p Paths) UnitLease() string { return filepath.Join(p.StateDir, "unit.lease") }

// PHPCache caches PHP-FPM discovery by binary path, size and mtime.
func (p Paths) PHPCache() string { return filepath.Join(p.StateDir, "php-versions.json") }

// Stage is the uninstall stage directory for a UTC timestamp.
func (p Paths) Stage(timestamp string) string {
	return filepath.Join(filepath.Dir(p.StateDir), Name+"-stage-"+timestamp)
}

// LegacyStage is the uninstall stage directory Gash 4.0 and older made.
func (p Paths) LegacyStage(timestamp string) string {
	return filepath.Join(filepath.Dir(p.StateDir), LegacyName+"-stage-"+timestamp)
}
