// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHeading(t *testing.T) {
	var b bytes.Buffer
	Heading(&b, false, 2, "", "USAGE")
	Heading(&b, false, 0, "note", "T")
	Heading(&b, true, 1, "n", "T")
	require.Equal(t, "  USAGE\nT  note\n \x1b[1mT\x1b[0m  \x1b[38;5;245mn\x1b[0m\n", b.String())
}

func TestColorChart(t *testing.T) {
	var b bytes.Buffer
	ColorChart(&b, false)
	lines := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	require.Len(t, lines, 256)
	require.Equal(t, "colour0", lines[0])
	require.Equal(t, "colour255", lines[255])

	b.Reset()
	ColorChart(&b, true)
	lines = strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	require.Len(t, lines, 256)
	require.Equal(t, "\x1b[38;5;0mcolour0\x1b[0m", lines[0])
	require.Equal(t, "\x1b[38;5;196mcolour196\x1b[0m", lines[196])
}
