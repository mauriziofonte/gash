// Gash 3.1. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExportState(t *testing.T) {
	decls, err := parseDecls("declare -- GASH_DIR=\"/g\"\ndeclare -x LANG=\"C\"\ndeclare -- GASH_PROMPT=\"0\"\ndeclare -a ARR=([0]=\"x\")\ndeclare -- __GASH_HELP_LOADED=\"1\"\n")
	require.NoError(t, err)
	got := exportState(decls, request{common: "GASH_DIR LANG NOT_SET", vars: "ARR GASH_DIR", settings: "GASH_PROMPT GASH_NVM"})
	require.Equal(t, []string{
		"GASH_DIR=/g", "LANG=C", "GASH_DIR=/g", "GASH_PROMPT=0",
		"GASH__SET=GASH_PROMPT", "GASH__ADDED=GASH_DIR ARR GASH_PROMPT", "GASH__HELP=1",
		"-GASH_LLM_EXEC", "-GASH_LLM_EXEC_ALLOW", "-GASH_LLM_SANDBOX", "-GASH_REMOTE_NO_BINARY",
	}, got)
}
