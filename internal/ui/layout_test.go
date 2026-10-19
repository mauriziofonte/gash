// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ui

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/term"
)

type boxVector struct {
	name   string
	border string
	lines  []string
	width  int
}

var boxVectors = []boxVector{
	{"plain", "", []string{"hello", "a longer line"}, 80},
	{"border", "\x1b[38;5;39m", []string{"x"}, 80},
	{"colored lines", "\x1b[38;5;245m", []string{"\x1b[1mbold\x1b[0m text", "plain"}, 80},
	{"no lines", "", nil, 80},
	{"empty line", "", []string{""}, 80},
	{"too wide", "\x1b[38;5;39m", []string{"\x1b[1m" + strings.Repeat("abcdefghij", 5) + "\x1b[0m", "short"}, 40},
	{"narrow terminal", "", []string{strings.Repeat("x", 30)}, 10},
	{"unicode", "", []string{"àèìòù", "✓ done"}, 80},
	{"nested sgr", "", []string{"a\x1b[\x1b[0mmb"}, 80},
}

func TestBoxMatches2x(t *testing.T) {
	for _, lang := range []string{"C.UTF-8", "C"} {
		for _, mode := range colorModes {
			var script strings.Builder
			for _, v := range boxVectors {
				args := []string{}
				if v.border != "" {
					args = append(args, "--color", shq(v.border))
				}
				for _, l := range v.lines {
					args = append(args, shq(l))
				}
				script.WriteString("COLUMNS=" + strconv.Itoa(v.width) + " __gash_box " + strings.Join(args, " ") + "; printf '\\0'\n")
			}
			want := strings.Split(run2x(t, []string{"LANG=" + lang, "LC_ALL=" + lang}, in2x(mode, script.String())), "\x00")
			require.Len(t, want, len(boxVectors)+1)
			for i, v := range boxVectors {
				if lang == "C" && v.name == "unicode" {
					continue // 2.x counted bytes in C (LEDGER CORE-16)
				}
				var b bytes.Buffer
				modeStyle(mode, lang == "C.UTF-8", v.width).Box(&b, v.border, v.lines...)
				require.Equal(t, want[i], b.String(), "%s, %s, %s", v.name, mode, lang)
			}
		}
	}
}

var wrapVectors = []struct {
	width int
	text  string
}{
	{20, "the quick brown fox jumps over the lazy dog"},
	{10, "a\n\nb c d e f g h i j k l m"},
	{12, "- a bullet that wraps with a hanging indent"},
	{12, "   indented text that wraps and keeps its indent"},
	{12, "   - indented bullet wraps twice over here"},
	{5, "averyveryverylongword short"},
	{72, ""},
	{72, "trailing newline\n"},
	{8, "tabs\tand  double  spaces"},
	{0, "zero width words"},
	{-3, "negative width"},
	{10, "     "},
	{10, "àèìòù àèìòù àèìòù"},
	{10, "back\\slash words \\n here"},
}

func TestWrapMatches2x(t *testing.T) {
	var script strings.Builder
	for _, v := range wrapVectors {
		script.WriteString("__gash_wrap out " + strconv.Itoa(v.width) + " " + shq(v.text) + "; printf '%s\\001' \"${out[@]}\"; printf '\\0'\n")
	}
	want := strings.Split(run2x(t, []string{"LANG=C.UTF-8", "LC_ALL=C.UTF-8"}, in2x("off", script.String())), "\x00")
	require.Len(t, want, len(wrapVectors)+1)
	for i, v := range wrapVectors {
		got := ""
		for _, l := range Wrap(v.width, v.text) {
			got += l + "\x01"
		}
		require.Equal(t, want[i], got, "%q", v.text)
	}
}

func TestBarAndDigitsMatch2x(t *testing.T) {
	bars := [][3]int{{0, 0, 10}, {1, 100, 10}, {50, 100, 10}, {3, 10, 10}, {10, 10, 10}, {20, 10, 10}, {5, 10, 0}, {1, 3, 7}, {-5, 10, 10}, {2, 4, -1}}
	digits := []string{"0", "12", "123", "1234", "1234567", "-1234567", "-12", "12345678901234", "", "abcdefg"}
	var script strings.Builder
	for _, b := range bars {
		script.WriteString("__gash_bar out " + strconv.Itoa(b[0]) + " " + strconv.Itoa(b[1]) + " " + strconv.Itoa(b[2]) + "; printf '%s\\0' \"$out\"\n")
	}
	for _, d := range digits {
		script.WriteString("__gash_group_digits out " + shq(d) + "; printf '%s\\0' \"$out\"\n")
	}
	for _, lang := range []string{"C.UTF-8", "C"} {
		want := strings.Split(run2x(t, []string{"LANG=" + lang, "LC_ALL=" + lang}, in2x("off", script.String())), "\x00")
		s := modeStyle("off", lang == "C.UTF-8", 80)
		for i, b := range bars {
			full, empty := s.Bar(b[0], b[1], b[2])
			require.Equal(t, want[i], full+"\t"+empty, "%v", b)
		}
		for i, d := range digits {
			require.Equal(t, want[len(bars)+i], GroupDigits(d), "%q", d)
		}
	}
}

func TestVisibleLenAndStrip(t *testing.T) {
	require.Equal(t, 4, VisibleLen("\x1b[1mab\x1b[0mcd"))
	require.Equal(t, "ab", StripSGR("a\x1b[\x1b[0mmb"))
	require.Equal(t, 3, VisibleLen("àèì"))
}

func TestInt(t *testing.T) {
	cases := map[string]int{"": 0, "12": 12, "-12": -12, "010": 8, "08": 0, "0": 0, "abc": 0, "1+1": 0, "+5": 5, "-": 0}
	for in, want := range cases {
		require.Equal(t, want, Int(in), in)
	}
}

func TestPads(t *testing.T) {
	require.Equal(t, "é   ", PadRight("é", 5))
	require.Equal(t, "   é", PadLeft("é", 5))
	require.Equal(t, "ab   ", PadLeft("ab", -5))
	require.Equal(t, "   ", Spaces(-3))
	require.Equal(t, "abc", Tail("abc", 0))
	require.Equal(t, "bc", Tail("abc", 2))
	require.Equal(t, "èì", Sub("àèìò", 1, 2))
	_ = term.Colors
}

var chooseVectors = []struct {
	def   string
	opts  []string
	input string
}{
	{"2", []string{"alpha", "beta", "gamma"}, "\n"},
	{"1", []string{"alpha", "beta"}, "2\n"},
	{"1", []string{"alpha", "beta"}, " 2 \n"},
	{"1", []string{"alpha", "beta"}, "q\n"},
	{"1", []string{"alpha", "beta"}, "9\nx\n0\n"},
	{"1", []string{"alpha", "beta"}, "9\n2\n"},
	{"1", []string{"alpha", "beta"}, ""},
	{"1", []string{"alpha", "beta"}, "2"},
	{"7", []string{"alpha", "beta"}, "\n"},
	{"1", nil, "1\n"},
	{"1", []string{"a"}, "01\n"},
	{"1", []string{"a", "b"}, "00002\n"},
	{"1", []string{"a", "b"}, "1\nleft\n"},
}

// The numbered prompt of __gash_choose (no terminal), with its answers on
// stdin: same output, same index, same status, and the rest of the input
// left unread.
func TestChoosePlainMatches2x(t *testing.T) {
	for _, v := range chooseVectors {
		args := shq(v.def)
		for _, o := range v.opts {
			args += " " + shq(o)
		}
		script := "v=; __gash_choose v " + args + " < <(printf '%s' " + shq(v.input) + ") ; rc=$?; printf '\\0%s\\0%s' \"$rc\" \"$v\""
		want := strings.Split(run2x(t, []string{"LANG=C.UTF-8"}, in2x("off", script)), "\x00")
		require.Len(t, want, 3)
		var out bytes.Buffer
		in := strings.NewReader(v.input)
		s := modeStyle("off", true, 80)
		idx, ok := s.Choose(t.Context(), term.Fixed(nil, false, false, false), in, &out, Int(v.def), v.opts)
		require.Equal(t, want[0], out.String(), "%v", v)
		rc, val := "0", strconv.Itoa(idx)
		if !ok {
			rc, val = "1", ""
		}
		require.Equal(t, want[1], rc, "%v", v)
		require.Equal(t, want[2], val, "%v", v)
	}
	// Only the answer's line is read.
	in := strings.NewReader("2\nnext\n")
	_, ok := modeStyle("off", true, 80).Choose(t.Context(), term.Fixed(nil, false, false, false), in, &bytes.Buffer{}, 1, []string{"a", "b"})
	require.True(t, ok)
	rest, _ := ReadLine(in)
	require.Equal(t, "next", rest)
}

// The terminal menu: what the 2.x loop draws for each key.
func TestChooseTerminal(t *testing.T) {
	env := mapEnv{"TERM": "xterm"}
	tty := term.Fixed(env, true, true, true)
	opts := []string{"one", "two", "three"}
	cases := []struct {
		keys  string
		color bool
		out   string
		idx   int
		ok    bool
	}{
		{"\n", false, "    1) one\n    2) two\n    3) three\n\r  ❯ 1) one\n", 1, true},
		{"j\n", false, "    1) one\n    2) two\n    3) three\n\r  ❯ 1) one\r  ❯ 2) two\n", 2, true},
		{"k\n", false, "    1) one\n    2) two\n    3) three\n\r  ❯ 1) one\r  ❯ 3) three\n", 3, true},
		{"kj\n", false, "    1) one\n    2) two\n    3) three\n\r  ❯ 1) one\r  ❯ 3) three\r  ❯ 1) one  \n", 1, true},
		{"\x1b[B\x1b[B\x1bOA\n", false, "    1) one\n    2) two\n    3) three\n\r  ❯ 1) one\r  ❯ 2) two\r  ❯ 3) three\r  ❯ 2) two  \n", 2, true},
		{"39\n", false, "    1) one\n    2) two\n    3) three\n\r  ❯ 1) one\r  ❯ 3) three\r  ❯ 3) three\n", 3, true},
		{"q", false, "    1) one\n    2) two\n    3) three\n\r  ❯ 1) one\n", 0, false},
		{"\x1b", false, "    1) one\n    2) two\n    3) three\n\r  ❯ 1) one\n", 0, false},
		{"", false, "    1) one\n    2) two\n    3) three\n\r  ❯ 1) one\n", 0, false},
		{"\x1b[Cj\n", false, "    1) one\n    2) two\n    3) three\n\r  ❯ 1) one\r  ❯ 1) one\r  ❯ 2) two\n", 2, true},
		{"j\n", true, "    1) one\n    2) two\n    3) three\n\r  \x1b[38;5;39m\x1b[1m❯ 1) one\x1b[0m\x1b[K\r  \x1b[38;5;39m\x1b[1m❯ 2) two\x1b[0m\x1b[K\n", 2, true},
	}
	for _, c := range cases {
		s := modeStyle("off", true, 80)
		if c.color {
			s = modeStyle("on", true, 80)
		}
		var out bytes.Buffer
		idx, ok := s.Choose(t.Context(), tty.WithKeys(c.keys), strings.NewReader(""), &out, 1, opts)
		require.Equal(t, c.out, out.String(), "%q", c.keys)
		require.Equal(t, c.ok, ok, "%q", c.keys)
		require.Equal(t, c.idx, idx, "%q", c.keys)
	}
	// GASH_TUI=plain and TERM=dumb take the numbered prompt.
	for _, e := range []mapEnv{{"GASH_TUI": "plain"}, {"TERM": "dumb"}} {
		var out bytes.Buffer
		idx, ok := modeStyle("off", true, 80).Choose(t.Context(), term.Fixed(e, true, true, true), strings.NewReader("2\n"), &out, 1, opts)
		require.True(t, ok)
		require.Equal(t, 2, idx)
		require.Equal(t, "  1) one\n  2) two\n  3) three\n  Choice [1]: ", out.String())
	}
}

type mapEnv map[string]string

func (m mapEnv) Lookup(k string) (string, bool) { v, ok := m[k]; return v, ok }
