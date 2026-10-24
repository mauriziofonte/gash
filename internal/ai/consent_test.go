// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"os"
	"strings"
	"testing"
)

// noTTY returns a module whose /dev/tty cannot be opened.
func noTTY() *Module {
	m := New()
	m.openTTY = func() (*os.File, error) { return nil, os.ErrNotExist }
	return m
}

// TestConfirmSend covers the consent matrix (LEDGER AI-23): auto asks only
// for collected data, 0 never asks, any other value asks every time;
// without a terminal auto and the others refuse, naming the size.
func TestConfirmSend(t *testing.T) {
	m := noTTY()

	// auto + query: no ask, allowed.
	c, _, _ := newCall(map[string]string{}, nil, false)
	if !m.confirmSend(c, "claude", "query", "your question", 10) {
		t.Fatal("auto/query should send without asking")
	}

	// auto + data, no terminal: refused, size in KB, hint.
	c, _, errb := newCall(map[string]string{}, nil, false)
	if m.confirmSend(c, "claude", "data", "system data", 4096) {
		t.Fatal("auto/data with no tty should refuse")
	}
	if !strings.Contains(errb.String(), "GASH_AI_CONFIRM=0") || !strings.Contains(errb.String(), "4 KB") {
		t.Fatalf("refusal message: %q", errb.String())
	}

	// 0 never asks.
	c, _, _ = newCall(map[string]string{"GASH_AI_CONFIRM": "0"}, nil, false)
	if !m.confirmSend(c, "claude", "data", "system data", 10) {
		t.Fatal("GASH_AI_CONFIRM=0 should send")
	}

	// 1 asks every request, so no terminal refuses even a query (AI-23).
	c, _, _ = newCall(map[string]string{"GASH_AI_CONFIRM": "1"}, nil, false)
	if m.confirmSend(c, "claude", "query", "your question", 10) {
		t.Fatal("GASH_AI_CONFIRM=1 with no tty should refuse")
	}

	// An unknown value also asks (fail closed), refusing without a tty.
	c, _, _ = newCall(map[string]string{"GASH_AI_CONFIRM": "off"}, nil, false)
	if m.confirmSend(c, "claude", "query", "your question", 10) {
		t.Fatal("GASH_AI_CONFIRM=off should fail closed")
	}
}

// 3.0: AI-23 fixed: a GASH_AI_CONFIRM value other than 0, 1 or auto still
// asks before every request (fail closed, so headless refuses), with one
// warning per call; 0, 1 and auto warn nothing.
func TestLedger_AI23_fix(t *testing.T) {
	m := noTTY()
	warn := "Warning: GASH_AI_CONFIRM=off is not 0, 1 or auto: asking before every request\n"
	c, _, errb := newCall(map[string]string{"GASH_AI_CONFIRM": "off"}, nil, false)
	// Two requests on one call: both refuse, the warning prints once.
	for i := 0; i < 2; i++ {
		if m.confirmSend(c, "claude", "query", "your question", 10) {
			t.Fatalf("request %d: off must fail closed", i+1)
		}
	}
	if got := errb.String(); strings.Count(got, "Warning:") != 1 || !strings.HasPrefix(got, warn) || !strings.Contains(got, "no terminal to confirm on") {
		t.Fatalf("stderr %q", got)
	}
	// A new call warns again.
	c, _, errb = newCall(map[string]string{"GASH_AI_CONFIRM": "AUTO"}, nil, false)
	m.confirmSend(c, "claude", "query", "q", 1)
	if !strings.HasPrefix(errb.String(), "Warning: GASH_AI_CONFIRM=AUTO is not 0, 1 or auto") {
		t.Fatalf("AUTO: %q", errb.String())
	}
	for _, v := range []string{"0", "1", "auto", ""} {
		c, _, errb := newCall(map[string]string{"GASH_AI_CONFIRM": v}, nil, false)
		m.confirmSend(c, "claude", "data", "x", 1)
		if strings.Contains(errb.String(), "Warning:") {
			t.Fatalf("%q warned: %q", v, errb.String())
		}
	}
}
