// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseJSONUnicodeEscapes: \uXXXX reads four hex digits (16 bits),
// joins a surrogate pair and turns a lone surrogate into U+FFFD.
func TestParseJSONUnicodeEscapes(t *testing.T) {
	for in, want := range map[string]string{
		`"\u00e9\uFFFF"`:  "\u00e9\uffff",
		`"\ud83d\ude00"`:  "\U0001f600",
		`"\ud83d x"`:      "\ufffd x",
		`"\u0041\u0000Z"`: "A\x00Z",
	} {
		v, err := parseJSON(in)
		require.NoError(t, err, in)
		require.Equal(t, []jv{jvString(want)}, v, in)
	}
	for _, in := range []string{`"\u00g1"`, `"\u12"`, `"\u+123"`} {
		_, err := parseJSON(in)
		require.Error(t, err, in)
	}
}
