// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package profiles holds the PHP-FPM pool profiles dev, staging and
// hardened with the exact values and rendering order of hte-cli
// app/Logic/Preprocessors/PhpProfiles.php (unchanged since 1.1.0). Only
// the two log paths are parameters: gash moves them out of the document
// root (LEDGER HTE-07), the renderer passes them in.
package profiles

import (
	"slices"
	"strings"
)

// Name is a profile name.
type Name string

// The three profiles.
const (
	Dev      Name = "dev"
	Staging  Name = "staging"
	Hardened Name = "hardened"
)

// All returns the profiles in hte-cli's order.
func All() []Name { return []Name{Dev, Staging, Hardened} }

// Parse returns the profile named s. Only the exact lowercase names are
// accepted; nothing falls back to dev.
func Parse(s string) (Name, bool) {
	n := Name(s)
	return n, slices.Contains(All(), n)
}

// Description returns hte-cli's one-line description of a profile.
func Description(n Name) string {
	switch n {
	case Dev:
		return "Development - Minimal restrictions, high memory limits"
	case Staging:
		return "Staging - Moderate security, open_basedir enabled, session hardening"
	case Hardened:
		return "Hardened - Maximum security (WARNING: may break Laravel/Composer/PHPUnit)"
	}
	return "Unknown profile"
}

// DefaultOpenBasedir is the directory list hte-cli appends to the document
// root in open_basedir (Debian's layout, used on every platform: LEDGER
// HTE-42, option a).
const DefaultOpenBasedir = "/var/lib/php:/usr/lib/php:/usr/share/php:/tmp:/dev/urandom"

var (
	baseDisabled     = []string{"apache_child_terminate", "apache_get_modules", "apache_getenv", "apache_note", "apache_setenv"}
	stagingDisabled  = []string{"dl", "highlight_file", "show_source", "phpinfo"}
	hardenedDisabled = []string{"exec", "shell_exec", "system", "passthru", "popen", "proc_open", "proc_close",
		"proc_get_status", "proc_terminate", "pcntl_exec", "pcntl_fork", "pcntl_signal", "symlink"}
)

// DisabledFunctions returns the comma-separated disable_functions value.
func DisabledFunctions(n Name) string {
	fns := slices.Clone(baseDisabled)
	if n == Staging || n == Hardened {
		fns = append(fns, stagingDisabled...)
	}
	if n == Hardened {
		fns = append(fns, hardenedDisabled...)
	}
	return strings.Join(fns, ",")
}

// Line is one line of a profile block: a blank line (zero value), a
// section comment (Comment set, written as "; <Comment>"), or a directive
// "Key = Value". Path marks values that hold a path: the renderer quotes
// them.
type Line struct {
	Comment string
	Key     string
	Value   string
	Path    bool
}

// IsBlank reports whether l is a blank line.
func (l Line) IsBlank() bool { return l == Line{} }

// Options are the per-site parameters of a block.
type Options struct {
	Docroot  string // document root, first entry of open_basedir
	SlowLog  string // value of slowlog
	ErrorLog string // value of php_admin_value[error_log]
	// OpenBasedirExtra is appended to open_basedir (the per-version session
	// directory of LEDGER HTE-41, where the platform default is not
	// writable).
	OpenBasedirExtra []string
}

type kv struct{ key, value string }

// additional returns PhpProfiles::getAdditionalConfig in its key order.
func additional(n Name, o Options) []kv {
	var out []kv
	if n == Staging || n == Hardened {
		basedir := o.Docroot + ":" + DefaultOpenBasedir
		for _, extra := range o.OpenBasedirExtra {
			basedir += ":" + extra
		}
		out = append(out,
			kv{"open_basedir", basedir},
			kv{"allow_url_include", "off"},
			kv{"session.cookie_httponly", "on"},
			kv{"session.use_strict_mode", "on"},
			kv{"session.use_only_cookies", "on"},
		)
	}
	if n == Hardened {
		out = append(out,
			kv{"auto_prepend_file", ""},
			kv{"auto_append_file", ""},
			kv{"expose_php", "off"},
			kv{"memory_limit", "256M"},
			kv{"post_max_size", "32M"},
			kv{"upload_max_filesize", "32M"},
			kv{"max_execution_time", "30"},
			kv{"max_input_time", "60"},
		)
	}
	return out
}

func lookup(list []kv, key, def string) string {
	for _, e := range list {
		if e.key == key {
			return e.value
		}
	}
	return def
}

// AdminDirective returns the pool directive PhpProfiles.php:233-246 uses
// for an ini setting: php_admin_flag for "on" and "off", php_admin_value
// otherwise, with "" written as a pair of double quotes.
func AdminDirective(key, value string) (directive, rendered string) {
	switch value {
	case "on", "off":
		return "php_admin_flag[" + key + "]", value
	case "":
		return "php_admin_value[" + key + "]", `""`
	}
	return "php_admin_value[" + key + "]", value
}

// Lines returns the profile block of a pool, from "; Security: Disabled
// functions" to the last profile setting, exactly as hte-cli renders it.
func Lines(n Name, o Options) []Line {
	add := additional(n, o)
	mem := lookup(add, "memory_limit", "1024M")
	post := lookup(add, "post_max_size", "128M")
	upload := lookup(add, "upload_max_filesize", "128M")
	d := func(k, v string) Line { return Line{Key: k, Value: v} }
	c := func(s string) Line { return Line{Comment: s} }
	lines := []Line{
		c("Security: Disabled functions"),
		d("php_admin_value[disable_functions]", DisabledFunctions(n)),
		{},
		c("Security: URL handling"),
		d("php_admin_flag[allow_url_fopen]", "off"),
		{},
		c("Process manager"),
		d("pm", "dynamic"),
		d("pm.max_children", "5"),
		d("pm.start_servers", "2"),
		d("pm.min_spare_servers", "1"),
		d("pm.max_spare_servers", "3"),
	}
	if n == Staging || n == Hardened {
		lines = append(lines, d("pm.max_requests", "500"))
	}
	lines = append(lines,
		d("chdir", "/"),
		Line{},
		c("Logging"),
		d("catch_workers_output", "yes"),
		d("request_terminate_timeout", "180s"),
		Line{Key: "slowlog", Value: o.SlowLog, Path: true},
		d("php_flag[display_errors]", "off"),
		Line{Key: "php_admin_value[error_log]", Value: o.ErrorLog, Path: true},
		d("php_admin_flag[log_errors]", "on"),
		Line{},
		c("Resource limits"),
		d("php_admin_value[post_max_size]", post),
		d("php_admin_value[upload_max_filesize]", upload),
		d("php_admin_value[memory_limit]", mem),
		d("php_value[memory_limit]", mem),
		Line{},
		c("PHP settings"),
		d("php_value[short_open_tag]", "On"),
	)
	for _, e := range add {
		switch e.key {
		case "memory_limit", "post_max_size", "upload_max_filesize":
			continue
		}
		key, val := AdminDirective(e.key, e.value)
		lines = append(lines, Line{Key: key, Value: val, Path: e.key == "open_basedir"})
	}
	return lines
}

// Text renders lines the way hte-cli wrote them: "; Comment", "key = value"
// and blank lines, every line ending in a newline, no quoting.
func Text(lines []Line) string {
	var b strings.Builder
	for _, l := range lines {
		switch {
		case l.IsBlank():
		case l.Comment != "":
			b.WriteString("; " + l.Comment)
		default:
			b.WriteString(l.Key + " = " + l.Value)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// Values returns the settings of a profile that a legacy pool can be
// compared with: every directive except the two log paths, with values as
// written (whitespace is the caller's to normalize).
func Values(n Name, docroot string) map[string]string {
	out := map[string]string{}
	for _, l := range Lines(n, Options{Docroot: docroot}) {
		if l.Key == "" || l.Key == "slowlog" || l.Key == "php_admin_value[error_log]" {
			continue
		}
		out[l.Key] = l.Value
	}
	return out
}
