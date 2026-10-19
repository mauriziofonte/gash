// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import "testing"

func TestSanitizeText(t *testing.T) {
	in := "ok\x1b]0;x\x07\tkeep\nline\x7fend"
	got := sanitizeText(in)
	want := "ok]0;x\tkeep\nlineend"
	if got != want {
		t.Fatalf("sanitizeText=%q want %q", got, want)
	}
}

func TestSanitizeJSON(t *testing.T) {
	// Control characters (escaped, as valid JSON carries them) are stripped
	// from string values; keys and order are kept; C1 controls go too.
	in := []byte(`{"b":"x\u0007y","a":["p\u0085q"],"n":2}`)
	got, ok := sanitizeJSON(in)
	if !ok {
		t.Fatal("valid JSON rejected")
	}
	if string(got) != `{"b":"xy","a":["pq"],"n":2}` {
		t.Fatalf("sanitizeJSON=%s", got)
	}

	if _, ok := sanitizeJSON([]byte("not json")); ok {
		t.Fatal("invalid JSON accepted")
	}
	if _, ok := sanitizeJSON([]byte(`{"a":1} {"b":2}`)); ok {
		t.Fatal("a stream of two values should be rejected")
	}
}
