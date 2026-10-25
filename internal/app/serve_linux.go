// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/mauriziofonte/gash/internal/cli"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/term"
)

// The per-shell server (`gash __serve`): gash.sh starts it as a coprocess
// of an interactive or headless shell and sends it the calls that shell
// makes, so a call costs a round trip on a pipe instead of a fork and an
// exec of the binary. Each call runs as the exec'd binary would run it:
//
//   - the environment the shell would give a program it runs (export -p,
//     with the variables the stub exports for the call);
//   - the shell's own working directory (/proc/PID/cwd) and umask;
//   - the shell's standard streams, opened again through /proc/PID/fd, so
//     a terminal stays a terminal, a pipe the same pipe;
//   - Ctrl-C cancels the call (status 130), a broken pipe on its output
//     ends it (status 141), as they end the exec'd binary.
//
// It runs the functions of registry.Served.
//
// A call it cannot run that way is declined before anything runs ("F"),
// and the stub execs the binary instead: a stream that is a socket or a
// regular file opened without O_APPEND (the offset is the shell's), an
// exported function, a TZ other than the server's own, a function it does
// not run.

// serveProtocol is the version of the request format of gash.sh.
const serveProtocol = "1"

type server struct {
	table  *invoke.Table
	served map[string]bool
	dir    string
	// initial* cache initial for the shell initialPid.
	initialPid    int
	initialPassed []string
	initialFuncs  bool
	tz            string
	tzSet         bool
	under         string // the server's own "_"
	active        atomic.Bool
	mu            sync.Mutex
	cancel        context.CancelFunc
	got           atomic.Int32
	quit          atomic.Int32
	cleanup       func()
	// pidfd is the shell's pidfd (pidfd_getfd), -1 when the kernel or
	// ptrace policy refuses it; held are the shell's standard streams as
	// they were when it started the server (descriptors 3, 4 and 5).
	pidfd    int
	pidfdPid int
	held     []int
	// lastExportText is the last call's export -p text, lastExport what
	// it parsed to.
	lastExportText string
	lastExport     []string
}

func serve(environ []string, in io.Reader, out io.Writer) int {
	held := closeInherited()
	env := invoke.EnvFrom(environ)
	dir, err := os.MkdirTemp(serveBase(env), "gash-serve-")
	if err != nil {
		return 1
	}
	// The stamp tells gash.sh a binary newer than the server.
	if os.WriteFile(filepath.Join(dir, "stamp"), nil, 0o600) != nil {
		_ = os.RemoveAll(dir)
		return 1
	}
	s := &server{table: newTable(), served: map[string]bool{}, under: env.Get("_"), pidfd: -1}
	s.pidfd, s.pidfdPid = pidfdOf(os.Getppid())
	if s.pidfd >= 0 {
		// The shell's streams are taken from it at each call: the copies
		// would only keep, say, a pipe it closes open.
		for _, fd := range held {
			_ = syscall.Close(fd)
		}
	} else {
		s.held = held
	}
	s.tz, s.tzSet = env.Lookup("TZ")
	s.dir = dir
	s.cleanup = func() { _ = os.RemoveAll(dir) }
	defer s.cleanup()
	for _, n := range registry.Served() {
		s.served[n] = true
	}

	// Notify, not Ignore: an ignored signal stays ignored in the programs
	// a call runs. The terminal's signals reach the server (it shares the
	// shell's process group); only a call in progress takes them.
	sigs := make(chan os.Signal, 8)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGHUP,
		syscall.SIGTSTP, syscall.SIGTTIN, syscall.SIGTTOU, syscall.SIGPIPE)
	go s.signals(sigs)

	// A request is the file req in the server's directory. The server
	// wakes when the shell closes it (inotify): the shell closes it as it
	// undoes the redirection it wrote it with, so its standard streams are
	// its own again when the server takes them. The standard input only
	// tells when the shell is gone.
	ifd, err := unix.InotifyInit1(unix.IN_CLOEXEC)
	if err != nil {
		return 1
	}
	if _, err := unix.InotifyAddWatch(ifd, dir, unix.IN_CLOSE_WRITE); err != nil {
		return 1
	}
	go func() {
		_, _ = io.Copy(io.Discard, in)
		s.cleanup()
		os.Exit(0)
	}()
	w := bufio.NewWriter(out)
	fmt.Fprintf(w, "gash-serve %s %s\n", serveProtocol, dir)
	if w.Flush() != nil {
		return 1
	}
	events := make([]byte, 4096)
	for {
		n, err := unix.Read(ifd, events)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return 1
		}
		if !closedReq(events[:n]) {
			continue
		}
		seq, status := "*", "F"
		if req, err := s.readRequest(); err == nil {
			seq, status = req.seq, s.handle(req)
		}
		fmt.Fprintf(w, "%s %s\n", seq, status)
		if w.Flush() != nil {
			return 0
		}
		if n := s.quit.Load(); n != 0 {
			return 128 + int(n)
		}
	}
}

// closedReq tells whether the inotify events in b hold the closing of req.
func closedReq(b []byte) bool {
	for len(b) >= unix.SizeofInotifyEvent {
		// struct inotify_event: wd, mask, cookie, len, then the name.
		end := unix.SizeofInotifyEvent + int(binary.NativeEndian.Uint32(b[12:16]))
		if end > len(b) {
			return false
		}
		name := b[unix.SizeofInotifyEvent:end]
		if i := bytes.IndexByte(name, 0); i >= 0 {
			name = name[:i]
		}
		if string(name) == "req" {
			return true
		}
		b = b[end:]
	}
	return false
}

// closeInherited closes the descriptors above 2 the server got from the
// shell (a coprocess inherits them all): holding one would keep, say, the
// write end of a pipe the shell closes open, and its reader waiting.
//
// Descriptors 3, 4 and 5 are the shell's standard streams, which gash.sh
// hands the server: they are returned, kept open.
func closeInherited() (held []int) {
	ents, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return nil
	}
	for _, e := range ents {
		fd, err := strconv.Atoi(e.Name())
		if err != nil || fd <= 2 {
			continue
		}
		if fd <= 5 {
			held = append(held, fd)
			continue
		}
		link, err := os.Readlink("/proc/self/fd/" + e.Name())
		if err != nil || strings.HasPrefix(link, "anon_inode:") || strings.HasPrefix(link, "/proc/") {
			continue // the runtime's own (epoll, eventfd) and the listing itself
		}
		_ = syscall.Close(fd)
	}
	return held
}

// pidfdOf opens pid's pidfd when pidfd_getfd works on it (Linux 5.6, and
// a ptrace policy that lets a child reach its parent's descriptors).
func pidfdOf(pid int) (int, int) {
	pfd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return -1, 0
	}
	fd, err := unix.PidfdGetfd(pfd, 0, 0)
	if err != nil && !errors.Is(err, unix.EBADF) {
		_ = unix.Close(pfd)
		return -1, 0
	}
	if err == nil {
		_ = unix.Close(fd)
	}
	return pfd, pid
}

// stream gets the shell's descriptor n for a call: the descriptor itself
// (pidfd_getfd: the same open file, offset and all), else one of the
// shell's streams the server holds when it is the same open file (kcmp),
// else the same terminal, pipe, device or append-only file opened again
// through /proc. A socket or a regular file it cannot share is declined.
// pipe reports a pipe, whose reader may go away.
func (s *server) stream(pid, n int, write, reads bool) (*os.File, bool, error) {
	name := "/proc/" + strconv.Itoa(pid) + "/fd/" + strconv.Itoa(n)
	if s.pidfd >= 0 && pid == s.pidfdPid {
		fd, err := unix.PidfdGetfd(s.pidfd, n, 0)
		if err != nil {
			return nil, false, err
		}
		return os.NewFile(uintptr(fd), name), isFifo(fd), nil
	}
	self := os.Getpid()
	for _, h := range s.held {
		if r, _, e := unix.Syscall6(unix.SYS_KCMP, uintptr(self), uintptr(pid), 0 /* KCMP_FILE */, uintptr(h), uintptr(n), 0); e == 0 && r == 0 {
			fd, err := unix.FcntlInt(uintptr(h), unix.F_DUPFD_CLOEXEC, 0)
			if err != nil {
				return nil, false, err
			}
			return os.NewFile(uintptr(fd), name), isFifo(fd), nil
		}
	}
	return openStream(pid, n, write, reads)
}

func isFifo(fd int) bool {
	var st unix.Stat_t
	return unix.Fstat(fd, &st) == nil && st.Mode&unix.S_IFMT == unix.S_IFIFO
}

// serveBase is where the server's private directory goes: a tmpfs when
// there is one (each call writes two small files there).
func serveBase(env invoke.Env) string {
	for _, d := range []string{env.Get("XDG_RUNTIME_DIR"), "/dev/shm", env.Get("TMPDIR")} {
		if filepath.IsAbs(d) {
			if fi, err := os.Stat(d); err == nil && fi.IsDir() {
				return d
			}
		}
	}
	return "/tmp"
}

func (s *server) signals(sigs chan os.Signal) {
	for sig := range sigs {
		n, _ := sig.(syscall.Signal)
		switch n {
		case syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGHUP:
		default:
			continue
		}
		if n == syscall.SIGTERM || n == syscall.SIGHUP {
			s.quit.Store(int32(n))
		}
		s.mu.Lock()
		if s.active.Load() {
			s.got.CompareAndSwap(0, int32(n))
			s.cancel()
			s.mu.Unlock()
			continue
		}
		s.mu.Unlock()
		if s.quit.Load() != 0 {
			s.cleanup()
			os.Exit(128 + int(n))
		}
	}
}

type request struct {
	seq  string
	pid  int
	args []string
	// The variable lists of __gash_export_state for the call: the common
	// ones, the function's, its settings (space-separated names).
	common, vars, settings string
	text                   string
}

// readRequest reads the request file: NUL-terminated fields (R, the
// sequence number, the shell's PID, the argument count, the three variable
// lists, the arguments), then the umask line, the declarations, \x01 and
// what export -p printed.
func (s *server) readRequest() (request, error) {
	var req request
	raw, err := os.ReadFile(filepath.Join(s.dir, "req"))
	if err != nil {
		return req, err
	}
	data := string(raw)
	field := func() (string, error) {
		f, rest, ok := strings.Cut(data, "\x00")
		if !ok {
			return "", errors.New("serve: short request")
		}
		data = rest
		return f, nil
	}
	head := make([]string, 7)
	for i := range head {
		if head[i], err = field(); err != nil {
			return req, err
		}
	}
	if head[0] != "R" {
		return req, errors.New("serve: bad request")
	}
	pid, err1 := strconv.Atoi(head[2])
	nargs, err2 := strconv.Atoi(head[3])
	if err1 != nil || err2 != nil || nargs < 1 {
		return req, errors.New("serve: bad request header")
	}
	req.seq, req.pid = head[1], pid
	req.common, req.vars, req.settings = head[4], head[5], head[6]
	for range nargs {
		f, err := field()
		if err != nil {
			return req, err
		}
		req.args = append(req.args, f)
	}
	req.text = data
	return req, nil
}

// handle runs one call and returns its status, or "F" when it declines.
func (s *server) handle(req request) string {
	name := req.args[0]
	if !s.served[name] {
		return "F"
	}
	environ, mask, ok := s.environ(req)
	if !ok {
		return "F"
	}
	var files []*os.File
	defer func() {
		for _, f := range files {
			_ = f.Close()
		}
	}()
	in, _, err := s.stream(req.pid, 0, false, registry.ServedReadsStdin(name))
	if err != nil {
		return "F"
	}
	files = append(files, in)
	outF, outPipe, err := s.stream(req.pid, 1, true, false)
	if err != nil {
		return "F"
	}
	files = append(files, outF)
	errF, errPipe, err := s.stream(req.pid, 2, true, false)
	if err != nil {
		return "F"
	}
	files = append(files, errF)
	if os.Chdir("/proc/"+strconv.Itoa(req.pid)+"/cwd") != nil {
		return "F"
	}
	syscall.Umask(mask)
	setEnv(environ)

	argv := req.args
	if name == "gash" {
		argv = argv[1:] // the gash card is the binary's root
	}
	return strconv.Itoa(s.run(argv, environ, in, outF, errF, outPipe || errPipe))
}

// setEnv makes the process environment environ (what the standard
// library reads: PATH for a lookup, HOME, TMPDIR), changing only the
// entries that differ: the programs a call runs get their environment
// from the call, never from the process.
func setEnv(environ []string) {
	want := make(map[string]string, len(environ))
	for _, kv := range environ {
		if i := strings.IndexByte(kv, '='); i > 0 {
			want[kv[:i]] = kv[i+1:]
		}
	}
	have, seen := map[string]bool{}, map[string]bool{}
	for _, kv := range os.Environ() {
		name, value, _ := strings.Cut(kv, "=")
		if name == "" || seen[name] {
			continue // a later duplicate is not what Getenv reads
		}
		seen[name] = true
		if v, ok := want[name]; !ok {
			_ = os.Unsetenv(name)
		} else if v == value {
			have[name] = true
		}
	}
	for name, v := range want {
		if !have[name] {
			_ = os.Setenv(name, v)
		}
	}
}

// run is Main for one call of the server.
func (s *server) run(argv, environ []string, in, out, errf *os.File, pipes bool) (status int) {
	env := invoke.EnvFrom(environ)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.mu.Lock()
	s.cancel = cancel
	s.got.Store(0)
	s.active.Store(true)
	s.mu.Unlock()

	g := &gate{cancel: cancel}
	var stdout, stderr io.Writer = out, errf
	if pipes {
		stdout, stderr = &gated{f: out, g: g}, &gated{f: errf, g: g}
	}
	defer func() {
		s.mu.Lock()
		s.active.Store(false)
		s.mu.Unlock()
		if r := recover(); r != nil {
			status = panicked(errf, env, r)
		}
		if n := s.got.Load(); n != 0 {
			status = 128 + int(n)
		}
		if g.broken.Load() {
			status = 128 + int(syscall.SIGPIPE)
		}
	}()
	return cli.Run(cli.Process{
		Ctx: ctx, Args: argv, Env: env,
		Stdin: in, Stdout: stdout, Stderr: stderr,
		Term:  term.New(env, in, out, errf),
		Table: s.table,
	})
}

// environ builds the environment of the call: the shell's exported
// variables (export -p), the names Bash passes through unread (not
// identifiers), what __gash_export_state exports in the exec'd binary's
// subshell (from the declarations of its variables), and "_", the
// program's path. An exported function, whose entry the text does not
// carry, declines the call.
func (s *server) environ(req request) (env []string, umask int, ok bool) {
	// The umask line, the declarations, \x01, export -p.
	maskLine, rest, _ := strings.Cut(req.text, "\n")
	m, err := strconv.ParseUint(maskLine, 8, 32)
	if err != nil {
		return nil, 0, false
	}
	declText, exportText, ok := strings.Cut(rest, "\x01\n")
	if !ok {
		return nil, 0, false
	}
	// A shell's exported variables seldom change between two calls: the
	// text parsed last time is not parsed again.
	if exportText == s.lastExportText && s.lastExport != nil {
		env = slices.Clone(s.lastExport)
	} else {
		if env, err = parseExportP(exportText); err != nil {
			return nil, 0, false
		}
		s.lastExportText, s.lastExport = exportText, slices.Clone(env)
	}
	decls, err := parseDecls(declText)
	if err != nil {
		return nil, 0, false
	}
	passed, funcs := s.initial(req.pid)
	if funcs {
		return nil, 0, false
	}
	env = append(env, passed...)
	set := func(name, kv string) {
		for i, e := range env {
			if strings.HasPrefix(e, name+"=") {
				if kv == "" {
					env = append(env[:i], env[i+1:]...)
				} else {
					env[i] = kv
				}
				return
			}
		}
		if kv != "" {
			env = append(env, kv)
		}
	}
	for _, kv := range exportState(decls, req) {
		if name, ok := strings.CutPrefix(kv, "-"); ok {
			set(name, "")
		} else if i := strings.IndexByte(kv, '='); i > 0 {
			set(kv[:i], kv)
		}
	}
	if s.under != "" {
		set("_", "_="+s.under)
	}
	tz, tzSet := "", false
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, "TZ="); ok {
			tz, tzSet = v, true
		}
	}
	if tz != s.tz || tzSet != s.tzSet {
		return nil, 0, false // time.Local was read from the server's TZ
	}
	return env, int(m), true
}

// initial reads, once per shell, what the shell's own environment (as it
// was exec'd) holds that export -p does not show: the entries Bash passes
// through unread (names that are not identifiers), and whether it holds
// exported functions.
func (s *server) initial(pid int) (passed []string, funcs bool) {
	if s.initialPid == pid {
		return s.initialPassed, s.initialFuncs
	}
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/environ")
	if err != nil {
		return nil, true
	}
	passed, funcs = nil, false
	for _, kv := range strings.Split(string(raw), "\x00") {
		i := strings.IndexByte(kv, '=')
		switch {
		case strings.HasPrefix(kv, "BASH_FUNC_"):
			funcs = true
		case i > 0 && !isIdentifier(kv[:i]):
			passed = append(passed, kv)
		}
	}
	s.initialPid, s.initialPassed, s.initialFuncs = pid, passed, funcs
	return passed, funcs
}

// exportState is __gash_export_state: the "NAME=value" entries it exports
// and the "-NAME" ones it un-exports, in order, from the declarations of
// the variables as the call sees them.
func exportState(decls []decl, req request) []string {
	byName := map[string]decl{}
	for _, d := range decls {
		byName[d.name] = d
	}
	var out []string
	exported := map[string]bool{}
	var added, set []string
	pass := func(names string, setting bool) {
		for _, n := range strings.Fields(names) {
			d, ok := byName[n]
			if !ok || !d.set {
				continue
			}
			if !d.exported() && !exported[n] {
				added = append(added, n)
			}
			exported[n] = true
			if d.scalar() {
				out = append(out, n+"="+d.value)
			}
			if setting {
				set = append(set, n)
			}
		}
	}
	pass(req.common+" "+req.vars, false)
	pass(req.settings, true)
	help := "0"
	if d, ok := byName["__GASH_HELP_LOADED"]; ok && d.set && d.value != "" {
		help = "1"
	}
	return append(out, "GASH__SET="+strings.Join(set, " "), "GASH__ADDED="+strings.Join(added, " "),
		"GASH__HELP="+help, "-GASH_LLM_EXEC", "-GASH_LLM_EXEC_ALLOW", "-GASH_LLM_SANDBOX")
}

func isIdentifier(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return s != ""
}

// openStream opens the shell's descriptor n again: the same terminal,
// pipe or device. pipe reports a pipe (its reader may go away).
func openStream(pid, n int, write, reads bool) (f *os.File, pipe bool, err error) {
	link := "/proc/" + strconv.Itoa(pid) + "/fd/" + strconv.Itoa(n)
	var st syscall.Stat_t
	if err := syscall.Stat(link, &st); err != nil {
		return nil, false, err
	}
	flags, pos, err := fdInfo(pid, n)
	if err != nil {
		return nil, false, err
	}
	mode := st.Mode & syscall.S_IFMT
	switch {
	case mode == syscall.S_IFSOCK || mode == syscall.S_IFDIR:
		return nil, false, errors.New("serve: unsupported stream")
	case mode == syscall.S_IFREG && write && flags&syscall.O_APPEND == 0:
		return nil, false, errors.New("serve: file offset")
	case mode == syscall.S_IFREG && !write && reads:
		return nil, false, errors.New("serve: file offset")
	}
	acc := flags & syscall.O_ACCMODE
	oflag := syscall.O_RDONLY
	if write {
		if acc == syscall.O_RDONLY {
			return nil, false, errors.New("serve: not writable")
		}
		oflag = syscall.O_WRONLY | flags&syscall.O_APPEND
	} else if acc == syscall.O_WRONLY {
		return nil, false, errors.New("serve: not readable")
	}
	// O_NONBLOCK: a pipe without a reader fails (ENXIO) instead of
	// blocking, one without a writer opens; the call then blocks as usual.
	fd, err := syscall.Open(link, oflag|syscall.O_NOCTTY|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.SetNonblock(fd, false); err != nil {
		_ = syscall.Close(fd)
		return nil, false, err
	}
	if mode == syscall.S_IFREG && !write {
		if _, err := syscall.Seek(fd, pos, io.SeekStart); err != nil {
			_ = syscall.Close(fd)
			return nil, false, err
		}
	}
	return os.NewFile(uintptr(fd), link), mode == syscall.S_IFIFO, nil
}

// fdInfo reads the flags and the offset of the shell's descriptor n.
func fdInfo(pid, n int) (flags int, pos int64, err error) {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/fdinfo/" + strconv.Itoa(n))
	if err != nil {
		return 0, 0, err
	}
	for _, line := range strings.Split(string(raw), "\n") {
		k, v, _ := strings.Cut(line, ":")
		v = strings.TrimSpace(v)
		switch k {
		case "flags":
			f, err := strconv.ParseInt(v, 8, 64)
			if err != nil {
				return 0, 0, err
			}
			flags = int(f)
		case "pos":
			if pos, err = strconv.ParseInt(v, 10, 64); err != nil {
				return 0, 0, err
			}
		}
	}
	return flags, pos, nil
}

// gate ends a call whose output pipe lost its reader, as SIGPIPE ends the
// exec'd binary: the call is canceled and writes no more.
type gate struct {
	broken atomic.Bool
	cancel context.CancelFunc
}

// gated is a stream of a call behind its gate; children get the file.
type gated struct {
	f *os.File
	g *gate
}

func (w *gated) Write(p []byte) (int, error) {
	if w.g.broken.Load() {
		return 0, syscall.EPIPE
	}
	n, err := w.f.Write(p)
	if errors.Is(err, syscall.EPIPE) {
		w.g.broken.Store(true)
		w.g.cancel()
	}
	return n, err
}

func (w *gated) File() *os.File { return w.f }
