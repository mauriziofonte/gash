// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import (
	"strings"
	"testing"

	"github.com/mauriziofonte/gash/internal/registry"
)

// scanners are the five functions of this package.
var scanners = []string{"files_largest", "dirs_largest", "dirs_find_large", "dirs_list_empty", "tree_stats"}

func helpBody(t *testing.T, name string) string {
	t.Helper()
	f, ok := registry.Lookup(name)
	if !ok {
		t.Fatalf("%s is not in the registry", name)
	}
	return f.Help.Body
}

// collapse joins the help text into one line with single spaces, so a
// sentence wrapped over several option lines can be matched.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

// TestLedger_FS34_fix: every size flag's help states the grammar ParseSize
// accepts, and the numfmt note is gone (3.0: FS-34 fixed).
func TestLedger_FS34_fix(t *testing.T) {
	for _, name := range []string{"files_largest", "dirs_largest", "dirs_find_large"} {
		body := collapse(helpBody(t, name))
		if !strings.Contains(body, SizeGrammar) {
			t.Errorf("%s help does not state the size grammar %q", name, SizeGrammar)
		}
		if strings.Contains(body, "numfmt") {
			t.Errorf("%s help still mentions numfmt", name)
		}
	}
}

// TestLedger_FS39_fix: one --exclude wording in all five helps, with the
// newline rule of FS-20 (3.0: FS-39, FS-40 fixed).
func TestLedger_FS39_fix(t *testing.T) {
	const want = "--exclude GLOB Skip entries whose name or path below PATH matches GLOB (repeatable; no newlines)"
	for _, name := range scanners {
		body := collapse(helpBody(t, name))
		if !strings.Contains(body, want) {
			t.Errorf("%s help lacks %q", name, want)
		}
		if strings.Contains(body, "substring") {
			t.Errorf("%s help still calls --exclude a substring match", name)
		}
	}
}

// TestLedger_FS33_fix: the tab/newline exclusion is stated in every help
// (3.0: FS-33 fixed).
func TestLedger_FS33_fix(t *testing.T) {
	for _, name := range scanners {
		if !strings.Contains(collapse(helpBody(t, name)), "Paths containing literal tab or newline characters are excluded") {
			t.Errorf("%s help does not state the tab/newline exclusion", name)
		}
	}
}

// TestLedger_FS46_fix: an example that scans / passes --allow-root, as the
// scanners require (3.0: FS-46 fixed).
func TestLedger_FS46_fix(t *testing.T) {
	for _, name := range scanners {
		for _, line := range strings.Split(helpBody(t, name), "\n") {
			l := strings.TrimSpace(line)
			if strings.HasPrefix(l, "#") || !strings.HasSuffix(l, " /") {
				continue
			}
			if !strings.Contains(l, "--allow-root") {
				t.Errorf("%s example %q scans / without --allow-root", name, l)
			}
		}
	}
}
