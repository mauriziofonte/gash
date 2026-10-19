// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package dev

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLedger_DEV13_2x: sortV reproduces GNU `sort -V`, including the
// prerelease rule (a prerelease sorts after its release line but a
// higher patch sorts above a lower one).
func TestLedger_DEV13_2x(t *testing.T) {
	cases := [][]string{
		{"20.11.1", "20.12.0-rc.1", "20.12.0", "18.18.0", "22.14.0"},
		{"zzz-broken", "20.11.1", "8.0.0", "10.0.0"},
		{"1.0", "1.0.1", "1.0~rc1", "1.0.0", "1"},
		{"v1", "v10", "v2", "v1.2", "v1.10", "v1.2.0"},
		{"a", "a1", "a1b", "a.b", "a..b", ".a", "..", "."},
	}
	for _, in := range cases {
		require.Equal(t, gnuSortV(t, in), sortV(in), "%q", in)
	}
}

// TestVerCmpSign pins a few comparisons that node_detect_version relies
// on, independent of whether GNU sort is installed.
func TestVerCmpSign(t *testing.T) {
	require.Negative(t, verCmp("20.11.1", "20.12.0-rc.1"))
	require.Negative(t, verCmp("20.11.1", "zzz-broken"))
	// A release sorts before its own -rc suffix in filevercmp (the extra
	// characters sort after end-of-string), the reverse of semver.
	require.Negative(t, verCmp("20.12.0", "20.12.0-rc.1"))
	require.Zero(t, verCmp("20.9.0", "20.9.0"))
}

// gnuSortV runs the installed `sort -V`; the test is skipped without a
// GNU sort.
func gnuSortV(t *testing.T, lines []string) []string {
	t.Helper()
	sortBin, err := exec.LookPath("sort")
	if err != nil {
		t.Skip("no sort")
	}
	if v, _ := exec.Command(sortBin, "--version").Output(); !strings.Contains(string(v), "GNU coreutils") {
		t.Skip("not GNU sort")
	}
	cmd := exec.Command(sortBin, "-V")
	cmd.Env = []string{"LC_ALL=C"}
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.Split(strings.TrimRight(string(out), "\n"), "\n")
}
