// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package state is the declarative state of gash hte: one TOML file per
// site, the hte.toml settings, their strict validation, atomic load and
// save, the mutation lock, and the file layout of design/hte.md section 2.
package state

import (
	"path/filepath"

	"github.com/mauriziofonte/gash/internal/platform"
)

// Paths is the userland file layout of design/hte.md section 2.
type Paths struct {
	ConfigDir  string // $XDG_CONFIG_HOME/gash/hte
	StateDir   string // $XDG_STATE_HOME/gash/hte
	DataDir    string // $XDG_DATA_HOME/gash/hte
	RuntimeDir string // $XDG_RUNTIME_DIR/gash-hte, else /tmp/gash-hte-<uid>
}

// NewPaths derives the layout from the XDG directories of uid.
func NewPaths(d platform.Dirs, uid int) Paths {
	return Paths{
		ConfigDir:  filepath.Join(d.ConfigHome, "gash", "hte"),
		StateDir:   filepath.Join(d.StateHome, "gash", "hte"),
		DataDir:    filepath.Join(d.DataHome, "gash", "hte"),
		RuntimeDir: d.RuntimeBase("gash-hte", uid),
	}
}

// Settings is hte.toml.
func (p Paths) Settings() string { return filepath.Join(p.ConfigDir, "hte.toml") }

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

// PHPOverlay is the file of the overlay directory that `gash hte php
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
	return filepath.Join(filepath.Dir(p.StateDir), "hte-stage-"+timestamp)
}
