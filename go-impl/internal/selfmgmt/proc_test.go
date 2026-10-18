// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"context"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
)

// A command run for a function sees the caller's exported environment
// only, never a shell variable the stub exported for the call.
func TestOSProcsChildEnv(t *testing.T) {
	sh, err := exec.LookPath("sh")
	require.NoError(t, err)
	c := &invoke.Call{Ctx: context.Background(), Env: invoke.EnvFrom([]string{
		"PATH=/nonexistent", "KEPT=1", "TOKEN=s3cret", "GASH__ADDED=TOKEN", "GASH__SET=",
	})}
	out := osProcs{}.output(c, "/nonexistent", sh, "-c", `echo "${KEPT-}|${TOKEN-unset}|${GASH__SET-unset}"`)
	require.Equal(t, "1|unset|unset", out)
	require.Equal(t, "", osProcs{}.output(c, "/nonexistent", "sh", "-c", "echo x"))
}
