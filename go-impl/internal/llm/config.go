// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
	"golang.org/x/sys/unix"

	"github.com/mauriziofonte/gash/internal/invoke"
)

// configMaxBytes is the default --max-bytes of llm_config.
const configMaxBytes = "1048576"

// config is llm_config (fs.sh __llm_config_impl).
func (m *Module) config(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	file, maxBytes := "", configMaxBytes
	args := c.Args
	for len(args) > 0 {
		a := args[0]
		switch {
		case a == "--max-bytes":
			if len(args) < 2 {
				return fail(c, "missing_value", a, a+" needs a value")
			}
			maxBytes = args[1]
			args = args[2:]
		case strings.HasPrefix(a, "-"):
			return fail(c, "unknown_option", a, "Use a file path and --max-bytes N")
		default:
			if file != "" {
				return fail(c, "unexpected_argument", a, "Pass one file per call")
			}
			file = a
			args = args[1:]
		}
	}
	if file == "" {
		return fail(c, "missing_file", "", "Provide a file path as argument")
	}
	if !validUint(c, "size", maxBytes) {
		return 1
	}
	if m.guard(c).IsSecretFile(file) {
		return fail(c, "secret_file_blocked", "Cannot read .env or credential files", "This file contains secrets and cannot be read")
	}
	safe, ok := m.validatePath(c, file)
	if !ok {
		return 1
	}
	if st, err := os.Stat(safe); err != nil || !st.Mode().IsRegular() {
		return fail(c, "file_not_found", safe, "Check the file path and try again")
	}
	if unix.Faccessat(unix.AT_FDCWD, safe, unix.R_OK, unix.AT_EACCESS) != nil {
		return fail(c, "path_unreadable", safe, "The file is not readable by this user")
	}
	if st, err := os.Stat(safe); err == nil && st.Size() > int64(atoi(maxBytes)) {
		return fail(c, "file_too_large", safe+" is "+strconv.FormatInt(st.Size(), 10)+" bytes",
			"Over the --max-bytes limit ("+maxBytes+"): raise --max-bytes, or read part of it with llm_exec \"head -c 65536 FILE\"")
	}
	if strings.HasSuffix(safe, ".yaml") || strings.HasSuffix(safe, ".yml") {
		data, err := m.readFile(safe)
		if err != nil {
			return fail(c, "path_unreadable", safe, "The file could not be read")
		}
		if js, err := yamlToJSON(data); err == nil {
			if js = strings.TrimRight(js, "\n"); js != "" {
				_, _ = io.WriteString(c.Stdout, js+"\n")
				return 0
			}
		}
		_, _ = c.Stdout.Write(data)
		return 0
	}
	f, err := os.Open(safe)
	if err != nil {
		return fail(c, "path_unreadable", safe, "The file could not be read")
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(c.Stdout, f); err != nil {
		return fail(c, "path_unreadable", safe, "The file could not be read")
	}
	return 0
}

// yamlToJSON converts every document of a YAML stream to JSON in the layout
// of mikefarah's yq eval -o=json (two-space indent, keys in document
// order, one document after another). Any parse error is an error: the
// caller prints the raw text, as 2.x did when yq failed.
func yamlToJSON(data []byte) (string, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var out strings.Builder
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		var b []byte
		var node *yaml.Node
		if len(doc.Content) > 0 {
			node = doc.Content[0]
		}
		b, err = appendYAMLJSON(b, node, "")
		if err != nil {
			return "", err
		}
		out.Write(b)
		out.WriteByte('\n')
	}
	return out.String(), nil
}

var errYAMLValue = errors.New("yaml value has no JSON form")

func appendYAMLJSON(b []byte, n *yaml.Node, indent string) ([]byte, error) {
	if n == nil {
		return append(b, "null"...), nil
	}
	inner := indent + "  "
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return append(b, "null"...), nil
		}
		return appendYAMLJSON(b, n.Content[0], indent)
	case yaml.AliasNode:
		return appendYAMLJSON(b, n.Alias, indent)
	case yaml.MappingNode:
		if len(n.Content) == 0 {
			return append(b, "{}"...), nil
		}
		b = append(b, "{\n"...)
		for i := 0; i+1 < len(n.Content); i += 2 {
			if i > 0 {
				b = append(b, ",\n"...)
			}
			b = append(b, inner...)
			b = appendJSONString(b, n.Content[i].Value)
			b = append(b, ": "...)
			var err error
			if b, err = appendYAMLJSON(b, n.Content[i+1], inner); err != nil {
				return nil, err
			}
		}
		return append(append(append(b, '\n'), indent...), '}'), nil
	case yaml.SequenceNode:
		if len(n.Content) == 0 {
			return append(b, "[]"...), nil
		}
		b = append(b, "[\n"...)
		for i, item := range n.Content {
			if i > 0 {
				b = append(b, ",\n"...)
			}
			b = append(b, inner...)
			var err error
			if b, err = appendYAMLJSON(b, item, inner); err != nil {
				return nil, err
			}
		}
		return append(append(append(b, '\n'), indent...), ']'), nil
	}
	switch n.ShortTag() {
	case "!!null":
		return append(b, "null"...), nil
	case "!!bool":
		var v bool
		if err := n.Decode(&v); err != nil {
			return nil, err
		}
		return strconv.AppendBool(b, v), nil
	case "!!int":
		var v int64
		if err := n.Decode(&v); err == nil {
			return strconv.AppendInt(b, v, 10), nil
		}
		var u uint64
		if err := n.Decode(&u); err != nil {
			return nil, err
		}
		return strconv.AppendUint(b, u, 10), nil
	case "!!float":
		var v float64
		if err := n.Decode(&v); err != nil {
			return nil, err
		}
		if math.IsInf(v, 0) || math.IsNaN(v) {
			return nil, errYAMLValue
		}
		js, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		return append(b, js...), nil
	}
	return appendJSONString(b, n.Value), nil
}

// appendJSONString is encoding/json with HTML escaping off, as yq's JSON
// encoder writes strings.
func appendJSONString(b []byte, s string) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return append(b, bytes.TrimRight(buf.Bytes(), "\n")...)
}
