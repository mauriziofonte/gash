// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/runner"
)

// lookPath is `type -P name` in the caller's PATH.
func (m *Module) lookPath(c *invoke.Call, name string) (string, bool) {
	if m.lookPathFn != nil {
		return m.lookPathFn(name)
	}
	p, err := runner.LookPath(name, c.Env.Child(), "")
	return p, err == nil
}

// output runs a program found in the caller's PATH with the caller's
// environment (plus extra settings) and returns its stdout; stderr is
// discarded, as the 2.x pipelines sent it to /dev/null.
func (m *Module) output(c *invoke.Call, extra []string, path string, args ...string) []byte {
	env := runner.NewEnv().AllowAll(c.Env.Child())
	for _, kv := range extra {
		k, v, _ := strings.Cut(kv, "=")
		env.Set(k, v)
	}
	res, err := m.run.Run(c.Ctx, runner.Cmd{Path: path, Args: args, Options: runner.Options{Env: env.Environ()}})
	if err != nil {
		return nil
	}
	return res.Stdout
}

// commandSubst is "$(cmd)" (trailing newlines removed) of a program run as
// output does it.
func (m *Module) commandSubst(c *invoke.Call, path string, args ...string) string {
	return runner.CommandSubst(m.output(c, nil, path, args...))
}

// ports is llm_ports (system.sh __llm_ports_impl).
func (m *Module) ports(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	scope, port, byPort := "a", "", false
	for args := c.Args; len(args) > 0; args = args[1:] {
		switch a := args[0]; a {
		case "--listen", "-l":
			scope = "l"
		case "--port": // 3.4 (plan N4): net_ports.go
			if len(args) < 2 {
				return fail(c, "missing_value", a, a+" needs a value")
			}
			port, byPort, args = args[1], true, args[1:]
		default:
			return fail(c, "unknown_option", a, "Use --listen, --port N or no option")
		}
	}
	if byPort {
		return m.portsByPort(c, scope, port)
	}
	var rows [][4]string
	if ss, ok := m.lookPath(c, "ss"); ok {
		rows = parseSS(m.output(c, nil, ss, "-tun"+scope))
	} else if ns, ok := m.lookPath(c, "netstat"); ok {
		rows = parseNetstat(m.output(c, nil, ns, "-tun"+scope))
	} else {
		return fail(c, "ports_tool_not_found", "Neither ss nor netstat is installed",
			"Install iproute2 (ss) or net-tools (netstat) to list ports. Skip this check")
	}
	// By port, then protocol, in the tool's order otherwise; one record
	// per (port, protocol, state, address), so an IPv4 and an IPv6
	// listener on one port are two records (LLM-11; 2.x kept one line
	// per port and protocol).
	sort.SliceStable(rows, func(i, j int) bool {
		if c := cmpNumeric(rows[i][0], rows[j][0]); c != 0 {
			return c < 0
		}
		return rows[i][1] < rows[j][1]
	})
	seen := map[[4]string]bool{}
	b := []byte{'['}
	for _, r := range rows {
		key := [4]string{numericText(r[0]), r[1], r[2], r[3]}
		if seen[key] {
			continue
		}
		seen[key] = true
		if len(b) > 1 {
			b = append(b, ',')
		}
		b = append(b, `{"port":`...)
		b = append(b, numericText(r[0])...)
		b = append(b, `,"proto":"`...)
		b = appendEscaped(b, r[1])
		b = append(b, `","state":"`...)
		b = appendEscaped(b, r[2])
		b = append(b, `","address":"`...)
		b = appendEscaped(b, r[3])
		b = append(b, `"}`...)
	}
	b = append(b, "]\n"...)
	_, _ = c.Stdout.Write(b)
	return 0
}

// appendEscaped appends s as the inside of a JSON string, the way jq
// writes it: every control character escaped and each invalid UTF-8
// sequence as one U+FFFD. The 2.x awk escapers left controls (some all,
// some all but tab, newline and CR) and invalid bytes raw, so a name with
// one made the output invalid JSON (3.0: LEDGER LLM-06 and LLM-23 fixed).
func appendEscaped(b []byte, s string) []byte {
	q := contract.AppendString(nil, s, contract.EscapeJQ)
	return append(b, q[1:len(q)-1]...)
}

var digitsRE = lazyre.MustCompile(`^[0-9]+$`)

// numericText is a digit string as awk's %d prints it.
func numericText(s string) string {
	t := strings.TrimLeft(s, "0")
	if t == "" {
		return "0"
	}
	return t
}

// cmpNumeric compares two digit strings by value (sort -n).
func cmpNumeric(a, b string) int {
	a, b = numericText(a), numericText(b)
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

// splitPort is the awk split on ":": the last part is the port, the rest
// (without its colon) the address.
func splitPort(local string) (port, addr string) {
	if local == "" {
		return "", ""
	}
	i := strings.LastIndexByte(local, ':')
	port = local[i+1:]
	if len(local)-len(port)-1 > 0 {
		addr = local[:len(local)-len(port)-1]
	}
	return port, addr
}

// parseSS reads ss -tun[al]: a Netid column only when several protocols
// are shown, UNCONN reported as LISTEN.
func parseSS(out []byte) [][4]string {
	var rows [][4]string
	netid := false
	for i, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		f := awkFields(line)
		if i == 0 {
			netid = len(f) > 0 && f[0] == "Netid"
			continue
		}
		var proto, state, local string
		if netid {
			proto, state, local = field(f, 1), field(f, 2), field(f, 5)
		} else {
			proto, state, local = "tcp", field(f, 1), field(f, 4)
		}
		port, addr := splitPort(local)
		if state == "UNCONN" {
			state = "LISTEN"
		}
		if digitsRE.MatchString(port) {
			rows = append(rows, [4]string{port, proto, state, addr})
		}
	}
	return rows
}

// parseNetstat reads netstat -tun[al]: tcp/udp lines, the IPv6 suffix of
// the protocol dropped, udp reported as LISTEN.
func parseNetstat(out []byte) [][4]string {
	var rows [][4]string
	for _, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		f := awkFields(line)
		p := field(f, 1)
		if !strings.HasPrefix(p, "tcp") && !strings.HasPrefix(p, "udp") {
			continue
		}
		port, addr := splitPort(field(f, 4))
		proto := strings.TrimSuffix(p, "6")
		state := field(f, 6)
		if proto == "udp" {
			state = "LISTEN"
		}
		if digitsRE.MatchString(port) {
			rows = append(rows, [4]string{port, proto, state, addr})
		}
	}
	return rows
}

// awkFields splits a line as awk's default FS does (runs of blanks).
func awkFields(line string) []string {
	return strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' })
}

// field is awk's $n (1-based, "" past the end).
func field(f []string, n int) string {
	if n >= 1 && n <= len(f) {
		return f[n-1]
	}
	return ""
}

// procs is llm_procs (system.sh __llm_procs_impl).
func (m *Module) procs(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	name, port := "", ""
	// 3.4 (plan P2): --pid, --alive, --tree select the 3.4 rows; --format
	// and the output budget apply to every mode.
	var q procsQuery
	o := llmfmt.New(llmfmt.FormatFlag | llmfmt.BudgetFlags)
	args := c.Args
	for len(args) > 0 {
		n, ok := o.Take(c, args)
		if !ok {
			return 1
		}
		if n > 0 {
			args = args[n:]
			continue
		}
		a := args[0]
		switch a {
		case "--alive", "--tree":
			q.alive = q.alive || a == "--alive"
			q.tree = q.tree || a == "--tree"
			args = args[1:]
			continue
		case "--pid":
			pids, n, ok := takePIDs(c, args)
			if !ok {
				return 1
			}
			q.pids = append(q.pids, pids...)
			args = args[n:]
			continue
		}
		if a != "--name" && a != "--port" {
			return fail(c, "unknown_option", a, "Use --name NAME, --port PORT, --pid PID..., --alive, --tree or no option")
		}
		if len(args) < 2 {
			return fail(c, "missing_value", a, a+" needs a value")
		}
		if a == "--name" {
			name = args[1]
		} else {
			port = args[1]
		}
		args = args[2:]
	}
	if port != "" {
		if !validUint(c, "port", port) {
			return 1
		}
		// Base 10, leading zeros dropped, so "port" is a plain JSON
		// number; above 65535 is no port at all (LLM-12).
		if n := numericText(port); len(n) > 5 || atoi(n) > 65535 {
			return fail(c, "invalid_port", "Value '"+port+"' is above 65535", "Provide a port number from 0 to 65535")
		}
		port = numericText(port)
	}
	if q.active() {
		q.name, q.port = name, port
		return m.procs34(c, o, q)
	}
	var items []string
	switch {
	case port != "":
		if lsof, ok := m.lookPath(c, "lsof"); ok {
			ps, _ := m.lookPath(c, "ps")
			for _, pid := range readWords(m.output(c, nil, lsof, "-i", ":"+port, "-t")) {
				if !digitsRE.MatchString(pid) {
					continue
				}
				cmd := m.psField(c, ps, "-p", pid, "-o", "comm=")
				items = append(items, `{"pid":`+pid+`,"name":`+contract.Quote(cmd, contract.EscapeJQ)+`,"port":`+port+`}`)
			}
		}
	case name != "":
		items = m.procsByName(c, name)
	default:
		ps, ok := m.lookPath(c, "ps")
		if !ok {
			break
		}
		out := m.output(c, []string{"LC_ALL=C"}, ps, "aux", "--no-headers")
		lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
		if len(out) == 0 {
			lines = nil
		}
		if len(lines) > 20 {
			lines = lines[:20]
		}
		for _, l := range lines {
			f := awkFields(l)
			var b []byte
			b = append(b, `{"pid":`...)
			b = append(b, field(f, 2)...)
			b = append(b, `,"user":"`...)
			b = appendEscaped(b, field(f, 1))
			b = append(b, `","cpu":`...)
			b = append(b, field(f, 3)...)
			b = append(b, `,"mem":`...)
			b = append(b, field(f, 4)...)
			b = append(b, `,"name":"`...)
			b = appendEscaped(b, field(f, 11))
			b = append(b, `"}`...)
			items = append(items, string(b))
		}
	}
	if o.Format != llmfmt.Default || o.Capped() || o.Width > 0 {
		return procsLegacyTable(c, o, items)
	}
	_, _ = c.Stdout.Write([]byte("[" + strings.Join(items, ",") + "]\n"))
	return 0
}

// readWords is `while read -r w`: each line with its blanks trimmed.
func readWords(out []byte) []string {
	if len(out) == 0 {
		return nil
	}
	var ws []string
	for _, l := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		ws = append(ws, strings.Trim(l, " \t\n"))
	}
	return ws
}

// psField is "$(ps ARGS)", empty when ps is missing.
func (m *Module) psField(c *invoke.Call, ps string, args ...string) string {
	if ps == "" {
		return ""
	}
	return m.commandSubst(c, ps, args...)
}

// procsByName is the --name branch: pgrep -f NAME without this process,
// its ancestors and the subshells they forked (same command line as their
// parent), nor a process gone before it was read.
func (m *Module) procsByName(c *invoke.Call, name string) []string {
	ps, _ := m.lookPath(c, "ps")
	self := map[string]bool{}
	p := strconv.Itoa(m.pid())
	for i := 0; i < 64; i++ {
		if !digitsRE.MatchString(p) || cmpNumeric(p, "1") <= 0 {
			break
		}
		self[p] = true
		p = strings.Map(func(r rune) rune {
			if r == ' ' || r == '\t' || r == '\n' || r == '\v' || r == '\f' || r == '\r' {
				return -1
			}
			return r
		}, m.psField(c, ps, "-o", "ppid=", "-p", p))
	}
	pgrep, ok := m.lookPath(c, "pgrep")
	if !ok {
		return nil
	}
	var items []string
	for _, pid := range readWords(m.output(c, nil, pgrep, "-f", name)) {
		if !digitsRE.MatchString(pid) || self[pid] {
			continue
		}
		ppid, args := readTwo(m.psField(c, ps, "-o", "ppid=,args=", "-p", pid))
		if !digitsRE.MatchString(ppid) {
			continue
		}
		if self[ppid] {
			if readLine(m.psField(c, ps, "-o", "args=", "-p", ppid)) == args {
				continue
			}
		}
		cmd := m.psField(c, ps, "-p", pid, "-o", "comm=")
		items = append(items, `{"pid":`+pid+`,"name":`+contract.Quote(cmd, contract.EscapeJQ)+`}`)
	}
	return items
}

// readLine is `read -r line <<<"$s"`: the first line, blanks trimmed.
func readLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.Trim(s, " \t")
}

// readTwo is `read -r a b <<<"$s"`: the first word of the first line and
// the rest, blanks trimmed.
func readTwo(s string) (string, string) {
	s = readLine(s)
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.Trim(s[i:], " \t")
}

var (
	envNameRE   = lazyre.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	envSecretRE = lazyre.MustCompile(`(PASSWORD|PASSWD|PASSPHRASE|PASS$|_PWD$|SECRET|TOKEN|KEY|CREDENTIAL|AUTH|PRIVATE|COOKIE|SIGNATURE|SESSION|BEARER|JWT|WEBHOOK|SALT|ENCRYPT|(^|_)PAT$|AWS_|DATABASE_URL|DSN|CONN(ECTION)?_?STRING)`)
	envKeyRE    = lazyre.MustCompilePOSIX(`-{5}BEGIN [[:upper:][:digit:] ]*PRIVATE KEY`)
	envTokenREs = []*lazyre.Regexp{
		lazyre.MustCompilePOSIX(`AKIA[[:upper:][:digit:]]{16}`),
		lazyre.MustCompilePOSIX(`(gh[pousr]_|github_pat_)[[:alnum:]_]{20,}`),
		lazyre.MustCompilePOSIX(`sk-(ant-)?[[:alnum:]_-]{20,}`),
		lazyre.MustCompilePOSIX(`xox[abposr]-[[:alnum:]-]{10,}`),
		lazyre.MustCompilePOSIX(`AIza[[:alnum:]_-]{35}`),
	}
)

// envURLRE is a URL with a password, from its scheme: scheme://user:
// then the password up to the last "@" of the token.
var envURLRE = lazyre.MustCompile(`^([A-Za-z][A-Za-z0-9+.-]*://[^/:@]*):.*@([^@]*)$`)

// isSchemeChar is [A-Za-z0-9+.-], the characters a URL scheme is made of.
func isSchemeChar(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '+' || c == '.' || c == '-'
}

// envMaskURLs masks the password of every URL in v: each blank-separated
// token is masked from the leftmost scheme that starts it or follows a
// character no scheme holds, the password running to the token's last
// "@". 2.x matched the whole value from its start and stopped the password
// at the first "/" or "@", so a password holding either, or a URL after
// other text, leaked (3.0: LEDGER LLM-02 fixed).
func envMaskURLs(v string) string {
	var b strings.Builder
	for len(v) > 0 {
		i := strings.IndexAny(v, " \t\n\r\v\f")
		if i < 0 {
			i = len(v)
		}
		b.WriteString(maskURLToken(v[:i]))
		j := i
		for j < len(v) && strings.IndexByte(" \t\n\r\v\f", v[j]) >= 0 {
			j++
		}
		b.WriteString(v[i:j])
		v = v[j:]
	}
	return b.String()
}

func maskURLToken(tok string) string {
	for i := 0; i < len(tok); i++ {
		if i > 0 && isSchemeChar(tok[i-1]) {
			continue
		}
		if m := envURLRE.FindStringSubmatch(tok[i:]); m != nil {
			return tok[:i] + m[1] + ":***@" + m[2]
		}
	}
	return tok
}

// envRedact is __llm_env_redact: URL passwords, a private key block and
// the known token formats masked.
func envRedact(v string) string {
	v = envMaskURLs(v)
	if envKeyRE.MatchString(v) {
		return "***"
	}
	for _, re := range envTokenREs {
		for {
			m := re.FindString(v)
			if m == "" {
				break
			}
			v = strings.ReplaceAll(v, m, "***")
		}
	}
	return v
}

// env is llm_env (system.sh __llm_env_impl): the caller's exported
// variables, secret names left out and secret values masked.
func (m *Module) env(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	filter := ""
	args := c.Args
	for len(args) > 0 {
		a := args[0]
		switch {
		case a == "--filter" || a == "-f":
			if len(args) < 2 {
				return fail(c, "missing_value", a, a+" needs a regular expression")
			}
			filter = args[1]
			args = args[2:]
		case strings.HasPrefix(a, "-"):
			return fail(c, "unknown_option", a, "Use --filter PATTERN")
		default:
			filter = a
			args = args[1:]
		}
	}
	var re *regexp.Regexp
	if filter != "" {
		var err error
		// An RE2 expression: 2.x took a POSIX ERE (forced divergence).
		if re, err = regexp.Compile(filter); err != nil {
			return fail(c, "invalid_filter", filter, "--filter takes an extended regular expression, e.g. '^PATH$' or 'NODE|NPM'")
		}
	}
	vars := map[string]string{}
	var names []string
	for _, kv := range c.Env.Child() {
		k, v, _ := strings.Cut(kv, "=")
		if k == "_" {
			continue // Bash does not list $_ among the exported variables
		}
		if _, dup := vars[k]; !dup {
			names = append(names, k)
		}
		vars[k] = v
	}
	sort.Strings(names)
	b := []byte{'{'}
	first := true
	for _, n := range names {
		if !envNameRE.MatchString(n) || (re != nil && !re.MatchString(n)) || envSecretRE.MatchString(strings.ToUpper(n)) {
			continue
		}
		if !first {
			b = append(b, ',')
		}
		first = false
		b = append(b, "\n  "...)
		b = contract.AppendString(b, n, contract.EscapeJQ)
		b = append(b, ": "...)
		// Invalid UTF-8 is one U+FFFD per bad sequence, as on the jq
		// paths (3.0: LEDGER LLM-23 fixed; 2.x let the bytes through).
		b = contract.AppendString(b, envRedact(vars[n]), contract.EscapeJQ)
	}
	b = append(b, "\n}\n"...)
	_, _ = c.Stdout.Write(b)
	return 0
}

// pid is this process, the subshell the stub ran the binary from.
func (m *Module) pid() int {
	if m.getpid != nil {
		return m.getpid()
	}
	return os.Getpid()
}
