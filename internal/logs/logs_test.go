// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package logs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	for _, c := range []struct {
		path string
		kind kind
		cat  string
	}{
		{"/var/log/nginx/access.log", active, "web-server"},
		{"/var/log/nginx/access.log.1", rotated, "web-server"},
		{"/var/log/nginx/error.log.2.gz", rotated, "web-server"},
		{"/var/log/apache2/other_vhosts_access.log.14.gz", rotated, "web-server"},
		{"/srv/app/storage/logs/laravel.log", active, "framework"},
		{"/home/me/.npm/_logs/2026-10-25T10_11_debug.log", active, "node"},
		{"/home/me/app/npm-debug.log", active, "node"},
		{"/var/log/php/php_errors.log", active, "php"},
		{"/var/log/journal/abc/system.journal", journal, "systemd-journal"},
		{"/var/log/apt/history.log.3.gz", rotated, "package-manager"},
		{"/var/log/syslog", active, "other"},
		{"/var/log/syslog.1", rotated, "other"},
		{"/var/log/app-2026-10-24.log", active, "other"}, // date is inside the name, ext is .log
		{"/var/log/app.2026-10-24", rotated, "other"},    // date is the trailing marker
		{"/home/me/project/README.md", notLog, ""},
		{"/home/me/project/main.go", notLog, ""},
		{"/home/me/project/notes.txt", notLog, ""}, // a loose .txt is never a log
		{"/var/log/install.txt", active, "other"},  // a .txt inside a log dir is
	} {
		k, cat := classify(c.path)
		if k != c.kind {
			t.Errorf("classify(%q) kind = %v, want %v", c.path, k, c.kind)
		}
		if k != notLog && cat != c.cat {
			t.Errorf("classify(%q) category = %q, want %q", c.path, cat, c.cat)
		}
	}
}

// writeFile writes n bytes with the given mtime.
func writeFile(t *testing.T, p string, n int, age time.Duration, now time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, make([]byte, n), 0o644); err != nil {
		t.Fatal(err)
	}
	mt := now.Add(-age)
	if err := os.Chtimes(p, mt, mt); err != nil {
		t.Fatal(err)
	}
}

func TestScanAndPlan(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	writeFile(t, filepath.Join(dir, "var/log/nginx/access.log"), 5000, time.Hour, now)
	writeFile(t, filepath.Join(dir, "var/log/nginx/access.log.1"), 3000, 48*time.Hour, now)
	writeFile(t, filepath.Join(dir, "var/log/nginx/access.log.2.gz"), 1000, 72*time.Hour, now)
	writeFile(t, filepath.Join(dir, "app/storage/logs/laravel.log"), 8000, 10*time.Minute, now)
	writeFile(t, filepath.Join(dir, "app/README.md"), 400, time.Hour, now)
	// A symlink must never be followed or counted.
	_ = os.Symlink(filepath.Join(dir, "var/log/nginx/access.log"), filepath.Join(dir, "link.log"))

	r := scan([]string{dir}, scanOptions{now: now})
	if r.TotalFiles != 4 {
		t.Fatalf("total files = %d, want 4 (symlink and README excluded); groups=%+v", r.TotalFiles, r.Groups)
	}
	if r.TotalReclaim != 17000 {
		t.Errorf("reclaim = %d, want 17000", r.TotalReclaim)
	}

	// min-size drops the 1000-byte gz and the 3000-byte rotation.
	r2 := scan([]string{dir}, scanOptions{now: now, minSize: 4000})
	if r2.TotalFiles != 2 {
		t.Errorf("min-size files = %d, want 2", r2.TotalFiles)
	}

	// older-than 24h keeps only the two rotated/old files.
	r3 := scan([]string{dir}, scanOptions{now: now, olderThan: 24 * time.Hour})
	if r3.TotalFiles != 2 {
		t.Errorf("older-than files = %d, want 2", r3.TotalFiles)
	}

	// Default plan (no truncate): only the two rotated copies are deleted.
	ops := plan(r, false)
	if len(ops) != 2 {
		t.Fatalf("plan = %d ops, want 2 (rotated only)", len(ops))
	}
	for _, o := range ops {
		if o.Truncate {
			t.Errorf("default plan truncates %s", o.Path)
		}
	}
	// With truncate, the two live logs join the plan.
	if got := len(plan(r, true)); got != 4 {
		t.Errorf("truncate plan = %d ops, want 4", got)
	}
}

func TestApplyDryRunAndReal(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	writeFile(t, filepath.Join(dir, "a.log"), 2000, 0, now)
	writeFile(t, filepath.Join(dir, "a.log.1"), 1000, 0, now)
	r := scan([]string{dir}, scanOptions{now: now})
	ops := plan(r, true)

	dry := cleaner{dryRun: true}.apply(context.Background(), ops)
	if dry.Freed != 3000 || dry.Deleted != 1 || dry.Truncated != 1 {
		t.Fatalf("dry run = %+v", dry)
	}
	if fi, err := os.Stat(filepath.Join(dir, "a.log.1")); err != nil || fi.Size() != 1000 {
		t.Errorf("dry run changed a file")
	}

	real := cleaner{}.apply(context.Background(), ops)
	if real.Freed != 3000 || real.Failed != 0 {
		t.Fatalf("real = %+v", real)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.log.1")); !os.IsNotExist(err) {
		t.Errorf("rotated log not deleted")
	}
	if fi, err := os.Stat(filepath.Join(dir, "a.log")); err != nil || fi.Size() != 0 {
		t.Errorf("live log not truncated: %v", err)
	}
}

func TestParseSize(t *testing.T) {
	for _, c := range []struct {
		in   string
		want int64
		ok   bool
	}{
		{"1024", 1024, true}, {"1K", 1024, true}, {"1k", 1024, true},
		{"2M", 2 << 20, true}, {"1G", 1 << 30, true}, {"5MB", 5 << 20, true},
		{"", 0, false}, {"abc", 0, false}, {"-5", 0, false},
	} {
		got, ok := parseSize(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseSize(%q) = %d, %v; want %d, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestParseDuration(t *testing.T) {
	for _, c := range []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"48h", 48 * time.Hour, true}, {"7d", 7 * 24 * time.Hour, true},
		{"30m", 30 * time.Minute, true}, {"", 0, false}, {"x", 0, false},
	} {
		got, ok := parseDuration(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseDuration(%q) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}
