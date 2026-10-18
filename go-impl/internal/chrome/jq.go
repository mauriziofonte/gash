// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package chrome

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

// The four jq programs of 2.x chrome.sh, evaluated natively where 2.x
// piped curl into `jq -r` (SYS-36). jq semantics kept: every JSON text of
// the input is a run of the program; a runtime error ends that run (its
// earlier outputs stay) and the next text is read; a text that does not
// parse ends the input; `-r` prints strings raw and other values as
// indented JSON.

var errJQ = errors.New("jq: runtime error")

// jqRun runs prog over every JSON text of body and returns the -r output
// as $(...) keeps it.
func jqRun(body []byte, prog func(json.RawMessage) ([]json.RawMessage, error)) string {
	var lines []string
	dec := json.NewDecoder(bytes.NewReader(body))
	for {
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			break
		}
		outs, _ := prog(v)
		for _, o := range outs {
			lines = append(lines, rawOut(o))
		}
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// rawOut is one value as jq -r prints it.
func rawOut(v json.RawMessage) string {
	if s, ok := jqString(v); ok {
		return s
	}
	var b bytes.Buffer
	if json.Indent(&b, v, "", "  ") != nil {
		return string(v)
	}
	return b.String()
}

func lead(v json.RawMessage) byte {
	t := bytes.TrimLeft(v, " \t\r\n")
	if len(t) == 0 {
		return 0
	}
	return t[0]
}

// jqString decodes a JSON string.
func jqString(v json.RawMessage) (string, bool) {
	if lead(v) != '"' {
		return "", false
	}
	var s string
	if json.Unmarshal(v, &s) != nil {
		return "", false
	}
	return s, true
}

var jsonNull = json.RawMessage("null")

// index is .NAME: the member of an object (null when absent), null on
// null, an error on anything else.
func index(v json.RawMessage, name string) (json.RawMessage, error) {
	switch lead(v) {
	case 'n':
		return jsonNull, nil
	case '{':
		var m map[string]json.RawMessage
		if json.Unmarshal(v, &m) != nil {
			return nil, errJQ
		}
		if x, ok := m[name]; ok {
			return x, nil
		}
		return jsonNull, nil
	}
	return nil, errJQ
}

// each is .[]: the elements of an array or the values of an object, in
// order; an error on anything else.
func each(v json.RawMessage) ([]json.RawMessage, error) {
	switch lead(v) {
	case '[':
		var a []json.RawMessage
		if json.Unmarshal(v, &a) != nil {
			return nil, errJQ
		}
		return a, nil
	case '{':
		dec := json.NewDecoder(bytes.NewReader(v))
		if _, err := dec.Token(); err != nil {
			return nil, errJQ
		}
		var vals []json.RawMessage
		for dec.More() {
			if _, err := dec.Token(); err != nil {
				return nil, errJQ
			}
			var x json.RawMessage
			if dec.Decode(&x) != nil {
				return nil, errJQ
			}
			vals = append(vals, x)
		}
		return vals, nil
	}
	return nil, errJQ
}

// truthy is jq's truth: anything but null and false.
func truthy(v json.RawMessage) bool {
	switch lead(v) {
	case 'n', 'f':
		return false
	}
	return true
}

// fieldIs is select(.NAME == "S") on one value: ok reports the match, err
// a value .NAME cannot index.
func fieldIs(v json.RawMessage, name, s string) (bool, error) {
	f, err := index(v, name)
	if err != nil {
		return false, err
	}
	got, ok := jqString(f)
	return ok && got == s, nil
}

// orEmpty is `X // empty`.
func orEmpty(v json.RawMessage) []json.RawMessage {
	if truthy(v) {
		return []json.RawMessage{v}
	}
	return nil
}

// jqVersion is `.version // empty`.
func jqVersion(v json.RawMessage) ([]json.RawMessage, error) {
	f, err := index(v, "version")
	if err != nil {
		return nil, err
	}
	return orEmpty(f), nil
}

// jqShell is `.browsers[] | select(.name == "chromium-headless-shell") |
// .FIELD // empty`.
func jqShell(field string) func(json.RawMessage) ([]json.RawMessage, error) {
	return func(v json.RawMessage) ([]json.RawMessage, error) {
		b, err := index(v, "browsers")
		if err != nil {
			return nil, err
		}
		all, err := each(b)
		if err != nil {
			return nil, err
		}
		var out []json.RawMessage
		for _, e := range all {
			ok, err := fieldIs(e, "name", "chromium-headless-shell")
			if err != nil {
				return out, err
			}
			if !ok {
				continue
			}
			f, err := index(e, field)
			if err != nil {
				return out, err
			}
			out = append(out, orEmpty(f)...)
		}
		return out, nil
	}
}

// jqLinux64 is `.downloads["chrome-headless-shell"][]? | select(.platform
// == "linux64") | .url` on one value.
func jqLinux64(v json.RawMessage) ([]json.RawMessage, error) {
	d, err := index(v, "downloads")
	if err != nil {
		return nil, err
	}
	s, err := index(d, "chrome-headless-shell")
	if err != nil {
		return nil, err
	}
	// `[]?` drops the error of the iteration only, as jq 1.7 (Ubuntu
	// 24.04, Debian 13) does; jq 1.6 dropped the indexing errors too.
	all, _ := each(s)
	var out []json.RawMessage
	for _, e := range all {
		ok, err := fieldIs(e, "platform", "linux64")
		if err != nil {
			return out, err
		}
		if !ok {
			continue
		}
		u, err := index(e, "url")
		if err != nil {
			return out, err
		}
		out = append(out, u)
	}
	return out, nil
}

// jqKnownGood is `.versions[] | select(.version == $v) | SEL`.
func jqKnownGood(version string) func(json.RawMessage) ([]json.RawMessage, error) {
	return func(v json.RawMessage) ([]json.RawMessage, error) {
		vs, err := index(v, "versions")
		if err != nil {
			return nil, err
		}
		all, err := each(vs)
		if err != nil {
			return nil, err
		}
		var out []json.RawMessage
		for _, e := range all {
			ok, err := fieldIs(e, "version", version)
			if err != nil {
				return out, err
			}
			if !ok {
				continue
			}
			urls, err := jqLinux64(e)
			out = append(out, urls...)
			if err != nil {
				return out, err
			}
		}
		return out, nil
	}
}
