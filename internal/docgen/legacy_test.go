// Gash 4.1. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docgen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mauriziofonte/gash/internal/registry"
)

// legacyAllowed are the files whose messages name a former flat name on
// purpose, and the names they may name.
var legacyAllowed = map[string][]string{
	// The route of a Gash 1.x configuration goes through 2.1.0, whose
	// commands are gash_migrate and gash_upgrade.
	"internal/selfmgmt/migrate.go": {"gash_migrate", "gash_upgrade"},
	// gash update --help names its 3.x name.
	"internal/cli/cli.go": {"gash_upgrade"},
}

// bareName is a string that is only a name: a function name as data (a
// handler, a lookup key), not a message.
var bareName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// The messages of the binary name what a 4.x shell defines: a former flat
// name (files_largest, hgrep) only in a line about GASH_LEGACY_NAMES, as a
// word of a gash command, or where legacyAllowed says why. The help texts
// of the catalog are written for the flat names and rewritten when shown:
// internal/help checks the pages instead.
func TestMessagesCallNoLegacyName(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			if d.IsDir() {
				if rel == filepath.Join("internal", "registry") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				s, err := strconv.Unquote(lit.Value)
				if err != nil || bareName.MatchString(s) {
					return true
				}
				var lines []string
				for _, l := range strings.Split(s, "\n") {
					if !strings.Contains(l, registry.LegacyVar) {
						lines = append(lines, l)
					}
				}
				for _, h := range registry.ScanLegacy(strings.Join(lines, "\n")) {
					if slices.Contains(legacyAllowed[filepath.ToSlash(rel)], h.Name) {
						continue
					}
					t.Errorf("%s: %q calls %s: use %s", fset.Position(lit.Pos()), s, h.Name, h.Command)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
