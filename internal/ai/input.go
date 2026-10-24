// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"os"
	"time"

	"golang.org/x/sys/unix"

	"github.com/mauriziofonte/gash/internal/invoke"
)

// pipedWait is how long a non-terminal stdin that is not a regular file
// may stay silent before ai_query and ai_ask ignore it (LEDGER AI-05).
const pipedWait = 2 * time.Second

// readPipedInput is the piped-input read of ai_query and ai_ask (LEDGER
// AI-05): a regular file is read; a pipe, socket or device is read only
// when it becomes readable (data or EOF) within m.pipeWait, otherwise it
// is ignored with a warning, so an idle open stdin (an agent harness, a
// cron job) no longer blocks the call. A reader that is not a file (tests)
// is read as it is.
func (m *Module) readPipedInput(c *invoke.Call) string {
	if f, ok := c.Stdin.(*os.File); ok && f != nil {
		if fi, err := f.Stat(); err == nil && !fi.Mode().IsRegular() && !readableWithin(f, m.pipeWait) {
			printer(c).Warning("stdin is open but sent nothing within 2s: ignored (redirect </dev/null to skip the wait)")
			return ""
		}
	}
	return readPiped(c.Stdin)
}

// readableWithin polls f for input (or hang-up) for at most d. A descriptor
// poll(2) cannot watch (POLLNVAL, a device on macOS) counts as readable.
func readableWithin(f *os.File, d time.Duration) bool {
	fds := []unix.PollFd{{Fd: int32(f.Fd()), Events: unix.POLLIN}}
	deadline := time.Now().Add(d)
	for {
		ms := int(time.Until(deadline) / time.Millisecond)
		if ms < 0 {
			ms = 0
		}
		n, err := unix.Poll(fds, ms)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return true
		}
		return n > 0 && fds[0].Revents != 0
	}
}
