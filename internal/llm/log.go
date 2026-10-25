// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
)

// logTailDefault is the number of lines llm_log prints when --tail and -n
// are not given.
const logTailDefault = 50

// logBlock is the read unit of the reverse reader (plan F9).
const logBlock = 64 << 10

// The log levels, least to most severe, and the words that name them
// (case-insensitive) in a line or in --level.
var (
	logLevelNames = []string{"trace", "debug", "info", "notice", "warning", "error", "critical", "alert", "emergency"}
	logLevelWords = map[string]int{
		"trace": 0, "finest": 0, "finer": 0,
		"debug": 1, "dbg": 1, "verbose": 1, "fine": 1,
		"info": 2, "inf": 2, "information": 2, "informational": 2,
		"notice":  3,
		"warning": 4, "warn": 4, "wrn": 4,
		"error": 5, "err": 5, "severe": 5,
		"critical": 6, "crit": 6, "fatal": 6,
		"alert":     7,
		"emergency": 8, "emerg": 8, "panic": 8,
	}
)

// logSpec is what llm_log selects.
type logSpec struct {
	tail  int
	grep  []*rgRegex
	since time.Time // zero: no --since
	level int       // -1: no --level
	loc   *time.Location
	now   time.Time
}

// logLine is one selected line: its byte offset in the file and its text.
type logLine struct {
	off  int64
	text []byte
	ts   time.Time
	tsOK bool
	lvl  int
}

// logResult is the selection of llm_log, in file order.
type logResult struct {
	lines   []logLine
	omitted int  // lines selected but cut by --tail (counted with --since only)
	sawTime bool // a timestamp was recognized
}

// reverseLines calls fn with the lines of f from the last to the first,
// reading logBlock bytes at a time from the end; fn returns false to
// stop. The final newline ends the last line, it does not start an empty
// one. The text is valid until fn returns.
func reverseLines(ctx context.Context, f *os.File, size int64, fn func(off int64, line []byte) bool) error {
	pos := size
	var win []byte // the bytes [pos, end of the next line to report)
	first := true
	for {
		j := bytes.LastIndexByte(win, '\n')
		if j >= 0 {
			if !fn(pos+int64(j)+1, win[j+1:]) {
				return nil
			}
			win = win[:j]
			continue
		}
		if pos == 0 {
			if len(win) > 0 || !first {
				fn(0, win)
			}
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		n := min(int64(logBlock), pos)
		pos -= n
		nw := make([]byte, int(n)+len(win))
		got, err := f.ReadAt(nw[:n], pos)
		if err != nil && !(errors.Is(err, io.EOF) && int64(got) == n) {
			return err
		}
		copy(nw[n:], win)
		win = nw
		if first {
			first = false
			win = bytes.TrimSuffix(win, []byte{'\n'})
			if len(win) == 0 && pos == 0 {
				// The file is one newline: one empty line.
				fn(0, win)
				return nil
			}
		}
	}
}

// selectLog walks f backwards and keeps the last spec.tail lines that pass
// the filters: --grep per line; --since and --level per entry (a line
// with a timestamp or a level, and the lines below it that have neither,
// such as a stack trace). With --since the walk stops at the first entry
// older than the bound, counting the selected lines --tail cut.
func selectLog(ctx context.Context, f *os.File, size int64, spec *logSpec) (logResult, error) {
	var res logResult
	var picked []logLine // reverse order
	grouped := !spec.since.IsZero() || spec.level >= 0
	var pending []logLine // lines below the next entry head, reverse order
	grep := func(text []byte) bool {
		if len(spec.grep) == 0 {
			return true
		}
		for _, re := range spec.grep {
			if re.Match(text) {
				return true
			}
		}
		return false
	}
	take := func(l logLine) {
		if !grep(l.text) {
			return
		}
		if len(picked) < spec.tail {
			l.text = bytes.Clone(l.text)
			picked = append(picked, l)
			return
		}
		// Only a walk bounded by --since knows how many lines --tail cut.
		if !spec.since.IsZero() {
			res.omitted++
		}
	}
	full := func() bool { return len(picked) >= spec.tail && spec.since.IsZero() }
	err := reverseLines(ctx, f, size, func(off int64, text []byte) bool {
		l := logLine{off: off, text: text, lvl: -1}
		l.ts, l.tsOK, l.lvl = logPrefix(text, spec.loc, spec.now)
		res.sawTime = res.sawTime || l.tsOK
		if !grouped {
			take(l)
			return !full()
		}
		if !l.tsOK && l.lvl < 0 {
			if grep(text) {
				l.text = bytes.Clone(text)
				pending = append(pending, l)
			}
			return true
		}
		if !spec.since.IsZero() && l.tsOK && l.ts.Before(spec.since) {
			return false
		}
		if spec.level < 0 || l.lvl >= spec.level {
			for _, p := range pending {
				p.ts, p.tsOK, p.lvl = l.ts, l.tsOK, l.lvl
				take(p)
			}
			take(l)
		}
		pending = pending[:0]
		return !full()
	})
	if err != nil {
		return res, err
	}
	// The lines left in pending sit above the first entry of the file:
	// they belong to no entry and are left out.
	if !spec.since.IsZero() && !res.sawTime {
		picked, res.omitted = nil, 0
	}
	for i := len(picked) - 1; i >= 0; i-- {
		res.lines = append(res.lines, picked[i])
	}
	return res, nil
}

// logPrefix reads the timestamp a log line starts with and its level.
func logPrefix(line []byte, loc *time.Location, now time.Time) (time.Time, bool, int) {
	b := line
	if len(b) > 200 {
		b = b[:200]
	}
	t, end, ok := logTime(b, loc, now)
	lvl := logLevel(b, end, ok)
	return t, ok, lvl
}

// logLevel is the level named by one of the first words after the
// timestamp (four words, two without a timestamp), -1 when none is.
func logLevel(b []byte, from int, hasTime bool) int {
	words := 2
	if hasTime {
		words = 4
	}
	i := from
	for words > 0 && i < len(b) {
		for i < len(b) && !isAlnum(b[i]) {
			i++
		}
		j := i
		for j < len(b) && isAlnum(b[j]) {
			j++
		}
		if j == i {
			break
		}
		if isLetter(b[i]) && j-i <= 13 {
			var w [13]byte
			for k := i; k < j; k++ {
				w[k-i] = b[k] | 0x20
			}
			if lvl, ok := logLevelWords[string(w[:j-i])]; ok {
				return lvl
			}
			words--
		}
		i = j
	}
	return -1
}

func isLetter(c byte) bool { return (c|0x20) >= 'a' && (c|0x20) <= 'z' }
func isDigit(c byte) bool  { return c >= '0' && c <= '9' }
func isAlnum(c byte) bool  { return isLetter(c) || isDigit(c) }

var monthNames = []string{"jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"}

// monthAt is the month (1-12) of the three-letter name at b[i:], 0 when
// there is none.
func monthAt(b []byte, i int) int {
	if i+3 > len(b) {
		return 0
	}
	s := string([]byte{b[i] | 0x20, b[i+1] | 0x20, b[i+2] | 0x20})
	for k, m := range monthNames {
		if s == m {
			return k + 1
		}
	}
	return 0
}

// num reads n digits at b[i:], -1 when they are not there.
func num(b []byte, i, n int) int {
	if i < 0 || i+n > len(b) {
		return -1
	}
	v := 0
	for k := i; k < i+n; k++ {
		if !isDigit(b[k]) {
			return -1
		}
		v = v*10 + int(b[k]-'0')
	}
	return v
}

// clock reads HH:MM:SS[.frac] at b[i:]: the time of day and the index
// after it.
func clock(b []byte, i int) (h, mi, s, ns, end int, ok bool) {
	h, mi, s = num(b, i, 2), num(b, i+3, 2), num(b, i+6, 2)
	if h < 0 || mi < 0 || s < 0 || b[i+2] != ':' || b[i+5] != ':' || h > 23 || mi > 59 || s > 60 {
		return 0, 0, 0, 0, 0, false
	}
	end = i + 8
	if end < len(b) && (b[end] == '.' || b[end] == ',') && end+1 < len(b) && isDigit(b[end+1]) {
		k, scale := end+1, 100000000
		for k < len(b) && isDigit(b[k]) {
			ns += int(b[k]-'0') * scale
			scale /= 10
			k++
		}
		end = k
	}
	return h, mi, s, ns, end, true
}

// isoAt reads an ISO 8601 date and time at b[i:] (YYYY-MM-DD, also
// YYYY/MM/DD as nginx writes it, then T or a space, HH:MM[:SS[.frac]],
// then an optional zone Z, +HH:MM or +HHMM). A time without a zone is in
// loc. It returns the time and the index after it.
func isoAt(b []byte, i int, loc *time.Location) (time.Time, int, bool) {
	y, mo, d := num(b, i, 4), num(b, i+5, 2), num(b, i+8, 2)
	if y < 0 || mo < 1 || mo > 12 || d < 1 || d > 31 || i+16 > len(b) {
		return time.Time{}, 0, false
	}
	sep := b[i+4]
	if (sep != '-' && sep != '/') || b[i+7] != sep || (b[i+10] != 'T' && b[i+10] != ' ') {
		return time.Time{}, 0, false
	}
	h, mi, s, ns, end := num(b, i+11, 2), num(b, i+14, 2), 0, 0, i+16
	if h < 0 || mi < 0 || b[i+13] != ':' || h > 23 || mi > 59 {
		return time.Time{}, 0, false
	}
	if end+2 < len(b) && b[end] == ':' {
		var ok bool
		if h, mi, s, ns, end, ok = clock(b, i+11); !ok {
			return time.Time{}, 0, false
		}
	}
	zone := loc
	switch {
	case end < len(b) && (b[end] == 'Z' || b[end] == 'z'):
		zone, end = time.UTC, end+1
	case end+2 < len(b) && (b[end] == '+' || b[end] == '-') && num(b, end+1, 2) >= 0:
		zh, zm, k := num(b, end+1, 2), 0, end+3
		if k < len(b) && b[k] == ':' && num(b, k+1, 2) >= 0 {
			zm, k = num(b, k+1, 2), k+3
		} else if num(b, k, 2) >= 0 {
			zm, k = num(b, k, 2), k+2
		}
		off := zh*3600 + zm*60
		if b[end] == '-' {
			off = -off
		}
		zone, end = time.FixedZone("", off), k
	}
	return time.Date(y, time.Month(mo), d, h, mi, s, ns, zone), end, true
}

// logTime reads the timestamp of a log line: ISO 8601 at the start (also
// in brackets, as Laravel and Monolog write it), a syslog "Mon DD
// HH:MM:SS" (the year is the one that puts it at most a day ahead of
// now), an Apache error log "[Www Mon DD HH:MM:SS[.frac] YYYY]", the
// "[DD/Mon/YYYY:HH:MM:SS +ZZZZ]" of nginx and Apache access logs, and an
// ISO 8601 time in the first 64 bytes (JSON lines, RFC 5424). It returns
// the time and the index after it.
func logTime(b []byte, loc *time.Location, now time.Time) (time.Time, int, bool) {
	i := 0
	if len(b) > 0 && b[0] == '[' {
		i = 1
	}
	closeBracket := func(end int) int {
		if i == 1 && end < len(b) && b[end] == ']' {
			return end + 1
		}
		return end
	}
	if t, end, ok := isoAt(b, i, loc); ok {
		return t, closeBracket(end), true
	}
	// syslog: "Oct 25 12:34:56", "Oct  5 12:34:56".
	if mo := monthAt(b, i); mo > 0 && i+15 <= len(b) && b[i+3] == ' ' {
		d := num(b, i+4, 2)
		if b[i+4] == ' ' {
			d = num(b, i+5, 1)
		}
		if h, mi, s, ns, end, ok := clock(b, i+7); ok && d >= 1 && d <= 31 && b[i+6] == ' ' {
			now = now.In(loc)
			t := time.Date(now.Year(), time.Month(mo), d, h, mi, s, ns, loc)
			if t.After(now.Add(24 * time.Hour)) {
				t = t.AddDate(-1, 0, 0)
			}
			return t, closeBracket(end), true
		}
	}
	// Apache error log: "[Sun Oct 25 12:34:56.123456 2026]".
	if i == 1 && len(b) > 24 && b[4] == ' ' && isLetter(b[1]) {
		if mo := monthAt(b, 5); mo > 0 && b[8] == ' ' {
			d := num(b, 9, 2)
			if b[9] == ' ' {
				d = num(b, 10, 1)
			}
			if h, mi, s, ns, end, ok := clock(b, 12); ok && d >= 1 && end+5 < len(b) && b[end] == ' ' {
				if y := num(b, end+1, 4); y > 0 {
					return time.Date(y, time.Month(mo), d, h, mi, s, ns, loc), closeBracket(end + 5), true
				}
			}
		}
	}
	// Access logs: "host - user [25/Oct/2026:12:34:56 +0000]".
	if j := bytes.IndexByte(b[:min(len(b), 96)], '['); j >= 0 && j+27 <= len(b) {
		k := j + 1
		d, mo, y := num(b, k, 2), monthAt(b, k+3), num(b, k+7, 4)
		if d > 0 && mo > 0 && y > 0 && b[k+2] == '/' && b[k+6] == '/' && b[k+11] == ':' {
			if h, mi, s, ns, end, ok := clock(b, k+12); ok {
				zone := loc
				if end+5 < len(b) && b[end] == ' ' && (b[end+1] == '+' || b[end+1] == '-') {
					if z := num(b, end+2, 4); z >= 0 {
						off := (z/100)*3600 + (z%100)*60
						if b[end+1] == '-' {
							off = -off
						}
						zone, end = time.FixedZone("", off), end+6
					}
				}
				if end < len(b) && b[end] == ']' {
					end++
				}
				return time.Date(y, time.Month(mo), d, h, mi, s, ns, zone), end, true
			}
		}
	}
	// An ISO 8601 time near the start: JSON lines, RFC 5424, prefixes.
	for j := 1; j < min(len(b), 64); j++ {
		if isDigit(b[j]) && !isDigit(b[j-1]) {
			if t, end, ok := isoAt(b, j, loc); ok {
				return t, end, true
			}
		}
	}
	return time.Time{}, 0, false
}

// parseSince reads --since: a duration back from now (90s, 10m, 2h, 1d,
// 1w, or a sum such as 1h30m) or an ISO 8601 date or time (a date alone
// is its midnight; no zone means loc).
func parseSince(v string, loc *time.Location, now time.Time) (time.Time, bool) {
	if d, ok := sumDuration(v); ok {
		return now.Add(-d), true
	}
	b := []byte(v)
	if len(b) == 10 {
		b = append(b, " 00:00"...)
	}
	t, end, ok := isoAt(b, 0, loc)
	if !ok || end != len(b) {
		return time.Time{}, false
	}
	return t, true
}

// sumDuration reads N<unit>[N<unit>...] with the units s, m, h, d, w.
func sumDuration(v string) (time.Duration, bool) {
	units := map[byte]time.Duration{'s': time.Second, 'm': time.Minute, 'h': time.Hour, 'd': 24 * time.Hour, 'w': 7 * 24 * time.Hour}
	var total time.Duration
	i := 0
	for i < len(v) {
		j := i
		for j < len(v) && isDigit(v[j]) {
			j++
		}
		if j == i || j-i > 9 || j == len(v) {
			return 0, false
		}
		u, ok := units[v[j]]
		if !ok {
			return 0, false
		}
		total += time.Duration(atoi(v[i:j])) * u
		i = j + 1
	}
	return total, i > 0
}

// logLocation is the zone of timestamps without one: the caller's TZ, or
// the process's local zone.
func logLocation(c *invoke.Call) *time.Location {
	if tz := strings.TrimPrefix(c.Env.Get("TZ"), ":"); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			return l
		}
	}
	return time.Local
}

// log is llm_log FILE [--tail N] [--last-match RE -n N] [--grep RE]
// [--since T] [--level L] (plan F9): the last lines of a log read from
// the end, filtered by pattern, time and level.
func (m *Module) log(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.FormatFlag)
	var p pathTaker
	spec := logSpec{tail: logTailDefault, level: -1, loc: logLocation(c), now: time.Now()}
	var pats []string
	since := ""
	args := c.Args
	for len(args) > 0 {
		n, ok := o.Take(c, args)
		if !ok {
			return 1
		}
		if n > 0 {
			args = args[n:]
			continue
		}
		a := args[0]
		switch a {
		case "--tail", "-n", "--grep", "--last-match", "--since", "--level", "--width":
			v, ok := takeOption(c, args)
			if !ok {
				return 1
			}
			switch a {
			case "--tail", "-n":
				if spec.tail, ok = takePositive(c, "invalid_tail", v); !ok {
					return 1
				}
			case "--grep", "--last-match":
				pats = append(pats, v)
			case "--since":
				since = v
			case "--level":
				lvl, known := logLevelWords[strings.ToLower(v)]
				if !known {
					return fail(c, "invalid_level", v, "Use a level: trace, debug, info, notice, warning, error, critical, alert, emergency")
				}
				spec.level = lvl
			default:
				if o.Width, ok = takePositive(c, "invalid_width", v); !ok {
					return 1
				}
			}
			args = args[2:]
			continue
		}
		if strings.HasPrefix(a, "-") {
			return fail(c, "unknown_option", a, "Use --tail N, --last-match RE, -n N, --grep RE, --since T, --level L, --width N or --format")
		}
		if !p.take(c, a) {
			return 1
		}
		args = args[1:]
	}
	if !p.set {
		return fail(c, "missing_file", "", "Give a log file: llm_log FILE [--tail N]")
	}
	for _, s := range pats {
		re, err := compileRg(s)
		if err != nil {
			return fail(c, "invalid_regex", s, "Patterns use ripgrep's regex syntax: "+err.Error())
		}
		spec.grep = append(spec.grep, re)
	}
	if since != "" {
		t, ok := parseSince(since, spec.loc, spec.now)
		if !ok {
			return fail(c, "invalid_duration", since, "Give a duration back from now (30s, 10m, 2h, 1d, 1w) or an ISO 8601 date or time (2026-10-25, 2026-10-25T14:00)")
		}
		spec.since = t
	}
	targets, ok := m.textTargets(c, []string{p.path}, true)
	if !ok {
		return 1
	}
	t := targets[0]
	f, size, err := openRegular(t.real)
	if err != nil {
		return fail(c, "file_not_found", t.arg, "Check the file path and try again")
	}
	defer func() { _ = f.Close() }()
	if bin, err := sniffBinary(f); err != nil {
		return fail(c, "path_unreadable", t.arg, "The file could not be read")
	} else if bin {
		return fail(c, "binary_file", t.arg, "A binary file (it holds NUL bytes) is not read as text; a compressed log needs zcat")
	}
	res, err := selectLog(c.Ctx, f, size, &spec)
	if err != nil {
		return fail(c, "path_unreadable", t.arg, "The file could not be read")
	}
	if res.omitted > 0 {
		c.NoteTruncated(res.omitted)
	}
	var meta map[string]any
	if !spec.since.IsZero() && !res.sawTime {
		meta = map[string]any{"warning": "no timestamp recognized: --since selected nothing"}
	}
	format := o.FormatOr(llmfmt.Default)
	if format == llmfmt.Default {
		var b bytes.Buffer
		if res.omitted > 0 {
			b.WriteString(llmfmt.OmittedMarker(res.omitted, "lines") + "\n")
		}
		for _, l := range res.lines {
			b.WriteString(llmfmt.Cut(string(l.text), o.Width))
			b.WriteByte('\n')
		}
		_, _ = c.Stdout.Write(b.Bytes())
		llmfmt.Meta(c, meta)
		return 0
	}
	// Rows carry line numbers: one count of the newlines above each line.
	table := llmfmt.Table{Cols: []string{"line", "ts", "level", "text"}}
	var lineNo, prev int64
	for _, l := range res.lines {
		nl, err := countNewlines(c.Ctx, f, prev, l.off)
		if err != nil {
			return fail(c, "path_unreadable", t.arg, "The file could not be read")
		}
		lineNo += nl
		prev = l.off
		row := []any{lineNo + 1, nil, nil, string(l.text)}
		if l.tsOK {
			row[1] = formatLogTime(l.ts, spec.loc)
		}
		if l.lvl >= 0 {
			row[2] = logLevelNames[l.lvl]
		}
		table.Rows = append(table.Rows, row)
	}
	if res.omitted > 0 {
		if meta == nil {
			meta = map[string]any{}
		}
		meta["truncated"] = res.omitted
	}
	o.Write(c, format, table, meta)
	return 0
}

// formatLogTime is a log timestamp in ISO 8601, the fraction kept: with
// the zone the line gave (Z for UTC), without one when it gave none (the
// time is then in loc).
func formatLogTime(t time.Time, loc *time.Location) string {
	s := t.Format("2006-01-02T15:04:05.999999999")
	switch t.Location() {
	case loc:
		return s
	case time.UTC:
		return s + "Z"
	}
	return s + t.Format("-07:00")
}
