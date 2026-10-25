// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package pathguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A path ValidatePath accepts does not lead into a forbidden tree, as the
// kernel resolves it: the existing part of the path is resolved by
// filepath.EvalSymlinks (an implementation independent of Realpath), the
// rest appended, and the result checked against the forbidden list with a
// plain prefix test. The working directory holds links into forbidden
// trees, relative links up the tree and a loop.
func FuzzValidatePath(f *testing.F) {
	cwd := f.TempDir()
	cwd, _ = filepath.EvalSymlinks(cwd)
	home := filepath.Join(cwd, "home")
	for _, d := range []string{"home/.ssh", "a/b/c", "x"} {
		if err := os.MkdirAll(filepath.Join(cwd, d), 0o755); err != nil {
			f.Fatal(err)
		}
	}
	for link, target := range map[string]string{
		"etc": "/etc", "proc": "/proc/self", "root": "/root", "sh": "/etc/shadow", "up": "../..",
		"a/b/up": "../../..", "loop": "loop", "x/dev": "/dev/null", "a/sys": "/sys/kernel",
		"chain": "a/b/up", "dot": ".", "boot2": "/boot/../boot",
	} {
		if err := os.Symlink(target, filepath.Join(cwd, link)); err != nil {
			f.Fatal(err)
		}
	}
	for _, s := range []string{
		"etc/passwd", "proc/environ", "root/.bashrc", "sh", "up", "a/b/up/etc/shadow", "chain/proc", "x/dev",
		"a/sys", "dot/etc/sudoers", "boot2", "/etc/hosts", "/etc//shadow", "/etc/shadow/", "//proc", "/./sys",
		"~/x", "~root", "a/b/c/../../x", "notes.txt", "/private/etc/shadow", "/etc/sudoers.d/x", "loop/x",
	} {
		f.Add(s)
	}
	g := NewGuard(OSFS{}, cwd, home, "/root")
	f.Fuzz(func(t *testing.T, p string) {
		if strings.IndexByte(p, 0) >= 0 {
			return
		}
		normalized, err := g.ValidatePath(p)
		if err != nil {
			return
		}
		abs := p
		if p == "~" || strings.HasPrefix(p, "~/") {
			abs = home + p[1:]
		}
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, abs)
		}
		real, ok := resolveExisting(abs)
		if !ok {
			return
		}
		for _, fp := range g.Forbidden {
			for _, c := range []string{real, filepath.Clean(normalized)} {
				if c == fp || strings.HasPrefix(c, fp+"/") {
					t.Fatalf("ValidatePath(%q) = %q accepted, but it resolves to %q, under %s", p, normalized, c, fp)
				}
			}
		}
	})
}

// resolveExisting resolves the longest existing prefix of abs with
// filepath.EvalSymlinks and appends the rest, cleaned; false on a loop.
func resolveExisting(abs string) (string, bool) {
	abs = filepath.Clean(abs)
	rest := ""
	for cur := abs; ; {
		r, err := filepath.EvalSymlinks(cur)
		if err == nil {
			return filepath.Clean(filepath.Join(r, rest)), true
		}
		if _, lerr := os.Lstat(cur); lerr == nil {
			return "", false // exists but does not resolve: a loop
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return filepath.Clean(abs), true
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}
