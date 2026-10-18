// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package term

import (
	"io"
	"os"
	"strings"
	"syscall"
)

// fakeTTY is the controlling terminal of a test term: none (the open
// fails with ENXIO, as without a controlling terminal), or one whose
// keyboard types input.
type fakeTTY struct {
	ok    bool
	input string
}

// OpenTTY opens the controlling terminal for reading, as `read
// </dev/tty` did in 2.x confirmations. Without one the error is ENXIO
// (or what the open returned). A Fixed term has no terminal unless
// WithTTY gave it one; it never opens the real /dev/tty.
func (t *Term) OpenTTY() (io.ReadCloser, error) {
	if t.tty != nil {
		if !t.tty.ok {
			return nil, &os.PathError{Op: "open", Path: "/dev/tty", Err: syscall.ENXIO}
		}
		return io.NopCloser(strings.NewReader(t.tty.input)), nil
	}
	return os.Open("/dev/tty")
}

// WithTTY returns t with a controlling terminal whose keyboard types
// input (then end of input), for tests.
func (t *Term) WithTTY(input string) *Term {
	c := *t
	c.tty = &fakeTTY{ok: true, input: input}
	return &c
}

// WithoutTTY returns t without a controlling terminal, for tests.
func (t *Term) WithoutTTY() *Term {
	c := *t
	c.tty = &fakeTTY{}
	return &c
}
