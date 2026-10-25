// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const sample = `# Changelog

Intro.

## [3.0.1] - 2026-11-01

### Fixed

- One.

## [3.0.0] - 2026-10-24

Body.

[3.0.1]: https://example.com/a
[3.0.0]: https://example.com/b
`

func TestEntry(t *testing.T) {
	got, err := entry(strings.NewReader(sample), "3.0.1")
	require.NoError(t, err)
	require.Equal(t, "### Fixed\n\n- One.\n", got)

	got, err = entry(strings.NewReader(sample), "3.0.0")
	require.NoError(t, err)
	require.Equal(t, "Body.\n", got, "the link references are not part of the last entry")

	_, err = entry(strings.NewReader(sample), "3.0.2")
	require.ErrorContains(t, err, `no "## [3.0.2]" section`)

	_, err = entry(strings.NewReader("## [1.0.0] - 2026-01-01\n\n## [0.9.0]\n"), "1.0.0")
	require.ErrorContains(t, err, "is empty")

	_, err = entry(strings.NewReader("## [3.0.10] - x\n\nBody\n"), "3.0.1")
	require.Error(t, err, "3.0.1 must not match the 3.0.10 heading")
}

// The VERSION being worked on always has its entry: a release tag of it can
// publish its notes.
func TestRepoChangelogHasVersion(t *testing.T) {
	v, err := os.ReadFile("../../../VERSION")
	require.NoError(t, err)
	f, err := os.Open("../../../CHANGELOG.md")
	require.NoError(t, err)
	defer f.Close()
	_, err = entry(f, strings.TrimSpace(string(v)))
	require.NoError(t, err)
}
