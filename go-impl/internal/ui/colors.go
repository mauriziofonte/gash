// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ui

import (
	"io"
	"strconv"

	"github.com/mauriziofonte/gash/internal/term"
)

// ColorChart writes the 256 swatches of __gash_color_chart to w: each
// "colourN" in its own color and reset when color is true (the runtime
// gate), plain otherwise.
func ColorChart(w io.Writer, color bool) {
	b := make([]byte, 0, 256*24)
	for i := range 256 {
		n := strconv.Itoa(i)
		if color {
			b = append(b, "\x1b[38;5;"...)
			b = append(b, n...)
			b = append(b, "mcolour"...)
			b = append(b, n...)
			b = append(b, term.SeqOff...)
		} else {
			b = append(b, "colour"...)
			b = append(b, n...)
		}
		b = append(b, '\n')
	}
	_, _ = w.Write(b)
}
