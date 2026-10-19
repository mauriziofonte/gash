// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package term

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
	xterm "golang.org/x/term"
)

// Width is __gash_tty_width: COLUMNS when it is a positive integer (Bash
// reads a leading 0 as octal), else the width of the terminal on stdin
// (2.x ran tput cols and stty size inside $(...), where stdout is a pipe
// and stderr went to /dev/null, so only stdin counted), else 80.
//
// When the compat shim pipes data into the binary, GASH__TTY_IN says
// whether the caller's stdin was a terminal (StdinTTY answers it) and the
// size is read from /dev/tty.
func (t *Term) Width() int {
	if v := get(t.env, "COLUMNS"); v != "" {
		if n, ok := positiveColumns(v); ok {
			return n
		}
	}
	if t.size != 0 {
		return t.size
	}
	if !t.stdinTTY {
		return 80
	}
	if t.stdin != nil && xterm.IsTerminal(int(t.stdin.Fd())) {
		if w, _, err := xterm.GetSize(int(t.stdin.Fd())); err == nil && w > 0 {
			return w
		}
		return 80
	}
	if f, err := os.Open("/dev/tty"); err == nil {
		defer func() { _ = f.Close() }()
		if w, _, err := xterm.GetSize(int(f.Fd())); err == nil && w > 0 {
			return w
		}
	}
	return 80
}

// positiveColumns is [[ $COLUMNS =~ ^[0-9]+$ ]] && ((COLUMNS > 0)).
func positiveColumns(v string) (int, bool) {
	for i := 0; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return 0, false
		}
	}
	base := 10
	if len(v) > 1 && v[0] == '0' {
		base = 8
	}
	n, err := strconv.ParseInt(v, base, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return int(n), true
}

// WithSize returns t with a fixed terminal width, for tests.
func (t *Term) WithSize(cols int) *Term {
	c := *t
	c.size = cols
	return &c
}

// WithKeys returns t whose key reader serves keys, for tests.
func (t *Term) WithKeys(keys string) *Term {
	c := *t
	c.keys = []byte(keys)
	return &c
}

// Keys reads single keys from stdin the way `read -rsn1` does on a
// terminal: no line buffering, no echo, signals kept, Enter read as a
// newline. Close restores the terminal.
type Keys interface {
	// Key returns the next byte of input; io.EOF at the end of input,
	// ctx.Err() when ctx is done.
	Key(ctx context.Context) (byte, error)
	// More returns up to n bytes that arrive within d (read -rsn2 -t d).
	More(n int, d time.Duration) []byte
	Close()
}

// ErrNoTerminal is returned by OpenKeys when stdin is not a terminal.
var ErrNoTerminal = errors.New("stdin is not a terminal")

// OpenKeys switches stdin to single-key input. A Fixed term with WithKeys
// serves its keys instead. On an error the Keys it returns is at the end
// of input, as a failing read is.
func (t *Term) OpenKeys() (Keys, error) {
	if t.keys != nil || t.stdin == nil {
		return &fakeKeys{b: t.keys}, nil
	}
	fd := int(t.stdin.Fd())
	old, err := unix.IoctlGetTermios(fd, ioctlGetTermios)
	if err != nil {
		return &fakeKeys{}, ErrNoTerminal
	}
	raw := *old
	raw.Lflag &^= unix.ICANON | unix.ECHO | unix.ECHOK | unix.ECHONL
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, ioctlSetTermios, &raw); err != nil {
		return &fakeKeys{}, ErrNoTerminal
	}
	return &ttyKeys{fd: fd, old: old}, nil
}

type ttyKeys struct {
	fd  int
	old *unix.Termios
}

// wait polls the descriptor for input for up to d; true when there is.
func (k *ttyKeys) wait(d time.Duration) bool {
	fds := []unix.PollFd{{Fd: int32(k.fd), Events: unix.POLLIN}}
	n, err := unix.Poll(fds, int(d/time.Millisecond))
	return err == nil && n > 0
}

func (k *ttyKeys) read() (byte, error) {
	var b [1]byte
	for {
		n, err := unix.Read(k.fd, b[:])
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return 0, err
		}
		if n == 0 {
			return 0, io.EOF
		}
		return b[0], nil
	}
}

func (k *ttyKeys) Key(ctx context.Context) (byte, error) {
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if k.wait(100 * time.Millisecond) {
			return k.read()
		}
	}
}

func (k *ttyKeys) More(n int, d time.Duration) []byte {
	var out []byte
	deadline := time.Now().Add(d)
	for len(out) < n {
		left := time.Until(deadline)
		if left <= 0 || !k.wait(left) {
			break
		}
		b, err := k.read()
		if err != nil {
			break
		}
		out = append(out, b)
	}
	return out
}

func (k *ttyKeys) Close() { _ = unix.IoctlSetTermios(k.fd, ioctlSetTermios, k.old) }

type fakeKeys struct{ b []byte }

func (k *fakeKeys) Key(ctx context.Context) (byte, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(k.b) == 0 {
		return 0, io.EOF
	}
	c := k.b[0]
	k.b = k.b[1:]
	return c, nil
}

func (k *fakeKeys) More(n int, _ time.Duration) []byte {
	n = min(n, len(k.b))
	out := k.b[:n]
	k.b = k.b[n:]
	return out
}

func (k *fakeKeys) Close() {}
