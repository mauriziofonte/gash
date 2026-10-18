// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package pathguard is the single implementation of the Gash 2.x path and
// secret rules (architecture.md 8.1). Every caller that 2.x guarded uses it:
// the parent-directory (..) traversal refusal, the forbidden tree list with
// whole-component matching over the resolved and lexical candidates (and the
// macOS /private prefix), the secret-file patterns with their exceptions and
// directories matched case-insensitively on the base name and on a symlink's
// target.
//
// The package is a library: it prints nothing and wires no CLI. A refused
// path is returned as a *Refusal carrying the exact 2.x error code, details,
// action and hint strings; the lead maps those onto the contract envelope.
package pathguard

// Refusal is the error a guard returns when a path or file is refused. Its
// fields carry, byte for byte, what the 2.x __gash_json_error would have
// printed: the error Code, the Details string, the Action (STOP, RETRY,
// CONTINUE or FATAL) and the Hint. recoverable is derived from Action by the
// contract layer (false only for FATAL), so it is not stored here.
type Refusal struct {
	Code    string
	Details string
	Action  string
	Hint    string
}

func (r *Refusal) Error() string {
	if r.Details != "" {
		return r.Code + ": " + r.Details
	}
	return r.Code
}
