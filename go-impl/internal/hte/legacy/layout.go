// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package legacy

import (
	"path"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/platform"
)

// Layout is where a platform keeps the system Apache and FPM files that
// migrate reads. hte-cli ran on Debian and Ubuntu only, so the hte-cli
// parts (pools, certificate directory, envvars) exist only there; the
// other platforms contribute raw vhosts.
type Layout struct {
	Family platform.Family
	// VhostDirs are walked recursively for *.conf files, as hte-cli did.
	VhostDirs []string
	// EnabledDir is Debian's sites-enabled: a vhost is enabled when an
	// entry of the same name exists there. Entries that are not links
	// into a VhostDir are vhosts of their own.
	EnabledDir string
	// AllEnabled: every file of VhostDirs is included (Fedora conf.d).
	AllEnabled bool
	// MainConf and ServerRoot (Arch, Homebrew): a file is enabled when an
	// uncommented Include or IncludeOptional line of MainConf names it,
	// absolute or relative to ServerRoot (globs allowed).
	MainConf   string
	ServerRoot string
	// ConfEnabledDir is Debian's conf-enabled: snippets that migrate
	// neither reads nor imports; the plan lists them as no longer served.
	ConfEnabledDir string
	PHPRoot        string // /etc/php: hte-cli pools at <PHPRoot>/<X.Y>/fpm/pool.d/<domain>.conf
	CertDir        string // hte-cli certificates: <CertDir>/<domain>.crt and .key
	Envvars        string // Debian envvars (APACHE_RUN_USER)
	Hosts          string
	// LogDirs are the system log directories; raw vhosts that log there
	// are rewritten to the userland logs directory.
	LogDirs []string
	// ListenConf are the main configuration files whose Listen directives
	// name the ports of the system Apache (the vhost files are read too).
	ListenConf []string
}

// LayoutFor returns the layout of a platform family. prefix is the
// Homebrew prefix on macOS.
func LayoutFor(f platform.Family, prefix string) (Layout, error) {
	switch f {
	case platform.FamilyDebian:
		return Layout{
			Family:         f,
			VhostDirs:      []string{"/etc/apache2/sites-available"},
			EnabledDir:     "/etc/apache2/sites-enabled",
			ConfEnabledDir: "/etc/apache2/conf-enabled",
			PHPRoot:        "/etc/php",
			CertDir:        "/etc/apache2/certs-selfsigned",
			Envvars:        "/etc/apache2/envvars",
			Hosts:          "/etc/hosts",
			LogDirs:        []string{"/var/log/apache2"},
			ListenConf:     []string{"/etc/apache2/ports.conf"},
		}, nil
	case platform.FamilyFedora:
		return Layout{
			Family:     f,
			VhostDirs:  []string{"/etc/httpd/conf.d"},
			AllEnabled: true,
			Hosts:      "/etc/hosts",
			LogDirs:    []string{"/var/log/httpd", "/etc/httpd/logs"},
			ListenConf: []string{"/etc/httpd/conf/httpd.conf"},
		}, nil
	case platform.FamilyArch:
		return Layout{
			Family:     f,
			VhostDirs:  []string{"/etc/httpd/conf/extra"},
			MainConf:   "/etc/httpd/conf/httpd.conf",
			ServerRoot: "/etc/httpd",
			Hosts:      "/etc/hosts",
			LogDirs:    []string{"/var/log/httpd"},
			ListenConf: []string{"/etc/httpd/conf/httpd.conf"},
		}, nil
	case platform.FamilyDarwin:
		if prefix == "" {
			return Layout{}, hteerr.New(hteerr.ApacheNotFound, "no Homebrew prefix", "brew install httpd")
		}
		return Layout{
			Family:     f,
			VhostDirs:  []string{path.Join(prefix, "etc/httpd/extra")},
			MainConf:   path.Join(prefix, "etc/httpd/httpd.conf"),
			ServerRoot: prefix,
			Hosts:      "/etc/hosts",
			LogDirs:    []string{path.Join(prefix, "var/log/httpd")},
			ListenConf: []string{path.Join(prefix, "etc/httpd/httpd.conf")},
		}, nil
	}
	return Layout{}, hteerr.Newf(hteerr.PlatformUnsupported, "", "no Apache layout for %s", f)
}

// PoolPath is the exact hte-cli pool of a site, or "" where hte-cli never
// wrote pools.
func (l Layout) PoolPath(version, domain string) string {
	if l.PHPRoot == "" {
		return ""
	}
	return path.Join(l.PHPRoot, version, "fpm/pool.d", domain+".conf")
}

// CertPair is the exact hte-cli certificate pair of a site, or "" twice.
func (l Layout) CertPair(domain string) (cert, key string) {
	if l.CertDir == "" {
		return "", ""
	}
	return path.Join(l.CertDir, domain+".crt"), path.Join(l.CertDir, domain+".key")
}
