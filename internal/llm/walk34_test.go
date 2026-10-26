// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/llmfmt"
)

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	loc := time.FixedZone("X", 3600)
	for _, tc := range []struct {
		in   string
		want time.Time
		ok   bool
	}{
		{"30s", now.Add(-30 * time.Second), true},
		{"15m", now.Add(-15 * time.Minute), true},
		{"2h", now.Add(-2 * time.Hour), true},
		{"3d", now.Add(-72 * time.Hour), true},
		{"1w", now.Add(-7 * 24 * time.Hour), true},
		{"1h30m", now.Add(-90 * time.Minute), true},
		{"1.5h", now.Add(-90 * time.Minute), true},
		{"2026-10-20", time.Date(2026, 10, 20, 0, 0, 0, 0, loc), true},
		{"2026-10-20T14:30", time.Date(2026, 10, 20, 14, 30, 0, 0, loc), true},
		{"2026-10-20 14:30:05", time.Date(2026, 10, 20, 14, 30, 5, 0, loc), true},
		{"2026-10-20T14:30:05Z", time.Date(2026, 10, 20, 14, 30, 5, 0, time.UTC), true},
		{"2026-10-20T14:30:05+02:00", time.Date(2026, 10, 20, 12, 30, 5, 0, time.UTC), true},
		{"@1700000000", time.Unix(1700000000, 0), true},
		{"", time.Time{}, false},
		{"h", time.Time{}, false},
		{"5", time.Time{}, false},
		{"5y", time.Time{}, false},
		{"5m3", time.Time{}, false},
		{"-5m", time.Time{}, false},
		{"@x", time.Time{}, false},
		{"@-1", time.Time{}, false},
		{"99999w", time.Time{}, false},
		{"2026-13-01", time.Time{}, false},
		{"yesterday", time.Time{}, false},
	} {
		got, ok := parseSince(tc.in, now, loc)
		require.Equal(t, tc.ok, ok, tc.in)
		if ok {
			require.True(t, tc.want.Equal(got), "%s: %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestMtimeFilter(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f := mtimeFilter{newer: t0, hasNewer: true}
	require.False(t, f.ok(t0), "--newer is strictly after")
	require.True(t, f.ok(t0.Add(time.Nanosecond)))
	f = mtimeFilter{since: t0, hasSince: true}
	require.True(t, f.ok(t0), "--since includes T")
	require.False(t, f.ok(t0.Add(-time.Second)))
	require.True(t, mtimeFilter{}.ok(time.Time{}))
	require.Equal(t, "2026-01-01T00:00:00Z", fmtMtime(t0.In(time.FixedZone("X", -7200))))
}

func TestWalkKeys(t *testing.T) {
	for in, want := range map[string]string{
		"a.php": "php", "A.PHP": "php", "x.tar.gz": "gz", ".bashrc": "(no ext)", "Makefile": "(no ext)",
		"..x": "x", ".": "(no ext)", "trailing.": "", "é.TXT": "txt",
	} {
		require.Equal(t, want, extKey(in), in)
	}
	require.Equal(t, ".", dirKey("a.php"))
	require.Equal(t, ".", dirKey(""))
	require.Equal(t, "src", dirKey("src/x/a.php"))
	require.Equal(t, "f", typeKey('f'))
}

func TestWalkExcluded(t *testing.T) {
	for _, tc := range []struct {
		globs     []string
		rel, name string
		want      bool
	}{
		{[]string{"tests"}, "a/tests", "tests", true},
		{[]string{"*.min.js"}, "web/x.min.js", "x.min.js", true},
		{[]string{"web/*.js"}, "web/x.js", "x.js", true},
		{[]string{"web/*.js"}, "a/web/x.js", "x.js", false},
		{[]string{"*"}, "a/b", "b", true},
		{[]string{"a/*"}, "a/b/c", "c", false},
		{[]string{"x", "y*"}, "y1", "y1", true},
		{nil, "x", "x", false},
	} {
		require.Equal(t, tc.want, walkExcluded(tc.globs, tc.rel, tc.name), "%v %s", tc.globs, tc.rel)
	}
}

func TestWalkSelKeep(t *testing.T) {
	s := &walkSel{prune: findPruned, badRel: "\n", pattern: "*.php", types: "f", maxDepth: -1}
	for _, tc := range []struct {
		name, rel   string
		typ         byte
		keep, enter bool
	}{
		{"a.php", "a.php", 'f', true, true},
		{"a.js", "a.js", 'f', false, true},
		{"vendor", "vendor", 'd', false, false},
		{"src", "src", 'd', false, true},
		{"a.php", "n\nl/a.php", 'f', false, false},
		{"x.php", "x.php", 'l', false, true},
		{"x.php", "x.php", '?', false, true},
	} {
		k, e := s.keep(tc.name, tc.rel, tc.typ, time.Time{})
		require.Equal(t, [2]bool{tc.keep, tc.enter}, [2]bool{k, e}, tc.rel)
	}
	s.prune = nil
	k, e := s.keep("vendor", "vendor", 'd', time.Time{})
	require.False(t, k)
	require.True(t, e, "--all enters vendor")
	s.tracked = &trackedSet{files: map[string]bool{"a.php": true}, dirs: map[string]bool{"": true}}
	k, _ = s.keep("b.php", "b.php", 'f', time.Time{})
	require.False(t, k)
	k, _ = s.keep("a.php", "a.php", 'f', time.Time{})
	require.True(t, k)
}

func TestKeeperMatchesCap(t *testing.T) {
	for _, b := range [][2]int{{0, 0}, {3, 0}, {0, 3}, {2, 2}, {5, 5}, {1, 9}} {
		for n := 0; n < 14; n++ {
			o := llmfmt.New(llmfmt.BudgetFlags)
			o.Head, o.Tail = b[0], b[1]
			k := keeper[int]{head: b[0], tail: b[1]}
			all := make([]int, n)
			for i := 0; i < n; i++ {
				all[i] = i
				k.add(i)
			}
			rows := k.rows(func(v int) []any { return []any{v} })
			require.Len(t, rows, n)
			gh, gt, gom := llmfmt.Cap(o, rows)
			wh, wt, wom := llmfmt.Cap(o, all)
			require.Equal(t, wom, gom, "%v n=%d", b, n)
			var got, want []int
			for _, r := range append(append([][]any{}, gh...), gt...) {
				require.NotNil(t, r, "%v n=%d: a kept position is empty", b, n)
				got = append(got, r[0].(int))
			}
			want = append(append(want, wh...), wt...)
			require.Equal(t, fmt.Sprint(want), fmt.Sprint(got), "%v n=%d", b, n)
		}
	}
}

func TestPoolWalkVisitsEveryDirectoryOnce(t *testing.T) {
	// A synthetic tree: each directory below depth 4 has three children.
	var visits atomic.Int64
	seen := make([]map[string]bool, 4)
	for i := range seen {
		seen[i] = map[string]bool{}
	}
	poolWalk(context.Background(), 4, walkDir{path: "r"}, func(w int, d walkDir) []walkDir {
		visits.Add(1)
		if seen[w][d.path] {
			t.Errorf("%s visited twice", d.path)
		}
		seen[w][d.path] = true
		if d.depth == 4 {
			return nil
		}
		var subs []walkDir
		for i := 0; i < 3; i++ {
			subs = append(subs, walkDir{path: fmt.Sprintf("%s/%d", d.path, i), depth: d.depth + 1})
		}
		return subs
	})
	require.Equal(t, int64(1+3+9+27+81), visits.Load())
	total := 0
	for _, s := range seen {
		total += len(s)
	}
	require.Equal(t, 121, total)

	// A cancelled context hands out nothing past the root.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n := 0
	poolWalk(ctx, 2, walkDir{path: "r"}, func(int, walkDir) []walkDir { n++; return nil })
	require.Zero(t, n)
}
