// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmfmt

import (
	"bufio"
	"io"
	"strconv"
)

// OmittedMarker is the line that stands for the lines a budget cut.
func OmittedMarker(k int, unit string) string {
	return "[... " + strconv.Itoa(k) + " " + unit + " omitted]"
}

// LineWriter writes text lines under the budget of Options: each line is
// cut at --width, the first Head and the last Tail lines are kept, and the
// cut lines are replaced by one OmittedMarker line. Lines are streamed:
// only the Tail lines are held in memory.
type LineWriter struct {
	w          *bufio.Writer
	width      int
	head, tail int
	unit       string
	n          int // lines seen
	ring       []string
	start      int
	omitted    int
	closed     bool
}

// Lines returns a LineWriter over w with the budget of o. unit names what
// a line is in the marker ("lines" for text, "rows" for a table).
func (o *Options) Lines(w io.Writer, unit string) *LineWriter {
	if unit == "" {
		unit = "lines"
	}
	return &LineWriter{w: bufio.NewWriterSize(w, 32<<10), width: o.Width, head: o.Head, tail: o.Tail, unit: unit}
}

// Line writes one line (s without its newline).
func (l *LineWriter) Line(s string) {
	l.n++
	s = Cut(s, l.width)
	capped := l.head > 0 || l.tail > 0
	if !capped || l.n <= l.head {
		l.emit(s)
		return
	}
	if l.tail == 0 {
		l.omitted++
		return
	}
	if len(l.ring) < l.tail {
		l.ring = append(l.ring, s)
		return
	}
	// The ring is full: its oldest line is cut.
	l.ring[l.start] = s
	l.start = (l.start + 1) % l.tail
	l.omitted++
}

func (l *LineWriter) emit(s string) {
	_, _ = l.w.WriteString(s)
	_ = l.w.WriteByte('\n')
}

// Close writes the marker and the held tail lines, flushes, and returns
// how many lines were omitted (0 when nothing was cut).
func (l *LineWriter) Close() int {
	if l.closed {
		return l.omitted
	}
	l.closed = true
	if l.omitted > 0 {
		l.emit(OmittedMarker(l.omitted, l.unit))
	}
	for i := 0; i < len(l.ring); i++ {
		l.emit(l.ring[(l.start+i)%len(l.ring)])
	}
	_ = l.w.Flush()
	return l.omitted
}

// Seen is the number of lines written so far, kept or not.
func (l *LineWriter) Seen() int { return l.n }

// Cap applies the line budget of o to a slice already in memory: it returns
// the kept head, the kept tail and how many items were cut between them.
// Without a budget, head is items and tail is nil.
func Cap[T any](o *Options, items []T) (head, tail []T, omitted int) {
	if !o.Capped() || len(items) <= o.Head+o.Tail {
		return items, nil, 0
	}
	if o.Tail == 0 && o.Head > 0 {
		return items[:o.Head], nil, len(items) - o.Head
	}
	return items[:o.Head], items[len(items)-o.Tail:], len(items) - o.Head - o.Tail
}
