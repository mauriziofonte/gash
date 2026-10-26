// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runcmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPlainWriter: escape sequences are dropped and carriage-return
// redraws keep the line's last state, whatever the write boundaries.
func TestPlainWriter(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"text", "a\nb", "a\nb"},
		{"colors", "\x1b[1;31mFAIL\x1b[0m tests/a.php\n", "FAIL tests/a.php\n"},
		{"cursor and modes", "\x1b[?25l\x1b[2Ahello\x1b[?25h\n", "hello\n"},
		{"osc title bel", "\x1b]0;title\x07done\n", "done\n"},
		{"osc link st", "\x1b]8;;https://example.com\x1b\\link\x1b]8;;\x1b\\\n", "link\n"},
		{"dcs", "\x1bPq#0;2\x1b\\x\n", "x\n"},
		{"charset", "\x1b(Bx\x1b)0y\n", "xy\n"},
		{"two-byte", "\x1b7a\x1b8b\x1bMc\n", "abc\n"},
		{"progress", "  1%\r 50%\r100%\n", "100%\n"},
		{"crlf", "one\r\ntwo\r\n", "one\ntwo\n"},
		{"cr before end", "kept\r\n", "kept\n"},
		{"cr erase", "Downloading 40%\r\x1b[KDone\n", "Done\n"},
		{"erase whole", "Downloading\x1b[2K\rDone\n", "Done\n"},
		{"erase then end", "spin |\r\x1b[K\n", "\n"},
		{"backspace", "ab\bc\n", "ac\n"},
		{"backspace utf8", "é\bx\n", "x\n"},
		{"controls", "a\x07b\x00c\td\x7f\n", "abc\td\n"},
		{"utf8 kept", "✔ ok — fine\n", "✔ ok — fine\n"},
		{"last line", "done\r", "done"},
		{"redraw last line", "50%\r100%", "100%"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Whole, then split at every boundary, then byte by byte.
			for split := 0; split <= len(tc.in); split++ {
				var out bytes.Buffer
				p := newPlainWriter(&out)
				_, err := p.Write([]byte(tc.in[:split]))
				require.NoError(t, err)
				_, err = p.Write([]byte(tc.in[split:]))
				require.NoError(t, err)
				require.NoError(t, p.Close())
				require.Equal(t, tc.want, out.String(), "split at %d", split)
			}
			var out bytes.Buffer
			p := newPlainWriter(&out)
			for i := 0; i < len(tc.in); i++ {
				_, err := p.Write([]byte{tc.in[i]})
				require.NoError(t, err)
			}
			require.NoError(t, p.Close())
			require.Equal(t, tc.want, out.String(), "byte by byte")
		})
	}

	// A line is written at its newline, not before.
	var out bytes.Buffer
	p := newPlainWriter(&out)
	_, _ = p.Write([]byte("first\nsec"))
	require.Equal(t, "first\n", out.String())
	_, _ = p.Write([]byte("ond\n"))
	require.Equal(t, "first\nsecond\n", out.String())

	// A line past the cap is written in pieces.
	out.Reset()
	p = newPlainWriter(&out)
	long := strings.Repeat("x", plainLineCap+10)
	_, _ = p.Write([]byte(long))
	require.Equal(t, plainLineCap, out.Len())
	_, _ = p.Write([]byte("\n"))
	require.Equal(t, long+"\n", out.String())
}

// TestRunPlain: gash run --plain cleans both streams, before the budget
// and before --test-summary; without it the bytes pass untouched.
func TestRunPlain(t *testing.T) {
	env := baseEnv(t, nil)
	bin := phys(t)
	stub(t, bin, "colors", `printf '\033[32mok\033[0m 1\n'; printf 'step 1\rstep 2\rstep 3\n'; printf '\033[31merr\033[0m\n' >&2; printf 'last\r'`+"\n")
	stub(t, bin, "pytest", `printf '\033[31mFAILED\033[0m tests/test_a.py::test_x - assert 1 == 2\n'`+
		`; printf '\033[31m=== 1 failed, 2 passed in 0.10s ===\033[0m\n'; exit 1`+"\n")
	env["PATH"] = bin + ":" + env["PATH"]
	d := phys(t)

	r := command(t, d, env, "run", "--", "colors")
	require.Equal(t, result{"\x1b[32mok\x1b[0m 1\nstep 1\rstep 2\rstep 3\nlast\r", "\x1b[31merr\x1b[0m\n", 0}, r)
	r = command(t, d, env, "run", "--plain", "--", "colors")
	require.Equal(t, result{"ok 1\nstep 3\nlast", "err\n", 0}, r)
	r = command(t, d, env, "run", "--plain", "--max-lines", "1", "--width", "3", "--", "colors")
	require.Equal(t, result{"ok…\n[... 2 lines omitted]\n", "err\n", 0}, r)

	r = command(t, d, env, "run", "--plain", "--test-summary", "--", "pytest")
	require.Equal(t, 1, r.RC)
	var sum struct {
		Failed, Passed int
		Failures       []struct{ Name, File string }
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &sum), r.Stdout)
	require.Equal(t, 1, sum.Failed)
	require.Equal(t, 2, sum.Passed)
	require.NotContains(t, r.Stdout, `\u001b`)

	r = command(t, d, env, "run", "__complete", "--pl")
	require.Equal(t, "--plain\n:4\n", r.Stdout)
}
