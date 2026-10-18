// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// prelude2x returns the 2.x child prelude as the runner embeds it:
// "$(__llm_exec_prelude EXTRA MODE)", trailing newlines dropped.
func prelude2x(t *testing.T, extra []string, mode string) string {
	t.Helper()
	dir := archive2x(t)
	x := " "
	for _, w := range extra {
		x += w + " "
	}
	script := "GASH_HEADLESS=1; source '" + dir + "/gash.sh'; " +
		`printf '%s' "$(__llm_exec_prelude "$GASH_X" "$GASH_M")"`
	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", script)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "GASH_X=" + x, "GASH_M=" + mode}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("2.x prelude: %v", err)
	}
	return string(out)
}

// bash52 reports whether the local bash is 5.2, whose `declare -f` layout
// childHelpers carries.
func bash52(t *testing.T) bool {
	t.Helper()
	out, err := exec.Command("bash", "--noprofile", "--norc", "-c",
		`printf '%s.%s' "${BASH_VERSINFO[0]}" "${BASH_VERSINFO[1]}"`).Output()
	return err == nil && string(out) == "5.2"
}

// TestPrelude_Parity: the child prelude is the 2.x one byte for byte, in
// restricted and unrestricted mode and with GASH_LLM_EXEC_ALLOW words that
// drop wrappers. Equal text means equal child behavior, and equal line
// counts mean a syntax error in the command line names the same line.
func TestPrelude_Parity(t *testing.T) {
	if !bash52(t) {
		t.Skip("childHelpers carries the bash 5.2 declare -f layout")
	}
	for _, tc := range []struct {
		name  string
		extra []string
		mode  string
	}{
		{"default", nil, ""},
		{"allow-sed-grep-git", []string{"sed", "grep", "git"}, ""},
		{"allow-the-rest", []string{"awk", "gawk", "egrep", "fgrep", "rg", "diff", "make"}, ""},
		{"unrestricted", nil, "unrestricted"},
		{"unrestricted-allow", []string{"make", "npm"}, "unrestricted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mine string
			if tc.mode == "unrestricted" {
				mine = preludeUnrestricted(tc.extra)
			} else {
				mine = prelude(tc.extra, OSProbe{})
			}
			mine = strings.TrimRight(mine, "\n")
			ref := prelude2x(t, tc.extra, tc.mode)
			if mine != ref {
				ml, rl := strings.Split(mine, "\n"), strings.Split(ref, "\n")
				for i := 0; i < len(ml) || i < len(rl); i++ {
					var a, b string
					if i < len(ml) {
						a = ml[i]
					}
					if i < len(rl) {
						b = rl[i]
					}
					if a != b {
						t.Fatalf("first difference at line %d (mine %d lines, 2.x %d):\n  mine = %q\n  2.x  = %q",
							i+1, len(ml), len(rl), a, b)
					}
				}
			}
		})
	}
}

func TestPrintfQ(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", "''"},
		{"--no-dereference", "--no-dereference"},
		{"--exclude=[Ii][Dd]_[Rr][Ss][Aa]", `--exclude=\[Ii\]\[Dd\]_\[Rr\]\[Ss\]\[Aa\]`},
		{"#x", `\#x`},
		{"x#", "x#"},
		{"~x", `\~x`},
		{"a:~x", `a:\~x`},
		{"a=~x", `a=\~x`},
		{"x~", "x~"},
		{"a b", `a\ b`},
		{":(exclude,icase,glob)**/*.pem", `:\(exclude\,icase\,glob\)\*\*/\*.pem`},
		{"a\tb", "'a\tb'"},
	} {
		if got := printfQ(tc.in); got != tc.want {
			t.Errorf("printfQ(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if got := quoteArray(nil); got != "'' " {
		t.Errorf("quoteArray(nil) = %q", got)
	}
}
