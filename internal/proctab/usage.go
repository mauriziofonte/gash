// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package proctab

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Usage is a process with the shares ps aux shows: CPU and Mem are tenths
// of a percent, cut as ps prints them (12.3% is 123).
type Usage struct {
	Proc
	CPU, Mem int
}

// clockTicks is USER_HZ, the unit of the times of /proc/PID/stat: 100 on
// every Linux architecture.
const clockTicks = 100

// ReadUsage reads every process of a Linux /proc at root with its shares,
// computed as procps ps computes them: %CPU is the CPU time used over the
// time since the process started, %MEM the resident size over MemTotal.
// Processes come by PID; one that ends while it is read is left out.
func ReadUsage(root string) ([]Usage, error) {
	t, err := ReadProcFS(root)
	if err != nil {
		return nil, err
	}
	uptime := 0.0
	if b, err := os.ReadFile(filepath.Join(root, "uptime")); err == nil {
		if f := strings.Fields(string(b)); len(f) > 0 {
			uptime, _ = strconv.ParseFloat(f[0], 64)
		}
	}
	memKiB := uint64(0)
	if b, err := os.ReadFile(filepath.Join(root, "meminfo")); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if f := strings.Fields(l); len(f) >= 2 && f[0] == "MemTotal:" {
				memKiB, _ = strconv.ParseUint(f[1], 10, 64)
			}
		}
	}
	page := uint64(os.Getpagesize()) //nolint:gosec // a page size is positive
	out := make([]Usage, 0, len(t.Procs()))
	for _, p := range t.Procs() {
		u := Usage{Proc: p}
		if st, ok := statTimes(root, p.PID); ok {
			if secs := uptime - float64(st.start)/clockTicks; secs > 0 {
				u.CPU = int(float64(st.utime+st.stime) * 1000 / clockTicks / secs)
			}
			if memKiB > 0 {
				u.Mem = int(st.rss * page / 1024 * 1000 / memKiB)
			}
		}
		out = append(out, u)
	}
	return out, nil
}

// times are the fields of /proc/PID/stat a ps aux row needs: the user and
// system CPU time and the start time in clock ticks, the resident size in
// pages.
type times struct {
	utime, stime, start, rss uint64
}

// statTimes reads the times of pid; the fields after the command name
// start at the state (field 3 of proc(5)).
func statTimes(root string, pid int) (times, bool) {
	b, err := os.ReadFile(filepath.Join(root, strconv.Itoa(pid), "stat"))
	if err != nil {
		return times{}, false
	}
	s := string(b)
	end := strings.LastIndexByte(s, ')')
	if end < 0 {
		return times{}, false
	}
	f := strings.Fields(s[end+1:])
	if len(f) < 22 {
		return times{}, false
	}
	n := func(i int) uint64 {
		v, _ := strconv.ParseUint(f[i], 10, 64)
		return v
	}
	return times{utime: n(11), stime: n(12), start: n(19), rss: n(21)}, true
}
