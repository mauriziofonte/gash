// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package state

import (
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
)

// LockWait is how long a mutation waits for the lock (design/hte.md 1).
const LockWait = 30 * time.Second

const lockRetry = 100 * time.Millisecond

// Clock is the time source of the lock wait; tests pass a fake.
type Clock interface {
	Now() time.Time
	Sleep(time.Duration)
}

type realClock struct{}

func (realClock) Now() time.Time        { return time.Now() }
func (realClock) Sleep(d time.Duration) { time.Sleep(d) }

// RealClock is the wall clock.
var RealClock Clock = realClock{}

// Lock is the exclusive mutation lock, an flock(2) on the lock file.
type Lock struct {
	fl *flock.Flock
}

// AcquireLock takes the exclusive lock at path, waiting up to wait. When
// another process holds it longer, it fails with lock_timeout (RETRY).
func AcquireLock(path string, wait time.Duration, clk Clock) (*Lock, error) {
	if clk == nil {
		clk = RealClock
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return nil, err
	}
	fl := flock.New(path, flock.SetPermissions(fileMode))
	start := clk.Now()
	for {
		ok, err := fl.TryLock()
		if err != nil {
			return nil, err
		}
		if ok {
			return &Lock{fl: fl}, nil
		}
		if clk.Now().Sub(start) >= wait {
			return nil, hteerr.Newf(hteerr.LockTimeout, "another gash hte command is running; retry when it ends",
				"%s is held by another process after %s", path, wait)
		}
		clk.Sleep(lockRetry)
	}
}

// Unlock releases the lock.
func (l *Lock) Unlock() error { return l.fl.Unlock() }
