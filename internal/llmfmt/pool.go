// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmfmt

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// NetworkJobs is the default -j of the functions that wait on the network
// (plan R6: CPUs locally, 8 for network).
const NetworkJobs = 8

// LocalJobs is the default -j of the functions bound by local CPU or disk.
func LocalJobs() int { return runtime.NumCPU() }

// Map runs fn on every item with at most jobs at a time and returns the
// results in input order (plan R4: parallel output is the -j 1 output).
// With timeout > 0 each item gets its own deadline: fn receives a context
// that ends after timeout, and is expected to turn that into a field of its
// result (plan S6: one target's timeout is not a failure of the call). The
// call's own context ends every item.
func Map[T, R any](ctx context.Context, jobs int, timeout time.Duration, items []T, fn func(context.Context, T) R) []R {
	out := make([]R, len(items))
	if len(items) == 0 {
		return out
	}
	if ctx == nil {
		ctx = context.Background()
	}
	jobs = max(1, min(jobs, len(items)))
	one := func(i int) {
		ictx, cancel := ctx, context.CancelFunc(func() {})
		if timeout > 0 {
			ictx, cancel = context.WithTimeout(ctx, timeout)
		}
		out[i] = fn(ictx, items[i])
		cancel()
	}
	if jobs == 1 {
		for i := range items {
			one(i)
		}
		return out
	}
	next := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < jobs; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				one(i)
			}
		}()
	}
	for i := range items {
		next <- i
	}
	close(next)
	wg.Wait()
	return out
}

// Ms is a duration in milliseconds with one decimal, the unit of every
// "ms" field.
func Ms(d time.Duration) float64 {
	return float64(d.Round(100*time.Microsecond).Microseconds()/100) / 10
}

// Rel returns p relative to base (plan S3): "." for base itself, the path
// below base otherwise. A p outside base is returned unchanged. Both are
// cleaned first; base "" or "." means p is already relative to the
// working directory.
func Rel(base, p string) string {
	if base == "" || base == "." {
		return filepath.Clean(p)
	}
	base, p = filepath.Clean(base), filepath.Clean(p)
	if p == base {
		return "."
	}
	if base == "/" {
		return strings.TrimPrefix(p, "/")
	}
	if strings.HasPrefix(p, base+"/") {
		return p[len(base)+1:]
	}
	return p
}
