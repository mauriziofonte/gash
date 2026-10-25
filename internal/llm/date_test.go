// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// runDate runs llm_date with now as the current time.
func runDate(t *testing.T, now time.Time, env map[string]string, args ...string) result {
	t.Helper()
	m := New()
	tbl := invoke.NewTable()
	tbl.Handle("llm_date", func(c *invoke.Call) int { return m.dateAt(c, now) })
	var out, errb bytes.Buffer
	e := invoke.EnvMap(env)
	c := &invoke.Call{Ctx: context.Background(), Name: "llm_date", Args: args, Env: e, Stdin: bytes.NewReader(nil),
		Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, false, false)}
	rc := tbl.Run(c)
	return result{out.String(), errb.String(), rc}
}

func TestDateModes(t *testing.T) {
	now := time.Date(2026, 10, 25, 14, 3, 5, 987654321, time.UTC)
	env := map[string]string{"TZ": "Europe/Rome"}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, `{"result":"2026-10-25T15:03:05+01:00","epoch":1792936985}`},
		{[]string{"--tz", "UTC"}, `{"result":"2026-10-25T14:03:05Z","epoch":1792936985}`},
		{[]string{"--tz", "-05:30"}, `{"result":"2026-10-25T08:33:05-05:30","epoch":1792936985}`},
		{[]string{"--fmt", "%Y-%m-%d %H:%M:%S %Z %z"}, `{"result":"2026-10-25 15:03:05 CET +0100","epoch":1792936985}`},
		{[]string{"--fmt", "epoch"}, `{"result":"1792936985","epoch":1792936985}`},
		// --diff: calendar days for two dates, exact time otherwise.
		{[]string{"--diff", "2026-10-25", "2026-12-31"}, `{"result":"67d","days":67,"seconds":5788800}`},
		{[]string{"--diff", "2026-12-31", "2026-10-25"}, `{"result":"-67d","days":-67,"seconds":-5788800}`},
		{[]string{"--diff", "2026-03-28", "2026-03-30"}, `{"result":"2d","days":2,"seconds":172800}`},
		{[]string{"--diff", "2026-03-28T12:00:00", "2026-03-30 12:00"}, `{"result":"1d23h","days":1,"seconds":169200}`},
		{[]string{"--diff", "2026-01-01T00:00:00Z", "2026-01-01T00:00:01.5Z"}, `{"result":"1.5s","days":0,"seconds":1.5}`},
		{[]string{"--diff", "today", "tomorrow"}, `{"result":"1d","days":1,"seconds":86400}`},
		{[]string{"--diff", "now", "2026-10-25T15:13:05"}, `{"result":"10m","days":0,"seconds":600}`},
		{[]string{"--diff", "2026-10-26T00:00:00Z", "2026-10-25T00:00:01Z"}, `{"result":"-23h59m59s","days":0,"seconds":-86399}`},
		// --add: months clamp, days follow the calendar, the offset stays.
		{[]string{"--add", "2026-01-31", "1mo"}, `{"result":"2026-02-28","epoch":1772233200}`},
		{[]string{"--add", "2024-01-31", "1mo", "--tz", "UTC"}, `{"result":"2024-02-29","epoch":1709164800}`},
		{[]string{"--add", "2026-10-25", "-1y2mo3d", "--tz", "UTC"}, `{"result":"2025-08-22","epoch":1755820800}`},
		{[]string{"--add", "2026-10-25", "2w", "--tz", "UTC"}, `{"result":"2026-11-08","epoch":1794096000}`},
		{[]string{"--add", "2026-10-25", "12h", "--tz", "UTC"}, `{"result":"2026-10-25T12:00:00Z","epoch":1792929600}`},
		{[]string{"--add", "2026-01-31T10:00:00+05:00", "1h30m"}, `{"result":"2026-01-31T11:30:00+05:00","epoch":1769841000}`},
		{[]string{"--add", "2026-01-31T10:00:00+05:00", "1h30m", "--tz", "UTC"}, `{"result":"2026-01-31T06:30:00Z","epoch":1769841000}`},
		{[]string{"--add", "2026-03-28T12:00:00", "1d"}, `{"result":"2026-03-29T12:00:00+02:00","epoch":1774778400}`},
		{[]string{"--add", "2026-03-28T12:00:00", "24h"}, `{"result":"2026-03-29T13:00:00+02:00","epoch":1774782000}`},
		{[]string{"--add", "now", "-600s", "--fmt", "%Y-%m-%dT%H:%M:%S"}, `{"result":"2026-10-25T14:53:05","epoch":1792936385}`},
		{[]string{"--add", "Mon, 02 Jan 2006 15:04:05 -0700", "1s"}, `{"result":"2006-01-02T15:04:06-07:00","epoch":1136239446}`},
		{[]string{"--add", "2026-10-25T08:00:00.250Z", "250ms"}, `{"result":"2026-10-25T08:00:00.5Z","epoch":1792915200}`},
		{[]string{"--add", "yesterday", "0d"}, `{"result":"2026-10-24","epoch":1792792800}`},
		// --epoch: both ways; 13 digits are milliseconds.
		{[]string{"--epoch", "1700000000", "--tz", "UTC"}, `{"result":"2023-11-14T22:13:20Z","epoch":1700000000}`},
		{[]string{"--epoch", "1700000000123", "--tz", "UTC"}, `{"result":"2023-11-14T22:13:20.123Z","epoch":1700000000}`},
		{[]string{"--epoch", "-1.5", "--tz", "UTC"}, `{"result":"1969-12-31T23:59:58.5Z","epoch":-2}`},
		{[]string{"--epoch", "2026-10-25", "--tz", "UTC"}, `{"result":"2026-10-25T00:00:00Z","epoch":1792886400}`},
		{[]string{"--epoch", "@0", "--tz", "Asia/Kolkata"}, `{"result":"1970-01-01T05:30:00+05:30","epoch":0}`},
		{[]string{"--epoch", "0", "--tz", "UTC", "--fmt", "%a %A %b %B %e|%-m|%j|%u|%w|%V|%G|%U|%W|%I%p|%l%P|%:z|%s|%N|%D|%r|%%"},
			`{"result":"Thu Thursday Jan January  1|1|001|4|4|01|1970|00|00|12AM|12am|+00:00|0|000000000|01/01/70|12:00:00 AM|%","epoch":0}`},
		{[]string{"--epoch", "0", "--tz", "UTC", "--fmt", "%c"}, `{"result":"Thu Jan  1 00:00:00 1970","epoch":0}`},
	} {
		got := runDate(t, now, env, tc.args...)
		require.Equal(t, result{tc.want + "\n", "", 0}, got, "%q", tc.args)
	}
}

func TestDateErrors(t *testing.T) {
	now := time.Date(2026, 10, 25, 14, 3, 5, 0, time.UTC)
	env := map[string]string{"TZ": "UTC"}
	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"--add", "bad", "1d"}, "invalid_date"},
		{[]string{"--diff", "2026-02-30", "2026-03-01"}, "invalid_date"},
		{[]string{"--epoch", "1e9"}, "invalid_date"},
		{[]string{"--epoch", "@x"}, "invalid_date"},
		{[]string{"--add", "2026-10-25", "1x"}, "invalid_duration"},
		{[]string{"--add", "2026-10-25", "1.5d"}, "invalid_duration"},
		{[]string{"--add", "2026-10-25", ""}, "invalid_duration"},
		{[]string{"--add", "2026-10-25", "d"}, "invalid_duration"},
		{[]string{"--tz", "Nowhere/X"}, "invalid_timezone"},
		{[]string{"--tz", "../../etc/passwd"}, "invalid_timezone"},
		{[]string{"--tz", "+25:00"}, "invalid_timezone"},
		{[]string{"--fmt", "%Q"}, "invalid_date_format"},
		{[]string{"--fmt", "words"}, "invalid_date_format"},
		{[]string{"--fmt", "100%"}, "invalid_date_format"},
		{[]string{"--diff", "a"}, "missing_value"},
		{[]string{"--diff", "2026-01-01", "2026-01-02", "--fmt", "date"}, "unexpected_argument"},
		{[]string{"--epoch", "1", "--add", "2026-01-01", "1d"}, "unexpected_argument"},
		{[]string{"2026-01-01"}, "unexpected_argument"},
		{[]string{"--bogus"}, "unknown_option"},
	} {
		got := runDate(t, now, env, tc.args...)
		require.Equal(t, 1, got.RC, "%q", tc.args)
		require.Empty(t, got.Stdout)
		var e struct{ Error, Action string }
		require.NoError(t, json.Unmarshal([]byte(got.Stderr), &e), got.Stderr)
		require.Equal(t, tc.code, e.Error, "%q", tc.args)
		require.Equal(t, "RETRY", e.Action, "%q", tc.args)
	}
}

func TestDateFormatDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0: "0s", time.Second: "1s", -600 * time.Second: "-10m", 90 * time.Minute: "1h30m",
		26*time.Hour + 3*time.Minute + 4*time.Second: "1d2h3m4s", 1500 * time.Millisecond: "1.5s",
	} {
		require.Equal(t, want, formatDuration(d), d.String())
	}
}
