// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSysinfoLLM runs llm_sysinfo on this machine: one JSON object with
// the keys an agent reads, / always among the disks, --mount naming the
// filesystems.
func TestSysinfoLLM(t *testing.T) {
	d := physTemp(t)
	env := baseEnv(physTemp(t), nil)
	r := runGo(t, New(), d, env, "llm_sysinfo")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Empty(t, r.Stderr)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &got), r.Stdout)
	for _, k := range []string{"host", "uptime_h", "load", "cpus", "mem_mb", "disks"} {
		require.Contains(t, got, k, r.Stdout)
	}
	require.EqualValues(t, runtime.NumCPU(), got["cpus"])
	disks := got["disks"].([]any)
	require.NotEmpty(t, disks)
	require.Equal(t, "/", disks[0].(map[string]any)["mount"])

	r = runGo(t, New(), d, env, "llm_sysinfo", "--mount", d)
	require.Equal(t, 0, r.RC, r.Stderr)
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &got))
	disks = got["disks"].([]any)
	require.Len(t, disks, 1)
	require.Equal(t, d, disks[0].(map[string]any)["mount"])

	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"--mount"}, "missing_value"},
		{[]string{"--mount", d + "/nope"}, "path_not_found"},
		{[]string{"/"}, "unexpected_argument"},
		{[]string{"--disk", "h"}, "unknown_option"},
	} {
		r := runGo(t, New(), d, env, "llm_sysinfo", tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Empty(t, r.Stdout, tc.args)
		require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, tc.args)
	}
}
