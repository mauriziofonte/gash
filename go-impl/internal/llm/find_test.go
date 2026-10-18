// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFindMatches2x(t *testing.T) {
	d := searchFixture(t)
	mustWrite(t, filepath.Join(d, "nl\nname.php"), "needle\n")
	mustWrite(t, filepath.Join(d, "vendor.php"), "needle\n")
	mustMkdir(t, filepath.Join(d, "src", "vendor"))
	mustWrite(t, filepath.Join(d, "src", "q[1].php"), "needle\n")
	home := physTemp(t)
	env := rgEnv(t, home)
	m := New()
	for _, args := range [][]string{
		{"*.php"},
		{"*.php", "."},
		{"*.php", d},
		{"*", "src"},
		{"*", "src", "--type", "d"},
		{"*", "--type", "d"},
		{"vendor", "--type", "d"},
		{"vendor*"},
		{"*.PHP"},
		{"[a-c].*", "src"},
		{"q\\[1\\].php", "src"},
		{"?.php", "src"},
		{"[[:upper:]]*", "src"},
		{"[![:lower:]_]*", "src"},
		{"[[:digit:][:punct:]]*"},
		{".*"},
		{"*", "-n", "3"},
		{"*", "--limit", "0"},
		{"*", "src/a.php"},
		{"a.*", "src/a.php"},
		{"*.php", "--contains", "needle"},
		{"*.php", ".", "--contains", "needle"},
		{"*", "--contains", "needle"},
		{"*", "--contains", "needle", "-n", "4"},
		{"*.md", "--contains", "needle"},
		{"*.nomatch", "--contains", "needle"},
		{"a.*", "src/a.php", "--contains", "needle"},
		{"b.*", "src/a.php", "--contains", "needle"},
		{"*", "bin.dat", "--contains", "after"},
		{"*", "--contains", "("},
		{"*", "--contains", "needle", "--type", "f"},
		{"[", "--contains", "needle"},
		{"*", "sub/.env.example", "--contains", "needle"},
		{"*", ".env", "--contains", "needle"},
		{"*", ".ssh", "--contains", "needle"},
		{"*", ".ssh"},
		{"*", ".env"},
		// errors, in the 2.x order
		{},
		{""},
		{"", "*.php"},
		{"*", "--type"},
		{"*", "--type", "x"},
		{"*", "--type", "d", "--contains", "x"},
		{"*", "--type", "x", "--contains", "x"},
		{"*", "--contains"},
		{"*", "-n"},
		{"*", "-n", "x"},
		{"*", "--bogus"},
		{"*", ".", "extra"},
		{"*", "../x"},
		{"*", "/proc"},
		{"*", "-h"},
		{"--type", "--help"},
	} {
		same(t, m, d, env, "llm_find", args...)
	}
}

// TestLedger_LLM18_2x: an empty first positional is skipped and the next
// word becomes PATTERN.
func TestLedger_LLM18_2x(t *testing.T) {
	m := New()
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "x"), "")
	env := baseEnv(physTemp(t), nil)
	r := same(t, m, d, env, "llm_find", "", "x")
	require.Equal(t, result{d + "/x\n", "", 0}, r)
}

// TestLedger_LLM19_2x_find: the name search leaves symlinks out.
func TestLedger_LLM19_2x_find(t *testing.T) {
	m := New()
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "f"), "")
	require.NoError(t, os.Symlink("f", filepath.Join(d, "ln")))
	env := baseEnv(physTemp(t), nil)
	r := same(t, m, d, env, "llm_find", "*")
	require.Equal(t, d+"/f\n", r.Stdout)
}

// TestLedger_LLM29_2x_find: the name search leaves out paths with a
// newline.
func TestLedger_LLM29_2x_find(t *testing.T) {
	m := New()
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "a\nb"), "")
	mustWrite(t, filepath.Join(d, "a\tb"), "")
	env := baseEnv(physTemp(t), nil)
	r := same(t, m, d, env, "llm_find", "a*")
	require.Equal(t, d+"/a\tb\n", r.Stdout)
}
