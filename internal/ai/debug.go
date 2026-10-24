// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"bytes"
	"encoding/json"
	"strconv"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// aiDebug emits the GASH_AI_DEBUG=1 trace of 2.x __ai_debug (lib/modules/
// ai/client.sh): "Debug: AI <label> <section>:" on stderr, then the
// sanitized content on the lines below. The net/http port has no curl, so
// the 2.x "Curl Stderr" block has no counterpart (the curl->net/http port,
// LEDGER AI-15); every other block -- request body, HTTP code and elapsed,
// raw response, extracted JSON -- is kept in its 2.x order. GASH_AI_DEBUG is
// an environment setting the shim passes (MATRIX), read through the call's
// environment, not a shell variable.
type aiDebug struct {
	p     ui.Printer
	label string
	on    bool
}

// debugger returns the trace emitter for a call: enabled when
// GASH_AI_DEBUG=1, a no-op otherwise. label is "Claude" or "Gemini".
func (m *Module) debugger(c *invoke.Call, label string) aiDebug {
	if c.Env.Get("GASH_AI_DEBUG") != "1" {
		return aiDebug{}
	}
	p := printer(c)
	p.DebugOn = true // __ai_debug forces GASH_DEBUG=1 around __gash_debug
	return aiDebug{p: p, label: label, on: true}
}

// emit prints one "AI <label> <section>:" block, its content sanitized as
// 2.x __ai_sanitize_text (control characters dropped, tab and newline kept).
// __gash_debug stops the spinner first; the Go spinner is driven by the
// caller, so there is nothing to stop here.
func (d aiDebug) emit(section, content string) {
	if !d.on {
		return
	}
	d.p.Debug("AI " + d.label + " " + section + ":\n" + sanitizeText(content))
}

// body is the "Request Body" block: the request JSON pretty-printed.
func (d aiDebug) body(b []byte) { d.emit("Request Body", prettyJSON(b)) }

// httpCode is the "HTTP Code" block: the status with the elapsed seconds.
func (d aiDebug) httpCode(code, elapsedSecs int) {
	d.emit("HTTP Code", strconv.Itoa(code)+" (elapsed: "+strconv.Itoa(elapsedSecs)+"s)")
}

// rawResponse is the "Raw Response" block: the response body pretty-printed.
func (d aiDebug) rawResponse(b []byte) { d.emit("Raw Response", prettyJSON(b)) }

// extractedJSON is the "Extracted JSON" block: the model's text (the
// structured reply), pretty-printed.
func (d aiDebug) extractedJSON(text string) { d.emit("Extracted JSON", prettyJSON([]byte(text))) }

// prettyJSON is `jq .`: the value indented two spaces with its members in
// their original order. A value that is not JSON is returned unchanged, as
// 2.x fell back to the raw string when jq failed.
func prettyJSON(raw []byte) string {
	var buf bytes.Buffer
	if json.Indent(&buf, raw, "", "  ") != nil {
		return string(raw)
	}
	return buf.String()
}
