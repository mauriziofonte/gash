// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/jsonpath"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/proctab"
	"github.com/mauriziofonte/gash/internal/runner"
)

// waitLineWidth is the default cut of the match and tail lines of
// llm_wait (and of the log lines of llm_service and llm_docker_logs):
// what agents cut by hand with `| cut -c1-300` (plan S2).
const waitLineWidth = 300

// The default poll intervals of llm_wait: a stat (file, path) is cheap;
// a process scan, a connection and a command cost more, and none of them
// may use more than 1% of a CPU while waiting (plan P1).
const (
	waitStatInterval = 200 * time.Millisecond
	waitPIDInterval  = 200 * time.Millisecond
	waitScanInterval = 500 * time.Millisecond
	waitNetInterval  = 500 * time.Millisecond
	waitRunInterval  = time.Second
	// waitMinInterval is the shortest --interval taken.
	waitMinInterval = 10 * time.Millisecond
	// waitPSInterval is the shortest poll that starts ps (no /proc).
	waitPSInterval = time.Second
	// waitMinLook is the least time a connection or a command run gets.
	waitMinLook = 500 * time.Millisecond
)

// waker sleeps between two looks at the condition; the inotify one wakes
// early when a watched path changes.
type waker interface {
	watchFile(path string)
	watchDir(dir string)
	wait(ctx context.Context, d time.Duration)
	close()
}

// sleepWaker is the waker without inotify.
type sleepWaker struct{}

func (sleepWaker) watchFile(string) {}
func (sleepWaker) watchDir(string)  {}
func (sleepWaker) close()           {}
func (sleepWaker) wait(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// waitCond is the one condition of an llm_wait call.
type waitCond struct {
	kind string // file, exists, gone, http, port, cmd

	file, re string // --file-match FILE RE
	path     string // --exists PATH
	gone     string // --gone PID|NAME

	url       string // --http URL
	code      int    // --code N (0: any 2xx)
	jsonPath  string // --json-path P
	equals    string // --equals V
	hasEquals bool

	addr string // --port HOST:PORT
	cmd  string // --cmd-ok CMD
}

// waitOut is the answer of llm_wait: met or not, after how long, and what
// the condition saw.
type waitOut struct {
	Met    bool     `json:"met"`
	AfterS float64  `json:"after_s"`
	Match  *string  `json:"match,omitempty"`
	Code   int      `json:"code,omitempty"`
	RC     *int     `json:"rc,omitempty"`
	Runs   int      `json:"runs,omitempty"`
	Last   string   `json:"last,omitempty"`
	Tail   []string `json:"tail,omitempty"`
}

// waitCall is one llm_wait run: the parsed options and the state the
// condition keeps between two looks.
type waitCall struct {
	m        *Module
	c        *invoke.Call
	cond     waitCond
	timeout  time.Duration
	interval time.Duration
	tail     int
	width    int
	start    time.Time
}

// wait is llm_wait (plan P1): one call instead of a sleep-and-check loop.
// The condition is looked at once at once and then at every --interval
// (the file and path conditions also wake on inotify events on Linux);
// met prints {"met":true,"after_s":S,...} and exits 0; the timeout prints
// {"met":false,"after_s":S} and exits 1, with no error envelope (waiting
// longer is the agent's choice, not an error).
func (m *Module) wait(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	w := &waitCall{m: m, c: c, width: waitLineWidth}
	o := llmfmt.New(llmfmt.BudgetFlags)
	var timeout, interval string
	widthSet := false
	args := c.Args
	need := func(n int) bool {
		if len(args) <= n {
			fail(c, "missing_value", args[0], args[0]+" needs "+map[bool]string{true: "two values", false: "a value"}[n == 2])
			return false
		}
		return true
	}
	setCond := func(kind string) bool {
		if w.cond.kind != "" {
			fail(c, "unexpected_argument", args[0], "Wait for one condition per call")
			return false
		}
		w.cond.kind = kind
		return true
	}
	for len(args) > 0 {
		a := args[0]
		switch a {
		case "--width":
			n, ok := o.Take(c, args)
			if !ok {
				return 1
			}
			w.width, widthSet = o.Width, true
			args = args[n:]
			continue
		case "--timeout", "--interval", "--tail", "--exists", "--gone", "--http", "--code", "--json-path", "--equals", "--port", "--cmd-ok":
			if !need(1) {
				return 1
			}
		case "--file-match":
			if !need(2) {
				return 1
			}
		default:
			if strings.HasPrefix(a, "-") {
				return fail(c, "unknown_option", a, "Use --timeout S with one of --file-match FILE RE, --exists PATH, --gone PID|NAME, --http URL, --port HOST:PORT, --cmd-ok CMD")
			}
			return fail(c, "unexpected_argument", a, "Name the condition with its option, e.g. --exists PATH")
		}
		v := args[1]
		step := 2
		switch a {
		case "--timeout":
			timeout = v
		case "--interval":
			interval = v
		case "--tail":
			if !validUint(c, "tail", v) {
				return 1
			}
			w.tail = atoi(v)
		case "--file-match":
			if !setCond("file") {
				return 1
			}
			w.cond.file, w.cond.re = v, args[2]
			step = 3
		case "--exists":
			if !setCond("exists") {
				return 1
			}
			w.cond.path = v
		case "--gone":
			if !setCond("gone") {
				return 1
			}
			w.cond.gone = v
		case "--http":
			if !setCond("http") {
				return 1
			}
			w.cond.url = v
		case "--code":
			if !validUint(c, "code", v) {
				return 1
			}
			if n := atoi(v); n < 100 || n > 599 {
				return fail(c, "invalid_code", "Value '"+v+"' is not an HTTP status", "Give a status from 100 to 599, e.g. --code 200")
			}
			w.cond.code = atoi(v)
		case "--json-path":
			w.cond.jsonPath = v
		case "--equals":
			w.cond.equals, w.cond.hasEquals = v, true
		case "--port":
			if !setCond("port") {
				return 1
			}
			w.cond.addr = v
		case "--cmd-ok":
			if !setCond("cmd") {
				return 1
			}
			w.cond.cmd = v
		}
		args = args[step:]
	}
	_ = widthSet
	if timeout == "" {
		return fail(c, "missing_value", "--timeout", "llm_wait needs --timeout S (seconds, or 500ms, 2m; at most 1h)")
	}
	d, err := llmfmt.ParseSeconds(timeout)
	if err != nil {
		return fail(c, "invalid_timeout", timeout, "Give seconds (2, 0.5) or a duration (500ms, 2s, 1m), at most 1h")
	}
	w.timeout = d
	if w.cond.kind == "" {
		return fail(c, "missing_condition", "", "Give one of --file-match FILE RE, --exists PATH, --gone PID|NAME, --http URL, --port HOST:PORT, --cmd-ok CMD")
	}
	if w.cond.kind != "http" {
		for _, f := range []struct {
			set  bool
			name string
		}{{w.cond.code != 0, "--code"}, {w.cond.jsonPath != "", "--json-path"}, {w.cond.hasEquals, "--equals"}} {
			if f.set {
				return fail(c, "unexpected_argument", f.name, f.name+" goes with --http URL")
			}
		}
	}
	if (w.cond.jsonPath != "") != w.cond.hasEquals {
		return fail(c, "unexpected_argument", map[bool]string{true: "--json-path", false: "--equals"}[w.cond.jsonPath != ""],
			"--json-path P and --equals V go together")
	}
	w.interval = w.defaultInterval()
	if interval != "" {
		iv, err := llmfmt.ParseSeconds(interval)
		if err != nil {
			return fail(c, "invalid_interval", interval, "Give seconds (0.2, 1) or a duration (200ms, 2s), at most 1h")
		}
		w.interval = max(iv, waitMinInterval)
	}
	w.start = time.Now()
	switch w.cond.kind {
	case "file":
		return w.waitFile()
	case "exists":
		return w.waitExists()
	case "gone":
		return w.waitGone()
	case "http":
		return w.waitHTTP()
	case "port":
		return w.waitPort()
	default:
		return w.waitCmd()
	}
}

func (w *waitCall) defaultInterval() time.Duration {
	switch w.cond.kind {
	case "file", "exists":
		return waitStatInterval
	case "gone":
		if digitsRE.MatchString(w.cond.gone) {
			return waitPIDInterval
		}
		return waitScanInterval
	case "http", "port":
		return waitNetInterval
	}
	return waitRunInterval
}

// loop looks at the condition now and then after every interval (or
// inotify event) until it holds, the timeout passes or the call ends. A
// look gets the call's context and the time left (at least
// waitMinLook, so the last look at the deadline still runs).
func (w *waitCall) loop(wk waker, interval time.Duration, check func(ctx context.Context, left time.Duration) bool) bool {
	deadline := w.start.Add(w.timeout)
	ctx := w.ctx()
	for {
		if check(ctx, max(time.Until(deadline), waitMinLook)) {
			return true
		}
		left := time.Until(deadline)
		if left <= 0 || ctx.Err() != nil {
			return false
		}
		wk.wait(ctx, min(interval, left))
	}
}

func (w *waitCall) ctx() context.Context {
	if w.c.Ctx != nil {
		return w.c.Ctx
	}
	return context.Background()
}

// done prints the answer: exit 0 when met, 1 on the timeout.
func (w *waitCall) done(out waitOut) int {
	out.AfterS = math.Round(time.Since(w.start).Seconds()*100) / 100
	_ = llmfmt.JSONLine(w.c.Stdout, out)
	if out.Met {
		return 0
	}
	return 1
}

// cut cuts a line to the call's width.
func (w *waitCall) cut(s string) string { return llmfmt.Cut(s, w.width) }

// waitFile is --file-match FILE RE: the file is scanned once, then only
// what is appended (a truncated or replaced file is scanned again from
// its start); a file that does not exist yet is waited for.
func (w *waitCall) waitFile() int {
	re, err := regexp.Compile(w.cond.re)
	if err != nil {
		return fail(w.c, "invalid_regex", w.cond.re, "RE is a regular expression (RE2 syntax), matched against each line")
	}
	safe, ok := w.m.validatePath(w.c, w.cond.file)
	if !ok {
		return 1
	}
	if w.m.guard(w.c).IsSecretFile(safe) {
		return fail(w.c, "secret_file_blocked", safe, "This file contains secrets and cannot be read")
	}
	if fi, err := os.Stat(safe); err == nil && fi.IsDir() {
		return fail(w.c, "unexpected_argument", w.cond.file, "--file-match needs a file, not a directory")
	}
	s := newLineScan(safe, re)
	wk := newWaker()
	defer wk.close()
	wk.watchDir(filepath.Dir(safe))
	var match string
	deadline := w.start.Add(w.timeout)
	stop := func() bool { return time.Now().After(deadline) || w.ctx().Err() != nil }
	met := w.loop(wk, w.interval, func(context.Context, time.Duration) bool {
		line, found, changed := s.poll(stop)
		if changed {
			wk.watchFile(safe)
		}
		match = line
		return found
	})
	out := waitOut{Met: met}
	if met {
		m := w.cut(match)
		out.Match = &m
	}
	if w.tail > 0 {
		out.Tail = w.cutAll(tailFile(safe, w.tail))
	}
	return w.done(out)
}

func (w *waitCall) cutAll(lines []string) []string {
	for i, l := range lines {
		lines[i] = w.cut(l)
	}
	return lines
}

// waitExists is --exists PATH: met when the path exists (test -e).
func (w *waitCall) waitExists() int {
	safe, ok := w.m.validatePath(w.c, w.cond.path)
	if !ok {
		return 1
	}
	wk := newWaker()
	defer wk.close()
	wk.watchDir(filepath.Dir(safe))
	met := w.loop(wk, w.interval, func(context.Context, time.Duration) bool {
		_, err := os.Stat(safe)
		return err == nil
	})
	return w.done(waitOut{Met: met})
}

// waitGone is --gone PID|NAME: met when the process no longer runs (a
// zombie counts as gone), or when no process but the caller's own chain
// has a command line NAME matches (a regular expression, as llm_procs
// --name). Nothing is signaled: on Linux it reads /proc, elsewhere it
// runs ps, at most once a second. Without either the missing ps is
// reported (3.4 before: --gone PID was never met).
func (w *waitCall) waitGone() int {
	interval := w.interval
	proc := w.m.proc
	if proc == "" {
		interval = max(interval, waitPSInterval)
	}
	if digitsRE.MatchString(w.cond.gone) {
		if len(w.cond.gone) > 9 {
			return fail(w.c, "invalid_pid", "Value '"+w.cond.gone+"' is not a process id", "Pass a process id as a plain integer, or a name")
		}
		pid := atoi(w.cond.gone)
		ps := ""
		if proc == "" {
			var ok bool
			if ps, ok = w.m.locate(w.c, "ps"); !ok {
				return prereq.Fail(w.c, "ps", "llm_wait --gone", true)
			}
		}
		met := w.loop(sleepWaker{}, interval, func(context.Context, time.Duration) bool { return w.pidGone(pid, proc, ps) })
		return w.done(waitOut{Met: met})
	}
	if w.cond.gone == "" {
		return fail(w.c, "invalid_pid", "Value '' is not a process id", "Pass a process id, or a name to match against command lines")
	}
	re, err := regexp.Compile(w.cond.gone)
	if err != nil {
		return fail(w.c, "invalid_regex", w.cond.gone, "NAME is a regular expression matched against the full command line")
	}
	t, ok := w.m.procTable(w.c, "llm_wait --gone")
	if !ok {
		return 1
	}
	ps, found := w.m.locate(w.c, "ps")
	self := t.Callers(w.m.pid())
	met := w.loop(sleepWaker{}, interval, func(ctx context.Context, _ time.Duration) bool {
		if proc != "" {
			return !anyProcMatches(proc, re, self)
		}
		if !found {
			return false
		}
		t, err := proctab.ReadPS(ctx, w.m.run, ps, append(w.c.Env.Child(), "LC_ALL=C"))
		if err != nil {
			return false
		}
		for _, p := range t.Match(re.MatchString, self) {
			if p.Alive() {
				return false
			}
		}
		return true
	})
	return w.done(waitOut{Met: met})
}

// pidGone reports that pid no longer runs: one small read of the Linux
// /proc at proc, or ps -p elsewhere.
func (w *waitCall) pidGone(pid int, proc, ps string) bool {
	if proc != "" {
		p, ok := proctab.ReadProc(proc, pid, false)
		return !ok || !p.Alive()
	}
	res, err := w.m.run.Run(w.ctx(), runner.Cmd{Path: ps, Args: []string{"-p", strconv.Itoa(pid), "-o", "stat="},
		Options: runner.Options{Env: append(w.c.Env.Child(), "LC_ALL=C")}})
	if err != nil {
		return false
	}
	st := strings.TrimSpace(string(res.Stdout))
	return st == "" || strings.HasPrefix(st, "Z")
}

// anyProcMatches scans the Linux /proc at proc for a live process,
// outside skip, whose command line matches re: one read of cmdline per
// process, and of stat only for a match.
func anyProcMatches(proc string, re *regexp.Regexp, skip map[int]bool) bool {
	ents, err := os.ReadDir(proc)
	if err != nil {
		return true
	}
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || skip[pid] {
			continue
		}
		cmd := proctab.ReadCmdline(proc, pid)
		if cmd == "" || !re.MatchString(cmd) {
			continue
		}
		if p, ok := proctab.ReadProc(proc, pid, false); ok && p.Alive() {
			return true
		}
	}
	return false
}

// waitHTTP is --http URL: a GET at every interval until the status is
// --code N (any 2xx without it) and, with --json-path P --equals V, the
// JSON body holds V at P.
func (w *waitCall) waitHTTP() int {
	u, err := url.Parse(w.cond.url)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fail(w.c, "invalid_url", w.cond.url, "Give an http:// or https:// URL")
	}
	var steps []jsonpath.Step
	if w.cond.jsonPath != "" {
		if steps, err = jsonpath.Parse(w.cond.jsonPath); err != nil {
			return fail(w.c, "invalid_json_path", w.cond.jsonPath, `Use dotted keys and [N] indexes, e.g. data.items[0].status or ["a.b"]`)
		}
	}
	client := &http.Client{Transport: &http.Transport{Proxy: proxyFor(w.c.Env)}}
	var out waitOut
	met := w.loop(sleepWaker{}, w.interval, func(ctx context.Context, left time.Duration) bool {
		rctx, cancel := context.WithTimeout(ctx, min(left, 10*time.Second))
		defer cancel()
		req, err := http.NewRequestWithContext(rctx, http.MethodGet, w.cond.url, nil)
		if err != nil {
			out.Last = err.Error()
			return false
		}
		req.Header.Set("User-Agent", "gash-llm_wait")
		resp, err := client.Do(req)
		if err != nil {
			out.Code, out.Last = 0, netReason(err)
			return false
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		out.Code, out.Last = resp.StatusCode, ""
		if w.cond.code != 0 && resp.StatusCode != w.cond.code || w.cond.code == 0 && (resp.StatusCode < 200 || resp.StatusCode > 299) {
			out.Last = "HTTP " + strconv.Itoa(resp.StatusCode)
			return false
		}
		if steps == nil && w.cond.jsonPath == "" {
			return true
		}
		var v any
		if err := json.Unmarshal(body, &v); err != nil {
			out.Last = "body is not JSON"
			return false
		}
		got, _, ok := jsonpath.Get(v, steps)
		if !ok {
			out.Last = w.cond.jsonPath + " not found"
			return false
		}
		text := jsonText(got)
		if text != w.cond.equals {
			out.Last = w.cond.jsonPath + " = " + llmfmt.Cut(text, 80)
			return false
		}
		return true
	})
	out.Met = met
	if met {
		out.Last = ""
	}
	return w.done(out)
}

// jsonText is a decoded JSON value as --equals compares it: a string as
// is, null, true and false as written, a number in its shortest form,
// anything else as compact JSON.
func jsonText(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// netReason is a short reason for a failed connection.
func netReason(err error) string {
	var op *net.OpError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.As(err, &op) && op.Err != nil:
		s := op.Err.Error()
		if i := strings.LastIndex(s, ": "); i >= 0 {
			s = s[i+2:]
		}
		return s
	}
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i >= 0 {
		s = s[i+2:]
	}
	return s
}

// proxyFor is the proxy of a request from the caller's environment
// (HTTPS_PROXY, HTTP_PROXY, NO_PROXY and their lower-case forms); never
// for a loopback host, as curl and Go do.
func proxyFor(env invoke.Env) func(*http.Request) (*url.URL, error) {
	get := func(names ...string) string {
		for _, n := range names {
			if v := env.Get(n); v != "" {
				return v
			}
		}
		return ""
	}
	https, plain := get("HTTPS_PROXY", "https_proxy"), get("HTTP_PROXY", "http_proxy")
	noProxy := get("NO_PROXY", "no_proxy")
	return func(r *http.Request) (*url.URL, error) {
		host := r.URL.Hostname()
		if host == "localhost" || strings.HasSuffix(host, ".localhost") {
			return nil, nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return nil, nil
		}
		for _, np := range strings.Split(noProxy, ",") {
			np = strings.TrimSpace(np)
			if np == "*" {
				return nil, nil
			}
			np = strings.TrimPrefix(np, ".")
			if np != "" && (host == np || strings.HasSuffix(host, "."+np)) {
				return nil, nil
			}
		}
		p := plain
		if r.URL.Scheme == "https" && https != "" {
			p = https
		}
		if p == "" {
			return nil, nil
		}
		if !strings.Contains(p, "://") {
			p = "http://" + p
		}
		return url.Parse(p)
	}
}

// waitPort is --port HOST:PORT: met when a TCP connection opens.
func (w *waitCall) waitPort() int {
	host, port, err := net.SplitHostPort(w.cond.addr)
	if err != nil || host == "" {
		return fail(w.c, "invalid_port", w.cond.addr, "Give HOST:PORT, e.g. 127.0.0.1:8080 or [::1]:5432")
	}
	if !uintRE.MatchString(port) || atoi(port) < 1 || atoi(port) > 65535 {
		return fail(w.c, "invalid_port", w.cond.addr, "Provide a port number from 1 to 65535")
	}
	var out waitOut
	met := w.loop(sleepWaker{}, w.interval, func(ctx context.Context, left time.Duration) bool {
		dctx, cancel := context.WithTimeout(ctx, min(left, 2*time.Second))
		defer cancel()
		var d net.Dialer
		conn, err := d.DialContext(dctx, "tcp", w.cond.addr)
		if err != nil {
			out.Last = netReason(err)
			return false
		}
		_ = conn.Close()
		return true
	})
	out.Met = met
	if met {
		out.Last = ""
	}
	return w.done(out)
}

// waitCmd is --cmd-ok CMD: the command line runs under the llm_exec rules
// (checked once, before the first run) until it exits 0; the last run's
// exit status and, with --tail N, its last output lines are reported.
func (w *waitCall) waitCmd() int {
	// Every run is a bash script: without bash 3.4 before ran it to the
	// timeout with rc 127.
	if _, ok := w.m.findBash(w.c); !ok {
		return prereq.Fail(w.c, "bash", "llm_wait --cmd-ok", true)
	}
	plan, pr, err := w.m.execPlan(w.c, w.cond.cmd)
	if err != nil {
		return pr.refusal(err)
	}
	var out waitOut
	var last []byte
	met := w.loop(sleepWaker{}, w.interval, func(ctx context.Context, left time.Duration) bool {
		var buf bytes.Buffer
		res, err := w.m.run.RunBash(ctx, runner.Bash{
			Use: runner.BashLLMExec, Wrap: plan.Wrap, Bash: plan.Bash, Script: plan.Script,
			Options: runner.Options{Env: plan.Env, Stdout: &buf, Stderr: &buf, Timeout: left, KillAfter: time.Second},
		})
		out.Runs++
		rc := res.ExitCode
		if err != nil && rc == 0 {
			rc = 1
		}
		out.RC = &rc
		last = buf.Bytes()
		return rc == 0 && err == nil
	})
	out.Met = met
	if w.tail > 0 {
		ls := textLines(last)
		if len(ls) > w.tail {
			ls = ls[len(ls)-w.tail:]
		}
		out.Tail = w.cutAll(ls)
	}
	return w.done(out)
}

// textLines are the lines of b without their newlines (and a final
// carriage return), as valid UTF-8.
func textLines(b []byte) []string {
	s := strings.TrimSuffix(string(b), "\n")
	if s == "" {
		return nil
	}
	ls := strings.Split(s, "\n")
	for i, l := range ls {
		ls[i] = strings.ToValidUTF8(strings.TrimSuffix(l, "\r"), "\uFFFD")
	}
	return ls
}
