// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mauriziofonte/gash/internal/invoke"
)

// claudeUser returns the user message content of a recorded Claude body.
func claudeUser(t *testing.T, body []byte) string {
	t.Helper()
	var v struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &v); err != nil || len(v.Messages) == 0 {
		t.Fatalf("decode body: %v (%s)", err, body)
	}
	return v.Messages[0].Content
}

// TestAIQuery covers the ai_query public path through the fake endpoint.
func TestAIQuery(t *testing.T) {
	f := newFakeAPI(t)
	m := testModule(f)
	home := gashEnvHome(t, "AI:claude=test-token")
	env := map[string]string{"HOME": home}

	// "how to" -> command (Command: + Explanation:).
	c, out, errb := newCall(env, nil, true, "how to list files?")
	if rc := m.aiQuery(c); rc != 0 {
		t.Fatalf("rc=%d err=%s", rc, errb.String())
	}
	if !strings.Contains(out.String(), "Command:") || !strings.Contains(out.String(), "Explanation:") {
		t.Fatalf("command output: %q", out.String())
	}

	// A specified provider and "what is" -> explanation.
	home2 := gashEnvHome(t, "AI:claude=c-token", "AI:gemini=g-token")
	c, out, _ = newCall(map[string]string{"HOME": home2}, nil, true, "gemini", "what is bash?")
	if rc := m.aiQuery(c); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if !strings.Contains(out.String(), "Explanation:") {
		t.Fatalf("explanation output: %q", out.String())
	}

	// A generic query -> fallback (no labels).
	c, out, _ = newCall(env, nil, true, "hello")
	if rc := m.aiQuery(c); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if strings.Contains(out.String(), "Command:") || strings.Contains(out.String(), "Explanation:") {
		t.Fatalf("fallback should have no labels: %q", out.String())
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Fatal("fallback produced no text")
	}
}

// TestAIQueryNoQuery: no argument -> "No query provided", no request.
func TestAIQueryNoQuery(t *testing.T) {
	f := newFakeAPI(t)
	m := testModule(f)
	c, _, errb := newCall(map[string]string{"HOME": gashEnvHome(t, "AI:claude=t")}, nil, true)
	if rc := m.aiQuery(c); rc != 1 {
		t.Fatalf("want rc 1")
	}
	if !strings.Contains(errb.String(), "No query provided") {
		t.Fatalf("err=%q", errb.String())
	}
	if f.count() != 0 {
		t.Fatalf("no request expected, got %d", f.count())
	}
}

// TestAIQueryNoProvider: an empty settings file -> provider error, no request.
func TestAIQueryNoProvider(t *testing.T) {
	f := newFakeAPI(t)
	m := testModule(f)
	c, _, errb := newCall(map[string]string{"HOME": t.TempDir()}, nil, true, "how to x")
	if rc := m.aiQuery(c); rc != 1 {
		t.Fatalf("want rc 1")
	}
	if !strings.Contains(errb.String(), "No AI provider") {
		t.Fatalf("err=%q", errb.String())
	}
	if f.count() != 0 {
		t.Fatalf("no request expected, got %d", f.count())
	}
}

// TestLedger_AI02_fix: `ai_query claude` names the provider and leaves the
// question empty: "No query provided", rc 1, no request, piped input too
// (3.0: AI-02 fixed; 2.x sent the word "claude"). `ai_query claude hi` is
// unchanged.
func TestLedger_AI02_fix(t *testing.T) {
	f := newFakeAPI(t)
	m := testModule(f)
	home := gashEnvHome(t, "AI:claude=t", "AI:gemini=g")
	for _, args := range [][]string{{"claude"}, {"gemini"}} {
		for _, piped := range []bool{false, true} {
			var in *strings.Reader
			if piped {
				in = strings.NewReader("some log\n")
			}
			c, _, errb := newCall(map[string]string{"HOME": home}, in, !piped, args...)
			if in == nil {
				c.Stdin = nil
			}
			if rc := m.aiQuery(c); rc != 1 {
				t.Fatalf("%v piped=%v: want rc 1", args, piped)
			}
			if errb.String() != "Error: No query provided\n" {
				t.Fatalf("%v: stderr %q", args, errb.String())
			}
		}
	}
	if f.count() != 0 {
		t.Fatalf("want no request, got %d", f.count())
	}
	c, _, _ := newCall(map[string]string{"HOME": home}, nil, true, "claude", "hi")
	if rc := m.aiQuery(c); rc != 0 || f.count() != 1 {
		t.Fatalf("claude hi: rc %d, %d requests", rc, f.count())
	}
	if u := claudeUser(t, f.lastBody()); !strings.HasSuffix(u, "\n\nhi") {
		t.Fatalf("question should be \"hi\", body user=%q", u)
	}
}

// TestLedger_AI24_fix: -h/--help is help as the first word or as the only
// word after a provider (ai_query, ai_ask), and anywhere in ai_sysinfo; a
// question that merely contains --help stays a question.
func TestLedger_AI24_fix(t *testing.T) {
	f := newFakeAPI(t)
	m := testModule(f)
	home := gashEnvHome(t, "AI:claude=t")
	for _, tc := range []struct {
		name string
		args []string
		run  func(*invoke.Call) int
	}{
		{"ai_query", []string{"claude", "--help"}, m.aiQuery},
		{"ai_query", []string{"--help"}, m.aiQuery},
		{"ai_ask", []string{"gemini", "-h"}, m.aiAsk},
		{"ai_sysinfo", []string{"claude", "--help"}, m.aiSysinfo},
		{"ai_sysinfo", []string{"--raw", "-h"}, m.aiSysinfo},
	} {
		c, out, _ := newCall(map[string]string{"HOME": home}, nil, true, tc.args...)
		c.Name = tc.name
		if rc := tc.run(c); rc != 0 || !strings.Contains(out.String(), tc.name) {
			t.Fatalf("%s %v: rc %d, out %q", tc.name, tc.args, rc, out.String())
		}
	}
	if f.count() != 0 {
		t.Fatalf("help sent %d requests", f.count())
	}
	c, out, _ := newCall(map[string]string{"HOME": home}, nil, true, "what", "does", "ls", "--help", "print")
	c.Name = "ai_query"
	if rc := m.aiQuery(c); rc != 0 || f.count() != 1 || strings.Contains(out.String(), "USAGE") {
		t.Fatalf("question with --help: rc %d, %d requests", rc, f.count())
	}
}

// TestLedger_AI16_17_2x: words join with a single space (not the caller's
// IFS), and a leading word that is not exactly claude/gemini is part of the
// question (case-sensitive).
func TestLedger_AI16_17_2x(t *testing.T) {
	f := newFakeAPI(t)
	m := testModule(f)
	home := gashEnvHome(t, "AI:claude=t")

	c, _, _ := newCall(map[string]string{"HOME": home}, nil, true, "what", "is", "up")
	if rc := m.aiQuery(c); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if u := claudeUser(t, f.lastBody()); !strings.HasSuffix(u, "what is up") {
		t.Fatalf("want 'what is up', got %q", u)
	}

	// "Claude" (capital) is not a provider: the whole thing is the question.
	c, _, _ = newCall(map[string]string{"HOME": home}, nil, true, "Claude", "hi")
	if rc := m.aiQuery(c); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if u := claudeUser(t, f.lastBody()); !strings.HasSuffix(u, "Claude hi") {
		t.Fatalf("want 'Claude hi', got %q", u)
	}
}

// TestReadPiped: up to 4096 bytes, NUL dropped, truncation marker at 4096.
func TestReadPiped(t *testing.T) {
	if got := readPiped(strings.NewReader("a\x00b\n")); got != "ab" {
		t.Fatalf("NUL/newline handling: %q", got)
	}
	big := strings.Repeat("x", 5000)
	got := readPiped(strings.NewReader(big))
	if !strings.HasSuffix(got, "\n[... truncated to 4KB]") {
		t.Fatalf("no truncation marker")
	}
	if len(got) != 4096+len("\n[... truncated to 4KB]") {
		t.Fatalf("cut not at 4096 bytes: %d", len(got))
	}
}
