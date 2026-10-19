// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"bytes"
	"strings"

	"encoding/json/jsontext"
)

// sanitizeText is __ai_sanitize_text: drop the control characters a reply
// could use to drive the terminal (0x01-0x08, 0x0B-0x1F, DEL), keeping
// tab and newline. Byte based, as the Bash parameter expansion was on
// these single-byte controls.
func sanitizeText(s string) string {
	if !strings.ContainsFunc(s, isTextControl) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 256 && isTextControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isTextControl(r rune) bool {
	return (r >= 0x01 && r <= 0x08) || (r >= 0x0b && r <= 0x1f) || r == 0x7f
}

// isJSONControl is the range the jq clean in __ai_sanitize_json strips from
// every string: 0x01-0x08, 0x0B-0x1F and 0x7F-0x9F (the C1 controls too).
func isJSONControl(r rune) bool {
	return (r >= 0x01 && r <= 0x08) || (r >= 0x0b && r <= 0x1f) || (r >= 0x7f && r <= 0x9f)
}

// sanitizeJSON is __ai_sanitize_json: strip the control characters from
// every string value (object keys are left alone, as jq's with_entries
// cleans only .value), and print compact JSON with the members in their
// original order (jq -c). Invalid JSON returns ok=false.
func sanitizeJSON(raw []byte) ([]byte, bool) {
	dec := jsontext.NewDecoder(bytes.NewReader(raw))
	var out bytes.Buffer
	enc := jsontext.NewEncoder(&out)
	if err := cleanValue(dec, enc, false); err != nil {
		return nil, false
	}
	// A trailing token means more than one JSON value; jq would have read a
	// stream, but the reply is one object.
	if _, err := dec.ReadToken(); err == nil {
		return nil, false
	}
	return bytes.TrimRight(out.Bytes(), "\n"), true
}

// cleanValue copies one JSON value from dec to enc, cleaning string values
// (not object keys). isKey marks a string that is an object member name.
func cleanValue(dec *jsontext.Decoder, enc *jsontext.Encoder, isKey bool) error {
	tok, err := dec.ReadToken()
	if err != nil {
		return err
	}
	switch tok.Kind() {
	case '{':
		if err := enc.WriteToken(jsontext.BeginObject); err != nil {
			return err
		}
		for dec.PeekKind() != '}' {
			if err := cleanValue(dec, enc, true); err != nil { // key
				return err
			}
			if err := cleanValue(dec, enc, false); err != nil { // value
				return err
			}
		}
		if _, err := dec.ReadToken(); err != nil { // consume '}'
			return err
		}
		return enc.WriteToken(jsontext.EndObject)
	case '[':
		if err := enc.WriteToken(jsontext.BeginArray); err != nil {
			return err
		}
		for dec.PeekKind() != ']' {
			if err := cleanValue(dec, enc, false); err != nil {
				return err
			}
		}
		if _, err := dec.ReadToken(); err != nil { // consume ']'
			return err
		}
		return enc.WriteToken(jsontext.EndArray)
	case '"':
		s := tok.String()
		if !isKey {
			s = cleanControls(s)
		}
		return enc.WriteToken(jsontext.String(s))
	default:
		return enc.WriteToken(tok)
	}
}

// cleanControls drops the JSON control range from s (rune based, so the
// C1 controls go too).
func cleanControls(s string) string {
	if !strings.ContainsFunc(s, isJSONControl) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isJSONControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
