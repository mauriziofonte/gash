// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package render

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
)

func gen(content string) *Generation {
	return &Generation{Files: []File{
		{"httpd.conf", []byte(content)},
		{"sites/a.test.conf", []byte("site " + content)},
	}}
}

func TestLayoutLifecycle(t *testing.T) {
	l := Layout{Dir: filepath.Join(t.TempDir(), "render")}
	if n, err := l.Next(); err != nil || n != 1 {
		t.Fatalf("Next on an empty layout = %d, %v", n, err)
	}
	if _, ok, err := l.Current(); ok || err != nil {
		t.Fatalf("Current before any swap: %v %v", ok, err)
	}
	if _, err := l.Rollback(); hteerr.CodeOf(err) != hteerr.InvalidState {
		t.Fatalf("Rollback without previous: %v", err)
	}

	for i := 1; i <= 3; i++ {
		n, err := l.Next()
		must(t, err)
		if n != i {
			t.Fatalf("Next = %d, want %d", n, i)
		}
		dir, err := l.Write(n, gen(string(rune('0'+i))))
		must(t, err)
		if dir != l.GenDir(n) {
			t.Fatalf("Write returned %s", dir)
		}
		must(t, l.Swap(n))
	}
	cur, ok, err := l.Current()
	must(t, err)
	prev, pok, err := l.Previous()
	must(t, err)
	if !ok || !pok || cur != 3 || prev != 2 {
		t.Fatalf("current %d %v, previous %d %v", cur, ok, prev, pok)
	}
	data, err := os.ReadFile(filepath.Join(l.Dir, "current", "sites", "a.test.conf"))
	must(t, err)
	if string(data) != "site 3" {
		t.Fatalf("current/sites/a.test.conf = %q", data)
	}
	if target, _ := os.Readlink(filepath.Join(l.Dir, "current")); target != "gen-3" {
		t.Fatalf("current is not relative: %q", target)
	}
	fi, err := os.Stat(filepath.Join(l.GenDir(3), "httpd.conf"))
	must(t, err)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("file mode %v", fi.Mode().Perm())
	}

	// Swapping to the current generation changes nothing.
	must(t, l.Swap(3))
	if prev, _, _ := l.Previous(); prev != 2 {
		t.Fatalf("Swap to current moved previous to %d", prev)
	}

	// An existing generation is never overwritten.
	if _, err := l.Write(3, gen("x")); hteerr.CodeOf(err) != hteerr.InvalidState {
		t.Fatalf("Write over gen-3: %v", err)
	}

	gens, err := l.Generations()
	must(t, err)
	if diff := cmp.Diff([]int{1, 2, 3}, gens); diff != "" {
		t.Fatal(diff)
	}
	removed, err := l.Prune()
	must(t, err)
	if diff := cmp.Diff([]string{"gen-1"}, removed); diff != "" {
		t.Fatal(diff)
	}

	restored, err := l.Rollback()
	must(t, err)
	if restored != 2 {
		t.Fatalf("Rollback restored %d", restored)
	}
	if cur, _, _ := l.Current(); cur != 2 {
		t.Fatalf("current after rollback = %d", cur)
	}
	if _, ok, _ := l.Previous(); ok {
		t.Fatal("previous kept after rollback")
	}
	// The failed generation stays until Prune; its number is never reused.
	if n, _ := l.Next(); n != 4 {
		t.Fatalf("Next after rollback = %d", n)
	}
	removed, err = l.Prune()
	must(t, err)
	if diff := cmp.Diff([]string{"gen-3"}, removed); diff != "" {
		t.Fatal(diff)
	}
}

func TestLayoutIncompleteAndForeign(t *testing.T) {
	l := Layout{Dir: t.TempDir()}
	// A crashed write leaves gen-<n>.tmp: it is never a generation, its
	// number is not reused, and Prune removes it.
	must(t, os.MkdirAll(filepath.Join(l.Dir, "gen-4.tmp", "sites"), 0o700))
	must(t, os.MkdirAll(filepath.Join(l.Dir, "notes"), 0o700))
	must(t, os.WriteFile(filepath.Join(l.Dir, "gen-9"), []byte("a file"), 0o600))
	gens, err := l.Generations()
	must(t, err)
	if len(gens) != 0 {
		t.Fatalf("generations %v", gens)
	}
	n, err := l.Next()
	must(t, err)
	if n != 10 {
		t.Fatalf("Next = %d", n)
	}
	if err := l.Swap(4); hteerr.CodeOf(err) != hteerr.InvalidState {
		t.Fatalf("Swap to an incomplete generation: %v", err)
	}
	removed, err := l.Prune()
	must(t, err)
	if diff := cmp.Diff([]string{"gen-4.tmp"}, removed); diff != "" {
		t.Fatal(diff)
	}
	if _, err := os.Stat(filepath.Join(l.Dir, "notes")); err != nil {
		t.Fatal("Prune removed a foreign directory")
	}

	// A current link that names anything but a generation is invalid_state.
	for _, target := range []string{"/etc", "../gen-1", "gen-01", "gen-77"} {
		must(t, os.RemoveAll(filepath.Join(l.Dir, "current")))
		must(t, os.Symlink(target, filepath.Join(l.Dir, "current")))
		if _, _, err := l.Current(); hteerr.CodeOf(err) != hteerr.InvalidState {
			t.Errorf("current -> %s: %v", target, err)
		}
	}

	// Write refuses paths that would leave the generation directory.
	for _, bad := range []string{"../escape", "/abs", "a/../b", ""} {
		if _, err := l.Write(20, &Generation{Files: []File{{bad, nil}}}); err == nil {
			t.Errorf("Write accepted %q", bad)
		}
	}
	ents, _ := os.ReadDir(l.Dir)
	for _, e := range ents {
		if e.Name() == "gen-20" || e.Name() == "gen-20.tmp" {
			t.Errorf("failed Write left %s", e.Name())
		}
	}
}

func TestParsePHPOverlay(t *testing.T) {
	got, err := ParsePHPOverlay([]byte("; comment\n\nmemory_limit = 512M\n  date.timezone=\"Europe/Rome\"  \nopcache.enable = on ; inline\nauto_prepend_file =\n"), "8.3.ini")
	must(t, err)
	want := []INIValue{
		{"memory_limit", "512M"},
		{"date.timezone", `"Europe/Rome"`},
		{"opcache.enable", "on ; inline"},
		{"auto_prepend_file", ""},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}
	if got, err := ParsePHPOverlay(nil, "empty.ini"); err != nil || len(got) != 0 {
		t.Fatalf("empty overlay: %v %v", got, err)
	}
	for name, bad := range map[string]string{
		"section":       "[PHP]\nmemory_limit = 1G\n",
		"hash comment":  "# memory_limit = 1G\n",
		"no equals":     "memory_limit 1G\n",
		"bad key":       "memory limit = 1G\n",
		"empty key":     "= 1G\n",
		"duplicate":     "memory_limit = 1G\nmemory_limit = 2G\n",
		"carriage ret":  "memory_limit = 1G\r\n",
		"nul":           "memory_limit = 1G\x00\n",
		"invalid utf-8": "date.timezone = \xff\n",
	} {
		_, err := ParsePHPOverlay([]byte(bad), "8.3.ini")
		e, ok := hteerr.As(err)
		if !ok || e.Code != hteerr.InvalidState {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if name != "invalid utf-8" && !slices.Contains([]string{"8.3.ini:1", "8.3.ini:2"}, e.Details[:min(len(e.Details), 9)]) {
			t.Errorf("%s: no line in %q", name, e.Details)
		}
	}
}
