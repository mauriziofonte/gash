// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const boxEnv = "HOST:box=deploy@box.example\n"

// argv is the arguments of the stub call named name in a log, in order.
func argv(log, name string) [][]string {
	var calls [][]string
	var cur *[]string
	for _, l := range strings.Split(log, "\n") {
		switch {
		case l == "ssh" || l == "scp" || l == "rsync":
			calls = append(calls, nil)
			if l == name {
				cur = &calls[len(calls)-1]
			} else {
				cur = nil
			}
		case cur != nil && strings.HasPrefix(l, "  [") && strings.HasSuffix(l, "]"):
			*cur = append(*cur, l[3:len(l)-1])
		}
	}
	var out [][]string
	for _, c := range calls {
		if c != nil {
			out = append(out, c)
		}
	}
	return out
}

// LEDGER REM-04 (forced divergence): script arguments are quoted
// POSIX-style, so sh and bash on the remote parse the same words, which
// are the caller's.
func TestLedger_REM04(t *testing.T) {
	vectors := []string{"a b", "x\ny", "é", "~", "", "$(id)", "it's", "--", "-c", "`id`", "\\", "*"}
	cmd := historyOff + "bash -s --" + scriptArgs(vectors)
	require.False(t, strings.HasSuffix(cmd, " "))
	for _, sh := range []string{"sh", "bash", "dash"} {
		if _, err := exec.LookPath(sh); err != nil {
			continue
		}
		require.Equal(t, vectors, parseArgs(t, sh, cmd), sh)
	}
	// 2.x quoted with printf %q: Bash-only, with a trailing space.
	got := bash2x(t, `printf '%q ' "a b" $'x\ny'`)
	require.Equal(t, `a\ b $'x\ny' `, got)
}
