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

// SedNamer is a Probe that can name the GNU sed it trusts: "gsed" where
// only that name has --sandbox (Homebrew installs GNU sed as gsed, beside
// the BSD sed of macOS), "sed" or "" otherwise. sed on the command line then
// runs as that program, with --sandbox (3.4).
type SedNamer interface {
	SedName() string
}

// sedProgram is the program the sed wrapper of the prelude runs: gsed when
// the probe names it, sed otherwise.
func sedProgram(pr Probe) string {
	if n, ok := pr.(SedNamer); ok && n.SedName() == "gsed" {
		return "gsed"
	}
	return "sed"
}

// noProbe is the Probe of a nil argument: no tool is there to trust.
type noProbe struct{}

func (noProbe) HasSandboxedSed() bool  { return false }
func (noProbe) HasSandboxedGawk() bool { return false }
func (noProbe) HasDiffNoDeref() bool   { return false }
func (noProbe) IsGNUGrep() bool        { return false }
func (noProbe) HasBwrap() bool         { return false }
