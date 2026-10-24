// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import "github.com/mauriziofonte/gash/internal/pathguard"

// Refusal is re-exported from pathguard so callers of llmexec need not import
// both packages to inspect the error.
type Refusal = pathguard.Refusal

// refuse builds a refusal with the four 2.x fields.
func refuse(code, details, action, hint string) *Refusal {
	return &Refusal{Code: code, Details: details, Action: action, Hint: hint}
}

// refuseOption is __llm_exec_refuse_option: "the option is not allowed for
// this command". Details is "NAME WORD".
func refuseOption(name, word string) *Refusal {
	return refuse("option_not_allowed", name+" "+word, "FATAL",
		"llm_exec runs read-only commands; '"+word+"' can write, delete, run programs or read past the secret-file and path checks")
}
