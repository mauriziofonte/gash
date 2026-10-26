// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"errors"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"golang.org/x/sys/unix"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/jsonpath"
)

// configMaxBytes is the default --max-bytes of llm_config.
const configMaxBytes = "1048576"

// config is llm_config (fs.sh __llm_config_impl). Without --path it
// prints what it printed before 3.4: YAML converted to JSON, any other
// file as its raw text. With --path (3.4, plan D6) it parses the file as
// YAML, TOML, INI or JSON and prints the value at the path as compact
// JSON.
func (m *Module) config(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	file, maxBytes := "", configMaxBytes
	path, pathSet := "", false
	args := c.Args
	for len(args) > 0 {
		a := args[0]
		switch {
		case a == "--max-bytes" || a == "--path":
			if len(args) < 2 {
				return fail(c, "missing_value", a, a+" needs a value")
			}
			if a == "--path" {
				path, pathSet = args[1], true
			} else {
				maxBytes = args[1]
			}
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
	var steps []jsonpath.Step
	if pathSet {
		var err error
		if steps, err = jsonpath.Parse(path); err != nil {
			return fail(c, "invalid_json_path", path, `Write keys with dots and indexes in brackets: a.b[0].c, tool.poetry["name"], [-1]; . is the whole file`)
		}
	}
	if m.refuseSecret(c, file, "Cannot read .env or credential files") {
		return 1
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
	if pathSet {
		data, err := m.readFile(safe)
		if err != nil {
			return fail(c, "path_unreadable", safe, "The file could not be read")
		}
		return configPath(c, safe, data, path, steps)
	}
	if strings.HasSuffix(safe, ".yaml") || strings.HasSuffix(safe, ".yml") {
		data, err := m.readFile(safe)
		if err != nil {
			return fail(c, "path_unreadable", safe, "The file could not be read")
		}
		// LEDGER LLM-35: JSON as python-yq and jq printed it; the raw
		// text for a file that does not convert or holds no document.
		if docs, err := yamlDocs(data); err == nil && len(docs) > 0 {
			_ = writeYAMLJSON(c.Stdout, docs)
			return 0
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

// The formats llm_config --path reads.
const (
	cfgJSON = "JSON"
	cfgTOML = "TOML"
	cfgINI  = "INI"
	cfgYAML = "YAML"
)

// configFormat is the format of file by its extension; "" when the
// extension names none.
func configFormat(file string) string {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".json":
		return cfgJSON
	case ".toml":
		return cfgTOML
	case ".ini", ".cfg":
		return cfgINI
	case ".yaml", ".yml":
		return cfgYAML
	}
	return ""
}

// configPath prints the value at steps of a config file as compact JSON.
// The extension names the format; for another extension the first of
// JSON, TOML, INI and YAML that parses is taken.
func configPath(c *invoke.Call, file string, data []byte, path string, steps []jsonpath.Step) int {
	format := configFormat(file)
	parseFail := func(format string, err error) int {
		return fail(c, "invalid_config", format+" "+err.Error(),
			"The file does not parse as "+format+": fix it, or read its raw text with llm_config FILE (without --path)")
	}
	if format == cfgJSON || format == "" {
		start, err := validJSON(data)
		if err == nil {
			v, n, found := walkPath(valueAt(data, start), steps)
			if !found {
				return fail(c, "json_path_not_found", path, pathHint(steps[:n], v))
			}
			_, _ = c.Stdout.Write(append(appendCompact(nil, v.raw(), 0), '\n'))
			return 0
		}
		if format == cfgJSON {
			return parseFail(format, jsonErrorText(data, err))
		}
	}
	var root *yv
	if format != "" {
		var err error
		if root, err = configValue(format, data); err != nil {
			return parseFail(format, err)
		}
	} else {
		for _, f := range []string{cfgTOML, cfgINI, cfgYAML} {
			if v, err := configValue(f, data); err == nil {
				root = v
				break
			}
		}
		if root == nil {
			return fail(c, "invalid_config", "not JSON, TOML, INI or YAML",
				"Read the raw text with llm_config FILE (without --path)")
		}
	}
	v, n, found := yvPath(root, steps)
	if !found {
		return fail(c, "json_path_not_found", path, yvHint(steps[:n], v))
	}
	_, _ = c.Stdout.Write(append(appendYV(nil, v), '\n'))
	return 0
}

// configValue parses data as format into one value. A YAML stream with
// several documents is the array of its documents; with none, null.
func configValue(format string, data []byte) (*yv, error) {
	switch format {
	case cfgTOML:
		return tomlValue(data)
	case cfgINI:
		return iniValue(data)
	}
	docs, err := yamlDocs(data)
	if err != nil {
		return nil, errors.New(strings.TrimPrefix(err.Error(), "yaml: "))
	}
	switch len(docs) {
	case 0:
		return nullValue(), nil
	case 1:
		return docs[0], nil
	}
	return &yv{kind: 'a', arr: docs}, nil
}

// yvPath walks steps from root: the value reached, how many steps it
// took and whether it took all of them.
func yvPath(root *yv, steps []jsonpath.Step) (*yv, int, bool) {
	cur := root
	for n, st := range steps {
		switch {
		case cur.kind == 'o' && !st.IsIndex:
			i := -1
			for j, k := range cur.keys {
				if k == st.Key {
					i = j
				}
			}
			if i < 0 {
				return cur, n, false
			}
			cur = cur.vals[i]
		case cur.kind == 'a' && st.IsIndex:
			i := st.Index
			if i < 0 {
				i += len(cur.arr)
			}
			if i < 0 || i >= len(cur.arr) {
				return cur, n, false
			}
			cur = cur.arr[i]
		default:
			return cur, n, false
		}
	}
	return cur, len(steps), true
}

// yvHint is the json_path_not_found hint for a converted value.
func yvHint(prefix []jsonpath.Step, at *yv) string {
	switch at.kind {
	case 'o':
		return prefixHint(prefix, "object", at.keys, 0)
	case 'a':
		return prefixHint(prefix, "array", nil, len(at.arr))
	case 's':
		return prefixHint(prefix, "string", nil, 0)
	case 'd':
		return prefixHint(prefix, "number", nil, 0)
	case 'b':
		return prefixHint(prefix, "boolean", nil, 0)
	}
	return prefixHint(prefix, "null", nil, 0)
}

// appendYV appends v as compact JSON, strings escaped as the YAML output
// escapes them.
func appendYV(b []byte, v *yv) []byte {
	switch v.kind {
	case 'n':
		return append(b, "null"...)
	case 'b':
		return strconv.AppendBool(b, v.b)
	case 'd':
		return append(b, v.num...)
	case 's':
		return contract.AppendString(b, v.str, contract.EscapeJQ)
	case 'a':
		b = append(b, '[')
		for i, item := range v.arr {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendYV(b, item)
		}
		return append(b, ']')
	}
	b = append(b, '{')
	for i, k := range v.keys {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(contract.AppendString(b, k, contract.EscapeJQ), ':')
		b = appendYV(b, v.vals[i])
	}
	return append(b, '}')
}

// tomlValue decodes a TOML document. Tables keep the order of their keys
// in the file (an array of tables, per element; the inline tables of an
// inline array, the order of the first one); dates and times are their
// RFC 3339 text (a local date, time or date-time without an offset).
func tomlValue(data []byte) (*yv, error) {
	var root map[string]any
	md, err := toml.Decode(string(data), &root)
	if err != nil {
		var pe toml.ParseError
		if errors.As(err, &pe) {
			return nil, errors.New("line " + strconv.Itoa(pe.Position.Line) + ": " + pe.Message)
		}
		return nil, errors.New(strings.TrimPrefix(err.Error(), "toml: "))
	}
	order := map[string]int{}
	elem := map[string]int{}
	for i, k := range md.Keys() {
		var cur any = root
		p := ""
		for j, part := range k {
			p += "\x00" + part
			if _, ok := order[p]; !ok {
				order[p] = i
			}
			var next any
			if t, ok := cur.(map[string]any); ok {
				next = t[part]
			}
			if tables, ok := next.([]map[string]any); ok {
				if j == len(k)-1 {
					// The [[header]] of the next element.
					if e, seen := elem[p]; seen {
						elem[p] = e + 1
					} else {
						elem[p] = 0
					}
				}
				e := elem[p]
				p += "\x00#" + strconv.Itoa(e)
				next = nil
				if e < len(tables) {
					next = tables[e]
				}
			}
			cur = next
		}
	}
	return tomlConvert(root, "", order)
}

func tomlConvert(v any, p string, order map[string]int) (*yv, error) {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.SliceStable(keys, func(i, j int) bool {
			oi, iok := order[p+"\x00"+keys[i]]
			oj, jok := order[p+"\x00"+keys[j]]
			if iok != jok {
				return iok
			}
			if iok && oi != oj {
				return oi < oj
			}
			return keys[i] < keys[j]
		})
		out := &yv{kind: 'o'}
		for _, k := range keys {
			cv, err := tomlConvert(x[k], p+"\x00"+k, order)
			if err != nil {
				return nil, err
			}
			out.keys = append(out.keys, k)
			out.vals = append(out.vals, cv)
		}
		return out, nil
	case []map[string]any:
		out := &yv{kind: 'a'}
		for i, t := range x {
			cv, err := tomlConvert(t, p+"\x00#"+strconv.Itoa(i), order)
			if err != nil {
				return nil, err
			}
			out.arr = append(out.arr, cv)
		}
		return out, nil
	case []any:
		out := &yv{kind: 'a'}
		for _, item := range x {
			cv, err := tomlConvert(item, p, order)
			if err != nil {
				return nil, err
			}
			out.arr = append(out.arr, cv)
		}
		return out, nil
	case string:
		return stringValue(x), nil
	case bool:
		return boolValue(x), nil
	case int64:
		return intValue(big.NewInt(x))
	case float64:
		return floatValue(x), nil
	case time.Time:
		switch x.Location().String() {
		case "datetime-local":
			return stringValue(x.Format("2006-01-02T15:04:05.999999999")), nil
		case "date-local":
			return stringValue(x.Format("2006-01-02")), nil
		case "time-local":
			return stringValue(x.Format("15:04:05.999999999")), nil
		}
		return stringValue(x.Format(time.RFC3339Nano)), nil
	}
	return nil, errors.New("a value with no JSON form")
}

// iniValue parses an INI file as Python's configparser.ConfigParser()
// reads it: [section] headers (DEFAULT holds the defaults every section
// inherits), "key = value" or "key: value" lines (keys lowercased, both
// sides stripped), whole-line # and ; comments, values continued on
// lines indented deeper than their key (joined with newlines; blank lines
// inside kept, trailing ones dropped), %(key)s and %% expanded as
// BasicInterpolation does (a value that does not expand stays as
// written). Unlike strict configparser a repeated section merges and a
// repeated key keeps its last value. The result is {section: {key:
// value}} in file order, DEFAULT first when it has keys, each section's
// own keys followed by the defaults it does not override.
func iniValue(data []byte) (*yv, error) {
	type section struct {
		name string
		keys []string
		vals map[string][]string
	}
	defaults := &section{name: "DEFAULT", vals: map[string][]string{}}
	var sections []*section
	byName := map[string]*section{}
	var cur *section
	opt := ""
	indent := 0
	text := strings.TrimPrefix(string(data), "\xef\xbb\xbf")
	for n, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		lineno := n + 1
		value := strings.TrimSpace(line)
		comment := strings.HasPrefix(value, "#") || strings.HasPrefix(value, ";")
		if value == "" || comment {
			if !comment && cur != nil && opt != "" {
				cur.vals[opt] = append(cur.vals[opt], "")
			}
			continue
		}
		lead := len(line) - len(strings.TrimLeft(line, " \t\f\v"))
		if cur != nil && opt != "" && lead > indent {
			cur.vals[opt] = append(cur.vals[opt], value)
			continue
		}
		indent = lead
		if strings.HasPrefix(value, "[") {
			if end := strings.LastIndexByte(value, ']'); end > 1 {
				name := value[1:end]
				switch {
				case name == "DEFAULT":
					cur = defaults
				case byName[name] != nil:
					cur = byName[name]
				default:
					cur = &section{name: name, vals: map[string][]string{}}
					byName[name] = cur
					sections = append(sections, cur)
				}
				opt = ""
				continue
			}
		}
		if cur == nil {
			return nil, errors.New("line " + strconv.Itoa(lineno) + ": a key before any [section] header")
		}
		i := strings.IndexAny(value, "=:")
		if i < 0 {
			return nil, errors.New("line " + strconv.Itoa(lineno) + ": no '=' or ':' after the key")
		}
		key := strings.ToLower(strings.TrimSpace(value[:i]))
		if key == "" {
			return nil, errors.New("line " + strconv.Itoa(lineno) + ": a value with no key")
		}
		if _, ok := cur.vals[key]; !ok {
			cur.keys = append(cur.keys, key)
		}
		cur.vals[key] = []string{strings.TrimSpace(value[i+1:])}
		opt = key
	}
	joined := func(s *section) map[string]string {
		out := make(map[string]string, len(s.vals))
		for k, lines := range s.vals {
			out[k] = strings.TrimRightFunc(strings.Join(lines, "\n"), func(r rune) bool { return strings.ContainsRune(" \t\n\r\f\v", r) })
		}
		return out
	}
	defs := joined(defaults)
	object := func(s *section) *yv {
		own := defs
		order := defaults.keys
		if s != defaults {
			own = joined(s)
			order = append([]string(nil), s.keys...)
			for _, k := range defaults.keys {
				if _, ok := own[k]; !ok {
					order = append(order, k)
				}
			}
		}
		lookup := func(k string) (string, bool) {
			if v, ok := own[k]; ok {
				return v, true
			}
			v, ok := defs[k]
			return v, ok
		}
		out := &yv{kind: 'o'}
		for _, k := range order {
			raw, _ := lookup(k)
			v, ok := iniInterpolate(raw, lookup, 1)
			if !ok {
				v = raw
			}
			out.keys = append(out.keys, k)
			out.vals = append(out.vals, stringValue(v))
		}
		return out
	}
	root := &yv{kind: 'o'}
	if len(defaults.keys) > 0 {
		root.keys = append(root.keys, "DEFAULT")
		root.vals = append(root.vals, object(defaults))
	}
	for _, s := range sections {
		root.keys = append(root.keys, s.name)
		root.vals = append(root.vals, object(s))
	}
	return root, nil
}

// iniInterpolate expands %(key)s and %% in raw as configparser's
// BasicInterpolation does, at most 10 levels deep; false where Python
// raises (a lone %, a missing key, too deep).
func iniInterpolate(raw string, lookup func(string) (string, bool), depth int) (string, bool) {
	if depth > 10 {
		return "", false
	}
	var b strings.Builder
	rest := raw
	for rest != "" {
		p := strings.IndexByte(rest, '%')
		if p < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:p])
		rest = rest[p:]
		switch {
		case strings.HasPrefix(rest, "%%"):
			b.WriteByte('%')
			rest = rest[2:]
		case strings.HasPrefix(rest, "%("):
			end := strings.Index(rest, ")s")
			name := ""
			if end > 2 {
				name = rest[2:end]
			}
			if name == "" || strings.ContainsRune(name, ')') {
				return "", false
			}
			v, ok := lookup(strings.ToLower(name))
			if !ok {
				return "", false
			}
			rest = rest[end+2:]
			if strings.Contains(v, "%") {
				if v, ok = iniInterpolate(v, lookup, depth+1); !ok {
					return "", false
				}
			}
			b.WriteString(v)
		default:
			return "", false
		}
	}
	return b.String(), true
}
