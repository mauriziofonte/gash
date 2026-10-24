// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"strings"
	"testing"
)

// TestFormatResponse: each response type prints as 2.x did.
func TestFormatResponse(t *testing.T) {
	m := New()
	cases := []struct {
		name    string
		reply   string
		want    []string
		notWant []string
	}{
		{"command", `{"type":"command","command":"ls -lhS","text":"lists by size"}`,
			[]string{"Command:", "`ls -lhS`", "Explanation:", "lists by size"}, nil},
		{"explanation", `{"type":"explanation","text":"a concept"}`,
			[]string{"Explanation:", "a concept"}, []string{"Command:"}},
		{"fallback", `{"type":"fallback","text":"hi there"}`,
			[]string{"hi there"}, []string{"Command:", "Explanation:"}},
		{"troubleshoot", `{"type":"troubleshoot","issue":"boom","suggestion":"fix it","text":"sum"}`,
			[]string{"Issue:", "boom", "Suggestion:", "fix it"}, nil},
		{"code", `{"type":"code","text":"desc","code":"echo hi","lang":"bash"}`,
			[]string{"desc", "# bash", "echo hi"}, nil},
		{"missingType", `{"text":"just text"}`,
			[]string{"just text"}, []string{"Command:"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			call, out, _ := newCall(map[string]string{}, nil, false)
			m.formatResponse(call, c.reply)
			got := out.String()
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Fatalf("want %q in %q", w, got)
				}
			}
			for _, nw := range c.notWant {
				if strings.Contains(got, nw) {
					t.Fatalf("did not want %q in %q", nw, got)
				}
			}
		})
	}
}
