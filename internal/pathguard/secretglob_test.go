// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package pathguard

import (
	"math/rand"
	"strings"
	"testing"
)

// secretByPattern is SecretName's verdict on a base name from the patterns
// and the exceptions alone (no directory rule).
func secretByPattern(name string) bool {
	if SecretException(name) {
		return false
	}
	lower := asciiLower(name)
	for _, p := range SecretPatterns {
		if matchGlob(p, lower) {
			return true
		}
	}
	return false
}

// execSecret is what the llm_exec exclude globs must match: a name some
// pattern matches with no exception that pattern honors (every literal
// exception; a "*Y" one only where suffixScope lists the pattern).
func execSecret(name string) bool {
	lower := asciiLower(name)
	for _, p := range SecretPatterns {
		if !matchGlob(p, lower) {
			continue
		}
		honored := false
		for _, e := range SecretExceptions {
			if strings.HasPrefix(e, "*") && !contains(suffixScope[e], p) {
				continue
			}
			if matchGlob(e, lower) {
				honored = true
			}
		}
		if !honored {
			return true
		}
	}
	return false
}

func anyGlob(globs []string, name string) bool {
	for _, g := range globs {
		if matchGlob(g, name) {
			return true
		}
	}
	return false
}

// secretGlobCorpus is the names the generated globs are checked on: every
// pattern and exception with its stars filled, pairs of them, case
// variants, all short tails after the prefixes, and random token runs.
func secretGlobCorpus() []string {
	fill := []string{"", "x", "pub", ".pub", "p", ".p", ".pu", "ub", "env.example", ".env.example", "example",
		".example", "sample", "dist", "template", "a.b", "é", "\xff", ".env.", "id_rsa", ".tfstate.", "-", "_", ".", "..",
		"examplex", "xexample", ".env.examplE", "env.exampl", "nv.example"}
	var base []string
	for _, p := range append(append([]string{}, SecretPatterns...), SecretExceptions...) {
		for _, f := range fill {
			base = append(base, strings.ReplaceAll(p, "*", f))
		}
	}
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range base {
		add(s)
		add(strings.ToUpper(s))
		if s != "" {
			add(strings.ToUpper(s[:1]) + s[1:])
		}
	}
	for _, a := range base[:len(base):len(base)] {
		for _, b := range []string{".pub", ".env.example", ".example", "x", ".env.sample", "_rsa"} {
			add(a + b)
			add(b + a)
		}
	}
	alpha := []string{"e", "x", "a", "m", "p", "l", ".", "u", "b", "s", "d", "i", "t", "n", "v", "-", "E", "é"}
	for _, pre := range []string{".env.", "id_rsa", "id_ed25519", "credentials.", "credentials_", "credentials-", "secrets.", "a.tfstate."} {
		add(pre)
		for _, x := range alpha {
			add(pre + x)
			for _, y := range alpha {
				add(pre + x + y)
				for _, z := range alpha {
					add(pre + x + y + z)
				}
			}
		}
	}
	tokens := []string{".env", ".", "example", "sample", "dist", "template", "pub", "id_rsa", "credentials", "secrets",
		".tfstate", "x", "-", "_", "e", "p", "u", "b", "env", "é", "EXAMPLE", "Pub"}
	r := rand.New(rand.NewSource(5))
	for i := 0; i < 20000; i++ {
		var b strings.Builder
		for j := r.Intn(7); j >= 0; j-- {
			b.WriteString(tokens[r.Intn(len(tokens))])
		}
		add(b.String())
	}
	return out
}

// TestFollowup_F05_globs: the generated exclude globs match exactly the
// names SecretName calls secret by pattern: the secret names, never an
// exception (.env.example, id_rsa.pub, credentials.env.example, ...), in
// both renderings.
func TestFollowup_F05_globs(t *testing.T) {
	icase, plain := SecretExcludeGlobs(true), SecretExcludeGlobs(false)
	t.Logf("%d globs", len(plain))
	for _, name := range secretGlobCorpus() {
		want := execSecret(name)
		// Never less than the path guard: a secret name is always excluded.
		if secretByPattern(name) && !want {
			t.Fatalf("%q is secret but execSecret lets it through", name)
		}
		if got := anyGlob(icase, name); got != want {
			t.Errorf("icase globs on %q: %v, want %v", name, got, want)
		}
		if got := anyGlob(plain, asciiLower(name)); got != want {
			t.Errorf("plain globs on %q: %v, want %v", name, got, want)
		}
	}
	for _, name := range []string{".env.example", ".ENV.Sample", ".env.dist", ".env.template", "id_rsa.pub",
		"ID_ED25519.PUB", "id_ecdsa.pub", "id_dsa.pub", "app.env.example", "key.pub"} {
		if anyGlob(icase, name) {
			t.Errorf("%q is an exception but a glob matches it", name)
		}
	}
	for _, name := range []string{".env", ".env.local", ".env.examples", ".env.exampl", "id_rsa", "id_rsa.pubx",
		"credentials.json", ".env.pu", ".env.dist.bak", "x.tfstate.backup", "server.key",
		// exceptions the path guard honors that stay excluded in llm_exec
		"credentials.pub", "secrets.env.example", "x.tfstate.pub", ".env.local.env.example"} {
		if !anyGlob(icase, name) {
			t.Errorf("%q is excluded but no glob matches it", name)
		}
	}
}

// TestFollowup_F05_exact: with every exception honored by every pattern
// (no scope), the construction is exactly SecretName's pattern verdict.
func TestFollowup_F05_exact(t *testing.T) {
	gs, err := buildSecretGlobs(SecretPatterns, SecretExceptions, nil)
	if err != nil {
		t.Fatal(err)
	}
	globs := make([]string, len(gs))
	for i, g := range gs {
		globs[i] = renderGlob(g, true)
	}
	for _, name := range secretGlobCorpus() {
		if got, want := anyGlob(globs, name), secretByPattern(name); got != want {
			t.Errorf("%q: %v, want %v", name, got, want)
		}
	}
}

// The constructions the generator refuses: an exception that would need
// the pattern's prefix, or a pattern ending in a literal that an exception
// matches.
func TestFollowup_F05_refused(t *testing.T) {
	for _, tc := range []struct{ pats, excs []string }{
		{[]string{"*.env"}, []string{"x.env"}},
		{[]string{"*.key"}, []string{"*.key"}},
		{[]string{"*.tfstate.*"}, []string{"*x.tfstate.y"}},
		{[]string{"a?b*"}, nil},
		{[]string{"a*b*"}, nil},
	} {
		if _, err := buildSecretGlobs(tc.pats, tc.excs, nil); err == nil {
			t.Errorf("%v minus %v: no error", tc.pats, tc.excs)
		}
	}
}
