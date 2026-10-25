// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"context"
	"time"

	"golang.org/x/sys/unix"
)

// inotifyWaker wakes llm_wait as soon as a watched file changes or a
// watched directory gains or loses an entry, and at the poll interval in
// any case (a change on a network or WSL drvfs mount sends no event).
type inotifyWaker struct {
	fd int
}

// newWaker returns the inotify waker, or the plain sleeper when inotify
// cannot start (no free instance, a seccomp filter).
func newWaker() waker {
	fd, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
	if err != nil {
		return sleepWaker{}
	}
	return &inotifyWaker{fd: fd}
}

// watchFile watches a file's content and its renaming or removal.
func (w *inotifyWaker) watchFile(path string) {
	_, _ = unix.InotifyAddWatch(w.fd, path, unix.IN_MODIFY|unix.IN_ATTRIB|unix.IN_CLOSE_WRITE|unix.IN_MOVE_SELF|unix.IN_DELETE_SELF)
}

// watchDir watches the entries of a directory (not their content, so a
// busy log directory does not wake the wait for every other file).
func (w *inotifyWaker) watchDir(dir string) {
	_, _ = unix.InotifyAddWatch(w.fd, dir, unix.IN_CREATE|unix.IN_MOVED_TO|unix.IN_MOVED_FROM|unix.IN_DELETE|unix.IN_ATTRIB)
}

// wait blocks until an event, d, or the end of ctx; the events are
// drained, the caller looks at the file again in any case.
func (w *inotifyWaker) wait(ctx context.Context, d time.Duration) {
	if ctx.Err() != nil {
		return
	}
	ms := int(d / time.Millisecond)
	if ms < 1 {
		ms = 1
	}
	fds := []unix.PollFd{{Fd: int32(w.fd), Events: unix.POLLIN}}
	n, err := unix.Poll(fds, ms)
	if err != nil || n <= 0 {
		return
	}
	var buf [8192]byte
	for {
		if _, err := unix.Read(w.fd, buf[:]); err != nil {
			return
		}
	}
}

func (w *inotifyWaker) close() { _ = unix.Close(w.fd) }
