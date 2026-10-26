// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

// llm_remote_run (plan R1, R2): a command line on registered hosts over
// ssh, the llm_exec rails applied here before anything connects. One ssh
// invocation per host reads its script on stdin (`sh -s`: no argv length
// limit, any login shell); the line runs as `bash --noprofile --norc -c`
// with the llm_exec prelude (sent once per invocation) and its canonical
// rendering. HOST... -- CMD fans out in parallel, one row per host;
// HOST --step CMD... runs the steps in order in the same invocation, one
// row per step.

import (
	"context"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmexec"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/remote"
)

const (
	// remoteRunTimeout bounds each host (R1) or the whole session (R2).
	remoteRunTimeout = 120 * time.Second
	// remoteRunLines are the default head and tail lines of each out and
	// err field when no budget is given.
	remoteRunLines = 100
	// remoteCwd stands for the remote home in the llm_exec checks: a
	// relative path is checked as a path below it.
	remoteCwd = "/home/gash-remote"
)

func (m *Module) remoteRun(c *invoke.Call) int {
	for _, a := range c.Args {
		if a == "--" {
			break
		}
		if a == "--help" || a == "-h" {
			help.NeedsOf(c, "--help")
			return 0
		}
	}
	o := llmfmt.New(llmfmt.FormatFlag | llmfmt.BudgetFlags | llmfmt.JobsFlag | llmfmt.TimeoutFlag)
	var aliases, steps, words []string
	dash, stop := false, false
	for args := c.Args; len(args) > 0; {
		n, ok := o.Take(c, args)
		if !ok {
			return 1
		}
		if n > 0 {
			args = args[n:]
			continue
		}
		a := args[0]
		switch {
		case a == "--":
			dash, words, args = true, args[1:], nil
			continue
		case a == "--step":
			if len(args) < 2 {
				return fail(c, "missing_value", a, a+" needs a value")
			}
			steps = append(steps, args[1])
			args = args[2:]
			continue
		case a == "--stop-on-error":
			stop = true
		case strings.HasPrefix(a, "-"):
			return fail(c, "unknown_option", a, "Run llm_remote_run --help for the options")
		default:
			aliases = append(aliases, a)
		}
		args = args[1:]
	}
	switch {
	case len(aliases) == 0:
		return fail(c, "missing_host", "", "Name one or more registered HOST: aliases before -- CMD")
	case dash && len(steps) > 0:
		return fail(c, "conflicting_options", "-- and --step", "Use either HOST... -- CMD or HOST --step CMD...")
	case !dash && len(steps) == 0:
		return fail(c, "empty_command", "", "Give the command after --, or one --step CMD per step")
	case len(steps) > 0 && len(aliases) > 1:
		return fail(c, "unexpected_argument", aliases[1], "--step runs on one HOST; fan out a single command with HOST... -- CMD")
	case stop && len(steps) == 0:
		return fail(c, "conflicting_options", "--stop-on-error", "--stop-on-error goes with --step")
	}
	hosts := make([]remote.Host, len(aliases))
	for i, a := range aliases {
		h, e := remote.Resolve(c.Env.Get, a)
		if e != nil {
			return c.Fail(*e)
		}
		hosts[i] = h
	}
	lines := steps
	if dash {
		lines = []string{joinWords(words)}
	}
	prelude, canons, err := remoteLines(c, lines)
	if err != nil {
		return refusal(c, err)
	}
	budget := *o
	if !budget.Capped() {
		budget.Head, budget.Tail = remoteRunLines, remoteRunLines
	}
	rows := llmfmt.New(llmfmt.FormatFlag)
	tr := remote.Transport{Run: m.run, Env: c.Env}
	nonce := remote.NewNonce()
	timeout := o.TimeoutOr(remoteRunTimeout)
	if dash {
		return m.fanOut(c, tr, hosts, nonce, runScript(nonce, prelude, canons[0]), timeout, o, &budget, rows)
	}
	return m.runSteps(c, tr, hosts[0], nonce, stepsScript(nonce, prelude, canons, stop), len(canons), timeout, o, &budget, rows)
}

// fanOut is R1: the same script on every host, at most -j at a time, one
// row per host in the order given.
func (m *Module) fanOut(c *invoke.Call, tr remote.Transport, hosts []remote.Host, nonce, script string, timeout time.Duration,
	o, budget, rows *llmfmt.Options) int {
	type hostRow struct {
		row []any
		cut int
	}
	done := llmfmt.Map(c.Ctx, o.JobsOr(llmfmt.NetworkJobs), 0, hosts, func(ctx context.Context, h remote.Host) hostRow {
		r := tr.Exec(ctx, h, nonce, "sh -s", strings.NewReader(script), timeout)
		out, n1 := capText(budget, afterBegin(r.Stdout, nonce))
		errText, n2 := capText(budget, afterBegin(r.Stderr, nonce))
		row := []any{h.Alias, nil, r.Ms, out, errText, nil}
		switch {
		case r.Tag == "rc":
			row[1] = r.Status
		case r.TimedOut:
			row[5] = "timeout"
		default:
			row[5] = r.Failure(h).Code
		}
		return hostRow{row, n1 + n2}
	})
	cut := 0
	table := llmfmt.Table{Cols: []string{"host", "rc", "ms", "out", "err", "error"}}
	for _, d := range done {
		table.Rows = append(table.Rows, d.row)
		cut += d.cut
	}
	if cut > 0 {
		c.NoteTruncated(cut)
	}
	rows.Write(c, o.Format, table, nil)
	return 0
}

// runSteps is R2: the steps in order in one invocation, one row per step
// that ran.
func (m *Module) runSteps(c *invoke.Call, tr remote.Transport, h remote.Host, nonce, script string, total int, timeout time.Duration,
	o, budget, rows *llmfmt.Options) int {
	r := tr.Exec(c.Ctx, h, nonce, "sh -s", strings.NewReader(script), timeout)
	if r.Tag == "" && !r.TimedOut {
		return c.Fail(*r.Failure(h))
	}
	done, rest := parseSteps(afterBegin(r.Stdout, nonce), nonce)
	table := llmfmt.Table{Cols: []string{"step", "rc", "ms", "out"}}
	cut := 0
	for _, s := range done {
		out, n := capText(budget, s.out)
		cut += n
		table.Rows = append(table.Rows, []any{s.n, s.rc, s.ms, out})
	}
	meta := map[string]any{}
	switch {
	case r.TimedOut:
		meta["timed_out"] = true
		if len(done) < total {
			out, n := capText(budget, rest)
			cut += n
			table.Rows = append(table.Rows, []any{len(done) + 1, nil, nil, out})
		}
	case len(done) < total:
		meta["stopped"] = true
		meta["skipped"] = total - len(done)
	}
	if cut > 0 {
		c.NoteTruncated(cut)
	}
	rows.Write(c, o.Format, table, meta)
	return 0
}

// stepResult is one step frame of an R2 session.
type stepResult struct {
	n   int
	rc  any
	ms  any
	out string
}

// parseSteps splits an R2 stdout at its step frames ("\n<nonce> step N RC
// START END"); rest is the output after the last frame (a step that did
// not finish).
func parseSteps(out, nonce string) ([]stepResult, string) {
	marker := "\n" + nonce + " step "
	var steps []stepResult
	for {
		i := strings.Index(out, marker)
		if i < 0 {
			return steps, out
		}
		seg, line := out[:i], out[i+len(marker):]
		out = ""
		if j := strings.IndexByte(line, '\n'); j >= 0 {
			line, out = line[:j], line[j+1:]
		}
		f := strings.Split(line, " ")
		for len(f) < 4 {
			f = append(f, "")
		}
		s := stepResult{out: seg}
		s.n, _ = strconv.Atoi(f[0])
		if rc, err := strconv.Atoi(f[1]); err == nil {
			s.rc = rc
		}
		s.ms = nanoMs(f[2], f[3])
		steps = append(steps, s)
	}
}

// nanoMs is the time between two `date +%s%N` readings in ms, nil when
// the remote date has no %N.
func nanoMs(start, end string) any {
	digits := func(s string) bool {
		if len(s) < 16 {
			return false
		}
		for i := 0; i < len(s); i++ {
			if s[i] < '0' || s[i] > '9' {
				return false
			}
		}
		return true
	}
	if !digits(start) || !digits(end) {
		return nil
	}
	a, err1 := strconv.ParseInt(start, 10, 64)
	b, err2 := strconv.ParseInt(end, 10, 64)
	if err1 != nil || err2 != nil || b < a {
		return nil
	}
	return llmfmt.Ms(time.Duration(b - a))
}

// afterBegin is the output after the "<nonce> begin" line, which the
// scripts print first on stdout and stderr: what a login file printed
// before it is dropped.
func afterBegin(b []byte, nonce string) string {
	s := string(b)
	if i := strings.Index(s, nonce+" begin\n"); i >= 0 {
		return s[i+len(nonce)+len(" begin\n"):]
	}
	return s
}

// capText is s under the output budget: lines cut at --width, the head
// and tail lines kept around an inline omitted marker, no final newline.
func capText(o *llmfmt.Options, s string) (string, int) {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return "", 0
	}
	var b strings.Builder
	lw := o.Lines(&b, "lines")
	for _, l := range strings.Split(s, "\n") {
		lw.Line(l)
	}
	n := lw.Close()
	return strings.TrimSuffix(b.String(), "\n"), n
}

// runHeader starts every remote run script: the begin lines, the nonce,
// the llm_exec prelude in P, and GL, the grep-order flag of the prelude
// as the remote grep needs it (GNU grep takes the secret excludes last,
// so they win over an --include; any other grep takes them first).
func runHeader(nonce, prelude string) string {
	return "N=" + remote.Quote(nonce) + "\n" +
		`printf '%s begin\n' "$N"; printf '%s begin\n' "$N" >&2` + "\n" +
		"NL='\n'\nGL=\n" +
		"if command grep --version 2>/dev/null | command grep GNU >/dev/null 2>&1; then GL='__LLM_CX_GREP_LAST=1'; fi\n" +
		"P=" + remote.Quote(prelude) + "\n"
}

// bashLine runs one canonical line under the prelude, stdin closed.
func bashLine(canon string) string {
	return `bash --noprofile --norc -c "$P$NL$GL$NL"` + remote.Quote(canon) + " </dev/null"
}

// runScript is the R1 script: the command's stdout and stderr, then the
// "rc" trailer with its status.
func runScript(nonce, prelude, canon string) string {
	return runHeader(nonce, prelude) + bashLine(canon) + "\ngr=$?\n" + remote.Trailer(nonce, "rc", `"$gr"`) + "\nexit 0\n"
}

// stepsScript is the R2 script: each step's merged output, then its frame
// on stdout; with stop, the first failing step ends the session.
func stepsScript(nonce, prelude string, canons []string, stop bool) string {
	var s strings.Builder
	s.WriteString(runHeader(nonce, prelude))
	s.WriteString("gash_t() { date +%s%N 2>/dev/null; }\n")
	end := remote.Trailer(nonce, "rc", "0") + "; exit 0"
	for i, cn := range canons {
		s.WriteString(`s=$(gash_t); ` + bashLine(cn) + ` 2>&1; r=$?; e=$(gash_t); printf '\n%s step %s %s %s %s\n' "$N" ` +
			strconv.Itoa(i+1) + ` "$r" "$s" "$e"` + "\n")
		if stop {
			s.WriteString(`[ "$r" = 0 ] || { ` + end + "; }\n")
		}
	}
	s.WriteString(end + "\n")
	return s.String()
}

// joinWords is the command line of the words after --: one word is the
// line itself (pipes and quotes as typed), several are quoted where they
// need it, so the line means those words.
func joinWords(words []string) string {
	if len(words) == 1 {
		return words[0]
	}
	q := make([]string, len(words))
	for i, w := range words {
		q[i] = w
		if w == "" || strings.IndexFunc(w, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_./=:,+@%-", r))
		}) >= 0 {
			q[i] = remote.Quote(w)
		}
	}
	return strings.Join(q, " ")
}

// remoteLines checks each line as llm_exec checks it (the settings of
// ~/.gash_env, GASH_LLM_SANDBOX aside: bwrap is a local sandbox) against
// the remote side: no local filesystem lookups, a relative path is a path
// below the remote home, and a glob or ~ the canonical rendering would
// expand here is refused. It returns the prelude (the same for every
// line) and each line's canonical script.
func remoteLines(c *invoke.Call, lines []string) (string, []string, error) {
	f := config.LoadCaller(c.Env.Get)
	setting := func(name string) string {
		v, _ := f.Setting(name)
		return v
	}
	s := llmexec.Settings{Exec: setting("GASH_LLM_EXEC"), ExecAllow: setting("GASH_LLM_EXEC_ALLOW")}
	env := llmexec.Environ{
		Cwd: remoteCwd, Home: remoteCwd, RootHome: "/root", BashPath: "bash", Vars: map[string]string{},
		FS: noFS{}, LookupUser: func(string) (string, bool) { return "", false },
	}
	base, err := llmexec.Build("true", s, env, remoteProbe{})
	if err != nil {
		return "", nil, err
	}
	i := strings.LastIndexByte(base.Script, '\n')
	if i < 0 {
		return "", nil, remoteRunInternal()
	}
	prelude := base.Script[:i]
	canons := make([]string, len(lines))
	for k, line := range lines {
		if s.Exec != "unrestricted" {
			if w, ok := localExpansion(line); ok {
				return "", nil, &llmexec.Refusal{Code: "remote_glob_unsupported", Details: w, Action: "RETRY",
					Hint: "Globs and ~ would expand on this machine, not on the host: give explicit paths (llm_find PATTERN DIR --host ALIAS lists them)"}
			}
		}
		p, err := llmexec.Build(line, s, env, remoteProbe{})
		if err != nil {
			return "", nil, err
		}
		cn, ok := strings.CutPrefix(p.Script, prelude+"\n")
		if !ok {
			return "", nil, remoteRunInternal()
		}
		canons[k] = cn
	}
	return prelude, canons, nil
}

func remoteRunInternal() error {
	return &llmexec.Refusal{Code: "internal_error", Details: "llm_remote_run: unexpected llm_exec script layout", Action: string(contract.Stop)}
}

// localExpansion returns the word of line that holds an unquoted glob
// (* ?, or [ with a ] after it) or a leading unquoted ~.
func localExpansion(line string) (string, bool) {
	inS, inD, start := false, false, true
	ws := 0
	word := func(i int) string {
		e := i
		for e < len(line) && !strings.ContainsRune(" \t\n;|&()<>", rune(line[e])) {
			e++
		}
		return line[ws:e]
	}
	for i := 0; i < len(line); i++ {
		ch := line[i]
		switch {
		case inS:
			inS = ch != '\''
			continue
		case inD:
			if ch == '\\' {
				i++
			} else if ch == '"' {
				inD = false
			}
			continue
		}
		if strings.IndexByte(" \t\n;|&()<>", ch) >= 0 {
			start, ws = true, i+1
			continue
		}
		atStart := start
		start = false
		switch ch {
		case '\\':
			i++
		case '\'':
			inS = true
		case '"':
			inD = true
		case '*', '?':
			return word(i), true
		case '[':
			if strings.IndexByte(word(i)[i-ws:], ']') > 0 {
				return word(i), true
			}
		case '~':
			if atStart {
				return word(i), true
			}
		}
	}
	return "", false
}

// noFS is a filesystem where nothing exists: the llm_exec checks of a
// remote line stay lexical.
type noFS struct{}

func (noFS) Lstat(string) (fs.FileInfo, error) { return nil, fs.ErrNotExist }
func (noFS) Readlink(string) (string, error)   { return "", fs.ErrNotExist }

// remoteProbe answers llm_exec's tool probes for a host it cannot ask
// without another round trip, failing closed: sed and awk run as `sed
// --sandbox` and `gawk --sandbox`, diff with --no-dereference, so a host
// without them fails the command instead of running it unconfined; the
// grep order is decided on the host (GL in runHeader); no bwrap.
type remoteProbe struct{}

func (remoteProbe) HasSandboxedSed() bool  { return true }
func (remoteProbe) HasSandboxedGawk() bool { return true }
func (remoteProbe) HasDiffNoDeref() bool   { return true }
func (remoteProbe) IsGNUGrep() bool        { return false }
func (remoteProbe) HasBwrap() bool         { return false }
