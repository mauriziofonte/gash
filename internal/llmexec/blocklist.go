// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import "strings"

// normalizeWS reproduces __llm_validate_command's whitespace normalization:
// split the line on space, tab and newline (IFS), drop empty runs, and rejoin
// the tokens with a single space. Trimming and collapsing match the bash
// `read -a` into `${words[*]}`.
func normalizeWS(cmd string) string {
	fields := strings.FieldsFunc(cmd, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n'
	})
	return strings.Join(fields, " ")
}

// validateCommand is __llm_validate_command: the dangerous-pattern blocklist,
// matched on the whitespace-normalized whole line, quotes included (EXEC-03).
// An empty command returns a refusal too (as 2.x returns 1), though Build has
// already rejected a blank line with empty_command.
func validateCommand(cmd string) error {
	if cmd == "" {
		return refuse("empty_command", "", "RETRY", "Provide a command to execute")
	}
	normalized := normalizeWS(cmd)
	for _, re := range blocklistRegexps {
		if re.MatchString(normalized) {
			return refuse("dangerous_command_blocked", "Command matches dangerous pattern", "FATAL",
				"This command is blocked for safety. Do not attempt to bypass")
		}
	}
	return nil
}
