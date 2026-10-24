// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

// Probe answers the tool-capability questions llm_exec asks while validating:
// GNU sed --sandbox and gawk --sandbox availability, GNU diff --no-dereference,
// whether grep is GNU grep, and whether bwrap can start a sandbox. 2.x runs
// these as external commands; the results drive refusals and the prelude, so
// Build takes them through this interface. The package starts no process:
// the caller answers (internal/llm runs the probes through internal/runner).
type Probe interface {
	HasSandboxedSed() bool
	HasSandboxedGawk() bool
	HasDiffNoDeref() bool
	IsGNUGrep() bool
	HasBwrap() bool
}

// noProbe is the Probe of a nil argument: no tool is there to trust.
type noProbe struct{}

func (noProbe) HasSandboxedSed() bool  { return false }
func (noProbe) HasSandboxedGawk() bool { return false }
func (noProbe) HasDiffNoDeref() bool   { return false }
func (noProbe) IsGNUGrep() bool        { return false }
func (noProbe) HasBwrap() bool         { return false }
