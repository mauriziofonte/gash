// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// protectFixture is DIR with vault/ (PROTECT:all), ledger/
// (PROTECT:content) and pub/, a link to vault next to them and one inside
// pub/, and the HOME whose ~/.gash_env protects them.
func protectFixture(t *testing.T) (d, home string) {
	t.Helper()
	d = physical(t, t.TempDir())
	mustMkdir(t, d, "vault/emptydir", "vault/sub", "ledger", "pub")
	writeN(t, filepath.Join(d, "vault", "app.log"), 3000)
	writeN(t, filepath.Join(d, "vault", "sub", "doc.txt"), 100)
	writeN(t, filepath.Join(d, "ledger", "old.log.1"), 2000)
	writeN(t, filepath.Join(d, "pub", "big.bin"), 4000)
	for link, target := range map[string]string{"vlink": "vault", "pub/into": "vault/sub"} {
		if err := os.Symlink(filepath.Join(d, target), filepath.Join(d, link)); err != nil {
			t.Fatal(err)
		}
	}
	home = t.TempDir()
	env := "PROTECT:all=" + d + "/vault\nPROTECT:content=" + d + "/ledger\n"
	if err := os.WriteFile(filepath.Join(home, ".gash_env"), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	return d, home
}

// TestProtectedTrees: the user's PROTECT:all paths (~/.gash_env) are left
// out of every scanner's walk, also from a root above them and through
// -L, and are forbidden_path as the PATH argument (as written, through a
// link, with ".."); a PROTECT:content path is listed, names and sizes;
// without the file nothing changes (3.4.1).
func TestProtectedTrees(t *testing.T) {
	d, home := protectFixture(t)
	vault := d + "/vault"
	for fn := range scanHandlers {
		out, errb, rc := runGo(fn, home, d, scanArgs(fn, "--json", d)...)
		envOf(t, fn, out)
		if rc != 0 || errb != "" || strings.Contains(out, vault) || strings.Contains(out, "emptydir") {
			t.Errorf("%s: rc=%d out=%q err=%q", fn, rc, out, errb)
		}
		if fn != "tree_stats" && fn != "dirs_list_empty" && !strings.Contains(out, d+"/ledger") {
			t.Errorf("%s: the content tree is listed: %q", fn, out)
		}
		for _, p := range []string{vault, vault + "/emptydir", d + "/vlink", d + "/pub/../vault/sub", "vault"} {
			out, errb, rc = runGo(fn, home, d, scanArgs(fn, "--json", p)...)
			if rc == 0 || out != "" || !strings.Contains(errb, `"error":"forbidden_path"`) {
				t.Errorf("%s %s: rc=%d out=%q err=%q", fn, p, rc, out, errb)
			}
		}
	}
	for _, fn := range []string{"files_largest", "tree_stats"} {
		out, errb, rc := runGo(fn, home, d, "-L", "--json", d+"/pub")
		if rc != 0 || errb != "" || strings.Contains(out, "doc.txt") {
			t.Errorf("%s -L: rc=%d out=%q err=%q", fn, rc, out, errb)
		}
	}
	out, _, _ := runGo("tree_stats", home, d, "--json", d)
	data := envOf(t, "tree_stats", out)["data"].(map[string]any)
	if data["files_total"] != 2.0 || data["dirs_total"] != 3.0 || data["empty_dirs"] != 0.0 {
		t.Errorf("tree_stats counts the PROTECT:all tree: %q", out)
	}

	// Without the file, the tree is scanned.
	out, _, rc := runGo("files_largest", t.TempDir(), d, "--json", d)
	if rc != 0 || !strings.Contains(out, vault+"/app.log") {
		t.Errorf("no PROTECT: lines: rc=%d out=%q", rc, out)
	}
}
