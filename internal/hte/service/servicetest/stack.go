// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package servicetest plays the Apache and PHP-FPM masters of a userland
// stack for tests: it answers their commands as a runner.Fake responder,
// is the process table supervision reads (service.Procs), and writes PID
// files and log lines the way the real masters do (verified in the Ubuntu
// 24.04 container).
package servicetest

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mauriziofonte/gash/internal/hte/render"
	"github.com/mauriziofonte/gash/internal/runner"
)

// Stack is the fake masters.
type Stack struct {
	t      testing.TB
	mu     sync.Mutex
	procs  map[int]string // pid -> command line
	next   int
	events []string
	// FailTest makes the config test of the named master ("apache" or
	// "fpm") fail.
	FailTest string
	// FailUSR1 makes Apache exit during a graceful restart (a busy port).
	FailUSR1 bool
	// Busy is a Listen address another process holds, Unopenable a log
	// file the user cannot open: an Apache whose configuration (httpd.conf
	// and the files it includes, read as Apache reads them) listens on
	// Busy, or logs to Unopenable, passes the config test, fails to start
	// and exits during a graceful restart, as Apache 2.4.58 does.
	Busy, Unopenable string
	// Deaf masters ignore SIGTERM.
	Deaf bool
}

// New returns a Stack with no process.
func New(t testing.TB) *Stack {
	return &Stack{t: t, procs: map[int]string{}, next: 100}
}

func (f *Stack) event(format string, args ...any) {
	f.events = append(f.events, fmt.Sprintf(format, args...))
}

// Events returns what happened since the last call: "test apache",
// "start fpm 8.3", "graceful apache", "reload fpm 8.3", "stop apache", ...
func (f *Stack) Events() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := slices.Clone(f.events)
	f.events = nil
	return out
}

// AddProcess puts a foreign process into the table.
func (f *Stack) AddProcess(pid int, cmdline string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.procs[pid] = cmdline
}

// HasProcess reports a process of the table.
func (f *Stack) HasProcess(pid int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.procs[pid]
	return ok
}

// Running reports how many masters run.
func (f *Stack) Running() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.procs {
		if strings.Contains(c, "httpd.conf") || strings.HasPrefix(c, "php-fpm: master") {
			n++
		}
	}
	return n
}

func appendLog(t testing.TB, path, line string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	fh, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(fh, line)
	fh.Close()
}

func (f *Stack) manifestOf(conf string) *render.Manifest {
	dir := filepath.Dir(conf)
	for !strings.HasSuffix(dir, "/current") && !strings.Contains(filepath.Base(dir), "gen-") {
		dir = filepath.Dir(dir)
	}
	m, err := render.ReadManifest(dir)
	if err != nil {
		f.t.Fatal(err)
	}
	return m
}

func (f *Stack) spawn(cmdline, pidFile string) int {
	f.next++
	f.procs[f.next] = cmdline
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(f.next)+"\n"), 0o600); err != nil {
		f.t.Fatal(err)
	}
	return f.next
}

// Master reports an Apache or PHP-FPM binary (apache2, httpd, php-fpm*).
func Master(bin string) bool {
	b := filepath.Base(bin)
	return b == "apache2" || b == "httpd" || strings.HasPrefix(b, "php-fpm")
}

// Answer answers the config tests and starts of the masters, as a
// runner.Fake responder; it declines any other command, and the probes of
// discovery (-v, -i, -l).
func (f *Stack) Answer(call runner.Call) (runner.Expect, bool) {
	if !Master(call.Argv[0]) || len(call.Argv) < 2 || slices.Contains([]string{"-v", "-i", "-l", "-M"}, call.Argv[1]) {
		return runner.Expect{}, false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	path, args, env := call.Argv[0], call.Argv[1:], call.Env
	if !call.Setsid {
		f.t.Errorf("%q without a new session", call.Argv)
	}
	b := filepath.Base(path)
	apache := b == "apache2" || b == "httpd"
	switch {
	case args[0] == "-t":
		name := "fpm"
		if apache {
			name = "apache"
		}
		f.event("test %s", name)
		if f.FailTest == name {
			return runner.Expect{Status: 1, Stderr: "AH00526: Syntax error on line 3"}, true
		}
		if apache {
			for _, d := range ReadConf(args[1]) {
				if d.Name == "Listen" && d.VirtualHost {
					return runner.Expect{Status: 1, Stderr: "AH00526: Syntax error on line " + strconv.Itoa(d.Line) + " of " + d.File +
						":\nListen cannot occur within <VirtualHost> section"}, true
				}
			}
		}
		if !apache && !slices.ContainsFunc(env, func(e string) bool { return strings.HasPrefix(e, "PHP_INI_SCAN_DIR=") }) {
			f.t.Errorf("fpm -t without PHP_INI_SCAN_DIR: %v", env)
		}
		return runner.Expect{}, true
	case apache:
		m := f.manifestOf(args[1])
		f.event("start apache")
		if lines := f.fails(args[1]); lines != nil {
			for _, l := range lines {
				appendLog(f.t, m.Apache.ErrorLog, l)
			}
			return runner.Expect{Status: 1, Stderr: strings.Join(lines, "\n") + "\n"}, true
		}
		f.spawn(path+" "+strings.Join(args, " "), m.Apache.PidFile)
		appendLog(f.t, m.Apache.ErrorLog, "AH00489: Apache/2.4.58 configured -- resuming normal operations")
		return runner.Expect{}, true
	default:
		m := f.manifestOf(args[1])
		v := filepath.Base(filepath.Dir(args[1]))
		fm, _ := m.Master(v)
		f.event("start fpm %s", v)
		if !slices.Contains(env, "PHP_INI_SCAN_DIR="+fm.ScanDir) {
			f.t.Errorf("fpm start without PHP_INI_SCAN_DIR: %v", env)
		}
		f.spawn("php-fpm: master process ("+args[1]+")", fm.PidFile)
		appendLog(f.t, fm.ErrorLog, "NOTICE: ready to handle connections")
		return runner.Expect{}, true
	}
}

// fails returns the log lines of an Apache that cannot start on conf
// (Busy, Unopenable), or nil.
func (f *Stack) fails(conf string) []string {
	for _, d := range ReadConf(conf) {
		switch {
		case f.Busy != "" && d.Name == "Listen" && !d.VirtualHost && d.Arg == f.Busy:
			return []string{"(98)Address already in use: AH00072: make_sock: could not bind to address " + f.Busy,
				"no listening sockets available, shutting down", "AH00015: Unable to open logs"}
		case f.Unopenable != "" && (d.Name == "CustomLog" || d.Name == "ErrorLog") && d.Arg == f.Unopenable:
			return []string{"(13)Permission denied: AH00649: could not open transfer log file " + f.Unopenable + ".",
				"AH00015: Unable to open logs"}
		}
	}
	return nil
}

// Directive is one directive line of a configuration tree.
type Directive struct {
	File        string
	Line        int
	Name        string // the first word
	Arg         string // the first argument, unquoted
	VirtualHost bool   // inside a <VirtualHost> section
}

// ReadConf returns the directives of conf and of the files its Include
// and IncludeOptional lines name (a missing file is skipped, as
// IncludeOptional does), in the order Apache reads them; a symlink in a
// path is resolved when the file is opened.
func ReadConf(conf string) []Directive {
	var out []Directive
	seen := map[string]bool{}
	var walk func(path string, vhost bool)
	walk = func(path string, vhost bool) {
		if seen[path] {
			return
		}
		seen[path] = true
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		for i, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			name, rest := line, ""
			if i := strings.IndexAny(line, " \t"); i >= 0 {
				name, rest = line[:i], line[i+1:]
			}
			switch {
			case name == "" || strings.HasPrefix(name, "#"):
				continue
			case strings.EqualFold(name, "<VirtualHost"):
				vhost = true
				continue
			case strings.EqualFold(name, "</VirtualHost>"):
				vhost = false
				continue
			}
			d := Directive{File: path, Line: i + 1, Name: name, Arg: firstArg(strings.TrimSpace(rest)), VirtualHost: vhost}
			out = append(out, d)
			if name == "Include" || name == "IncludeOptional" {
				walk(d.Arg, vhost)
			}
		}
	}
	walk(conf, false)
	return out
}

// firstArg is the first argument of a directive's arguments: a quoted
// one with \" and \\ unescaped (ap_getword_conf), or the first word.
func firstArg(rest string) string {
	if !strings.HasPrefix(rest, `"`) {
		if words := strings.Fields(rest); len(words) > 0 {
			return words[0]
		}
		return ""
	}
	var b strings.Builder
	for i := 1; i < len(rest); i++ {
		switch c := rest[i]; {
		case c == '\\' && i+1 < len(rest) && (rest[i+1] == '"' || rest[i+1] == '\\'):
			b.WriteByte(rest[i+1])
			i++
		case c == '"':
			return b.String()
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// Cmdline implements service.Procs.
func (f *Stack) Cmdline(pid int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.procs[pid]
	if !ok {
		return "", os.ErrNotExist
	}
	return c, nil
}

// Started implements service.Procs.
func (f *Stack) Started(int) (time.Time, error) { return time.Now().Add(-time.Minute), nil }

// Signal implements service.Procs: SIGUSR1 is Apache's graceful restart,
// SIGUSR2 FPM's re-exec (a new PID), SIGTERM and SIGKILL end a master.
func (f *Stack) Signal(pid int, sig syscall.Signal) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cmd, ok := f.procs[pid]
	if !ok {
		return syscall.ESRCH
	}
	apache := !strings.HasPrefix(cmd, "php-fpm")
	name := "apache"
	conf := ""
	if apache {
		conf = strings.Fields(cmd)[2]
	} else {
		conf = strings.TrimSuffix(strings.TrimPrefix(cmd, "php-fpm: master process ("), ")")
		name = "fpm " + filepath.Base(filepath.Dir(conf))
	}
	m := f.manifestOf(conf)
	switch sig {
	case syscall.SIGUSR1:
		f.event("graceful apache")
		lines := f.fails(conf)
		if f.FailUSR1 {
			lines = []string{"(98)Address already in use: AH00072: make_sock: could not bind to address 127.0.0.1:3000",
				"no listening sockets available, shutting down"}
		}
		if lines != nil {
			delete(f.procs, pid)
			for _, l := range lines {
				appendLog(f.t, m.Apache.ErrorLog, l)
			}
			return nil
		}
		appendLog(f.t, m.Apache.ErrorLog, "AH00489: resuming normal operations")
	case syscall.SIGUSR2:
		f.event("reload %s", name)
		fm, _ := m.Master(filepath.Base(filepath.Dir(conf)))
		delete(f.procs, pid)
		f.spawn(cmd, fm.PidFile) // FPM re-execs and daemonizes: a new PID
		appendLog(f.t, fm.ErrorLog, "NOTICE: ready to handle connections")
	case syscall.SIGTERM:
		f.event("stop %s", name)
		if f.Deaf {
			return nil
		}
		delete(f.procs, pid)
	case syscall.SIGKILL:
		f.event("kill %s", name)
		delete(f.procs, pid)
	}
	return nil
}
