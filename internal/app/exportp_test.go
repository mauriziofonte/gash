// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package app

import (
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// tricky are values export -p must quote: every byte but NUL, quotes, the
// expansion characters, newlines, UTF-8 and invalid UTF-8.
var tricky = func() []string {
	var all strings.Builder
	for b := 1; b < 256; b++ {
		all.WriteByte(byte(b))
	}
	return []string{
		"", "plain", "two words", "q\"uote", "s'ingle", `back\slash`, "$HOME", "`date`", "$(id)",
		"line1\nline2\n", "tab\there", "\x01\x1b[0m\x7f", "é ü 日本", "\xff\xfe", all.String(),
		"trailing\\", "\\n", "!bang", "*glob?", "a=b=c",
	}
}()

func TestParseExportPMatchesBash(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash")
	}
	major, err := exec.Command(bash, "-c", `echo "${BASH_VERSINFO[0]}"`).Output()
	require.NoError(t, err)
	for _, mode := range []string{"", "set -o posix; ", "LC_ALL=C.UTF-8; ", "LC_ALL=C; "} {
		var env []string
		for i, v := range tricky {
			env = append(env, "V"+string(rune('A'+i))+"="+v)
		}
		env = append(env, "PATH=/usr/bin:/bin")
		// export -p first, a NUL, then env -0 for what a program gets (exec in
		// a subshell, as the stub runs the binary).
		cmd := exec.Command(bash, "-c", mode+`declare -x UNSET_X; declare -ax ARR=(1 "x y"); declare -ix N=5; export -p; printf '\0'; ( exec env -0 )`)
		cmd.Env = env
		out, err := cmd.Output()
		require.NoError(t, err, mode)
		text, rest, ok := strings.Cut(string(out), "\x00")
		require.True(t, ok)
		got, err := parseExportP(text)
		require.NoError(t, err, mode)
		var want []string
		for _, kv := range strings.Split(strings.TrimSuffix(rest, "\x00"), "\x00") {
			if !strings.HasPrefix(kv, "_=") {
				want = append(want, kv)
			}
		}
		if string(major) == "4\n" {
			// Bash 4.4's exec takes one from SHLVL, as the server does.
			for i, kv := range got {
				if v, ok := strings.CutPrefix(kv, "SHLVL="); ok {
					got[i] = "SHLVL=" + execShellLevel(v)
				}
			}
		}
		slices.Sort(got)
		slices.Sort(want)
		require.Equal(t, want, got, "mode %q", mode)
	}
}

func TestExecShellLevel(t *testing.T) {
	for in, want := range map[string]string{
		"1": "0", "": "0", "x": "0", "7": "6", " 3\t": "2", "+2": "1", "-4": "0",
		"1000": "999", "1001": "1", "99999999999999999999": "0",
	} {
		require.Equal(t, want, execShellLevel(in), in)
	}
}

func TestParseDeclsFlags(t *testing.T) {
	d, err := parseDecls("declare -- A=\"1\"\ndeclare -x B\ndeclare -a C=([0]=\"x\ny\" [1]=\")\")\ndeclare -rx D=$'a\\nb'\n")
	require.NoError(t, err)
	require.Len(t, d, 4)
	require.Equal(t, decl{name: "A", flags: "--", value: "1", set: true}, d[0])
	require.False(t, d[1].set)
	require.True(t, d[1].exported())
	require.False(t, d[2].scalar())
	require.Equal(t, "a\nb", d[3].value)
}

func TestExportState(t *testing.T) {
	decls, err := parseDecls("declare -- GASH_DIR=\"/g\"\ndeclare -x LANG=\"C\"\ndeclare -- GASH_PROMPT=\"0\"\ndeclare -a ARR=([0]=\"x\")\ndeclare -- __GASH_HELP_LOADED=\"1\"\n")
	require.NoError(t, err)
	got := exportState(decls, request{common: "GASH_DIR LANG NOT_SET", vars: "ARR GASH_DIR", settings: "GASH_PROMPT GASH_NVM"})
	require.Equal(t, []string{
		"GASH_DIR=/g", "LANG=C", "GASH_DIR=/g", "GASH_PROMPT=0",
		"GASH__SET=GASH_PROMPT", "GASH__ADDED=GASH_DIR ARR GASH_PROMPT", "GASH__HELP=1",
		"-GASH_LLM_EXEC", "-GASH_LLM_EXEC_ALLOW", "-GASH_LLM_SANDBOX",
	}, got)
}
