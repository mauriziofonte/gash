// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docgen

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/registry"
)

// codeSpanRe is inline code; fnNameRe a name shaped like a Gash function;
// settingRe a GASH_ variable.
var (
	codeSpanRe = regexp.MustCompile("`([^`\n]+)`")
	fnNameRe   = regexp.MustCompile(`^(llm|agent|gash|git|docker|remote|files|dirs|tree|archive|file|php|node|nvm|chrome|ai|wsl|ssh|port|process|services|sysinfo|disk|history|mkdir|path|needs|all)_[a-z0-9_]+$`)
	settingRe  = regexp.MustCompile(`\bGASH_[A-Z0-9_]*[A-Z0-9]\b`)
)

// TestNamesExist: a function a page names in inline code (`git_sync`) is
// a function or alias of the registry (or an error code), and every GASH_ variable a page
// names is read somewhere in the code, so a renamed or removed name cannot
// survive in the documentation.
func TestNamesExist(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, f := range registry.Functions() {
		known[f.Name] = true
		for _, a := range f.Aliases {
			known[a] = true
		}
	}
	for _, a := range registry.Aliases() {
		known[a.Name] = true
	}
	for _, c := range contract.Codes() {
		// Error codes share the shape (git_not_found).
		known[c.Name] = true
	}
	// Names of other programs' files and directories share it too.
	for _, n := range []string{"node_modules"} {
		known[n] = true
	}
	settings := codeSettings(t, root)
	for _, page := range markdownFiles(t, root) {
		rel, _ := filepath.Rel(root, page)
		if rel == "CHANGELOG.md" {
			// History names what each release had.
			continue
		}
		b, err := os.ReadFile(page)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		for _, m := range codeSpanRe.FindAllStringSubmatch(src, -1) {
			if name := m[1]; fnNameRe.MatchString(name) && !known[name] {
				t.Errorf("%s: `%s` is no function of Gash", rel, name)
			}
		}
		for _, name := range settingRe.FindAllString(src, -1) {
			if !settings[name] {
				t.Errorf("%s: %s is read nowhere in the code", rel, name)
			}
		}
	}
}

// codeSettings are the GASH_ names in the code: the Go sources (tests
// aside), the Bash of lib/, gash.sh, install.sh and bin/, and the scripts
// of dev/, bench/ and the workflows.
func codeSettings(t *testing.T, root string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	read := func(p string) {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range settingRe.FindAllString(string(b), -1) {
			out[n] = true
		}
	}
	for _, dir := range []string{"cmd", "internal", "lib", "bin", "dev", "bench", ".github"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(p, "_test.go") {
				return nil
			}
			switch filepath.Ext(p) {
			case ".go", ".sh", ".py", ".tmpl", ".yml", ".yaml", ".tape":
				read(p)
			case "":
				if dir == "bin" {
					read(p)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"gash.sh", "install.sh", ".gashrc", "bash_completion"} {
		if _, err := os.Stat(filepath.Join(root, f)); err == nil {
			read(filepath.Join(root, f))
		}
	}
	return out
}
