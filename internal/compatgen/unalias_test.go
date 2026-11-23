// Gash 4.1. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package compatgen

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The command names inside $(...) and <(...) get a backslash, at any depth
// and with prefixed assignments; names outside a substitution, names that
// are not plain literals and backquoted substitutions stay as they are.
func TestUnalias(t *testing.T) {
	src := `f() {
    tail -1 file
    x="$(grep a <<<"$y" | sort -V | tail -1)"
    y=$(echo "$(LC_ALL=C sort file)")
    while read -r l; do :; done < <(nvm ls)
    z="$("$cmd" -x; \tail -1; ${tool} x; /usr/bin/tail -1)"
    w=` + "`tail -1`" + `
    [[ -n "$(command -v tail)" ]] && v=$((1 + 2))
}
`
	want := `f() {
    tail -1 file
    x="$(\grep a <<<"$y" | \sort -V | \tail -1)"
    y=$(\echo "$(LC_ALL=C \sort file)")
    while read -r l; do :; done < <(\nvm ls)
    z="$("$cmd" -x; \tail -1; ${tool} x; /usr/bin/tail -1)"
    w=` + "`tail -1`" + `
    [[ -n "$(\command -v tail)" ]] && v=$((1 + 2))
}
`
	out, err := unalias("x.sh", []byte(src))
	require.NoError(t, err)
	require.Equal(t, want, string(out))

	again, err := unalias("x.sh", out)
	require.NoError(t, err)
	require.Equal(t, want, string(again), "a second pass changes nothing")

	_, err = unalias("bad.sh", []byte("f() {\n"))
	require.ErrorContains(t, err, "bad.sh")
}

// Every generated file is already unaliased: no command of a substitution
// is left where an alias of the user's shell would reach it.
func TestGeneratedUnaliased(t *testing.T) {
	files, err := Generate(true)
	require.NoError(t, err)
	n := 0
	for _, f := range files {
		if !lazyFile(f.Path) && !strings.HasPrefix(f.Path, ColdDir) {
			continue
		}
		out, err := unalias(f.Path, f.Data)
		require.NoError(t, err)
		require.Equal(t, string(f.Data), string(out), f.Path)
		n++
	}
	require.Positive(t, n)
}

// The proof on a real Bash: a function defined with aliases off, as
// __gash_cold does, then run with an alias of tail in effect. Its
// substitutions see tail itself once unaliased, the alias before.
func TestUnaliasInBash(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not installed")
	}
	def := `f() { printf '%s|' "$(printf 'a\nb\n' | tail -1)"; cat <(printf 'c\nd\n' | tail -1); }`
	run := func(def string) string {
		script := "shopt -s expand_aliases\nalias tail='echo ALIASED'\nshopt -u expand_aliases\n" +
			"eval '" + strings.ReplaceAll(def, "'", `'\''`) + "'\nshopt -s expand_aliases\nf\n"
		out, err := exec.Command("bash", "--noprofile", "--norc", "-c", script).CombinedOutput()
		require.NoError(t, err, string(out))
		return string(out)
	}
	fixed, err := unalias("f.sh", []byte(def))
	require.NoError(t, err)
	require.Equal(t, "b|d\n", run(string(fixed)))
	if v := run(def); v == "b|d\n" {
		t.Log("this Bash parses substitutions once: the alias never reached them")
	} else {
		require.Equal(t, "ALIASED -1|ALIASED -1\n", v)
	}
}
