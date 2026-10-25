// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package legacy

import (
	"fmt"
	"strings"

	"github.com/mauriziofonte/gash/internal/hte/profiles"
	"github.com/mauriziofonte/gash/internal/lazyre"
)

// PoolValue is one key = value line of a pool.
type PoolValue struct {
	Key   string `json:"key"`
	Value string `json:"value"` // whitespace normalized
	Line  int    `json:"line"`
}

// Pool is a parsed PHP-FPM pool file.
type Pool struct {
	Path          string      `json:"path"`
	Section       string      `json:"section"`        // the first [section]
	ProfileMarker string      `json:"profile_marker"` // the word after "; Profile:", "" when absent
	Values        []PoolValue `json:"values"`         // of the first section, in order
	Data          []byte      `json:"-"`
}

var profileMarkerRE = lazyre.MustCompile(`;\s*Profile:\s*(\w+)`)

// ParsePool reads a pool file: ";" comments, "[section]" headers and
// "key = value" lines; values are whitespace normalized (runs of blanks
// become one space). Only the first section's values are kept; a line
// that is none of these is an error.
func ParsePool(p string, data []byte) (Pool, error) {
	pool := Pool{Path: p, Data: data}
	if m := profileMarkerRE.FindSubmatch(data); m != nil {
		pool.ProfileMarker = string(m[1])
	}
	sections := 0
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#"):
			continue
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			sections++
			if sections == 1 {
				pool.Section = strings.TrimSpace(line[1 : len(line)-1])
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Pool{}, fmt.Errorf("%s:%d: not a key = value line", p, i+1)
		}
		if sections == 0 {
			return Pool{}, fmt.Errorf("%s:%d: a value before the first [section]", p, i+1)
		}
		if sections > 1 {
			continue
		}
		pool.Values = append(pool.Values, PoolValue{
			Key:   strings.TrimSpace(key),
			Value: strings.Join(strings.Fields(value), " "),
			Line:  i + 1,
		})
	}
	if sections == 0 {
		return Pool{}, fmt.Errorf("%s: no [section]", p)
	}
	return pool, nil
}

// Get returns the last value of key.
func (p Pool) Get(key string) (string, bool) {
	val, ok := "", false
	for _, v := range p.Values {
		if v.Key == key {
			val, ok = v.Value, true
		}
	}
	return val, ok
}

// identityKeys are rewritten by gash for every pool, so they are not
// compared with the profile: who runs the pool, its socket, and the log
// paths that move out of the document root (LEDGER HTE-07, HTE-18).
var identityKeys = map[string]bool{
	"user": true, "group": true, "listen": true, "listen.owner": true, "listen.group": true,
	"listen.mode": true, "slowlog": true, "php_admin_value[error_log]": true,
}

// PoolDiff is a pool value that differs from the profile. Pool is "" for
// a profile value the pool lacks; Profile is "" for a pool value the
// profile does not set.
type PoolDiff struct {
	Key     string `json:"key"`
	Pool    string `json:"pool"`
	Profile string `json:"profile"`
	Line    int    `json:"line,omitempty"`
}

// ProfileSource says how the profile of an imported pool was chosen.
type ProfileSource string

// The profile rule of design/hte.md section 10 step 4 and LEDGER HTE-66.
const (
	ProfileFromMarker  ProfileSource = "marker"  // "; Profile: <name>"
	ProfileFromValues  ProfileSource = "values"  // no marker; the values match a profile exactly
	ProfileFromDefault ProfileSource = "default" // no usable marker and no exact match: dev
)

// ProfileMatch is the profile chosen for a legacy pool.
type ProfileMatch struct {
	Profile profiles.Name `json:"profile"`
	Source  ProfileSource `json:"source"`
	Warning string        `json:"warning,omitempty"`
	Diffs   []PoolDiff    `json:"diffs,omitempty"`
}

// Diff compares the pool's values with profile n for docroot.
func (p Pool) Diff(n profiles.Name, docroot string) []PoolDiff {
	want := profiles.Values(n, docroot)
	seen := map[string]bool{}
	var out []PoolDiff
	for _, v := range p.Values {
		if identityKeys[v.Key] {
			continue
		}
		val, _ := p.Get(v.Key)
		if seen[v.Key] {
			continue
		}
		seen[v.Key] = true
		w, ok := want[v.Key]
		if !ok || strings.Join(strings.Fields(w), " ") != val {
			out = append(out, PoolDiff{Key: v.Key, Pool: val, Profile: w, Line: v.Line})
		}
	}
	for _, k := range profileKeys(n, docroot) {
		if !seen[k] {
			out = append(out, PoolDiff{Key: k, Profile: want[k]})
		}
	}
	return out
}

// profileKeys are the keys of a profile in rendering order.
func profileKeys(n profiles.Name, docroot string) []string {
	var out []string
	values := profiles.Values(n, docroot)
	for _, l := range profiles.Lines(n, profiles.Options{Docroot: docroot}) {
		if _, ok := values[l.Key]; ok && l.Key != "" {
			out = append(out, l.Key)
		}
	}
	return out
}

// DetectProfile applies the profile rule: the "; Profile:" marker
// lowercased when it names a profile (an unknown one gives dev with a
// warning); without a marker, the profile whose values match the pool
// exactly; else dev. The differences from the chosen profile are listed.
func DetectProfile(p Pool, docroot string) ProfileMatch {
	if p.ProfileMarker != "" {
		if n, ok := profiles.Parse(strings.ToLower(p.ProfileMarker)); ok {
			return ProfileMatch{Profile: n, Source: ProfileFromMarker, Diffs: p.Diff(n, docroot)}
		}
		return ProfileMatch{Profile: profiles.Dev, Source: ProfileFromDefault,
			Warning: fmt.Sprintf("unknown profile %q in the pool; dev is used", p.ProfileMarker),
			Diffs:   p.Diff(profiles.Dev, docroot)}
	}
	for _, n := range profiles.All() {
		if len(p.Diff(n, docroot)) == 0 {
			return ProfileMatch{Profile: n, Source: ProfileFromValues}
		}
	}
	return ProfileMatch{Profile: profiles.Dev, Source: ProfileFromDefault,
		Warning: "the pool matches no profile; dev is used and the differences are listed",
		Diffs:   p.Diff(profiles.Dev, docroot)}
}

// INIKey returns the php.ini name of a pool key that sets a PHP value
// (php_value[x], php_admin_value[x], php_flag[x], php_admin_flag[x]), so
// that --keep-pool-values can move it to the site's [php_ini].
func INIKey(poolKey string) (string, bool) {
	for _, prefix := range []string{"php_admin_value[", "php_admin_flag[", "php_value[", "php_flag["} {
		if rest, ok := strings.CutPrefix(poolKey, prefix); ok {
			if name, ok := strings.CutSuffix(rest, "]"); ok && name != "" {
				return name, true
			}
		}
	}
	return "", false
}
