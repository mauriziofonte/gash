// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

// The start-up budget of design/architecture.md 1: every 2.x function is
// one exec of the binary, so `gash version` (the cheapest command: the
// whole process start, package initialization and the cobra tree for one
// command) must stay under 10 ms.
//
// The binary is built as the release builds it (CGO_ENABLED=0, -trimpath,
// -s -w) and started from this process, stdout to a pipe. The check takes
// the median of many runs after a warm-up, so one slow start does not
// fail it. Machines differ, so the budget is 10 ms times a factor:
//
//	GASH_VERSION_BUDGET_FACTOR  default 3 (30 ms): `go test ./...` runs
//	                            this next to other packages, under -race,
//	                            on loaded developer machines and macOS
//	                            runners; it catches gross regressions
//	                            (a heavy init(), an SDK linked in).
//	                            CI sets 1 in a step of its own on an idle
//	                            Linux runner: the real 10 ms budget.
//
// BenchmarkVersionStartup reports the same measurement as ns/op.

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/mauriziofonte/gash/internal/compatgen"
)

const versionBudget = 10 * time.Millisecond

// releaseBinary builds ./cmd/gash with the release flags into a
// temporary directory of tb.
func releaseBinary(tb testing.TB) string {
	tb.Helper()
	root, err := compatgen.FindRoot(".")
	if err != nil {
		tb.Skip("not in a Gash checkout:", err)
	}
	bin := filepath.Join(tb.TempDir(), "gash")
	cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags", "-s -w", "-o", bin, "./cmd/gash")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		tb.Fatalf("building gash: %v\n%s", err, out)
	}
	return bin
}

// startVersion runs `bin version` once and returns how long it took.
func startVersion(tb testing.TB, bin string) time.Duration {
	tb.Helper()
	cmd := exec.Command(bin, "version")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.TempDir(), "LANG=C"}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	start := time.Now()
	if err := cmd.Run(); err != nil {
		tb.Fatalf("gash version: %v", err)
	}
	return time.Since(start)
}

func budgetFactor(t *testing.T) float64 {
	t.Helper()
	s := os.Getenv("GASH_VERSION_BUDGET_FACTOR")
	if s == "" {
		return 3
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		t.Fatalf("GASH_VERSION_BUDGET_FACTOR=%q is not a positive number", s)
	}
	return f
}

func TestVersionStartupBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("start-up timing: not in -short runs")
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("gash runs on linux and darwin only")
	}
	factor := budgetFactor(t)
	bin := releaseBinary(t)
	for range 5 { // page cache, dynamic loader caches, macOS first-exec checks
		startVersion(t, bin)
	}
	const runs = 51
	samples := make([]time.Duration, runs)
	for i := range samples {
		samples[i] = startVersion(t, bin)
	}
	slices.Sort(samples)
	median := samples[runs/2]
	limit := time.Duration(float64(versionBudget) * factor)
	t.Logf("gash version: median %v, min %v, max %v over %d runs (limit %v = %v x %g)",
		median, samples[0], samples[runs-1], runs, limit, versionBudget, factor)
	if median > limit {
		t.Fatalf("gash version takes %v (median of %d runs), over the %v start-up budget x %g (architecture.md 1); samples: %v",
			median, runs, versionBudget, factor, samples)
	}
}

// BenchmarkVersionStartup: go test ./cmd/gash -run '^$' -bench VersionStartup
func BenchmarkVersionStartup(b *testing.B) {
	bin := releaseBinary(b)
	startVersion(b, bin)
	b.ResetTimer()
	for b.Loop() {
		startVersion(b, bin)
	}
}
