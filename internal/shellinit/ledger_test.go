// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package shellinit

import (
	"os/exec"
	"regexp"
	"strings"
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

// TestLedger_SHELL01_Lint is the SHELL-01 lint: in the prompt builder, every
// value concatenated into ps1, seg, PS1 or PS2 by value (an unescaped $NAME or
// ${NAME...}) is a palette entry, a \[ \] wrapper or a value Gash computed
// itself. Any other variable (a branch, a path, a file's text) must go
// through a __GASH_PS1_* holder and be referenced as \${HOLDER}, so Bash never
// re-scans it as code.
func TestLedger_SHELL01_Lint(t *testing.T) {
	out := newHarness().emit(t)
	allowed := map[string]bool{
		// \[ \] wrappers, the reset sequence and the pieces being built.
		"o": true, "c": true, "R": true, "ps1": true, "seg": true,
		"ps1_o": true, "ps1_c": true,
		// Values computed by Gash: numbers and a palette escape.
		"ahead": true, "behind": true, "exit_code": true, "host_color": true,
		"#nl": true, "#sessions": true,
		// The git segment, itself built under this lint.
		"__GASH_PS1_GIT_SEG": true,
		// ${HOLDER:+...} only tests the holder; its value is referenced inside.
		"__GASH_PS1_STY": true,
	}
	palette := regexp.MustCompile(`^__GASH_PS1_(OFF|RED|GREEN|BLUE|BRED|BGREEN|BYELLOW|BBLUE|BPURPLE|BWHITE|DIM|AHEAD|BEHIND)$`)
	assign := regexp.MustCompile(`(?:\bps1\+?=|\bseg\+?=|\bPS1=|\bPS2=|\b__GASH_PS1_GIT_SEG=)(.*)$`)
	name := regexp.MustCompile(`^(#?[A-Za-z_][A-Za-z0-9_]*)`)

	checked := 0
	for _, fn := range []string{"__gash_ps1_git", "__gash_ps1_build"} {
		start := strings.Index(out, "\n"+fn+"() {\n")
		require.GreaterOrEqual(t, start, 0, "%s not emitted", fn)
		body := out[start+1:]
		body = body[:strings.Index(body, "\n}\n")]
		for _, line := range strings.Split(body, "\n") {
			m := assign.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			checked++
			rhs := m[1]
			for i := 0; i < len(rhs); i++ {
				switch rhs[i] {
				case '\\':
					i++ // an escaped character, e.g. \${HOLDER}: deferred, not by value
					continue
				case '$':
				default:
					continue
				}
				rest := rhs[i+1:]
				if strings.HasPrefix(rest, "{") {
					rest = rest[1:]
				}
				n := name.FindString(rest)
				if n == "" {
					continue // $' quoting, $( or $(( handled below
				}
				require.True(t, allowed[n] || palette.MatchString(n),
					"%s concatenates $%s into the prompt by value: %q", fn, n, line)
			}
			require.NotRegexp(t, `\$\([^(]`, rhs, "%s: command substitution in a prompt assignment: %q", fn, line)
		}
	}
	require.Greater(t, checked, 20, "the lint found too few prompt assignments to be meaningful")
}

// TestLedger_SHELL02 proves the terminal title replaces every control
// character of its parts (cwd, USER, HOSTNAME) by "?" before the OSC frame
// is printed, so a name cannot end the frame early (SHELL-02).
func TestLedger_SHELL02(t *testing.T) {
	out := newHarness().emit(t)
	start := strings.Index(out, "\n__gash_set_terminal_title() {\n")
	require.GreaterOrEqual(t, start, 0)
	body := out[start:]
	body = body[:strings.Index(body, "\n}\n")]
	require.Contains(t, body, `printf '\e]2;%s\a' "${title//[[:cntrl:]]/?}"`)
	require.Equal(t, 1, strings.Count(body, "printf "), "every title goes through the sanitized printf")
}

// TestLedger_SHELL04 proves the history block keeps a HISTCONTROL the user
// set: ignoreboth is added only when none of ignorespace, ignoredups,
// ignoreboth is present (3.0: SHELL-04 fixed; 2.x overwrote it).
func TestLedger_SHELL04(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	for _, tc := range []struct{ before, want string }{
		{"unset", "ignoreboth"},
		{"", "ignoreboth"},
		{"erasedups", "erasedups:ignoreboth"},
		{"ignoredups", "ignoredups"},
		{"ignorespace:erasedups", "ignorespace:erasedups"},
		{"ignoreboth", "ignoreboth"},
	} {
		set := "HISTCONTROL=" + quote(tc.before) + "; "
		if tc.before == "unset" {
			set = "unset HISTCONTROL; "
		}
		out, err := exec.Command("bash", "--noprofile", "--norc", "-c", set+historyBlock+"\nprintf %s \"$HISTCONTROL\"").Output()
		require.NoError(t, err)
		require.Equal(t, tc.want, string(out), tc.before)
	}
}
