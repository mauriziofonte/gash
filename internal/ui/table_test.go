// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ui

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/term"
)

type tableVector struct {
	name  string
	rows  string
	args  []string
	width int
}

var tableVectors = []tableVector{
	{"basic", "NAME\tSIZE\na\t1\nbb\t22\n", nil, 80},
	{"right", "NAME\tSIZE\na\t1\nbb\t22\n", []string{"--right", "2"}, 80},
	{"indent gap", "A\tB\tC\nx\ty\tz\n", []string{"--indent", "2", "--gap", "3"}, 80},
	{"no header", "a\tb\ncc\td\n", []string{"--no-header"}, 80},
	{"colors", "N\tS\tT\na\tb\tc\n", []string{"--color", "2:primary", "--color", "1:error", "--color", "3:\x1b[1m"}, 80},
	{"cell color", "N\tV\tSTYLE\na\t1\tsuccess\nb\t2\t\nc\t3\twarning\n", []string{"--cell-color", "2:3"}, 80},
	{"bar", "NAME\tUSE\nx\t3/10\ny\t0/5\nz\tnone\nw\t9/9\n", []string{"--bar", "2:10", "--right", "1"}, 80},
	{"bar styled", "NAME\tUSE\tPCT\nx\t3/10\t30\n", []string{"--bar", "2:5", "--color", "2:warning"}, 80},
	{"bar no header", "x\t1/2\ny\t2/2\n", []string{"--bar", "2:4", "--no-header"}, 80},
	{"min", "A\tB\nx\ty\n", []string{"--min", "1:8", "--min", "2:4"}, 80},
	{"max", "NAME\tX\nabcdefghij\t1\nab\t2\n", []string{"--max", "1:4"}, 80},
	{"max left", "PATH\tX\n/very/long/path/name\t1\n", []string{"--max-left", "1:8"}, 80},
	{"max one", "N\tX\nabcdef\t1\n", []string{"--max", "1:1"}, 80},
	{"max left one", "N\tX\nabcdef\t1\n", []string{"--max-left", "1:1"}, 80},
	{"fit", "SIZE\tPATH\n10\t/a/very/long/path/that/does/not/fit/in/thirty/columns\n2\tshort\n", []string{"--fit"}, 30},
	{"fit column", "PATH\tSIZE\n/a/very/long/path/that/does/not/fit\t10\n", []string{"--fit", "1", "--max-left", "1:200"}, 25},
	{"fit min ten", "AAAAAAAAAAAAAAAAAAAA\tLONG\nx\tabcdefghijklmnopqrstuvwxyz\n", []string{"--fit"}, 20},
	{"fit hidden", "A\tB\tC\nx\ty\tprimary\n", []string{"--fit", "3", "--cell-color", "1:3"}, 10},
	{"fit zero", "A\tB\nx\tyyyyyyyyyyyyyyyyyyyyyyyyyy\n", []string{"--fit", "0"}, 12},
	{"short rows", "A\tB\tC\nx\nyy\tz\n", nil, 80},
	{"short rows right", "A\tB\tC\nx\nyy\tz\n", []string{"--right", "1,2,3"}, 80},
	{"empty", "", nil, 80},
	{"all hidden", "a\nb\n", []string{"--cell-color", "1:1"}, 80},
	{"unicode", "NAME\tX\né\t1\nab\t2\n", nil, 80},
	{"unicode cut", "NAME\tX\nàbcdéfgh\t1\nab\t2\n", []string{"--max", "1:4"}, 80},
	{"trailing value option", "A\tB\nx\ty\n", []string{"--right", "1", "--gap"}, 80},
	{"trailing right", "A\tB\nx\ty\n", []string{"--right"}, 80},
	{"odd keys", "A\tB\tC\nx\ty\tsuccess\n", []string{"--color", "01:primary", "--color", "2x:error", "--cell-color", "2:03"}, 80},
	{"octal", "A\tB\nx\t010/20\n", []string{"--bar", "2:010", "--indent", "010", "--gap", "01"}, 80},
	{"bad indent", "A\tB\nx\ty\n", []string{"--indent", "abc", "--gap", "x"}, 80},
	{"unknown words", "A\tB\nx\ty\n", []string{"stray", "--nope", "--no-header"}, 80},
	{"cr and no newline", "A\tB\r\nx\ty", nil, 80},
	{"cell color from end", "A\tB\nx\tsuccess\n", []string{"--cell-color", "1:0"}, 80},
	{"wide gap", "A\tB\nx\ty\n", []string{"--gap", "0"}, 80},
	{"tabs only", "\t\t\n\t\n", nil, 80},
}

// Table writes what 2.x __gash_table wrote, for every vector, color mode
// and locale.
func TestTableMatches2x(t *testing.T) {
	for _, lang := range []string{"C.UTF-8", "C"} {
		for _, mode := range colorModes {
			var script strings.Builder
			for _, v := range tableVectors {
				var a []string
				for _, x := range v.args {
					a = append(a, shq(x))
				}
				script.WriteString("printf '%s' " + shq(v.rows) + " | COLUMNS=" + strconv.Itoa(v.width) +
					" __gash_table " + strings.Join(a, " ") + "; printf '\\0'\n")
			}
			got := run2x(t, []string{"LANG=" + lang, "LC_ALL=" + lang}, in2x(mode, script.String()))
			want := strings.Split(got, "\x00")
			require.Len(t, want, len(tableVectors)+1)
			for i, v := range tableVectors {
				s := modeStyle(mode, lang == "C.UTF-8", v.width)
				var b bytes.Buffer
				s.Table(&b, ReadRows(strings.NewReader(v.rows)), v.args...)
				if lang == "C" && strings.ContainsAny(v.rows, "éà") {
					continue // 2.x counted bytes in C (LEDGER CORE-16)
				}
				require.Equal(t, want[i], b.String(), "%s, %s, %s", v.name, mode, lang)
			}
		}
	}
}

func TestTableLines(t *testing.T) {
	s := Style{Width: 80, G: term.ASCIIGlyphs}
	require.Equal(t, []string{"A  B", "----", "x  y"}, s.TableLines([]string{"A\tB", "x\ty"}))
	require.Nil(t, s.TableLines(nil))
}
