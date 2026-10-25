// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package logs

import (
	"path"
	"strings"
)

// A log file is reclaimable in one of two ways. An active log is the file
// a program still writes to (app.log): deleting it would leak the inode a
// running daemon holds, so Gash truncates it in place, which frees the
// space and keeps the file. A rotated log is a past copy
// (app.log.1, app.log.2.gz, app-2026-10-24.log): nothing writes to it, so
// Gash deletes it.
type kind int

const (
	notLog  kind = iota // not a log file
	active              // the live log: truncate to reclaim
	rotated             // a past copy: delete to reclaim
	journal             // a systemd journal: neither (vacuum instead)
)

// category groups logs for the report, so a human sees "nginx 1.2 GiB"
// rather than a flat list.
type category struct {
	name string
	// dir matches when a path component equals one of these (apache2,
	// nginx, journal).
	dir []string
	// base matches when the file's base name, lower-cased, has one of
	// these prefixes.
	base []string
}

// categories are tried in order; the first match wins, else "other".
var categories = []category{
	{name: "systemd-journal", dir: []string{"journal"}, base: []string{"system.journal", "user-"}},
	{name: "web-server", dir: []string{"apache2", "httpd", "nginx", "caddy", "lighttpd"},
		base: []string{"access", "error", "ssl_access", "ssl_error", "other_vhosts"}},
	{name: "php", dir: []string{"php", "php-fpm", "fpm"}, base: []string{"php_error", "php-fpm", "php8", "php7", "xdebug"}},
	{name: "node", dir: []string{"_logs", "pm2", ".pm2", ".npm"},
		base: []string{"npm-debug", "yarn-error", "yarn-debug", "pnpm-debug", "lerna-debug"}},
	{name: "python", dir: []string{"__pycache__"}, base: []string{"pip-", "pip.log", "django", "celery", "gunicorn", "uwsgi"}},
	{name: "package-manager", dir: []string{"apt", "dpkg", "dnf", "yum", "pacman"},
		base: []string{"dpkg.log", "apt", "history.log", "term.log", "unattended"}},
	{name: "database", dir: []string{"mysql", "postgresql", "mongodb", "redis"},
		base: []string{"mysql", "mariadb", "postgres", "mongod", "redis", "slow"}},
	{name: "framework", dir: []string{"storage"}, base: []string{"laravel", "production", "development", "rails"}},
}

// logDirs are directory names whose files are logs even when the name
// alone is not conclusive (e.g. a bare *.txt under logs/ is a log).
var logDirs = map[string]bool{
	"log": true, "logs": true, "_logs": true, "journal": true,
	"apache2": true, "httpd": true, "nginx": true, "php-fpm": true,
}

// activeExt are the extensions of a live log.
var activeExt = map[string]bool{".log": true, ".out": true, ".err": true}

// compressExt mark a rotated, compressed log.
var compressExt = map[string]bool{".gz": true, ".xz": true, ".bz2": true, ".zst": true, ".z": true, ".lz4": true}

// knownBase are exact or prefix base names that are logs wherever they sit.
var knownBase = []string{
	"npm-debug.log", "yarn-error.log", "yarn-debug.log", "pnpm-debug.log",
	"lerna-debug.log", "php_errors.log", "debug.log", "error.log", "access.log",
}

// classify decides what a file is from its path and base name. underLogDir
// says a parent directory is a known log directory.
func classify(p string) (kind, string) {
	base := strings.ToLower(path.Base(p))
	ext := path.Ext(base)
	parts := splitDirs(p)
	inLogDir := false
	for _, d := range parts {
		if logDirs[d] {
			inLogDir = true
			break
		}
	}

	// systemd journals: never touched.
	if ext == ".journal" || strings.HasSuffix(base, ".journal~") {
		return journal, "systemd-journal"
	}

	k := kindOf(base, ext, inLogDir)
	if k == notLog {
		return notLog, ""
	}
	return k, categorize(parts, base)
}

// kindOf returns active, rotated or notLog for a base name.
func kindOf(base, ext string, inLogDir bool) kind {
	// Compressed: always a rotated copy, but only of a log.
	if compressExt[ext] {
		if isLogBase(strings.TrimSuffix(base, ext), inLogDir) {
			return rotated
		}
		return notLog
	}
	// A rotation number or a date after a .log: app.log.1, app.log.2026-10-24.
	if trimmed, ok := trimRotation(base); ok {
		if looksLog(trimmed, path.Ext(trimmed), inLogDir) {
			return rotated
		}
		return notLog
	}
	if activeExt[ext] {
		return active
	}
	// A bare *.txt counts only inside a known log directory, never loose.
	if ext == ".txt" && inLogDir {
		return active
	}
	if looksLog(base, ext, inLogDir) {
		return active
	}
	return notLog
}

// isLogBase reports a base name (a compressed file's inner name) that is a
// log, stripping a rotation marker first (access.log.14 -> access.log).
func isLogBase(base string, inLogDir bool) bool {
	if t, ok := trimRotation(base); ok {
		base = t
	}
	return looksLog(base, path.Ext(base), inLogDir)
}

// looksLog reports a base name that reads as a log: a known name, an
// active extension, or any file inside a known log directory.
func looksLog(base, ext string, inLogDir bool) bool {
	if activeExt[ext] {
		return true
	}
	for _, k := range knownBase {
		if base == k || strings.HasPrefix(base, k) {
			return true
		}
	}
	if inLogDir && (ext == "" || ext == ".txt" || ext == ".log") {
		return true
	}
	return false
}

// trimRotation strips a trailing rotation marker (".1", ".20261024",
// ".2026-10-24") and reports whether it found one.
func trimRotation(base string) (string, bool) {
	ext := path.Ext(base)
	if len(ext) < 2 {
		return base, false
	}
	suf := ext[1:]
	// A pure number (logrotate's .1, .2) or a date (.20261024, .2026-10-24).
	if isAllDigits(suf) || isDate(suf) {
		return strings.TrimSuffix(base, ext), true
	}
	return base, false
}

func categorize(parts []string, base string) string {
	for _, c := range categories {
		for _, d := range c.dir {
			for _, p := range parts {
				if p == d {
					return c.name
				}
			}
		}
		for _, b := range c.base {
			if strings.HasPrefix(base, b) {
				return c.name
			}
		}
	}
	return "other"
}

func splitDirs(p string) []string {
	p = path.Clean(p)
	var out []string
	for _, s := range strings.Split(p, "/") {
		if s != "" {
			out = append(out, strings.ToLower(s))
		}
	}
	if len(out) > 0 {
		out = out[:len(out)-1] // drop the base name
	}
	return out
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// isDate reports a YYYY-MM-DD or YYYYMMDD style marker.
func isDate(s string) bool {
	d := strings.ReplaceAll(s, "-", "")
	d = strings.ReplaceAll(d, "_", "")
	return len(d) == 8 && isAllDigits(d)
}
