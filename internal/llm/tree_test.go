// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// treeFixture builds a tree with every case the 2.x find/awk pipeline
// treats specially.
func treeFixture(t *testing.T) string {
	t.Helper()
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "README.md"), "hello\n")
	mustWrite(t, filepath.Join(d, "src", "main.go"), "package main\n")
	mustWrite(t, filepath.Join(d, "src", "lib", "a.go"), "package lib // a\n")
	mustWrite(t, filepath.Join(d, "src", "lib", "deep", "b.go"), strings.Repeat("x", 300))
	mustWrite(t, filepath.Join(d, "node_modules", "pkg", "index.js"), "x")
	mustWrite(t, filepath.Join(d, "vendor"), "a file named vendor")
	mustWrite(t, filepath.Join(d, ".git", "HEAD"), "ref\n")
	mustWrite(t, filepath.Join(d, "src", ".cache", "c"), "c")
	mustWrite(t, filepath.Join(d, "src", "__pycache__", "x.pyc"), "c")
	mustWrite(t, filepath.Join(d, `q"uo\te`), "q")
	mustWrite(t, filepath.Join(d, "ctl\x01\x1b\x7f\r"), "c")
	if rawNamesOK(t, d) { // APFS rejects a name that is not valid UTF-8
		mustWrite(t, filepath.Join(d, "bad\xff\xfe"), "b")
	}
	mustWrite(t, filepath.Join(d, "tab\tname"), "t")
	mustWrite(t, filepath.Join(d, "nl\nname"), "n")
	mustWrite(t, filepath.Join(d, "nl\ndir", "inner"), "n")
	mustWrite(t, filepath.Join(d, "-n"), "")
	mustWrite(t, filepath.Join(d, "-e"), "")
	mustMkdir(t, filepath.Join(d, "empty"))
	require.NoError(t, os.Symlink("src", filepath.Join(d, "link-to-src")))
	require.NoError(t, os.Symlink("missing", filepath.Join(d, "dangling")))
	return d
}

func TestTreeMatches2x(t *testing.T) {
	d := treeFixture(t)
	home := physTemp(t)
	env := baseEnv(home, nil)
	m := New()
	for _, args := range [][]string{
		{},
		{"."},
		{d},
		{"src"},
		{"--depth", "2"},
		{"--max-depth", "3", "--stats"},
		{"--stats"},
		{"--stats", "--depth", "1", "src"},
		{"--depth", "9", "--stats"},
		{"--depth", ""},
		{"--depth", "007"},
		{"--text"},
		{"--text", "--depth", "1"},
		{"--text", "--depth", "5", "src"},
		{"--text", "--stats"},
		// errors, in the 2.x order
		{"--depth"},
		{"--text", "--depth"},
		{"--depth", "abc"},
		{"--depth", "-1"},
		{"--depth", "0"},
		{"--depth", "000"},
		{"--depth", "1234567890"},
		{"--bogus"},
		{"-"},
		{".", "src"},
		{"a", "b", "--bogus"},
		{"--bogus", "a", "b"},
		{"../x"},
		{"src/.."},
		{"/proc"},
		{"/etc/ssh"},
		{"README.md"},
		{"nonexistent"},
		{"~"},
		{"~/x"},
		{"--depth", "abc", "-h"},
		{"x", "y", "--help"},
	} {
		same(t, m, d, env, "llm_tree", args...)
	}
}

func TestTreeTextRootAndSlash(t *testing.T) {
	m := New()
	home := physTemp(t)
	env := baseEnv(home, nil)
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "vendor", "inside"), "x")
	mustWrite(t, filepath.Join(d, "star*dir", "f"), "x")
	mustWrite(t, filepath.Join(d, "star*dir", "g", "h"), "x")
	same(t, m, d, env, "llm_tree", "--text", "star*dir")
	same(t, m, d, env, "llm_tree", "--text", "--depth", "1", "/")
	same(t, m, d, env, "llm_tree", "--depth", "1", "/")
}

// TestLedger_LLM19_2x: llm_tree --text prunes the start point itself when it
// is named like a pruned directory (blank output) and leaves symlinks out.
func TestLedger_LLM19_2x(t *testing.T) {
	m := New()
	env := baseEnv(physTemp(t), nil)
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "vendor", "inside"), "x")
	require.NoError(t, os.Symlink("vendor", filepath.Join(d, "ln")))
	r := same(t, m, d, env, "llm_tree", "--text", "vendor")
	require.Equal(t, result{"", "", 0}, r)
	r = same(t, m, d, env, "llm_tree", "--text")
	require.NotContains(t, r.Stdout, "ln")
}

// TestLedger_LLM08: a root with a quote or backslash gives valid JSON (2.x
// printed it raw through awk -v); a root with a tab or a newline still lists
// nothing, as the 2.x ! -path tests saw the whole path.
func TestLedger_LLM08(t *testing.T) {
	m := New()
	env := baseEnv(physTemp(t), nil)
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, `q"uote`, "f1"), "x")
	mustWrite(t, filepath.Join(d, `b\ack`, "f1"), "x")
	mustWrite(t, filepath.Join(d, "t\tab", "f1"), "x")
	mustWrite(t, filepath.Join(d, "n\nl", "f1"), "x")
	for _, root := range []string{`q"uote`, `b\ack`} {
		r := runGo(t, m, d, env, "llm_tree", root)
		var v struct {
			Path     string
			Children []json.RawMessage
		}
		require.NoError(t, json.Unmarshal([]byte(r.Stdout), &v), r.Stdout)
		require.Equal(t, filepath.Join(d, root), v.Path)
		require.Len(t, v.Children, 1)
	}
	for _, root := range []string{"t\tab", "n\nl"} {
		r := runGo(t, m, d, env, "llm_tree", "--stats", root)
		want := `{"path":` + string(mustJSON(t, filepath.Join(d, root))) + `,"type":"directory","size":0,"children_count":0,"children":[]}` + "\n"
		require.Equal(t, result{want, "", 0}, r)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// TestLedger_LLM06_2x: tree names keep C0 controls other than tab, newline
// and CR raw, as the awk esc() did; DEL and bytes >= 0x80 too (LLM-23).
func TestLedger_LLM06_2x(t *testing.T) {
	m := New()
	env := baseEnv(physTemp(t), nil)
	d := physTemp(t)
	if !rawNamesOK(t, d) { // the \xff byte makes this name invalid UTF-8 (APFS)
		t.Skip("filesystem rejects non-UTF-8 names")
	}
	mustWrite(t, filepath.Join(d, "a\x01b\x1bc\x7fd\re\xff"), "x")
	r := same(t, m, d, env, "llm_tree")
	require.Contains(t, r.Stdout, `"name":"a`+"\x01b\x1bc\x7fd"+`\re`+"\xff"+`"`)
}

// TestLedger_LLM23_2x: llm_tree names pass invalid UTF-8 through raw.
func TestLedger_LLM23_2x(t *testing.T) {
	m := New()
	env := baseEnv(physTemp(t), nil)
	d := physTemp(t)
	if !rawNamesOK(t, d) {
		t.Skip("filesystem rejects non-UTF-8 names (APFS)")
	}
	mustWrite(t, filepath.Join(d, "bad\xff\xfe"), "x")
	r := same(t, m, d, env, "llm_tree")
	require.Contains(t, r.Stdout, "\"bad\xff\xfe\"")
}

// TestLedger_LLM29_2x: names with a tab or a newline are left out of the
// JSON tree.
func TestLedger_LLM29_2x(t *testing.T) {
	m := New()
	env := baseEnv(physTemp(t), nil)
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "t\tx"), "x")
	mustWrite(t, filepath.Join(d, "n\nx"), "x")
	mustWrite(t, filepath.Join(d, "ok"), "x")
	r := same(t, m, d, env, "llm_tree", "--stats")
	require.Equal(t, `{"path":"`+d+`","type":"directory","size":1,"children_count":1,"children":[{"name":"ok","type":"file","size":1}]}`+"\n", r.Stdout)
}

// TestLedger_LLM28_2x: invalid_depth has two detail formats.
func TestLedger_LLM28_2x(t *testing.T) {
	m := New()
	env := baseEnv(physTemp(t), nil)
	d := physTemp(t)
	r := same(t, m, d, env, "llm_tree", "--depth", "0")
	require.Equal(t, `{"error":"invalid_depth","details":"0","action":"RETRY","recoverable":true,"hint":"Provide a positive integer"}`+"\n", r.Stderr)
	r = same(t, m, d, env, "llm_tree", "--depth", "abc")
	require.Equal(t, `{"error":"invalid_depth","details":"Value 'abc' is not a positive integer below 1000000000","action":"RETRY","recoverable":true,"hint":"Provide a plain integer for depth"}`+"\n", r.Stderr)
}

// TestLedger_LLM09_2x: only the start point is guarded; the walk enters
// protected trees below it.
func TestLedger_LLM09_2x(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the walk-into-protected-tree case uses /proc, a Linux procfs")
	}
	m := New()
	env := baseEnv(physTemp(t), nil)
	r := runGo(t, m, "/", env, "llm_tree", "--depth", "1", "/")
	require.Equal(t, 0, r.RC)
	require.Contains(t, r.Stdout, `{"name":"proc","type":"directory"}`)
}

// TestLedger_LLM24: a trailing newline in PATH is kept (2.x lost it in a
// $(...) capture).
func TestLedger_LLM24(t *testing.T) {
	m := New()
	env := baseEnv(physTemp(t), nil)
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "x\n", "in-nl"), "x")
	mustWrite(t, filepath.Join(d, "x", "in-plain"), "x")
	r := runGo(t, m, d, env, "llm_tree", "--text", "x\n")
	require.Equal(t, 0, r.RC)
	require.Contains(t, r.Stdout, "in-nl")
	require.NotContains(t, r.Stdout, "in-plain")
}

// TestLedger_LLM25_2x: a leading ~ is replaced by $HOME textually, ~user
// is not expanded: ~root/x is ${HOME}root/x.
func TestLedger_LLM25_2x(t *testing.T) {
	m := New()
	d := physTemp(t)
	home := filepath.Join(d, "h")
	mustMkdir(t, home)
	mustWrite(t, filepath.Join(d, "hroot", "x", "f"), "x")
	env := baseEnv(home, nil)
	r := same(t, m, home, env, "llm_tree", "~root/x")
	require.Equal(t, `{"path":"`+home+`root/x","type":"directory","children":[{"name":"f","type":"file"}]}`+"\n", r.Stdout)
}

func TestTreeCaps(t *testing.T) {
	m := New()
	env := baseEnv(physTemp(t), nil)
	d := physTemp(t)
	for i := 0; i < 260; i++ {
		mustWrite(t, filepath.Join(d, fmt.Sprintf("d%03d", i%3), fmt.Sprintf("f%03d", i)), "xy")
	}
	for i := 0; i < 300; i++ {
		mustWrite(t, filepath.Join(d, fmt.Sprintf("top%03d", i)), "z")
	}
	r := same(t, m, d, env, "llm_tree", "--depth", "2", "--stats")
	require.Contains(t, r.Stdout, `"truncated":true`)
	same(t, m, d, env, "llm_tree", "--depth", "2")
	same(t, m, d, env, "llm_tree", "--text")
}

// TestLedger_LLM36: llm_tree --text lists in bytewise (C) order whatever
// the locale: upper case before lower, punctuation by its byte, UTF-8
// after ASCII; 2.x's order under LC_ALL=C.
func TestLedger_LLM36(t *testing.T) {
	archive2x(t) // before runGo changes the directory
	m := New()
	d := physTemp(t)
	names := []string{"é", "e", "ab", "a_b", "a.b", "a-b", "a", "_u", "Z1", "B", "b"}
	for _, n := range names {
		mustWrite(t, filepath.Join(d, n), "x")
	}
	want := "B\nZ1\n_u\na\na-b\na.b\na_b\nab\nb\ne\né\n"
	for _, loc := range []string{"C", "C.UTF-8", "en_US.UTF-8", "de_DE.UTF-8"} {
		env := baseEnv(physTemp(t), map[string]string{"LC_ALL": loc, "LANG": loc})
		r := runGo(t, m, d, env, "llm_tree", "--text")
		require.Equal(t, 0, r.RC)
		_, body, _ := strings.Cut(r.Stdout, "\n") // the root line
		require.Equal(t, want, body, loc)
	}
	same(t, m, d, baseEnv(physTemp(t), map[string]string{"LC_ALL": "C"}), "llm_tree", "--text")
}
