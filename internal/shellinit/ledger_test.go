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
	require.Contains(t, out, "__gash_path_prepend '~/a' '~/b' '/c*'\n")
}

// TestLedger_SHELL08: the words of GASH_PATH_ADD go to the option-less
// __gash_path_prepend, so -h and --append are directory names (missing,
// skipped) and never print the path_add help or append (3.0: SHELL-08
// fixed; 2.x passed them to path_add). The probes' PATH follows the same
// rule. testdata/script/path_add_no_options.txtar runs it in a shell.
func TestLedger_SHELL08(t *testing.T) {
	h := newHarness()
	h.env["GASH_PATH_ADD"] = "--append /x"
	out := h.emit(t)
	require.Contains(t, out, "__gash_path_prepend --append /x\n")
	require.NotContains(t, out, "path_add ")

	h2 := newHarness()
	h2.env["GASH_PATH_ADD"] = "-h /opt/x"
	require.Contains(t, h2.emit(t), "__gash_path_prepend -h /opt/x\n")
}

// TestLedger_SHELL06: the grep color alias is defined when `grep
// --color=auto -q x` reading "x" succeeds, and not for a grep that rejects
// the flag (3.0: SHELL-06 fixed; the 2.x probe grepped an empty file and the
// alias was never defined).
func TestLedger_SHELL06(t *testing.T) {
	for _, honors := range []bool{true, false} {
		h := newHarness().withProgram("grep")
		h.probe = func(c runner.Call) (runner.Expect, bool) {
			if len(c.Argv) > 0 && c.Argv[0] == "grep" {
				if !honors {
					return runner.Expect{Status: 2, Stderr: "grep: unrecognized option '--color=auto'\n"}, true
				}
				require.Equal(t, []string{"grep", "--color=auto", "-q", "x"}, c.Argv)
				if strings.Contains(string(c.Stdin), "x") {
					return runner.Expect{Status: 0}, true
				}
				return runner.Expect{Status: 1}, true
			}
			return runner.Expect{}, false
		}
		out := h.emit(t)
		if honors {
			require.Contains(t, out, "alias grep='grep --color=auto'")
		} else {
			require.NotContains(t, out, "alias grep=")
		}
	}
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

// TestLedger_SHELL09 proves every pack alias is guarded by the user's
// aliases recorded at load (__GASH_USER_ALIASES): a name the user defined
// first is left alone, and a name that hides a program is not defined at
// all, not even over a user alias (3.0: SHELL-09 fixed; 2.x overwrote the
// user's alias).
func TestLedger_SHELL09(t *testing.T) {
	out := newHarness().withProgram("git", "gp").emit(t)
	require.Contains(t, out, "[[ -v '__GASH_USER_ALIASES[ll]' ]] || alias ll='ls -l'\n")
	require.Contains(t, out, "[[ -v '__GASH_USER_ALIASES[gst]' ]] || alias gst='git status'\n")
	require.NotContains(t, out, "alias gp=", "gp hides a program")
	for _, line := range strings.Split(out, "\n") {
		if i := strings.Index(line, "alias "); i >= 0 && strings.Contains(line[i:], "=") && !strings.HasPrefix(strings.TrimSpace(line), "#") &&
			strings.HasPrefix(strings.TrimSpace(line[i:]), "alias ") && !strings.Contains(line, "unalias") {
			require.Contains(t, line, "__GASH_USER_ALIASES[", "an unguarded alias: %q", line)
		}
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	// In Bash: the user's ll stays, a free name is defined.
	script := "shopt -s expand_aliases; declare -A __GASH_USER_ALIASES=([ll]=1); alias ll='ls -alF'\n" +
		"[[ -v '__GASH_USER_ALIASES[ll]' ]] || alias ll='ls -l'\n[[ -v '__GASH_USER_ALIASES[la]' ]] || alias la='ls -la'\nalias ll la"
	b, err := exec.Command("bash", "--noprofile", "--norc", "-c", script).Output()
	require.NoError(t, err)
	require.Equal(t, "alias ll='ls -alF'\nalias la='ls -la'\n", string(b))
}

// TestLedger_SHELL10: the init script never runs =~ (which would leave
// BASH_REMATCH set in the caller's shell) and declares nothing readonly,
// whatever the settings; GASH_UMASK is validated and the nvm directory
// resolved by the binary. The generated files are linted in
// internal/compatgen, the loaded shell in testdata/script/load_hygiene.txtar.
func TestLedger_SHELL10(t *testing.T) {
	bad := regexp.MustCompile(`=~|\breadonly\b|declare\s+-[A-Za-z]*r`)
	for _, env := range []map[string]string{
		{},
		{"GASH_UMASK": "027", "GASH_NVM": "lazy", "GASH_PATH_ADD": "~/bin"},
		{"GASH_NVM": "on", "GASH_PROMPT": "0", "GASH_HISTORY_SYNC": "1"},
	} {
		h := newHarness()
		for k, v := range env {
			h.env[k] = v
		}
		out := h.emit(t)
		require.NotEmpty(t, out)
		for i, l := range strings.Split(out, "\n") {
			require.False(t, bad.MatchString(l), "%v: line %d: %q", env, i+1, l)
		}
	}
}

// LEDGER SHELL-12: a module short alias that is also a program on PATH is
// removed again by the init script (fake dcc on PATH: unalias dcc, unless
// the user defined dcc before Gash loaded); without the program nothing is
// emitted for it and gash.sh's alias stays. sl is no longer exempt from the
// pack rule (3.0: SHELL-12 fixed). testdata/script/module_alias_shadow.txtar
// runs it in a shell.
func TestLedger_SHELL12(t *testing.T) {
	out := newHarness().withProgram("dcc").withProgram("sl").emit(t)
	require.Contains(t, out, "[[ -v '__GASH_USER_ALIASES[dcc]' ]] || unalias dcc 2>/dev/null || true\n")
	require.NotContains(t, out, "alias sl=")
	require.NotContains(t, out, "unalias gdoctor")

	out = newHarness().emit(t)
	require.NotContains(t, out, "unalias dcc")
	require.NotContains(t, out, "unalias ")
	require.Contains(t, out, "alias sl=")
}
