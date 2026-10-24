// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runner

import (
	"slices"
	"testing"
)

func TestEnvAllowlist(t *testing.T) {
	parent := []string{"PATH=/bin", "HOME=/h", "SECRET=s", "LC_ALL=C", "LANG=en", "LC_CTYPE=x", "HOME=/dup", "BROKEN", "=x"}
	got := NewEnv().Allow(parent, "HOME", "PATH", "LC_*", "MISSING").Environ()
	if want := []string{"HOME=/h", "PATH=/bin", "LC_ALL=C", "LC_CTYPE=x"}; !slices.Equal(got, want) {
		t.Errorf("Allow = %q, want %q", got, want)
	}

	e := NewEnv().Set("TERM", "dumb").Allow(parent, "TERM", "HOME")
	e.Set("TERM", "xterm").Set("GIT_TERMINAL_PROMPT", "0").Unset("HOME").Unset("NOPE")
	if got, want := e.Environ(), []string{"TERM=xterm", "GIT_TERMINAL_PROMPT=0"}; !slices.Equal(got, want) {
		t.Errorf("Set/Unset = %q, want %q", got, want)
	}
	if v, ok := e.Lookup("TERM"); !ok || v != "xterm" {
		t.Errorf("Lookup = %q %v", v, ok)
	}
	if _, ok := e.Lookup("HOME"); ok {
		t.Error("HOME still set")
	}

	all := NewEnv().AllowAll(parent).Environ()
	if want := []string{"PATH=/bin", "HOME=/h", "SECRET=s", "LC_ALL=C", "LANG=en", "LC_CTYPE=x"}; !slices.Equal(all, want) {
		t.Errorf("AllowAll = %q", all)
	}

	var zero Env
	if got := zero.Environ(); got == nil || len(got) != 0 {
		t.Errorf("zero Env = %#v", got)
	}
	zero.Set("A", "")
	if got := zero.Environ(); !slices.Equal(got, []string{"A="}) {
		t.Errorf("zero Env after Set = %q", got)
	}

	for _, bad := range []string{"", "A=B", "A\x00"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Set(%q) did not panic", bad)
				}
			}()
			NewEnv().Set(bad, "v")
		}()
	}
}

func TestNormalizeEnv(t *testing.T) {
	got := normalizeEnv([]string{"A=1", "B", "=x", "A=2", "C=a=b", "D=\x00", "E="})
	if want := []string{"A=1", "C=a=b", "E="}; !slices.Equal(got, want) {
		t.Errorf("normalizeEnv = %q, want %q", got, want)
	}
	if got := normalizeEnv(nil); got == nil {
		t.Error("nil environment must become an empty one, not the parent's")
	}
}
