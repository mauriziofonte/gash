// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
)

// llm_date (plan V4): date arithmetic without GNU or BSD date and without
// a Python interpreter, the same on Linux and macOS (pure Go; time zones
// from the system's zoneinfo).
//
//	llm_date                       now: {"result":"...","epoch":N}
//	llm_date --epoch 1700000000    a Unix time as a date (13 digits: ms)
//	llm_date --epoch 2026-10-25    a date as a Unix time
//	llm_date --add DATE DURATION   {"result":"...","epoch":N}
//	llm_date --diff A B            B - A: {"result":"67d","days":67,"seconds":N}
//
// A date is now, today, yesterday, tomorrow, YYYY-MM-DD, an ISO 8601 /
// RFC 3339 date and time (T or a blank between them; seconds, fraction
// and offset optional), an RFC 1123/2822 date, or @EPOCH. One without an
// offset is in --tz (default: TZ, else the system's zone). The result
// keeps the input's offset unless --tz is given; --fmt formats it.

// dateFormats are the layouts a date and time is read with, tried in
// order (a date alone is handled first).
var dateFormats = []string{
	"2006-01-02T15:04:05.999999999Z07:00",
	"2006-01-02T15:04:05.999999999Z0700",
	"2006-01-02T15:04:05.999999999Z07",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02T15:04Z07:00",
	"2006-01-02T15:04",
	time.RFC1123Z,
	time.RFC1123,
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 MST",
	"2 Jan 2006 15:04:05 -0700",
}

// dateHint is the RETRY hint of an unparsable date.
const dateHint = "Use now, today, yesterday, tomorrow, YYYY-MM-DD, YYYY-MM-DDTHH:MM[:SS][Z|+HH:MM], an RFC 2822 date or @EPOCH"

// parsedDate is a date argument: its instant and whether it was a date
// without a time (YYYY-MM-DD, today, ...).
type parsedDate struct {
	t        time.Time
	dateOnly bool
}

// parseDate reads a date argument in loc (for one without an offset).
func parseDate(s string, now time.Time, loc *time.Location) (parsedDate, bool) {
	s = strings.TrimSpace(s)
	day := func(t time.Time) time.Time {
		y, m, d := t.In(loc).Date()
		return time.Date(y, m, d, 0, 0, 0, 0, loc)
	}
	switch strings.ToLower(s) {
	case "now":
		return parsedDate{t: now.In(loc)}, true
	case "today":
		return parsedDate{t: day(now), dateOnly: true}, true
	case "yesterday":
		return parsedDate{t: day(now).AddDate(0, 0, -1), dateOnly: true}, true
	case "tomorrow":
		return parsedDate{t: day(now).AddDate(0, 0, 1), dateOnly: true}, true
	}
	if e, ok := strings.CutPrefix(s, "@"); ok {
		t, ok := epochTime(e)
		if !ok {
			return parsedDate{}, false
		}
		return parsedDate{t: t.In(loc)}, true
	}
	if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
		return parsedDate{t: t, dateOnly: true}, true
	}
	if len(s) > 10 && s[10] == ' ' {
		s = s[:10] + "T" + s[11:]
	}
	for _, f := range dateFormats {
		if t, err := time.ParseInLocation(f, s, loc); err == nil {
			return parsedDate{t: t}, true
		}
	}
	return parsedDate{}, false
}

// epochTime reads a Unix time: seconds, with an optional fraction; an
// integer of 13 or more digits is milliseconds.
func epochTime(s string) (time.Time, bool) {
	if s == "" || strings.ContainsAny(s, "eEnN_xX") {
		return time.Time{}, false
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if len(strings.TrimLeft(s, "+-")) >= 13 {
			return time.UnixMilli(n), true
		}
		return time.Unix(n, 0), true
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(f, 0) || math.Abs(f) > 1e15 {
		return time.Time{}, false
	}
	sec := math.Floor(f)
	return time.Unix(int64(sec), int64(math.Round((f-sec)*1e9))), true
}

// dateDuration is a parsed DURATION: calendar years, months and days
// (added on the calendar, so a day across a DST change keeps the wall
// time) and an exact remainder.
type dateDuration struct {
	years, months, days int
	exact               time.Duration
	timeUnits           bool // h, m, s or ms appear
}

// parseDateDuration reads a DURATION: an optional sign, then number+unit
// pairs: y, mo, w, d, h, m, s, ms (e.g. 3d, -600s, 1h30m, 1y2mo, +2w).
func parseDateDuration(s string) (dateDuration, bool) {
	var d dateDuration
	sign := 1
	switch {
	case strings.HasPrefix(s, "-"):
		sign, s = -1, s[1:]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}
	if s == "" {
		return d, false
	}
	for s != "" {
		i := 0
		for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
			i++
		}
		if i == 0 || i > 12 {
			return d, false
		}
		num := s[:i]
		s = s[i:]
		j := 0
		for j < len(s) && s[j] >= 'a' && s[j] <= 'z' {
			j++
		}
		unit := s[:j]
		s = s[j:]
		whole, err := strconv.Atoi(num)
		frac := err != nil
		f, ferr := strconv.ParseFloat(num, 64)
		if ferr != nil {
			return d, false
		}
		switch unit {
		case "y", "mo", "w", "d":
			if frac {
				return d, false // a calendar unit takes whole numbers
			}
			switch unit {
			case "y":
				d.years += sign * whole
			case "mo":
				d.months += sign * whole
			case "w":
				d.days += sign * 7 * whole
			default:
				d.days += sign * whole
			}
		case "h", "m", "s", "ms":
			per := map[string]float64{"h": 3600e9, "m": 60e9, "s": 1e9, "ms": 1e6}[unit]
			v := f * per
			if v > float64(math.MaxInt64)/2 {
				return d, false
			}
			d.exact += time.Duration(float64(sign) * v)
			d.timeUnits = true
		default:
			return d, false
		}
	}
	return d, true
}

// addDate adds d to t: years and months clamped to the month's last day
// (Jan 31 + 1mo is Feb 28 or 29), then days on the calendar, then the
// exact part.
func addDate(t time.Time, d dateDuration) time.Time {
	if d.years != 0 || d.months != 0 {
		y, m, day := t.Date()
		total := int(m) - 1 + d.months + 12*d.years
		ny, nm := y+floorDiv(total, 12), time.Month(floorMod(total, 12)+1)
		last := time.Date(ny, nm+1, 0, 0, 0, 0, 0, time.UTC).Day()
		h, mi, sec := t.Clock()
		t = time.Date(ny, nm, min(day, last), h, mi, sec, t.Nanosecond(), t.Location())
	}
	if d.days != 0 {
		t = t.AddDate(0, 0, d.days)
	}
	return t.Add(d.exact)
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func floorMod(a, b int) int { return a - floorDiv(a, b)*b }

// dateLocation reads --tz: an IANA name, UTC, Local, or an offset
// (+02:00, -0530, Z).
func dateLocation(name string) (*time.Location, bool) {
	switch name {
	case "Z", "z", "UTC", "utc":
		return time.UTC, true
	}
	if name != "" && (name[0] == '+' || name[0] == '-') {
		digits := strings.ReplaceAll(name[1:], ":", "")
		if len(digits) == 2 {
			digits += "00"
		}
		if len(digits) == 4 && strings.Trim(digits, "0123456789") == "" {
			h, _ := strconv.Atoi(digits[:2])
			m, _ := strconv.Atoi(digits[2:])
			if h <= 23 && m <= 59 {
				off := (h*60 + m) * 60
				if name[0] == '-' {
					off = -off
				}
				return time.FixedZone(name, off), true
			}
		}
		return nil, false
	}
	if name == "" || strings.Contains(name, "..") || strings.HasPrefix(name, "/") {
		return nil, false
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, false
	}
	return loc, true
}

// formatDuration is a duration as days, hours, minutes and seconds:
// 67d, 1d2h3m4s, -600s, 1.5s, 0s.
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	sign := ""
	if d < 0 {
		sign = "-"
		if d == math.MinInt64 {
			d++
		}
		d = -d
	}
	days := d / (24 * time.Hour)
	d %= 24 * time.Hour
	h := d / time.Hour
	d %= time.Hour
	m := d / time.Minute
	d %= time.Minute
	var b strings.Builder
	b.WriteString(sign)
	for _, p := range []struct {
		n    time.Duration
		unit string
	}{{days, "d"}, {h, "h"}, {m, "m"}} {
		if p.n > 0 {
			b.WriteString(strconv.FormatInt(int64(p.n), 10) + p.unit)
		}
	}
	if d > 0 {
		b.WriteString(strconv.FormatFloat(d.Seconds(), 'f', -1, 64) + "s")
	}
	return b.String()
}

var (
	weekdays = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	months   = []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
)

var errDateFormat = errors.New("unknown directive")

// strftime formats t as date(1) +FORMAT does, with the directives GNU and
// BSD date share (and GNU's %N, %P, %:z), in the C locale; - after % drops
// the padding (%-d).
func strftime(t time.Time, f string) (string, error) {
	var b strings.Builder
	pad := func(n, width int, fill byte, nopad bool) {
		s := strconv.Itoa(n)
		if !nopad {
			for len(s) < width {
				s = string(fill) + s
			}
		}
		b.WriteString(s)
	}
	for i := 0; i < len(f); i++ {
		c := f[i]
		if c != '%' {
			b.WriteByte(c)
			continue
		}
		i++
		if i >= len(f) {
			return "", errDateFormat
		}
		nopad := false
		if f[i] == '-' {
			nopad = true
			i++
			if i >= len(f) {
				return "", errDateFormat
			}
		}
		if f[i] == ':' {
			if i+1 < len(f) && f[i+1] == 'z' {
				b.WriteString(t.Format("-07:00"))
				i++
				continue
			}
			return "", errDateFormat
		}
		y, mo, d := t.Date()
		h, mi, s := t.Clock()
		switch f[i] {
		case 'Y':
			pad(y, 4, '0', nopad)
		case 'C':
			pad(y/100, 2, '0', nopad)
		case 'y':
			pad(y%100, 2, '0', nopad)
		case 'm':
			pad(int(mo), 2, '0', nopad)
		case 'd':
			pad(d, 2, '0', nopad)
		case 'e':
			pad(d, 2, ' ', nopad)
		case 'H':
			pad(h, 2, '0', nopad)
		case 'k':
			pad(h, 2, ' ', nopad)
		case 'I', 'l':
			h12 := h % 12
			if h12 == 0 {
				h12 = 12
			}
			fill := byte('0')
			if f[i] == 'l' {
				fill = ' '
			}
			pad(h12, 2, fill, nopad)
		case 'M':
			pad(mi, 2, '0', nopad)
		case 'S':
			pad(s, 2, '0', nopad)
		case 'N':
			pad(t.Nanosecond(), 9, '0', false)
		case 'j':
			pad(t.YearDay(), 3, '0', nopad)
		case 'p', 'P':
			ap := "AM"
			if h >= 12 {
				ap = "PM"
			}
			if f[i] == 'P' {
				ap = strings.ToLower(ap)
			}
			b.WriteString(ap)
		case 'a':
			b.WriteString(weekdays[t.Weekday()][:3])
		case 'A':
			b.WriteString(weekdays[t.Weekday()])
		case 'b', 'h':
			b.WriteString(months[mo-1][:3])
		case 'B':
			b.WriteString(months[mo-1])
		case 'u':
			wd := int(t.Weekday())
			if wd == 0 {
				wd = 7
			}
			b.WriteString(strconv.Itoa(wd))
		case 'w':
			b.WriteString(strconv.Itoa(int(t.Weekday())))
		case 'V':
			_, w := t.ISOWeek()
			pad(w, 2, '0', nopad)
		case 'G':
			iy, _ := t.ISOWeek()
			pad(iy, 4, '0', nopad)
		case 'U':
			pad((t.YearDay()+6-int(t.Weekday()))/7, 2, '0', nopad)
		case 'W':
			pad((t.YearDay()+6-(int(t.Weekday())+6)%7)/7, 2, '0', nopad)
		case 's':
			b.WriteString(strconv.FormatInt(t.Unix(), 10))
		case 'z':
			b.WriteString(t.Format("-0700"))
		case 'Z':
			name, _ := t.Zone()
			if name == "" || name[0] == '+' || name[0] == '-' {
				name = t.Format("-07")
				if _, off := t.Zone(); off%3600 != 0 {
					name = t.Format("-0700")
				}
			}
			b.WriteString(name)
		case 'F':
			b.WriteString(t.Format("2006-01-02"))
		case 'T':
			b.WriteString(t.Format("15:04:05"))
		case 'R':
			b.WriteString(t.Format("15:04"))
		case 'D':
			b.WriteString(t.Format("01/02/06"))
		case 'r':
			b.WriteString(t.Format("03:04:05 PM"))
		case 'c':
			b.WriteString(t.Format("Mon Jan _2 15:04:05 2006"))
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case '%':
			b.WriteByte('%')
		default:
			return "", errDateFormat
		}
	}
	return b.String(), nil
}

// dateText is t as --fmt F asks (RFC 3339 by default; a date alone for a
// date-only result).
func dateText(t time.Time, f string, dateOnly bool) (string, error) {
	switch {
	case f == "" && dateOnly:
		return t.Format("2006-01-02"), nil
	case f == "" || f == "rfc3339" || f == "iso":
		return t.Format(time.RFC3339Nano), nil
	case f == "date":
		return t.Format("2006-01-02"), nil
	case f == "time":
		return t.Format("15:04:05"), nil
	case f == "epoch":
		return strconv.FormatInt(t.Unix(), 10), nil
	case f == "rfc2822":
		return t.Format(time.RFC1123Z), nil
	case strings.Contains(f, "%"):
		return strftime(t, f)
	}
	return "", errDateFormat
}

// date is llm_date.
func (m *Module) date(c *invoke.Call) int { return m.dateAt(c, time.Now()) }

// dateAt is llm_date with now as the current time.
func (m *Module) dateAt(c *invoke.Call, now time.Time) int {
	if helpAnywhere(c) {
		return 0
	}
	now = now.Truncate(time.Second) // now is whole seconds, as date(1) prints it
	mode := ""
	var ops []string
	tz, tzSet, fmtSpec := "", false, ""
	args := c.Args
	for len(args) > 0 {
		w := args[0]
		need := 0
		switch w {
		case "--diff", "--add":
			need = 2
		case "--epoch", "--tz", "--fmt":
			need = 1
		default:
			if strings.HasPrefix(w, "-") && w != "-" {
				return fail(c, "unknown_option", w, "Use --diff A B, --add DATE DURATION, --epoch X, --tz ZONE, --fmt FORMAT")
			}
			return fail(c, "unexpected_argument", w, "Dates go after --diff, --add or --epoch")
		}
		if len(args) < need+1 {
			return fail(c, "missing_value", w, w+" needs "+map[int]string{1: "a value", 2: "two values"}[need])
		}
		vals := args[1 : need+1]
		switch w {
		case "--tz":
			tz, tzSet = vals[0], true
		case "--fmt":
			fmtSpec = vals[0]
		default:
			if mode != "" && mode != w {
				return fail(c, "unexpected_argument", w, "Use one of --diff, --add and --epoch per call")
			}
			mode, ops = w, vals
		}
		args = args[need+1:]
	}
	loc := time.Local
	if !tzSet {
		if v, ok := c.Env.Lookup("TZ"); ok && v != "" {
			if l, ok := dateLocation(strings.TrimPrefix(v, ":")); ok {
				loc = l
			}
		}
	} else {
		l, ok := dateLocation(tz)
		if !ok {
			return fail(c, "invalid_timezone", tz, "Use an IANA zone (Europe/Rome, America/New_York), UTC, or an offset like +02:00")
		}
		loc = l
	}
	if mode == "--diff" && fmtSpec != "" {
		return fail(c, "unexpected_argument", "--fmt", "--fmt formats a date; --diff prints a duration")
	}
	if fmtSpec != "" {
		if _, err := dateText(time.Unix(0, 0).UTC(), fmtSpec, false); err != nil {
			return fail(c, "invalid_date_format", fmtSpec, "Use rfc3339, date, time, epoch, rfc2822, or a date(1) format such as '%Y-%m-%d %H:%M:%S'")
		}
	}
	date := func(s string) (parsedDate, bool) {
		p, ok := parseDate(s, now, loc)
		if !ok {
			fail(c, "invalid_date", s, dateHint)
		}
		return p, ok
	}
	out := func(t time.Time, dateOnly bool) int {
		if tzSet {
			t = t.In(loc)
		}
		text, _ := dateText(t, fmtSpec, dateOnly)
		b := []byte(`{"result":`)
		b = contract.AppendString(b, text, contract.EscapeJQ)
		b = append(b, `,"epoch":`...)
		b = strconv.AppendInt(b, t.Unix(), 10)
		_, _ = c.Stdout.Write(append(b, '}', '\n'))
		return 0
	}
	switch mode {
	case "--diff":
		a, ok := date(ops[0])
		if !ok {
			return 1
		}
		z, ok := date(ops[1])
		if !ok {
			return 1
		}
		d := z.t.Sub(a.t)
		if a.dateOnly && z.dateOnly {
			// Calendar days: a DST change in between is not an hour off.
			ay, am, ad := a.t.Date()
			zy, zm, zd := z.t.Date()
			d = time.Date(zy, zm, zd, 0, 0, 0, 0, time.UTC).Sub(time.Date(ay, am, ad, 0, 0, 0, 0, time.UTC))
		}
		b := []byte(`{"result":`)
		b = contract.AppendString(b, formatDuration(d), contract.EscapeJQ)
		b = append(b, `,"days":`...)
		b = strconv.AppendInt(b, int64(d/(24*time.Hour)), 10)
		b = append(b, `,"seconds":`...)
		if d%time.Second == 0 {
			b = strconv.AppendInt(b, int64(d/time.Second), 10)
		} else {
			b = strconv.AppendFloat(b, d.Seconds(), 'f', -1, 64)
		}
		_, _ = c.Stdout.Write(append(b, '}', '\n'))
		return 0
	case "--add":
		p, ok := date(ops[0])
		if !ok {
			return 1
		}
		d, ok := parseDateDuration(ops[1])
		if !ok {
			return fail(c, "invalid_duration", ops[1], "Use number+unit pairs: y, mo, w, d, h, m, s, ms (3d, -600s, 1h30m, 1y2mo); calendar units take whole numbers")
		}
		return out(addDate(p.t, d), p.dateOnly && !d.timeUnits)
	case "--epoch":
		x := strings.TrimSpace(ops[0])
		if t, ok := epochTime(x); ok {
			return out(t.In(loc), false)
		}
		p, ok := date(x)
		if !ok {
			return 1
		}
		return out(p.t, false)
	}
	return out(now.In(loc), false)
}
