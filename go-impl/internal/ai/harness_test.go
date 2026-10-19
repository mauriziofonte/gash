// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// fakeAPI is a test stand-in for the Claude/Gemini HTTPS endpoints: it
// records request bodies and answers with the mock curl's canned replies,
// or a per-test handler.
type fakeAPI struct {
	srv *httptest.Server
	mu  sync.Mutex
	n   int
	// bodies holds every request body in order.
	bodies [][]byte
	// handle, when set, writes the reply; otherwise the default canned reply.
	handle func(n int, body []byte, w http.ResponseWriter)
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	f := &fakeAPI{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.n++
		n := f.n
		f.bodies = append(f.bodies, body)
		h := f.handle
		f.mu.Unlock()
		gemini := strings.Contains(r.URL.Path, "generateContent")
		if h != nil {
			h(n, body, w)
			return
		}
		inner := innerJSON(detectType(body))
		w.Header().Set("Content-Type", "application/json")
		if gemini {
			_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":`+jsonString(inner)+`}]}}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"content":[{"type":"text","text":`+jsonString(inner)+`}],"model":"claude-haiku-4-5","stop_reason":"end_turn"}`)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n
}

func (f *fakeAPI) lastBody() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.bodies) == 0 {
		return nil
	}
	return f.bodies[len(f.bodies)-1]
}

// jsonString quotes s as a JSON string for embedding in a reply.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// detectType mimics the mock curl's response-type detection: it reads the
// user query (the text after the context, i.e. after "\n\n"), not the whole
// body (the system prompt names the types).
func detectType(body []byte) string {
	l := strings.ToLower(userContent(body))
	switch {
	case strings.Contains(l, "[pipe input"):
		return "troubleshoot"
	case strings.Contains(l, "how to"), strings.Contains(l, "how do i"):
		return "command"
	case strings.Contains(l, "what is"), strings.Contains(l, "explain"):
		return "explanation"
	case strings.Contains(l, "write a script"), strings.Contains(l, "write code"):
		return "code"
	}
	return "fallback"
}

// userContent returns the query part of a request body (Claude or Gemini):
// the user text after the "\n\n" that follows the Context/analysis prefix.
func userContent(body []byte) string {
	var cl struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	var gm struct {
		Contents []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
	}
	text := ""
	if json.Unmarshal(body, &cl) == nil && len(cl.Messages) > 0 {
		text = cl.Messages[0].Content
	} else if json.Unmarshal(body, &gm) == nil && len(gm.Contents) > 0 && len(gm.Contents[0].Parts) > 0 {
		text = gm.Contents[0].Parts[0].Text
	}
	if i := strings.LastIndex(text, "\n\n"); i >= 0 {
		return text[i+2:]
	}
	return text
}

func innerJSON(kind string) string {
	switch kind {
	case "command":
		return `{"type":"command","command":"ls -lhS","text":"Lists files sorted by size in descending order."}`
	case "explanation":
		return `{"type":"explanation","text":"This is a conceptual explanation of the topic."}`
	case "troubleshoot":
		return `{"type":"troubleshoot","issue":"PHP Fatal error","suggestion":"Check init","text":"Analysis complete"}`
	case "code":
		return `{"type":"code","text":"Creates a backup script.","code":"#!/bin/bash\ntar -czf backup.tar.gz ~/","lang":"bash"}`
	}
	return `{"type":"fallback","text":"I am a bash/linux terminal assistant. How can I help?"}`
}

// testModule wires a Module to the fake endpoints with no real sleeping.
func testModule(f *fakeAPI) *Module {
	m := New()
	m.client = f.srv.Client()
	m.claude = f.srv.URL + "/v1/messages"
	m.gemini = f.srv.URL + "/v1beta/models"
	m.sleep = func(time.Duration) {}
	m.now = time.Now
	m.openTTY = func() (*os.File, error) { return nil, os.ErrNotExist }
	return m
}

// newCall builds an invoke.Call with the given env, stdin and arguments.
func newCall(env map[string]string, stdin io.Reader, stdinTTY bool, args ...string) (*invoke.Call, *bytes.Buffer, *bytes.Buffer) {
	e := invoke.EnvMap(env)
	var out, errb bytes.Buffer
	c := &invoke.Call{Name: "ai", Args: args, Env: e, Stdin: stdin,
		Stdout: &out, Stderr: &errb, Term: term.Fixed(e, stdinTTY, false, false)}
	return c, &out, &errb
}

// gashEnvHome writes a ~/.gash_env with the given lines and returns the
// HOME holding it (mode 0600, so the loader accepts it).
func gashEnvHome(t *testing.T, lines ...string) string {
	t.Helper()
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".gash_env"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}
