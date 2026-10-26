// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docgen

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mauriziofonte/gash/internal/config"
)

// TestSettingsReference: the settings table of docs/configuration.md lists
// exactly the settings ~/.gash_env accepts, each with the default the code
// applies, so a setting cannot be added, dropped or changed in one place
// only.
func TestSettingsReference(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "configuration.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(string(b), "\n## Settings reference\n")
	if !ok {
		t.Fatal("docs/configuration.md has no Settings reference section")
	}
	if i := strings.Index(section, "\n## "); i >= 0 {
		section = section[:i]
	}
	documented := map[string]string{}
	for _, line := range strings.Split(section, "\n") {
		cells := strings.Split(line, "|")
		if len(cells) < 4 || !strings.HasPrefix(strings.TrimSpace(cells[1]), "`GASH_") {
			continue
		}
		name := strings.Trim(strings.TrimSpace(cells[1]), "`")
		documented[name] = strings.Trim(strings.TrimSpace(cells[2]), "`")
	}
	var want, got []string
	for _, s := range config.Known {
		want = append(want, s.Name)
		if def, ok := documented[s.Name]; ok && s.Default != "" && def != s.Default {
			t.Errorf("%s: the default is %q, the page says %q", s.Name, s.Default, def)
		}
	}
	for name := range documented {
		got = append(got, name)
	}
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(want, got) {
		t.Errorf("the settings table of docs/configuration.md lists %v; ~/.gash_env accepts %v", got, want)
	}
}
