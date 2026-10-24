// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// TestParseCompletion: the answer of the hte tree to a cobra __complete
// request is relayed candidate by candidate, descriptions kept, with its
// directive (testdata/script/completion.txtar drives the whole path).
func TestParseCompletion(t *testing.T) {
	comps, d := parseCompletion("service\tStart, stop and inspect the stack\nsite\tCreate, change and remove sites\n:4\n")
	require.Equal(t, []cobra.Completion{"service\tStart, stop and inspect the stack", "site\tCreate, change and remove sites"}, comps)
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp, d)

	comps, d = parseCompletion(":0\n")
	require.Empty(t, comps)
	require.Equal(t, cobra.ShellCompDirectiveDefault, d)

	comps, d = parseCompletion("")
	require.Empty(t, comps)
	require.Equal(t, cobra.ShellCompDirectiveDefault, d)
}
