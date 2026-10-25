// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLogTime_Formats(t *testing.T) {
	loc := time.FixedZone("CET", 3600)
	now := time.Date(2026, 10, 25, 15, 0, 0, 0, loc)
	for _, tc := range []struct {
		line string
		want string // RFC 3339 with nanoseconds, "" for no timestamp
		lvl  int
	}{
		{"2026-10-25T14:02:01 INFO [customer] x", "2026-10-25T14:02:01+01:00", 2},
		{"2026-10-25 14:02:01.123456 ERROR x", "2026-10-25T14:02:01.123456+01:00", 5},
		{"2026-10-25T14:02:01Z warn x", "2026-10-25T14:02:01Z", 4},
		{"2026-10-25T14:02:01+02:00 x", "2026-10-25T14:02:01+02:00", -1},
		{"2026-10-25T14:02:01.5-0330 x", "2026-10-25T14:02:01.5-03:30", -1},
		{"2026-10-25 14:02 short", "2026-10-25T14:02:00+01:00", -1},
		{"[2026-10-25 14:02:01] production.ERROR: Laravel", "2026-10-25T14:02:01+01:00", 5},
		{"[2026-10-25T14:02:01.000000+00:00] app.WARNING: Monolog", "2026-10-25T14:02:01Z", 4},
		{"2026/10/25 14:02:01 [error] 123#0: nginx", "2026-10-25T14:02:01+01:00", 5},
		{"Oct 25 14:02:01 web01 sshd[123]: error: syslog", "2026-10-25T14:02:01+01:00", 5},
		{"Oct  5 04:02:01 web01 cron[1]: x", "2026-10-05T04:02:01+01:00", -1},
		{"Dec 31 23:59:59 h x", "2025-12-31T23:59:59+01:00", -1}, // a day ahead of now: last year
		{"[Sun Oct 25 14:02:01.123456 2026] [core:error] [pid 1] apache", "2026-10-25T14:02:01.123456+01:00", 5},
		{`127.0.0.1 - - [25/Oct/2026:14:02:01 +0000] "GET / HTTP/1.1" 200`, "2026-10-25T14:02:01Z", -1},
		{`{"time":"2026-10-25T14:02:01Z","level":"critical","msg":"json"}`, "2026-10-25T14:02:01Z", 6},
		{"<34>1 2026-10-25T14:02:01Z host app - - msg", "2026-10-25T14:02:01Z", -1},
		{"    at Foo.bar(Foo.java:12)", "", -1},
		{"ERROR: no timestamp", "", 5},
		{"Caused by: error", "", -1},
		{"2026-13-25T14:02:01 bad month", "", -1},
		{"", "", -1},
	} {
		ts, ok, lvl := logPrefix([]byte(tc.line), loc, now)
		got := ""
		if ok {
			got = ts.Format(time.RFC3339Nano)
		}
		require.Equal(t, tc.want, got, tc.line)
		require.Equal(t, tc.lvl, lvl, tc.line)
	}
}

func TestParseSince(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 10, 25, 15, 0, 0, 0, loc)
	for v, want := range map[string]string{
		"10m":                    "2026-10-25T14:50:00Z",
		"1h30m":                  "2026-10-25T13:30:00Z",
		"2d":                     "2026-10-23T15:00:00Z",
		"1w":                     "2026-10-18T15:00:00Z",
		"2026-10-25":             "2026-10-25T00:00:00Z",
		"2026-10-25T14:00":       "2026-10-25T14:00:00Z",
		"2026-10-25 14:00:30":    "2026-10-25T14:00:30Z",
		"2026-10-25T14:00+02:00": "2026-10-25T14:00:00+02:00",
	} {
		got, ok := parseSince(v, loc, now)
		require.True(t, ok, v)
		require.Equal(t, want, got.Format(time.RFC3339), v)
	}
	for _, v := range []string{"", "10", "m", "10x", "yesterday", "2026-10-25T", "2026-10-25 14:00 extra", "1h-"} {
		_, ok := parseSince(v, loc, now)
		require.False(t, ok, v)
	}
}

// reverseAll is every line reverseLines reports, in file order.
func reverseAll(t *testing.T, content string) []string {
	t.Helper()
	p := filepath.Join(physTemp(t), "f")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	f, err := os.Open(p)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	var out []string
	require.NoError(t, reverseLines(context.Background(), f, int64(len(content)), func(off int64, line []byte) bool {
		require.Equal(t, content[off:off+int64(len(line))], string(line))
		out = append([]string{string(line)}, out...)
		return true
	}))
	return out
}

func TestReverseLines(t *testing.T) {
	require.Empty(t, reverseAll(t, ""))
	require.Equal(t, []string{""}, reverseAll(t, "\n"))
	require.Equal(t, []string{"", ""}, reverseAll(t, "\n\n"))
	require.Equal(t, []string{"a", "b"}, reverseAll(t, "a\nb"))
	require.Equal(t, []string{"a", "b"}, reverseAll(t, "a\nb\n"))
	require.Equal(t, []string{"a", "", "b"}, reverseAll(t, "a\n\nb\n"))
	// Lines across 64 KiB blocks, one longer than three blocks.
	long := strings.Repeat("y", 3*logBlock+17)
	var want []string
	var b strings.Builder
	for i := range 5000 {
		l := fmt.Sprintf("line %d %s", i, strings.Repeat("x", i%97))
		if i == 2500 {
			l = long
		}
		want = append(want, l)
		b.WriteString(l + "\n")
	}
	require.Equal(t, want, reverseAll(t, b.String()))
}

// logFixture is a Laravel log with one entry per minute up to now, an
// ERROR every fourth with a two-line stack trace.
func logFixture(t *testing.T, now time.Time) string {
	t.Helper()
	var b strings.Builder
	for i := 200; i >= 1; i-- {
		ts := now.Add(-time.Duration(i) * time.Minute).UTC().Format("2006-01-02 15:04:05")
		lvl := []string{"INFO", "DEBUG", "WARNING", "ERROR"}[i%4]
		fmt.Fprintf(&b, "[%s] production.%s: message %d\n", ts, lvl, i)
		if lvl == "ERROR" {
			b.WriteString("#0 /app/x.php(12): trace\n#1 {main}\n")
		}
	}
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "laravel.log"), b.String())
	return d
}

func TestLog_Selections(t *testing.T) {
	now := time.Now()
	d := logFixture(t, now)
	env := baseEnv(physTemp(t), map[string]string{"TZ": "UTC"})
	run := func(args ...string) result {
		return runGo(t, New(), d, env, "llm_log", append([]string{"laravel.log"}, args...)...)
	}
	lines := func(r result) []string {
		require.Equal(t, 0, r.RC, r.Stderr)
		return strings.Split(strings.TrimSuffix(r.Stdout, "\n"), "\n")
	}
	// Default: the last 50 lines, as tail -n 50.
	all := strings.Split(strings.TrimSuffix(string(must(os.ReadFile(filepath.Join(d, "laravel.log")))), "\n"), "\n")
	require.Equal(t, all[len(all)-50:], lines(run()))
	require.Equal(t, all[len(all)-3:], lines(run("--tail", "3")))

	// --last-match RE -n N: the last N matching lines, as grep | tail.
	got := lines(run("--last-match", `message 1[0-9]$`, "-n", "3"))
	require.Equal(t, []string{"message 12", "message 11", "message 10"}, suffixes(got, ": "))

	// --since: whole entries (the stack trace below an ERROR comes along).
	got = lines(run("--since", "10m"))
	require.Equal(t, "production.DEBUG: message 9", afterTime(got[0]))
	require.Equal(t, "#0 /app/x.php(12): trace", got[3])
	require.Equal(t, "production.DEBUG: message 1", afterTime(got[len(got)-1]))

	// --level error: ERROR entries and their traces; --tail cuts lines.
	got = lines(run("--level", "error", "--tail", "6"))
	require.Equal(t, []string{"production.ERROR: message 7", "#0 /app/x.php(12): trace", "#1 {main}", "production.ERROR: message 3", "#0 /app/x.php(12): trace", "#1 {main}"},
		[]string{afterTime(got[0]), got[1], got[2], afterTime(got[3]), got[4], got[5]})

	// --since with more lines than --tail: the cut is marked.
	got = lines(run("--since", "30m", "--level", "err", "--tail", "3"))
	require.Equal(t, "[... 18 lines omitted]", got[0])
	require.Len(t, got, 4)

	// --grep applies per line, inside the --level entries.
	got = lines(run("--level", "error", "--grep", "trace", "-n", "2"))
	require.Equal(t, []string{"#0 /app/x.php(12): trace", "#0 /app/x.php(12): trace"}, got)

	// Rows carry the line number, the timestamp and the level.
	r := run("--level", "error", "--tail", "1", "--format", "tsv")
	require.Equal(t, 0, r.RC)
	rows := strings.Split(strings.TrimSuffix(r.Stdout, "\n"), "\n")
	require.Equal(t, "line\tts\tlevel\ttext", rows[0])
	// The last ERROR is message 3; messages 2 and 1 follow its trace.
	require.Equal(t, fmt.Sprint(len(all)-2), strings.Split(rows[1], "\t")[0])
	require.Equal(t, "error", strings.Split(rows[1], "\t")[2])
	require.Equal(t, "#1 {main}", strings.Split(rows[1], "\t")[3])

	// --width cuts each line.
	got = lines(run("--tail", "1", "--width", "5"))
	require.Equal(t, []string{"[202…"}, got)
}

func TestLog_SinceWithoutTimestamps(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "plain.log"), "a\nb\n")
	r := runGo(t, New(), d, baseEnv(physTemp(t), nil), "llm_log", "plain.log", "--since", "1h")
	require.Equal(t, result{"", `{"warning":"no timestamp recognized: --since selected nothing"}` + "\n", 0}, r)
}

func TestLog_Errors(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "a.log"), "x\n")
	mustWrite(t, filepath.Join(d, "b.log"), "x\x00\n")
	mustWrite(t, filepath.Join(d, "secrets.log"), "x\n")
	env := baseEnv(physTemp(t), nil)
	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{}, "missing_file"},
		{[]string{"a.log", "a.log"}, "unexpected_argument"},
		{[]string{"nope.log"}, "file_not_found"},
		{[]string{"b.log"}, "binary_file"},
		{[]string{"a.log", "--since", "soon"}, "invalid_duration"},
		{[]string{"a.log", "--level", "loud"}, "invalid_level"},
		{[]string{"a.log", "--tail", "0"}, "invalid_tail"},
		{[]string{"a.log", "--grep", "("}, "invalid_regex"},
		{[]string{"a.log", "--head", "1"}, "unknown_option"},
		{[]string{"a.log", "--width", "x"}, "invalid_width"},
		{[]string{"/etc/shadow"}, "forbidden_path"},
	} {
		r := runGo(t, New(), d, env, "llm_log", tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, tc.args)
		require.Empty(t, r.Stdout, tc.args)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// afterTime drops the "[YYYY-MM-DD HH:MM:SS] " prefix.
func afterTime(s string) string {
	if i := strings.Index(s, "] "); i >= 0 {
		return s[i+2:]
	}
	return s
}

// suffixes is the part of each line after its last sep.
func suffixes(lines []string, sep string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l[strings.LastIndex(l, sep)+len(sep):]
	}
	return out
}
