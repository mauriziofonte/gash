// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/invoke"
)

// The --since T of every 3.4 function (llm_find, llm_tree, llm_log, ...)
// is read here, so that one value means the same instant everywhere.

// envLocation is the caller's time zone: TZ of the call's environment,
// the process zone when it is unset or unknown.
func envLocation(c *invoke.Call) *time.Location {
	tz := strings.TrimPrefix(c.Env.Get("TZ"), ":")
	if tz == "" {
		return time.Local
	}
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	return time.Local
}

// sinceLayouts are the dates --since reads in the caller's zone.
var sinceLayouts = []string{
	"2006-01-02", "2006-01-02T15:04", "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02 15:04:05",
}

// parseSince reads --since T: an age back from now (a number with s, m,
// h, d or w, the pairs adding up: 90s, 1h30m, 2d, 1.5h), a date (local
// midnight), a date and time (local, or RFC 3339 with its zone), or @EPOCH
// seconds.
func parseSince(v string, now time.Time, loc *time.Location) (time.Time, bool) {
	if v == "" {
		return time.Time{}, false
	}
	if rest, ok := strings.CutPrefix(v, "@"); ok {
		n, err := strconv.ParseInt(rest, 10, 64)
		if err != nil || n < 0 || n > 1<<40 {
			return time.Time{}, false
		}
		return time.Unix(n, 0), true
	}
	if d, ok := parseAge(v); ok {
		return now.Add(-d), true
	}
	for _, l := range sinceLayouts {
		if t, err := time.ParseInLocation(l, v, loc); err == nil {
			return t, true
		}
	}
	for _, l := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02T15:04Z07:00"} {
		if t, err := time.Parse(l, v); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// ageUnits are the units of an age, in seconds.
var ageUnits = map[byte]float64{'s': 1, 'm': 60, 'h': 3600, 'd': 86400, 'w': 7 * 86400}

// parseAge reads an age: one or more NUMBER UNIT pairs, at most 100 years.
func parseAge(v string) (time.Duration, bool) {
	total := 0.0
	for i := 0; i < len(v); {
		j := i
		for j < len(v) && (v[j] >= '0' && v[j] <= '9' || v[j] == '.') {
			j++
		}
		if j == i || j >= len(v) {
			return 0, false
		}
		n, err := strconv.ParseFloat(v[i:j], 64)
		unit, ok := ageUnits[v[j]]
		if err != nil || !ok {
			return 0, false
		}
		total += n * unit
		i = j + 1
	}
	if total > 100*365*86400 {
		return 0, false
	}
	return time.Duration(total * float64(time.Second)), true
}
