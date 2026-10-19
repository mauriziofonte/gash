// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"
)

// bodyField parses the recorded request body and returns the value at a
// dotted path of object keys as compact JSON.
func bodyField(t *testing.T, body []byte, path ...string) json.RawMessage {
	t.Helper()
	cur := json.RawMessage(body)
	for _, k := range path {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(cur, &m); err != nil {
			t.Fatalf("decode %v: %v", path, err)
		}
		cur = m[k]
	}
	return cur
}

// TestClaudeRequestShape: the default model gets effort low, the schema is
// json_schema, max_tokens is the argument; an older model gets no effort.
func TestClaudeRequestShape(t *testing.T) {
	f := newFakeAPI(t)
	m := testModule(f)

	c, _, _ := newCall(map[string]string{}, nil, false)
	if _, rc := m.request(c, "claude", "tok", "sys", "hello", responseSchema, 1024, time.Second); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	body := f.lastBody()
	if got := string(bodyField(t, body, "model")); got != `"claude-haiku-5-5"` {
		t.Fatalf("model=%s", got)
	}
	if got := string(bodyField(t, body, "max_tokens")); got != "1024" {
		t.Fatalf("max_tokens=%s", got)
	}
	if got := string(bodyField(t, body, "output_config", "effort")); got != `"low"` {
		t.Fatalf("effort=%s", got)
	}
	if got := string(bodyField(t, body, "output_config", "format", "type")); got != `"json_schema"` {
		t.Fatalf("format.type=%s", got)
	}

	c2, _, _ := newCall(map[string]string{"GASH_AI_MODEL_CLAUDE": "claude-haiku-4-5"}, nil, false)
	if _, rc := m.request(c2, "claude", "tok", "sys", "hello", responseSchema, 1024, time.Second); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	body = f.lastBody()
	if got := string(bodyField(t, body, "model")); got != `"claude-haiku-4-5"` {
		t.Fatalf("model=%s", got)
	}
	var oc map[string]json.RawMessage
	_ = json.Unmarshal(bodyField(t, body, "output_config"), &oc)
	if _, has := oc["effort"]; has {
		t.Fatalf("older model should carry no effort: %s", bodyField(t, body, "output_config"))
	}
}

// TestGeminiRequestShape: the Gemini body carries the schema under
// generationConfig.responseJsonSchema and the model in the URL.
func TestGeminiRequestShape(t *testing.T) {
	f := newFakeAPI(t)
	m := testModule(f)
	c, _, _ := newCall(map[string]string{}, nil, false)
	if _, rc := m.request(c, "gemini", "tok", "sys", "what is bash?", responseSchema, 2048, time.Second); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	body := f.lastBody()
	if got := string(bodyField(t, body, "generationConfig", "maxOutputTokens")); got != "2048" {
		t.Fatalf("maxOutputTokens=%s", got)
	}
	if got := string(bodyField(t, body, "generationConfig", "responseMimeType")); got != `"application/json"` {
		t.Fatalf("mime=%s", got)
	}
	if sc := bodyField(t, body, "generationConfig", "responseJsonSchema"); len(sc) == 0 || sc[0] != '{' {
		t.Fatalf("schema missing: %s", sc)
	}
}

// TestLedger_AI13_retry: a 529 is retried once after Retry-After, the
// warning names the delay and attempt, and the reply then parses.
func TestLedger_AI13_retry(t *testing.T) {
	f := newFakeAPI(t)
	f.handle = func(n int, _ []byte, w http.ResponseWriter) {
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(529)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"{\"type\":\"explanation\",\"text\":\"ok\"}"}],"stop_reason":"end_turn"}`)
	}
	m := testModule(f)
	c, _, errb := newCall(map[string]string{}, nil, false)
	reply, rc := m.request(c, "claude", "tok", "sys", "what is bash?", responseSchema, 1024, time.Second)
	if rc != 0 {
		t.Fatalf("rc=%d err=%s", rc, errb.String())
	}
	if f.count() != 2 {
		t.Fatalf("want 2 requests, got %d", f.count())
	}
	if !strings.Contains(errb.String(), "retrying in 1s (1/3)") {
		t.Fatalf("warning missing: %q", errb.String())
	}
	if !strings.Contains(reply, `"explanation"`) {
		t.Fatalf("reply=%s", reply)
	}
}

// TestLedger_AI_refusal: a refusal stop_reason is reported, not parsed.
func TestClaudeRefusal(t *testing.T) {
	f := newFakeAPI(t)
	f.handle = func(_ int, _ []byte, w http.ResponseWriter) {
		_, _ = io.WriteString(w, `{"content":[],"stop_reason":"refusal"}`)
	}
	m := testModule(f)
	c, _, errb := newCall(map[string]string{}, nil, false)
	if _, rc := m.request(c, "claude", "tok", "sys", "x", responseSchema, 1024, time.Second); rc != 1 {
		t.Fatalf("want rc 1")
	}
	if !strings.Contains(errb.String(), "declined") {
		t.Fatalf("want declined: %q", errb.String())
	}
}

// TestSanitizeReply: terminal escapes in the reply text are stripped.
func TestSanitizeReply(t *testing.T) {
	f := newFakeAPI(t)
	f.handle = func(_ int, _ []byte, w http.ResponseWriter) {
		inner := `{"type":"explanation","text":"ok\u001b]0;pwned\u0007\u001b[31mred"}`
		_, _ = io.WriteString(w, `{"content":[{"type":"text","text":`+jsonString(inner)+`}],"stop_reason":"end_turn"}`)
	}
	m := testModule(f)
	c, _, _ := newCall(map[string]string{}, nil, false)
	reply, rc := m.request(c, "claude", "tok", "sys", "x", responseSchema, 1024, time.Second)
	if rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if strings.ContainsAny(reply, "\x1b\x07") {
		t.Fatalf("escapes not stripped: %q", reply)
	}
	var r struct{ Text string }
	_ = json.Unmarshal([]byte(reply), &r)
	if r.Text != "ok]0;pwned[31mred" {
		t.Fatalf("text=%q", r.Text)
	}
}

// TestLedger_AI15_2x: injected transport errors map to the curl-exit-code
// messages; a 301 is an "API error (301): Unknown error".
func TestLedger_AI15_2x(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"dns", &net.DNSError{Name: "api.anthropic.com"}, "Claude API: Could not resolve host (check DNS/internet)"},
		{"refused", &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, "Claude API: Failed to connect (server unreachable)"},
		{"eof", io.EOF, "Claude API: Server returned empty response"},
		{"unexpected", io.ErrUnexpectedEOF, "Claude API: Partial response received (transfer interrupted)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New()
			m.client = &http.Client{Transport: errTransport{tc.err}}
			m.claude = "https://api.anthropic.com/v1/messages"
			m.sleep = func(time.Duration) {}
			c, _, errb := newCall(map[string]string{}, nil, false)
			if _, rc := m.request(c, "claude", "tok", "sys", "x", responseSchema, 1024, time.Second); rc != 1 {
				t.Fatalf("want rc 1")
			}
			if !strings.Contains(errb.String(), tc.want) {
				t.Fatalf("want %q in %q", tc.want, errb.String())
			}
		})
	}

	// A 301 is returned (redirects not followed) and read as an API error.
	f := newFakeAPI(t)
	f.handle = func(_ int, _ []byte, w http.ResponseWriter) { w.WriteHeader(301) }
	m := testModule(f)
	c, _, errb := newCall(map[string]string{}, nil, false)
	if _, rc := m.request(c, "claude", "tok", "sys", "x", responseSchema, 1024, time.Second); rc != 1 {
		t.Fatalf("want rc 1")
	}
	if !strings.Contains(errb.String(), "Claude API error (301): Unknown error") {
		t.Fatalf("301 mapping: %q", errb.String())
	}
}

// errTransport returns a fixed error from every round trip.
type errTransport struct{ err error }

func (e errTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, e.err }

// TestRetryDelay pins the delay computation: Retry-After seconds, Gemini
// RetryInfo Nms, clamping and the fallback.
func TestRetryDelay(t *testing.T) {
	for _, tc := range []struct{ after, want string }{
		{"1", "1"},
		{"5", "5"},
		{"90", "60"},
		{"0", "1"},
		{"600ms", "1"},
		{"1500ms", "1.5"},
		{"", "5"},
	} {
		if got := retryDelay(tc.after, nil); got != tc.want {
			t.Fatalf("retryDelay(%q)=%q want %q", tc.after, got, tc.want)
		}
	}
	// Gemini RetryInfo via the body when no header.
	body := []byte(`{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"3s"}]}}`)
	if got := retryDelay("", body); got != "3" {
		t.Fatalf("gemini retryDelay=%q", got)
	}
}
