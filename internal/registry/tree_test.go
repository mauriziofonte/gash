// Gash 3.5. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

import (
	"slices"
	"strings"
	"testing"
)

// Every catalog function has exactly one canonical command, or is exempt.
func TestTree_EveryFunctionHasOneCommand(t *testing.T) {
	seen := map[string]string{}
	for _, c := range Tree() {
		if c.Handler == "" {
			if c.Command3x == "" {
				t.Errorf("%s: no handler and no 3.x command", c.Name())
			}
			continue
		}
		if _, ok := Lookup(c.Handler); !ok {
			t.Errorf("%s: handler %q is not a catalog function", c.Name(), c.Handler)
		}
		if c.Hidden {
			continue
		}
		if prev, dup := seen[c.Handler]; dup {
			t.Errorf("%s: handler %q already runs %s", c.Name(), c.Handler, prev)
		}
		seen[c.Handler] = c.Name()
	}
	for _, f := range Functions() {
		_, has := seen[f.Name]
		exempt := slices.Contains(Exempt, f.Name)
		_, former := map[string]bool{"gash_help": true, "gash_upgrade": true}[f.Name]
		switch {
		case has && exempt:
			t.Errorf("%s is exempt and has a command", f.Name)
		case !has && !exempt && !former:
			t.Errorf("%s has no canonical command", f.Name)
		}
	}
}

// Paths are unique, words are lower case with dashes, at most 3 words, and
// a group is never also a command except gash git log (its views are
// commands below it).
func TestTree_Paths(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Tree() {
		w := c.Words()
		if seen[w] {
			t.Errorf("duplicate path %q", w)
		}
		seen[w] = true
		if len(c.Path) == 0 || len(c.Path) > 3 {
			t.Errorf("%q: %d words", w, len(c.Path))
		}
		for _, p := range c.Path {
			for _, r := range p {
				if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
					t.Errorf("%q: word %q has %q", w, p, r)
				}
			}
		}
		if len(c.Path) > 1 {
			if _, ok := NamespaceOf(c.Path[0]); !ok && c.Path[0] != "config" {
				t.Errorf("%q: %q is not a namespace", w, c.Path[0])
			}
		}
	}
}

// The llm_* and agent_* functions map mechanically.
func TestTree_AgentNames(t *testing.T) {
	for _, f := range Functions() {
		for _, p := range []string{"llm_", "agent_"} {
			if !strings.HasPrefix(f.Name, p) {
				continue
			}
			c, ok := CommandOf(f.Name)
			if !ok {
				t.Fatalf("%s: no command", f.Name)
			}
			want := "gash " + strings.TrimSuffix(p, "_") + " " + strings.ReplaceAll(strings.TrimPrefix(f.Name, p), "_", "-")
			if c.Name() != want || !c.Permanent() || len(c.FormerNames()) != 0 {
				t.Errorf("%s: %s permanent=%v former=%v, want %s", f.Name, c.Name(), c.Permanent(), c.FormerNames(), want)
			}
		}
	}
}

// Shortcuts are unique, never a namespace or a top-level word, and the
// module short alias of a function is the shortcut of its command.
func TestTree_Shortcuts(t *testing.T) {
	seen := map[string]string{}
	for _, c := range Tree() {
		if c.Shortcut == "" {
			continue
		}
		if prev, dup := seen[c.Shortcut]; dup {
			t.Errorf("shortcut %q: %s and %s", c.Shortcut, prev, c.Name())
		}
		seen[c.Shortcut] = c.Name()
		if slices.Contains(Children(), c.Shortcut) {
			t.Errorf("shortcut %q is a first word of gash", c.Shortcut)
		}
	}
	for _, a := range Aliases() {
		if a.Pack != "" || a.Function == "" {
			continue
		}
		c, ok := CommandOf(a.Function)
		if !ok {
			t.Errorf("alias %s -> %s: no command", a.Name, a.Function)
			continue
		}
		if c.Shortcut != a.Name {
			t.Errorf("alias %s -> %s: the command %s has shortcut %q", a.Name, a.Function, c.Name(), c.Shortcut)
		}
	}
}

func TestResolve(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
		rest int
	}{
		{[]string{"files", "largest", "/tmp"}, "files_largest", 1},
		{[]string{"git", "log", "all", "-5"}, "gla", 1},
		{[]string{"git", "log", "-5"}, "gl", 1},
		{[]string{"git", "tag", "add", "v1"}, "git_add_tag", 1},
		{[]string{"llm", "db-query", "SELECT 1"}, "llm_db_query", 1},
		{[]string{"doctor"}, "gash_doctor", 0},
	} {
		c, rest, ok := Resolve(tc.args)
		if !ok || c.Handler != tc.want || len(rest) != tc.rest {
			t.Errorf("Resolve(%q) = %v %q %v", tc.args, c, rest, ok)
		}
	}
	if _, _, ok := Resolve([]string{"git"}); ok {
		t.Error("gash git resolved to a command")
	}
	for name, want := range map[string]string{
		"flf": "gash files largest", "files_largest": "gash files largest", "files largest": "gash files largest",
		"gash files largest": "gash files largest", "llm_read": "gash llm read", "gash_help": "gash help",
		"gash_upgrade": "gash update", "gdoctor": "gash doctor",
	} {
		if c, ok := CanonicalName(name); !ok || c.Name() != want {
			t.Errorf("CanonicalName(%q) = %v, want %s", name, c, want)
		}
	}
}
