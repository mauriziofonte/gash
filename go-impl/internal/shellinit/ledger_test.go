// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package shellinit

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/runner"
)

// TestLedger_SHELL01 proves the prompt hook stores each untrusted string in
// its own variable and PS1 holds only the reference, so Bash expands it once
// and never re-scans a branch name, venv path, chroot file or screen session
// as code (forced divergence SHELL-01). The template must reference, never
// interpolate, the four values.
func TestLedger_SHELL01(t *testing.T) {
	out := newHarness().emit(t)

	// Each untrusted value reaches its holder by a plain assignment (no
	// re-parse), and PS1/seg hold only the deferred reference.
	for _, pair := range [][2]string{
		{`__GASH_PS1_BRANCH="$branch"`, `(\${__GASH_PS1_BRANCH}`},
		{`__GASH_PS1_CHROOT="$debian_chroot"`, `(\${__GASH_PS1_CHROOT})`},
		{`__GASH_PS1_VENV="${VIRTUAL_ENV##*/}"`, `(\${__GASH_PS1_VENV})`},
		{`__GASH_PS1_STY="${STY-}"`, `\${__GASH_PS1_STY}`},
	} {
		require.Contains(t, out, pair[0], "holder assignment missing")
		require.Contains(t, out, pair[1], "deferred reference missing")
	}

	// The 2.x vulnerable splices (the raw value spliced into the prompt) must
	// be gone: no branch, chroot or venv value concatenated into ps1/seg.
	for _, bad := range []string{
		`(\${branch}`, `(${branch}`,
		`(\${debian_chroot})`, `(${debian_chroot})`,
		`(\${VIRTUAL_ENV##*/})`, `(${VIRTUAL_ENV##*/})`,
	} {
		require.NotContains(t, out, bad, "an untrusted value is still spliced into the prompt")
	}

	// PS1 is a parameter expansion of the holders only, never a command or
	// arithmetic expansion of them.
	require.NotContains(t, out, "$(${__GASH_PS1_BRANCH")
	require.NotContains(t, out, "$((${__GASH_PS1_BRANCH")
}

// TestLedger_SHELL07 proves GASH_PATH_ADD is split on space, tab and newline
// only, never glob-expanded, and emitted as a path_add call of quoted words
// (forced divergence SHELL-07).
func TestLedger_SHELL07(t *testing.T) {
	require.Equal(t, []string{"~/a", "~/b", "/c*"}, config.SplitPathAdd("~/a  ~/b\t/c*"))

	h := newHarness()
	h.env["GASH_PATH_ADD"] = "~/a  ~/b\t/c*"
	out := h.emit(t)
	require.Contains(t, out, "path_add '~/a' '~/b' '/c*'\n")
}

// TestLedger_SHELL08 proves a first word of --append, -h or --help keeps its
// 2.x path_add meaning: the binary quotes and emits the words, it never
// consumes the option itself.
func TestLedger_SHELL08(t *testing.T) {
	h := newHarness()
	h.env["GASH_PATH_ADD"] = "--append /x"
	require.Contains(t, h.emit(t), "path_add --append /x\n")

	h2 := newHarness()
	h2.env["GASH_PATH_ADD"] = "-h"
	require.Contains(t, h2.emit(t), "path_add -h\n")
}

// TestLedger_SHELL06 proves the grep color alias is never defined: the 2.x
// probe greps an empty pattern from /dev/null, which always exits 1, so the
// defect (no grep alias) is reproduced in rc.A.
func TestLedger_SHELL06(t *testing.T) {
	h := newHarness().withProgram("grep")
	// The probe behaves like real grep on an empty file: no match, exit 1.
	h.probe = func(c runner.Call) (runner.Expect, bool) {
		if len(c.Argv) > 0 && c.Argv[0] == "grep" {
			return runner.Expect{Status: 1}, true
		}
		return runner.Expect{}, false
	}
	out := h.emit(t)
	require.NotContains(t, out, "alias grep=")
}
