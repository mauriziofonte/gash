// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runcmd

import (
	"io"
	"sync"
	"unicode/utf8"
)

// plainLineCap bounds the line a plainWriter holds: past it the line is
// written as it is and the next bytes start a new piece of the same line
// (a redraw can then no longer take back what was written).
const plainLineCap = 1 << 20

// plain parser states: text, after ESC, an escape sequence's intermediate
// bytes, a CSI sequence, a control string (OSC, DCS, SOS, PM, APC) and
// an ESC inside a control string (the start of its ST terminator).
const (
	plainText = iota
	plainEsc
	plainInter
	plainCSI
	plainString
	plainStringEsc
)

// plainWriter is gash run --plain: what a command writes for a terminal,
// as the text a terminal leaves on screen, line by line. Escape sequences
// (CSI such as colors and cursor moves, OSC such as titles and links, the
// other ESC sequences and control strings) are dropped, even when a write
// splits them. A carriage return that does not end the line restarts it:
// the next text replaces what the line held (a progress redraw keeps its
// last state), a line ended right after one keeps its text, and an erase
// in line (ESC [ K) after one empties it. A backspace takes back the last
// character; the other control characters except tab are dropped. Lines
// are written whole, at their newline (or at Close).
type plainWriter struct {
	mu    sync.Mutex
	w     io.Writer
	line  []byte
	state int
	// csiParam is the parameter of the CSI sequence read so far, while it
	// could still be an erase in line (one number at most, final byte to
	// come); csiK is false once it cannot.
	csiParam []byte
	csiK     bool
	// restart is a carriage return not yet followed by text.
	restart bool
}

func newPlainWriter(w io.Writer) *plainWriter { return &plainWriter{w: w} }

func (p *plainWriter) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range b {
		switch p.state {
		case plainEsc:
			switch {
			case c == '[':
				p.state, p.csiK, p.csiParam = plainCSI, true, p.csiParam[:0]
			case c == ']' || c == 'P' || c == 'X' || c == '^' || c == '_':
				p.state = plainString
			case c >= 0x20 && c <= 0x2f:
				p.state = plainInter
			default:
				// A two-byte sequence (ESC 7, ESC M, ESC c), or a byte
				// that ends it: dropped either way.
				p.state = plainText
			}
			continue
		case plainInter:
			if c < 0x20 || c > 0x2f {
				p.state = plainText
			}
			continue
		case plainCSI:
			switch {
			case c >= 0x40 && c <= 0x7e:
				if c == 'K' && p.csiK {
					p.eraseLine(string(p.csiParam))
				}
				p.state = plainText
			case c >= '0' && c <= '9' && len(p.csiParam) < 4:
				p.csiParam = append(p.csiParam, c)
			default:
				p.csiK = false
			}
			continue
		case plainString:
			switch c {
			case 0x07:
				p.state = plainText
			case 0x1b:
				p.state = plainStringEsc
			}
			continue
		case plainStringEsc:
			if c == '\\' {
				p.state = plainText
			} else if c != 0x1b {
				p.state = plainString
			}
			continue
		}
		switch {
		case c == 0x1b:
			p.state = plainEsc
		case c == '\n':
			if err := p.flush(true); err != nil {
				return 0, err
			}
		case c == '\r':
			p.restart = true
		case c == '\b':
			if !p.restart && len(p.line) > 0 {
				_, n := utf8.DecodeLastRune(p.line)
				p.line = p.line[:len(p.line)-n]
			}
		case c < 0x20 && c != '\t' || c == 0x7f:
			// Another control character: nothing on screen.
		default:
			if p.restart {
				p.line, p.restart = p.line[:0], false
			}
			p.line = append(p.line, c)
			if len(p.line) >= plainLineCap {
				if err := p.flush(false); err != nil {
					return 0, err
				}
			}
		}
	}
	return len(b), nil
}

// eraseLine is an erase in line (ESC [ n K): the whole line (2), or the
// part on one side of the cursor, which is at the start of the line after
// a carriage return and at its end otherwise (0 or none: to the right,
// 1: to the left).
func (p *plainWriter) eraseLine(n string) {
	switch {
	case n == "2", n == "1" && !p.restart, (n == "" || n == "0") && p.restart:
		p.line, p.restart = p.line[:0], false
	}
}

// flush writes the line, with a newline when it ended.
func (p *plainWriter) flush(ended bool) error {
	if ended {
		p.line = append(p.line, '\n')
	}
	_, err := p.w.Write(p.line)
	p.line, p.restart = p.line[:0], false
	return err
}

// Close writes a last line that has no newline.
func (p *plainWriter) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.line) == 0 {
		return nil
	}
	return p.flush(false)
}
