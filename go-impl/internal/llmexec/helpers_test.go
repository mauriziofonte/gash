// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

type fakeProbe struct{ sed, gawk, diff, gnu, bwrap bool }

func (f fakeProbe) HasSandboxedSed() bool  { return f.sed }
func (f fakeProbe) HasSandboxedGawk() bool { return f.gawk }
func (f fakeProbe) HasDiffNoDeref() bool   { return f.diff }
func (f fakeProbe) IsGNUGrep() bool        { return f.gnu }
func (f fakeProbe) HasBwrap() bool         { return f.bwrap }

// permissiveProbe reports every tool present except bwrap (matching the CI
// machine: GNU grep/sed/gawk/diff, no bwrap by default).
func permissiveProbe() Probe { return fakeProbe{sed: true, gawk: true, diff: true, gnu: true} }

func testEnv(cwd string) Environ {
	return Environ{
		Cwd:      cwd,
		Home:     cwd,
		RootHome: "/root",
		BashPath: "bash",
		Vars:     map[string]string{"PATH": os.Getenv("PATH"), "HOME": cwd},
		LookupUser: func(n string) (string, bool) {
			if n == "root" {
				return "/root", true
			}
			return "", false
		},
	}
}

// errCode returns the Refusal code for an error, or "" when nil, or the
// literal error text prefixed when it is not a Refusal.
func errCode(err error) string {
	if err == nil {
		return ""
	}
	if r, ok := err.(*Refusal); ok {
		return r.Code
	}
	return "non-refusal: " + err.Error()
}

// runPlan runs a built Plan in cwd and returns stdout, combined, and exit code.
func runPlan(t *testing.T, p *Plan, cwd string) (stdout string, exit int) {
	t.Helper()
	cmd := exec.Command(p.Argv[0], p.Argv[1:]...)
	cmd.Env = p.Env
	cmd.Dir = cwd
	var out strings.Builder
	cmd.Stdout = &out
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		return out.String(), ee.ExitCode()
	}
	return out.String(), 0
}
