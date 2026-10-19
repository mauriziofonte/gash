// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// blockVector is one call of a layout emitter: the 2.x function, its
// words, and the Go call that ports it.
type blockVector struct {
	fn   string
	args []string
}

var blockVectors = []blockVector{
	{"__gash_print", []string{"primary", "hello"}},
	{"__gash_print", []string{"bold_red", "x"}},
	{"__gash_print", []string{"red", "a\\nb %s"}},
	{"__gash_print", []string{"nonsense", "plain"}},
	{"__gash_print", []string{"", "empty style"}},
	{"__gash_print", []string{"underline"}},
	{"__gash_section", []string{"Title"}},
	{"__gash_section", []string{""}},
	{"__gash_heading", []string{"Title"}},
	{"__gash_heading", []string{"--indent", "4", "--note", "a note", "Title"}},
	{"__gash_heading", []string{"--indent", "x", "T"}},
	{"__gash_heading", []string{"--indent", "010", "T"}},
	{"__gash_heading", []string{"--note"}},
	{"__gash_heading", []string{"--note", "n"}},
	{"__gash_heading", []string{}},
	{"__gash_kv", []string{"Name", "gash", "Version", "3.0"}},
	{"__gash_kv", []string{"--indent", "2", "--sep", ": ", "--value-color", "accent", "a", "1", "long key", "2"}},
	{"__gash_kv", []string{"--width", "0", "k", "v", "key", "value"}},
	{"__gash_kv", []string{"--width", "12", "k", "v"}},
	{"__gash_kv", []string{"--width", "-3", "k", "v", "kk", "w"}},
	{"__gash_kv", []string{"--width", "abc", "k", "v", "kk", "w"}},
	{"__gash_kv", []string{"--value-color", "\x1b[7m", "k", "v"}},
	{"__gash_kv", []string{"--value-color", "nope", "k", "v"}},
	{"__gash_kv", []string{"odd", "pairs", "last"}},
	{"__gash_kv", []string{"àèì", "1", "a", "2"}},
	{"__gash_kv", []string{"--indent"}},
	{"__gash_kv", []string{"k", "v", "--sep", "x"}},
	{"__gash_kv", []string{}},
	{"__gash_list", []string{"one", "two"}},
	{"__gash_list", []string{"--indent", "4", "--color", "success", "a"}},
	{"__gash_list", []string{"--color", "", "a"}},
	{"__gash_list", []string{"--indent", "bad", "a"}},
	{"__gash_list", []string{"--indent"}},
	{"__gash_list", []string{}},
	{"__gash_mark", []string{"ok", "fine"}},
	{"__gash_mark", []string{"success", "fine"}},
	{"__gash_mark", []string{"warning", "careful"}},
	{"__gash_mark", []string{"error", "bad"}},
	{"__gash_mark", []string{"other", "info"}},
	{"__gash_mark", []string{"--indent", "2", "ok", "x"}},
	{"__gash_mark", []string{"--indent", "y", "ok", "x"}},
	{"__gash_mark", []string{"--indent"}},
	{"__gash_mark", []string{}},
	{"__gash_ask_render", []string{"Delete 3 files?", "y/N"}},
	{"__gash_ask_render", []string{"Plain \\t %s", "Y/n"}},
}

func (v blockVector) run(s Style, w *bytes.Buffer) {
	switch v.fn {
	case "__gash_print":
		args := append(v.args, "", "")
		s.Print(w, args[0], args[1])
	case "__gash_section":
		s.Section(w, v.args[0])
	case "__gash_heading":
		s.Heading(w, v.args...)
	case "__gash_kv":
		s.KV(w, v.args...)
	case "__gash_list":
		s.List(w, v.args...)
	case "__gash_mark":
		s.Mark(w, v.args...)
	case "__gash_ask_render":
		s.AskRender(w, v.args[0], v.args[1])
	}
}

// The emitters print what 2.x printed, in both locales and in the three
// color modes (off, on, and a --no-color call on a color terminal).
func TestBlocksMatch2x(t *testing.T) {
	for _, lang := range []string{"C.UTF-8", "C"} {
		for _, cols := range []string{"80", "30", ""} {
			for _, mode := range colorModes {
				var script strings.Builder
				for _, v := range blockVectors {
					words := []string{v.fn}
					for _, a := range v.args {
						words = append(words, shq(a))
					}
					script.WriteString(strings.Join(words, " ") + " 2>&1; printf '\\0'\n")
				}
				env := []string{"LANG=" + lang, "LC_ALL=" + lang}
				if cols != "" {
					env = append(env, "COLUMNS="+cols)
				}
				want := strings.Split(run2x(t, env, in2x(mode, script.String())), "\x00")
				require.Len(t, want, len(blockVectors)+1)
				s := modeStyle(mode, lang == "C.UTF-8", 80)
				s.Columns = Columns(cols)
				for i, v := range blockVectors {
					if lang == "C" && v.fn == "__gash_kv" && len(v.args) > 0 && v.args[0] == "àèì" {
						continue // 2.x counted bytes in C (LEDGER CORE-16)
					}
					var b bytes.Buffer
					v.run(s, &b)
					require.Equal(t, want[i], b.String(), "%s %q, %s, %s, COLUMNS=%s", v.fn, v.args, mode, lang, cols)
				}
			}
		}
	}
}

// __gash_task around a command that succeeds and one that fails, plain
// and on a color stderr.
func TestTaskMatches2x(t *testing.T) {
	for _, fancy := range []bool{false, true} {
		pre := "__gash_stderr_fancy() { return 1; }; "
		mode := "off"
		if fancy {
			pre, mode = "__gash_stderr_fancy() { return 0; }; ", "on"
		}
		want := strings.Split(run2x(t, []string{"LANG=C.UTF-8"}, in2x(mode, pre+
			`__gash_task "Reading" printf 'out\n' 2>&1; printf '\0%s\0' $?; f3() { return 3; }; __gash_task "Failing" f3 2>&1; printf '\0%s' $?`)), "\x00")
		s := modeStyle(mode, true, 80)
		s.StderrTTY = fancy
		var b bytes.Buffer
		rc := s.Task(&b, "Reading", func() int { b.WriteString("out\n"); return 0 })
		require.Equal(t, want[0], b.String())
		require.Equal(t, want[1], "0")
		require.Equal(t, 0, rc)
		b.Reset()
		rc = s.Task(&b, "Failing", func() int { return 3 })
		require.Equal(t, want[2], b.String())
		require.Equal(t, want[3], "3")
		require.Equal(t, 3, rc)
	}
}

// LEDGER SYS-34: a COLUMNS that is not a run of digits is 80 for the
// section rule (60 glyphs) and is never evaluated.
func TestLedger_SYS34(t *testing.T) {
	for _, v := range []string{"a[$(touch MARK)]", "abc", "-5", "1+1", " 70", ""} {
		require.Equal(t, 80, Columns(v), v)
	}
	require.Equal(t, 8, Columns("010"))
	require.Equal(t, 30, Columns("30"))
	var b bytes.Buffer
	Style{G: modeStyle("off", false, 80).G, Columns: Columns("x[$(id)]")}.Section(&b, "T")
	require.Equal(t, "\n"+strings.Repeat("-", 60)+"\nT\n\n", b.String())
}

func TestProgressAndClear(t *testing.T) {
	s := modeStyle("off", false, 80)
	var b bytes.Buffer
	s.Progress(&b, 5, 10, "half")
	require.Empty(t, b.String(), "no terminal stderr")
	s.StderrTTY = true
	s.Progress(&b, 5, 10, "half")
	require.Equal(t, "\r###############...............  50% half", b.String())
	b.Reset()
	s.Progress(&b, 12, 10, "")
	require.Equal(t, "\r"+strings.Repeat("#", 30)+" 100% \n", b.String())
	b.Reset()
	s.Progress(&b, 0, 0, "z")
	require.Equal(t, "\r"+strings.Repeat(".", 30)+"   0% z", b.String())
	b.Reset()
	c := modeStyle("on", true, 80)
	c.StderrTTY = true
	c.Progress(&b, 1, 3, "m")
	require.Equal(t, "\r\x1b[38;5;39m█████████"+strings.Repeat("░", 21)+"\x1b[0m  33% m\x1b[K", b.String())
	c.Headless = true
	b.Reset()
	c.Progress(&b, 1, 3, "m")
	require.Empty(t, b.String())

	b.Reset()
	s.ClearScreen(&b)
	require.Empty(t, b.String())
	c.Headless = false
	c.ClearScreen(&b)
	require.Equal(t, "\x1b[H\x1b[2J", b.String())
}
