// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mauriziofonte/gash/internal/hte/proc"
)

// Procs is the view of the process table supervision needs. A PID is
// acted on only when its command line names our configuration path, so a
// recycled PID is never signalled (design/hte.md 5).
type Procs interface {
	// Cmdline is the process's command line, arguments joined by spaces;
	// an error when there is no such process. A zombie has an empty one.
	Cmdline(pid int) (string, error)
	// Started is when the process started.
	Started(pid int) (time.Time, error)
	// Signal sends sig to pid.
	Signal(pid int, sig syscall.Signal) error
}

// LinuxProcs reads /proc.
type LinuxProcs struct {
	Root string // "/proc"; a fixture directory in tests
}

func (p LinuxProcs) root() string {
	if p.Root == "" {
		return "/proc"
	}
	return p.Root
}

// Cmdline reads /proc/<pid>/cmdline. FPM rewrites its title to
// "php-fpm: master process (<config>)", which still names the path.
func (p LinuxProcs) Cmdline(pid int) (string, error) {
	data, err := os.ReadFile(filepath.Join(p.root(), strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bytes.ReplaceAll(data, []byte{0}, []byte{' '}))), nil
}

// clockTicks is USER_HZ, 100 on every Linux architecture Go supports.
const clockTicks = 100

// Started reads the start time from /proc/<pid>/stat (field 22, in clock
// ticks after boot) and the boot time from /proc/stat.
func (p LinuxProcs) Started(pid int) (time.Time, error) {
	stat, err := os.ReadFile(filepath.Join(p.root(), strconv.Itoa(pid), "stat"))
	if err != nil {
		return time.Time{}, err
	}
	// The command name (field 2) is in parentheses and may hold blanks.
	i := bytes.LastIndexByte(stat, ')')
	if i < 0 {
		return time.Time{}, errors.New("unexpected /proc stat format")
	}
	fields := strings.Fields(string(stat[i+1:]))
	if len(fields) < 20 {
		return time.Time{}, errors.New("unexpected /proc stat format")
	}
	ticks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	all, err := os.ReadFile(filepath.Join(p.root(), "stat"))
	if err != nil {
		return time.Time{}, err
	}
	for _, line := range strings.Split(string(all), "\n") {
		if v, ok := strings.CutPrefix(line, "btime "); ok {
			boot, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil {
				return time.Time{}, err
			}
			return time.Unix(boot, 0).Add(time.Duration(ticks) * time.Second / clockTicks), nil
		}
	}
	return time.Time{}, errors.New("no btime in /proc/stat")
}

// Signal sends a signal with kill(2).
func (LinuxProcs) Signal(pid int, sig syscall.Signal) error { return syscall.Kill(pid, sig) }

// DarwinProcs asks ps.
type DarwinProcs struct {
	Runner proc.Runner
	PS     string // /bin/ps
}

func (p DarwinProcs) ps(pid int, field string) (string, error) {
	res, err := p.Runner.Run(context.Background(), proc.Cmd{Path: p.PS, Args: []string{"-o", field + "=", "-p", strconv.Itoa(pid)}, Timeout: 5 * time.Second})
	if err != nil {
		return "", err
	}
	if !res.OK() {
		return "", fmt.Errorf("no process %d", pid)
	}
	return strings.TrimSpace(string(res.Stdout)), nil
}

// Cmdline runs ps -o command= -p PID.
func (p DarwinProcs) Cmdline(pid int) (string, error) { return p.ps(pid, "command") }

// Started runs ps -o etime= -p PID ([[dd-]hh:]mm:ss).
func (p DarwinProcs) Started(pid int) (time.Time, error) {
	out, err := p.ps(pid, "etime")
	if err != nil {
		return time.Time{}, err
	}
	d, err := ParseEtime(out)
	if err != nil {
		return time.Time{}, err
	}
	return time.Now().Add(-d), nil
}

// Signal sends a signal with kill(2).
func (DarwinProcs) Signal(pid int, sig syscall.Signal) error { return syscall.Kill(pid, sig) }

// ParseEtime parses the ps elapsed time [[dd-]hh:]mm:ss.
func ParseEtime(s string) (time.Duration, error) {
	var days int
	if d, rest, ok := strings.Cut(s, "-"); ok {
		n, err := strconv.Atoi(d)
		if err != nil {
			return 0, fmt.Errorf("etime %q", s)
		}
		days, s = n, rest
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("etime %q", s)
	}
	total := 0
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return 0, fmt.Errorf("etime %q", s)
		}
		total = total*60 + n
	}
	return time.Duration(days)*24*time.Hour + time.Duration(total)*time.Second, nil
}
