// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package jsonpath

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

const doc = `{"a":{"b":[{"c":1},{"c":2}]},"paths":{"/api/x":{"post":"ok"}},"k.e y":true,"list":[10,20,30]}`

func decode(t *testing.T) any {
	t.Helper()
	var v any
	require.NoError(t, json.Unmarshal([]byte(doc), &v))
	return v
}

func TestGet(t *testing.T) {
	v := decode(t)
	for p, want := range map[string]any{
		"a.b[1].c":                2.0,
		".a.b[0].c":               1.0,
		"$.a.b[-1].c":             2.0,
		`paths["/api/x"].post`:    "ok",
		`paths['/api/x']['post']`: "ok",
		`["k.e y"]`:               true,
		"list[2]":                 30.0,
	} {
		steps, err := Parse(p)
		require.NoError(t, err, p)
		got, _, ok := Get(v, steps)
		require.True(t, ok, p)
		require.Equal(t, want, got, p)
	}
	steps, err := Parse("")
	require.NoError(t, err)
	got, _, ok := Get(v, steps)
	require.True(t, ok)
	require.Equal(t, v, got)
}

func TestGetMissingGivesNearestPrefix(t *testing.T) {
	v := decode(t)
	steps, err := Parse("a.b[5].c")
	require.NoError(t, err)
	_, near, ok := Get(v, steps)
	require.False(t, ok)
	require.Equal(t, "a.b", Format(near))
	steps, _ = Parse("a.zz")
	_, near, ok = Get(v, steps)
	require.False(t, ok)
	require.Equal(t, "a", Format(near))
	steps, _ = Parse("list.x")
	_, near, _ = Get(v, steps)
	require.Equal(t, "list", Format(near))
}

func TestParseErrors(t *testing.T) {
	for _, p := range []string{"a..b", "a[", "a[x]", `a["x`, "a.", "a]b", `a"b`, "a[0]b"} {
		_, err := Parse(p)
		require.ErrorIs(t, err, ErrSyntax, p)
	}
}

func TestFormat(t *testing.T) {
	steps, err := Parse(`paths["/api/x"].post[0]`)
	require.NoError(t, err)
	require.Equal(t, `paths["/api/x"].post[0]`, Format(steps))
	require.Equal(t, ".", Format(nil))
}
