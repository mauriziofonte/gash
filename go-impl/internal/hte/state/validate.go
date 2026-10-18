// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package state

import (
	"cmp"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mauriziofonte/gash/internal/hostshelper"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/profiles"
)

// Hints shared by the validation errors.
const (
	hintDomain       = "a site name is a lowercase hostname such as app.test (labels of a-z, 0-9 and -, at least two)"
	hintDocrootChars = "rename the directory or use a symlink without these characters"
	hintDocrootMake  = "create the directory first"
)

// NormalizeDomain lowercases a domain or alias argument and checks it
// against the hostname grammar shared with the hosts helper. Nothing is
// stripped: a scheme, a path, a port or a trailing dot fails (LEDGER
// HTE-20), and so does an IP address (HTE-21).
func NormalizeDomain(arg string) (string, error) {
	if arg == "" {
		return "", hteerr.New(hteerr.MissingValue, "a domain is required", "pass --domain NAME")
	}
	d := strings.ToLower(arg)
	if !hostshelper.ValidHostname(d) {
		return "", hteerr.Newf(hteerr.InvalidDomain, hintDomain, "%q is not a valid site name", arg)
	}
	return d, nil
}

// docrootRefused are the characters Apache (${NAME} expansion, <Directory>
// wildcards, quoting) or the PHP-FPM INI parser (; comments, ${NAME})
// would interpret (design/hte.md section 3, LEDGER HTE-08).
const docrootRefused = "$*?[;\"\\"

// CheckDocrootText applies the character rules to a document root: valid
// UTF-8, no control character (newline included) and none of $ * ? [ ; " \.
func CheckDocrootText(p string) error {
	if !utf8.ValidString(p) {
		return hteerr.Newf(hteerr.InvalidDocroot, hintDocrootChars, "document root %q is not valid UTF-8", p)
	}
	for _, r := range p {
		if unicode.IsControl(r) {
			return hteerr.Newf(hteerr.InvalidDocroot, hintDocrootChars, "document root %q contains a control character", p)
		}
		if strings.ContainsRune(docrootRefused, r) {
			return hteerr.Newf(hteerr.InvalidDocroot, hintDocrootChars, "document root %q contains %q", p, r)
		}
	}
	return nil
}

// ResolveDocroot turns a --docroot argument into the stored document root:
// a relative argument is resolved against cwd (LEDGER HTE-29, option a),
// the result is cleaned (no trailing slash), checked by CheckDocrootText,
// and must be an existing directory. It is never created (HTE-60).
func ResolveDocroot(arg, cwd string, stat func(string) (fs.FileInfo, error)) (string, error) {
	if arg == "" {
		return "", hteerr.New(hteerr.MissingValue, "a document root is required", "pass --docroot PATH")
	}
	if err := CheckDocrootText(arg); err != nil {
		return "", err
	}
	p := arg
	if !filepath.IsAbs(p) {
		if cwd == "" || !filepath.IsAbs(cwd) {
			return "", hteerr.Newf(hteerr.InvalidDocroot, "give an absolute path", "relative document root %q and no current directory", arg)
		}
		if err := CheckDocrootText(cwd); err != nil {
			return "", err
		}
		p = filepath.Join(cwd, p)
	}
	p = filepath.Clean(p)
	if stat == nil {
		stat = os.Stat
	}
	fi, err := stat(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", hteerr.Newf(hteerr.DocrootNotFound, hintDocrootMake, "document root %s does not exist", p)
	case err != nil:
		return "", hteerr.Newf(hteerr.InvalidDocroot, "", "document root %s: %v", p, err)
	case !fi.IsDir():
		return "", hteerr.Newf(hteerr.InvalidDocroot, "give a directory", "document root %s is not a directory", p)
	}
	return p, nil
}

// checkStoredDocroot is the syntax part of the docroot rules for a stored
// value: absolute, already clean, and the character rules.
func checkStoredDocroot(p string) error {
	if !filepath.IsAbs(p) || filepath.Clean(p) != p {
		return hteerr.Newf(hteerr.InvalidDocroot, "", "document root %q is not an absolute clean path", p)
	}
	return CheckDocrootText(p)
}

// SupportedPHP are the PHP versions discovery probes (hte-cli's list).
var SupportedPHP = []string{"5.6", "7.0", "7.1", "7.2", "7.3", "7.4", "8.0", "8.1", "8.2", "8.3", "8.4", "8.5"}

var phpVersionRE = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

// ComparePHP orders "X.Y" versions numerically.
func ComparePHP(a, b string) int {
	am, an := splitVersion(a)
	bm, bn := splitVersion(b)
	return cmp.Or(cmp.Compare(am, bm), cmp.Compare(an, bn))
}

func splitVersion(v string) (int, int) {
	maj, min, _ := strings.Cut(v, ".")
	x, _ := strconv.Atoi(maj)
	y, _ := strconv.Atoi(min)
	return x, y
}

func phpHint(installed []string) string {
	if len(installed) == 0 {
		return "no PHP-FPM version is installed; gash hte doctor prints the install command"
	}
	s := slices.Clone(installed)
	slices.SortFunc(s, ComparePHP)
	return "installed versions: " + strings.Join(s, ", ")
}

// ResolvePHP returns the PHP version of a site: the flag when given (it
// must be installed), else the hte.toml default (which must be installed
// too), else the highest installed version (LEDGER HTE-55, option a). An
// invalid value never falls back to a default.
func ResolvePHP(flag *string, env Env) (string, error) {
	if flag != nil {
		v := *flag
		if !slices.Contains(env.Installed, v) {
			return "", hteerr.Newf(hteerr.PHPVersionNotInstalled, phpHint(env.Installed), "PHP %q is not installed", v)
		}
		return v, nil
	}
	if env.DefaultPHP != "" {
		if !slices.Contains(env.Installed, env.DefaultPHP) {
			return "", hteerr.Newf(hteerr.PHPVersionNotInstalled, phpHint(env.Installed),
				"the default PHP %s of hte.toml is not installed", env.DefaultPHP)
		}
		return env.DefaultPHP, nil
	}
	if len(env.Installed) == 0 {
		return "", hteerr.New(hteerr.PHPVersionNotInstalled, "no PHP-FPM version is installed", phpHint(nil))
	}
	return slices.MaxFunc(env.Installed, ComparePHP), nil
}

// ParseProfile returns the profile of the flag, dev when absent.
func ParseProfile(flag *string) (profiles.Name, error) {
	if flag == nil {
		return profiles.Dev, nil
	}
	n, ok := profiles.Parse(*flag)
	if !ok {
		return "", hteerr.Newf(hteerr.InvalidProfile, "profiles: dev, staging, hardened", "unknown profile %q", *flag)
	}
	return n, nil
}

// BuildAliases computes a site's aliases: www.<domain> unless noWWW or the
// domain already starts with "www." (LEDGER HTE-47, option 1a), then every
// --alias in order. Aliases are lowercased, must be valid hostnames, and
// may not repeat each other or the domain.
func BuildAliases(domain string, noWWW bool, extra []string) ([]string, error) {
	out := []string{}
	if !noWWW && !strings.HasPrefix(domain, "www.") {
		out = append(out, "www."+domain)
	}
	for _, a := range extra {
		n, err := NormalizeDomain(a)
		if err != nil {
			if e, ok := hteerr.As(err); ok && e.Code == hteerr.MissingValue {
				return nil, hteerr.New(hteerr.InvalidDomain, "an alias is empty", hintDomain)
			}
			return nil, err
		}
		if n == domain {
			return nil, hteerr.Newf(hteerr.InvalidDomain, "", "alias %s is the site's own domain", n)
		}
		if slices.Contains(out, n) {
			if n == "www."+domain && !noWWW {
				continue // --alias www.<domain> restates the default alias
			}
			return nil, hteerr.Newf(hteerr.InvalidDomain, "", "alias %s is given twice", n)
		}
		out = append(out, n)
	}
	return out, nil
}

// iniKeyRE is the name of a PHP ini setting.
var iniKeyRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.]*$`)

// CheckINI validates one [php_ini] entry: the key is an ini name, the value
// is UTF-8 text without control characters, written verbatim after the
// directive (as in php.ini).
func CheckINI(key, value string) error {
	if !iniKeyRE.MatchString(key) {
		return hteerr.Newf(hteerr.InvalidState, "", "php_ini key %q is not a PHP setting name", key)
	}
	if !utf8.ValidString(value) || strings.ContainsFunc(value, unicode.IsControl) {
		return hteerr.Newf(hteerr.InvalidState, "", "php_ini value of %s contains a control character", key)
	}
	return nil
}
