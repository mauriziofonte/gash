// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/renameio/v2"

	"github.com/mauriziofonte/gash/internal/hte/discover"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/state"
)

// PHPVersions is php versions: the usable PHP-FPM versions and the
// binaries that did not qualify. It needs no install.
func (e *Engine) PHPVersions(ctx context.Context) (*discover.PHPResult, error) {
	if err := e.CheckIdentity("php versions"); err != nil {
		return nil, err
	}
	v, err := e.resolve()
	if err != nil {
		return nil, err
	}
	res := e.discoverPHP(ctx, v)
	if res.Versions == nil {
		res.Versions = []discover.PHP{}
	}
	if res.Problems == nil {
		res.Problems = []string{}
	}
	return &res, nil
}

// PHPIniGet is php ini get: the site's [php_ini] value of key.
func (e *Engine) PHPIniGet(ctx context.Context, domain, key string) (string, bool, error) {
	v, err := e.ready("php ini get")
	if err != nil {
		return "", false, err
	}
	s, err := v.store.LoadSite(domain)
	if err != nil {
		return "", false, err
	}
	val, ok := s.PHPIni[key]
	return val, ok, nil
}

// PHPIniSet is php ini set: one key of the site's [php_ini], rendered into
// its pool (design/hte.md 4), then a reload.
func (e *Engine) PHPIniSet(ctx context.Context, domain, key, value string) (*SiteResult, error) {
	if err := state.CheckINI(key, value); err != nil {
		return nil, err
	}
	return e.phpIni(ctx, "php ini set", domain, func(ini map[string]string) { ini[key] = value })
}

// PHPIniUnset is php ini unset.
func (e *Engine) PHPIniUnset(ctx context.Context, domain, key string) (*SiteResult, error) {
	return e.phpIni(ctx, "php ini unset", domain, func(ini map[string]string) { delete(ini, key) })
}

func (e *Engine) phpIni(ctx context.Context, command, domain string, edit func(map[string]string)) (*SiteResult, error) {
	return e.mutate(ctx, command, domain, func(v *env, set state.Settings, sites []state.Site) ([]state.Site, *state.Site, bool, error) {
		cur, err := v.store.LoadSite(domain)
		if err != nil {
			return nil, nil, false, err
		}
		if !cur.HasPool() {
			return nil, nil, false, hteerr.Newf(hteerr.ConflictingOptions, "", "%s has no PHP pool", cur.Domain)
		}
		next := cur
		next.PHPIni = maps.Clone(cur.PHPIni)
		if next.PHPIni == nil {
			next.PHPIni = map[string]string{}
		}
		edit(next.PHPIni)
		if len(next.PHPIni) == 0 {
			next.PHPIni = nil
		}
		if err := next.Validate(); err != nil {
			return nil, nil, false, err
		}
		return replace(sites, next), &next, !maps.Equal(cur.PHPIni, next.PHPIni), nil
	})
}

// GlobalResult is the outcome of php global set and unset.
type GlobalResult struct {
	File     string `json:"file"`
	Changed  bool   `json:"changed"`
	Reloaded bool   `json:"reloaded"`
}

// overlayKey reports whether an overlay line sets key.
func overlayKey(line []byte, key string) bool {
	k, _, ok := bytes.Cut(line, []byte("="))
	return ok && string(bytes.TrimSpace(k)) == key
}

// overlayLine is how the overlay writes a value: a boolean word bare
// (php.ini reads it as the flag), anything else in double quotes.
func overlayLine(key, value string) string {
	if slices.Contains([]string{"on", "off", "yes", "no", "true", "false"}, strings.ToLower(value)) {
		return key + " = " + value
	}
	return key + ` = "` + value + `"`
}

// PHPGlobalGet is php global get: the overlay value of key for version.
func (e *Engine) PHPGlobalGet(ctx context.Context, version, key string) (string, bool, error) {
	v, err := e.ready("php global get")
	if err != nil {
		return "", false, err
	}
	data, err := os.ReadFile(v.paths.PHPOverlay(version))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	for _, line := range bytes.Split(data, []byte("\n")) {
		if overlayKey(line, key) {
			_, val, _ := bytes.Cut(line, []byte("="))
			return strings.Trim(strings.TrimSpace(string(val)), `"`), true, nil
		}
	}
	return "", false, nil
}

// PHPGlobalSet is php global set: one key of the version's overlay
// (global.ini in its overlay directory), then that master reloads; a
// master that does not come back gets the previous file.
func (e *Engine) PHPGlobalSet(ctx context.Context, version, key, value string) (*GlobalResult, error) {
	if err := state.CheckINI(key, value); err != nil {
		return nil, err
	}
	line := overlayLine(key, value)
	return e.phpGlobal(ctx, "php global set", version, func(lines [][]byte) [][]byte {
		i := slices.IndexFunc(lines, func(l []byte) bool { return overlayKey(l, key) })
		if i < 0 {
			return append(lines, []byte(line))
		}
		lines[i] = []byte(line)
		return lines
	})
}

// PHPGlobalUnset is php global unset.
func (e *Engine) PHPGlobalUnset(ctx context.Context, version, key string) (*GlobalResult, error) {
	return e.phpGlobal(ctx, "php global unset", version, func(lines [][]byte) [][]byte {
		return slices.DeleteFunc(lines, func(l []byte) bool { return overlayKey(l, key) })
	})
}

func (e *Engine) phpGlobal(ctx context.Context, command, version string, edit func([][]byte) [][]byte) (*GlobalResult, error) {
	v, err := e.ready(command)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(e.discoverPHP(ctx, v).Installed(), version) {
		return nil, hteerr.Newf(hteerr.PHPVersionNotInstalled, "gash hte php versions lists them", "PHP %q is not installed", version)
	}
	file := v.paths.PHPOverlay(version)
	res, err := e.locked(v, command, "", func() (any, error) {
		old, err := os.ReadFile(file)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		var lines [][]byte
		if len(old) > 0 {
			lines = bytes.Split(bytes.TrimSuffix(old, []byte("\n")), []byte("\n"))
		}
		lines = edit(lines)
		next := []byte{}
		if len(lines) > 0 {
			next = append(bytes.Join(lines, []byte("\n")), '\n')
		}
		out := &GlobalResult{File: file, Changed: !bytes.Equal(old, next)}
		if !out.Changed {
			return out, nil
		}
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			return nil, err
		}
		if err := renameio.WriteFile(file, next, 0o600); err != nil {
			return nil, err
		}
		sup := e.supervisor(v)
		if err := sup.ReloadFPM(ctx, version); err != nil {
			rerr := os.Remove(file)
			if old != nil {
				rerr = renameio.WriteFile(file, old, 0o600)
			}
			if rerr != nil {
				return nil, errors.Join(err, rerr)
			}
			return nil, errors.Join(err, sup.ReloadFPM(ctx, version))
		}
		running, _ := sup.Running()
		out.Reloaded = running
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return res.(*GlobalResult), nil
}

// ApacheConfigPath is apache config path: the global Apache overlay, read
// by every generation (IncludeOptional), never written by gash.
func (e *Engine) ApacheConfigPath(ctx context.Context) (string, error) {
	v, err := e.ready("apache config path")
	if err != nil {
		return "", err
	}
	return v.paths.ApacheOverlay(), nil
}
