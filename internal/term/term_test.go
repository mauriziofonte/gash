// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package term

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

type mapEnv map[string]string

func (m mapEnv) Lookup(k string) (string, bool) { v, ok := m[k]; return v, ok }

func TestColorEnvOK(t *testing.T) {
	cases := []struct {
		env  mapEnv
		want bool
	}{
		{mapEnv{}, true},
		{mapEnv{"GASH_HEADLESS": "1"}, false},
		{mapEnv{"GASH_HEADLESS": "0"}, true},
		{mapEnv{"NO_COLOR": "1"}, false},
		{mapEnv{"NO_COLOR": ""}, true},
		{mapEnv{"NO_COLOR": "0"}, false},
		{mapEnv{"GASH_NO_COLOR": "1"}, false},
		{mapEnv{"GASH_NO_COLOR": "0"}, true},
		{mapEnv{"GASH_NO_COLOR": ""}, true},
		{mapEnv{"GASH_NO_COLOR": "yes"}, false},
	}
	for _, c := range cases {
		require.Equal(t, c.want, ColorEnvOK(c.env), "%v", c.env)
	}
}

func TestRuntimeGate(t *testing.T) {
	// Colors only on a terminal stdout, whatever stderr is.
	require.True(t, Fixed(mapEnv{}, false, true, false).UseColor())
	require.False(t, Fixed(mapEnv{}, true, false, true).UseColor())
	require.False(t, Fixed(mapEnv{"NO_COLOR": "x"}, true, true, true).UseColor())
	require.False(t, Fixed(mapEnv{}, true, true, true).WantColors(true))
	require.True(t, Fixed(mapEnv{}, true, true, true).WantColors(false))
	require.False(t, Fixed(mapEnv{}, true, true, false).StderrFancy())
	require.True(t, Fixed(mapEnv{}, true, true, true).StderrFancy())
	require.True(t, Fixed(mapEnv{}, true, true, false).TTYIO())
	require.False(t, Fixed(mapEnv{}, false, true, false).TTYIO())
}

func TestLoadTimePalette(t *testing.T) {
	// Load-time variables are set whenever the environment permits colors,
	// even when stdout is a pipe.
	require.Equal(t, Colors, Fixed(mapEnv{}, false, false, false).Palette())
	require.Equal(t, NoColors, Fixed(mapEnv{"GASH_HEADLESS": "1"}, true, true, true).Palette())
	require.Equal(t, "\x1b[38;5;214m", Colors.Style("accent"))
	require.Equal(t, "", Colors.Style("none"))
	require.Equal(t, "raw", Colors.Style("raw"))
}

func TestLocaleGlyphs(t *testing.T) {
	require.True(t, LocaleUTF8(mapEnv{"LANG": "en_US.UTF-8"}))
	require.True(t, LocaleUTF8(mapEnv{"LC_ALL": "C.utf8", "LANG": "C"}))
	require.False(t, LocaleUTF8(mapEnv{"LC_ALL": "C", "LANG": "en_US.UTF-8"}))
	require.True(t, LocaleUTF8(mapEnv{"LC_ALL": "", "LC_CTYPE": "", "LANG": "C.UTF-8"}))
	require.False(t, LocaleUTF8(mapEnv{}))
	require.Equal(t, "✓", Fixed(mapEnv{"LANG": "C.UTF-8"}, false, false, false).Glyphs().OK)
	require.Equal(t, "+", Fixed(mapEnv{"LANG": "C"}, false, false, false).Glyphs().OK)
}

// A test terminal types the same input at every open, or successive
// answers at successive opens; without one the open fails.
func TestFakeTTYAnswers(t *testing.T) {
	read := func(tt *Term) string {
		f, err := tt.OpenTTY()
		require.NoError(t, err)
		b, err := io.ReadAll(f)
		require.NoError(t, err)
		return string(b)
	}
	same := Fixed(nil, true, true, true).WithTTY("y\n")
	answers := Fixed(nil, true, true, true).WithTTYAnswers("y\n", "n\n")
	for _, want := range []string{"y\n", "n\n", ""} {
		require.Equal(t, "y\n", read(same))
		require.Equal(t, want, read(answers))
	}
	_, err := Fixed(nil, true, true, true).WithoutTTY().OpenTTY()
	require.Error(t, err)
}
