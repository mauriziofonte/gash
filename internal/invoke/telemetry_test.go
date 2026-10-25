// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package invoke

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// GASH_TELEMETRY_FILE (plan S10): one line per agent function call with
// the name, time, output bytes, exit status and truncation, never the
// arguments; off when unset; other functions are not logged.
func TestTelemetryLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	var out bytes.Buffer
	c := &Call{Name: "llm_find", Args: []string{"secret-arg"}, Env: EnvMap(map[string]string{TelemetryVar: path}), Stdout: &out}
	rc := WithTelemetry(c, c.Name, func() int {
		_, _ = c.Stdout.Write([]byte("abc\n"))
		c.NoteTruncated(2)
		return 3
	})
	require.Equal(t, 3, rc)
	require.Equal(t, "abc\n", out.String())
	require.Same(t, &out, c.Stdout.(*bytes.Buffer))
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(b), "secret-arg")
	var line struct {
		TS        int64   `json:"ts"`
		Fn        string  `json:"fn"`
		Ms        float64 `json:"ms"`
		OutBytes  int     `json:"out_bytes"`
		RC        int     `json:"rc"`
		Truncated int     `json:"truncated"`
	}
	require.NoError(t, json.Unmarshal(b, &line))
	require.Equal(t, "llm_find", line.Fn)
	require.Equal(t, 4, line.OutBytes)
	require.Equal(t, 3, line.RC)
	require.Equal(t, 2, line.Truncated)
	require.Positive(t, line.TS)
	st, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), st.Mode().Perm())

	// A second call appends.
	WithTelemetry(c, c.Name, func() int { return 0 })
	b, _ = os.ReadFile(path)
	require.Equal(t, 2, strings.Count(string(b), "\n"))

	// Unset: nothing is written; an unwritable path never fails the call.
	c.Env = EnvMap(nil)
	require.Equal(t, 0, WithTelemetry(c, c.Name, func() int { return 0 }))
	c.Env = EnvMap(map[string]string{TelemetryVar: filepath.Join(t.TempDir(), "no", "such", "dir", "t.jsonl")})
	require.Equal(t, 5, WithTelemetry(c, c.Name, func() int { return 5 }))

	require.True(t, Telemetered("llm_grep"))
	require.True(t, Telemetered("agent_replace"))
	require.False(t, Telemetered("files_largest"))
}
