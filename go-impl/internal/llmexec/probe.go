// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import (
	"os/exec"
	"strings"
)

// Probe answers the tool-capability questions llm_exec asks while validating:
// GNU sed --sandbox and gawk --sandbox availability, GNU diff --no-dereference,
// whether grep is GNU grep, and whether bwrap can start a sandbox. 2.x runs
// these as external commands; the results drive refusals and the prelude, so
// Build takes them through this interface (injectable for tests).
type Probe interface {
	HasSandboxedSed() bool
	HasSandboxedGawk() bool
	HasDiffNoDeref() bool
	IsGNUGrep() bool
	HasBwrap() bool
}

// OSProbe runs the real probes, exactly as lib/modules/llm/exec.sh does.
type OSProbe struct{}

func (OSProbe) HasSandboxedSed() bool {
	cmd := exec.Command("sed", "--sandbox", "-n", "p")
	cmd.Stdin = strings.NewReader("x\n")
	return cmd.Run() == nil
}

func (OSProbe) HasSandboxedGawk() bool {
	if _, err := exec.LookPath("gawk"); err != nil {
		return false
	}
	return exec.Command("gawk", "--sandbox", "BEGIN{}").Run() == nil
}

func (OSProbe) HasDiffNoDeref() bool {
	return exec.Command("diff", "--no-dereference", "/dev/null", "/dev/null").Run() == nil
}

func (OSProbe) IsGNUGrep() bool {
	out, err := exec.Command("grep", "--version").Output()
	return err == nil && strings.Contains(string(out), "GNU")
}

func (OSProbe) HasBwrap() bool {
	if _, err := exec.LookPath("bwrap"); err != nil {
		return false
	}
	return exec.Command("bwrap", "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc",
		"--unshare-all", "--die-with-parent", "true").Run() == nil
}
