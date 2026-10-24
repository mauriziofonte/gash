// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

// The envelope schemas (design/architecture.md 4, testing.md 4): one JSON
// Schema per output shape under testdata/schema/, validated here in unit
// tests and, through the `schema` command of the testscript suite, against
// the real output of every --json function (testdata/script/schema_*.txtar).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/compatgen"
	"github.com/mauriziofonte/gash/internal/contract"
)

var (
	schemaMu    sync.Mutex
	schemaCache = map[string]*jsonschema.Schema{}
)

// loadSchema compiles testdata/schema/<name>.json of the checkout at root
// (once per name).
func loadSchema(root, name string) (*jsonschema.Schema, error) {
	schemaMu.Lock()
	defer schemaMu.Unlock()
	if s, ok := schemaCache[name]; ok {
		return s, nil
	}
	path := filepath.Join(root, "testdata", "schema", name+".json")
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("no schema %q: %w", name, err)
	}
	s, err := jsonschema.NewCompiler().Compile(path)
	if err != nil {
		return nil, err
	}
	schemaCache[name] = s
	return s, nil
}

// validateJSON checks that data is one or more JSON values (a pretty
// object, or one compact value per line) and that each one is valid
// against s. Text that is not JSON is an error.
func validateJSON(s *jsonschema.Schema, data []byte) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return errors.New("no JSON value")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	for n := 1; ; n++ {
		var raw json.RawMessage
		err := dec.Decode(&raw)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("value %d: %v", n, err)
		}
		// UnmarshalJSON keeps numbers as json.Number, as the validator
		// wants them (integer vs number).
		v, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			return fmt.Errorf("value %d: %v", n, err)
		}
		if err := s.Validate(v); err != nil {
			return fmt.Errorf("value %d (%s): %v", n, raw, err)
		}
	}
}

func schemaRoot(t testing.TB) string {
	t.Helper()
	root, err := compatgen.FindRoot(".")
	if err != nil {
		t.Skip("not in a Gash checkout:", err)
	}
	return root
}

// cmdSchema is the testscript command `schema NAME stdout|stderr|FILE`:
// the content must validate against testdata/schema/NAME.json. With `!`
// it must not (a negative check that the schema is not vacuous).
func cmdSchema(root string) func(ts *testscript.TestScript, neg bool, args []string) {
	return func(ts *testscript.TestScript, neg bool, args []string) {
		if len(args) != 2 {
			ts.Fatalf("usage: schema NAME stdout|stderr|FILE")
		}
		s, err := loadSchema(root, args[0])
		ts.Check(err)
		data := ts.ReadFile(args[1]) // stdout, stderr or a file of $WORK
		err = validateJSON(s, []byte(data))
		switch {
		case err != nil && !neg:
			ts.Fatalf("%s does not match schema %s: %v\n%s", args[1], args[0], err, data)
		case err == nil && neg:
			ts.Fatalf("%s unexpectedly matches schema %s:\n%s", args[1], args[0], data)
		}
	}
}

// TestSchemasCompile: every schema file is a valid 2020-12 schema.
func TestSchemasCompile(t *testing.T) {
	root := schemaRoot(t)
	files, err := filepath.Glob(filepath.Join(root, "testdata", "schema", "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".json")
		_, err := loadSchema(root, name)
		require.NoError(t, err, name)
	}
}

// TestSchemaErrorEnvelope: the envelope of every code of the contract
// table, with and without details, hint and query_time, matches
// error.json; a broken envelope does not.
func TestSchemaErrorEnvelope(t *testing.T) {
	s, err := loadSchema(schemaRoot(t), "error")
	require.NoError(t, err)
	codes := contract.Codes()
	require.NotEmpty(t, codes)
	for _, c := range codes {
		for _, e := range []contract.Envelope{
			contract.New(c.Name, "", ""),
			contract.New(c.Name, "some \"details\"\n", "Run: x"),
			contract.New(c.Name, "d", "h").With("query_time", "12.5ms"),
		} {
			require.NoError(t, validateJSON(s, e.Bytes()), "%s: %s", c.Name, e.Bytes())
		}
	}
	// Legacy envelopes (the compat layer's __gash_json_error) too.
	require.NoError(t, validateJSON(s, contract.Legacy("", "", "bogus", "").Bytes()))
	bad := map[string]string{
		"FATAL recoverable":   `{"error":"x","action":"FATAL","recoverable":true}`,
		"RETRY unrecoverable": `{"error":"x","action":"RETRY","recoverable":false}`,
		"unknown action":      `{"error":"x","action":"MAYBE","recoverable":true}`,
		"missing action":      `{"error":"x","recoverable":true}`,
		"empty details":       `{"error":"x","details":"","action":"STOP","recoverable":true}`,
		"empty hint":          `{"error":"x","action":"STOP","recoverable":true,"hint":""}`,
		"extra member":        `{"error":"x","action":"STOP","recoverable":true,"other":1}`,
		"bad code":            `{"error":"Not A Code","action":"STOP","recoverable":true}`,
		"bad query_time":      `{"error":"x","action":"STOP","recoverable":true,"query_time":"soon"}`,
		"not JSON":            `Error: x`,
	}
	names := make([]string, 0, len(bad))
	for k := range bad {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		require.Error(t, validateJSON(s, []byte(bad[k])), k)
	}
}

// TestSchemaDocumentedShapes: the examples of AGENTS.md section 4 (real
// output, whitespace changed) match their schemas, so the schemas and the
// documentation describe the same shapes.
func TestSchemaDocumentedShapes(t *testing.T) {
	root := schemaRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	require.NoError(t, err)
	doc := string(b)
	sections := map[string]string{
		"### 4.1 Size list":    "size_list",
		"### 4.2 Empty dirs":   "empty_dirs",
		"### 4.3 Tree stats":   "tree_stats",
		"### 4.4 Database":     "db",
		"### 4.5 Tree":         "llm_tree",
		"### 4.6 Git status":   "git_status",
		"## 5. Error contract": "error",
	}
	for heading, name := range sections {
		i := strings.Index(doc, heading)
		require.GreaterOrEqual(t, i, 0, heading)
		rest := doc[i:]
		start := strings.Index(rest, "```json\n")
		require.GreaterOrEqual(t, start, 0, heading)
		rest = rest[start+len("```json\n"):]
		end := strings.Index(rest, "```")
		require.GreaterOrEqual(t, end, 0, heading)
		s, err := loadSchema(root, name)
		require.NoError(t, err)
		require.NoError(t, validateJSON(s, []byte(rest[:end])), heading)
	}
}
