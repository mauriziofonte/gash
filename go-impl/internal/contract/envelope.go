// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package contract

import (
	"fmt"
	"io"
)

// Envelope is the machine error of the contract, one JSON line on stderr:
//
//	{"error":CODE,"details":..,"action":ACTION,"recoverable":BOOL,"hint":..}
//
// details and hint are left out when empty; recoverable is false exactly
// for FATAL; Extra members (query_time of the llm_db_* family) follow, in
// order.
type Envelope struct {
	Code    string
	Details string
	Action  Action
	Hint    string
	// Extra members appended after hint. 2.x added them with jq
	// (`. + {query_time: $qt}`), which re-encoded the whole object: an
	// envelope with Extra members is written with EscapeJQ.
	Extra []Field
	// Escape overrides the string escaping. The zero value picks EscapeBash
	// (the printf envelope of __gash_json_error) when Extra is empty and
	// EscapeJQ otherwise.
	Escape *Escape
}

// Field is one extra string member of an envelope.
type Field struct {
	Key, Value string
}

// New returns the envelope of a code with its fixed action. It panics on
// a code that is not in the table: a code can only be emitted with its
// action, so an unknown code is a programming error.
func New(code, details, hint string) Envelope {
	a, ok := ActionOf(code)
	if !ok {
		panic(fmt.Sprintf("contract: unknown error code %q", code))
	}
	return Envelope{Code: code, Details: details, Action: a, Hint: hint}
}

// Legacy returns the envelope exactly as 2.x __gash_json_error built it
// from free arguments: an empty code becomes unknown_error, an action that
// is not one of the four becomes STOP. It exists for the compat layer,
// which forwards calls made by external callers; Go code uses New.
func Legacy(code, details, action, hint string) Envelope {
	if code == "" {
		code = "unknown_error"
	}
	if action == "" {
		action = string(Stop)
	}
	return Envelope{Code: code, Details: details, Action: NormalizeAction(action), Hint: hint}
}

// With returns a copy of e with an extra member appended.
func (e Envelope) With(key, value string) Envelope {
	e.Extra = append(append([]Field(nil), e.Extra...), Field{key, value})
	return e
}

func (e Envelope) escape() Escape {
	if e.Escape != nil {
		return *e.Escape
	}
	if len(e.Extra) > 0 {
		return EscapeJQ
	}
	return EscapeBash
}

// Bytes returns the envelope line, newline included.
func (e Envelope) Bytes() []byte {
	action := NormalizeAction(string(e.Action))
	var o Object
	o.String("error", e.Code)
	if e.Details != "" {
		o.String("details", e.Details)
	}
	o.String("action", string(action))
	o.Bool("recoverable", action.Recoverable())
	if e.Hint != "" {
		o.String("hint", e.Hint)
	}
	for _, f := range e.Extra {
		o.String(f.Key, f.Value)
	}
	b, _ := o.Append(make([]byte, 0, 128), e.escape()) // no raw members: cannot fail
	return append(b, '\n')
}

// Write writes the envelope line to w (stderr by the stream policy).
func (e Envelope) Write(w io.Writer) error {
	_, err := w.Write(e.Bytes())
	return err
}

// String returns the envelope line without the newline.
func (e Envelope) String() string {
	b := e.Bytes()
	return string(b[:len(b)-1])
}
