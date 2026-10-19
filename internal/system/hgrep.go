// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import (
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/system/handoff"
	"github.com/mauriziofonte/gash/internal/term"
	"github.com/mauriziofonte/gash/internal/ui"
)

// hgrepOpts are the parsed words of an hgrep call.
type hgrepOpts struct {
	pattern                                     string
	limit                                       int64
	includeSelf, reverse, json, regex, count    bool
	group, pick, noHighlight, noColor, wantHelp bool
}

// parseHgrep is the 2.x option loop: every word starting with "-" is an
// option (`--` included, SYS-07), -h only as the first word, the first
// other word is the pattern. It prints the 2.x errors and returns false.
func parseHgrep(c *invoke.Call, p ui.Printer) (hgrepOpts, bool) {
	var o hgrepOpts
	if a := c.Arg(0); a == "-h" || a == "--help" {
		o.wantHelp = true
		return o, true
	}
	o.noHighlight = c.Env.Get("GASH_HEADLESS") == "1"
	args := c.Args
	for i := 0; i < len(args); {
		a := args[i]
		switch a {
		case "-n", "--limit":
			v := ""
			if i+1 < len(args) {
				v = args[i+1]
			}
			// The 2.x digits check, then the value as a decimal number:
			// leading zeros are allowed and nothing is evaluated (SYS-30).
			n, err := strconv.ParseInt(v, 10, 64)
			if v == "" || !isDigits(v) || err != nil {
				p.Error("Option -n requires a numeric argument")
				return o, false
			}
			o.limit = n
			i += 2
			continue
		case "-a", "--all":
			o.includeSelf = true
		case "-r", "--reverse":
			o.reverse = true
		case "-j", "--json":
			o.json = true
		case "-H", "--no-highlight":
			o.noHighlight = true
		case "-E", "--regex":
			o.regex = true
		case "-c", "--count":
			o.count = true
		case "-g", "--group":
			o.group = true
		case "--pick":
			o.pick = true
		case "--no-color":
			o.noColor = true
		default:
			if strings.HasPrefix(a, "-") {
				p.Error("Unknown option: " + a + ". Use -h for help.")
				return o, false
			}
			if o.pattern != "" {
				p.Error("Multiple patterns not supported: " + a)
				return o, false
			}
			o.pattern = a
		}
		i++
	}
	if o.pattern == "" {
		p.Error("Missing required argument: pattern")
		p.Usage("hgrep <pattern> [options]. Use -h for help.")
		return o, false
	}
	return o, true
}

// isDigits reports a non-empty run of ASCII digits.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// hgrep is the smart history search. The stub override runs `history -a`
// between the checks and the search (the hand-off); the binary reads the
// history file itself and takes the gawk path of every awk-dependent
// result (LEDGER SYS-28 to SYS-32).
func (m *Module) hgrep(c *invoke.Call) int {
	p := ui.NewPrinter(c.Stderr, c.Term, false)
	o, ok := parseHgrep(c, p)
	if !ok {
		return 1
	}
	if o.wantHelp {
		write(c.Stdout, hgrepHelp)
		return 0
	}
	histfile := c.Env.Get("HISTFILE")
	if histfile == "" {
		histfile = c.Env.Get("HOME") + "/.bash_history"
	}
	if !m.readable(histfile) {
		p.Error("Cannot read history file: " + histfile)
		return 1
	}
	// history -a: the parent-shell step of the override.
	if ch := handoff.Open(c.Env, m.open); ch.Active() {
		ch.Send(handoff.Go)
		return 0
	}

	data, _ := os.ReadFile(histfile) // a directory reads as empty
	mt := newHistMatcher(o.pattern, o.regex)
	recs := hgrepRecords(string(data), mt, o.includeSelf)
	if mt.invalid {
		// awk died on the regex: an empty result with status 0 (SYS-05);
		// its awk-specific diagnostic becomes one line of ours.
		p.Error("Invalid regex: " + o.pattern)
		recs = nil
	}
	if o.reverse {
		slices.Reverse(recs)
	}
	if o.limit > 0 && int64(len(recs)) > o.limit {
		recs = recs[int64(len(recs))-o.limit:]
	}

	if len(recs) == 0 {
		switch {
		case o.count:
			write(c.Stdout, "0\n")
		case o.json:
			write(c.Stdout, "[]\n")
		}
		return 0
	}
	if o.count {
		write(c.Stdout, strconv.Itoa(len(recs))+"\n")
		return 0
	}
	if o.json {
		write(c.Stdout, hgrepJSON(recs, m.loc))
		return 0
	}

	interactive := !o.noHighlight && c.Term.UseColor()
	r := hgrepRender{loc: m.loc, g: c.Term.Glyphs()}
	if interactive && c.Term.WantColors(o.noColor) {
		pal := c.Term.Palette()
		r.color = true
		r.off, r.ts, r.hl, r.grp, r.cnt, r.tree = pal.Off, pal.Muted, pal.Accent, pal.Bold, pal.Muted, pal.Primary
		r.match = mt
	}

	if o.pick {
		return m.hgrepPick(c, p, o, recs)
	}
	if o.group && interactive {
		write(c.Stdout, r.grouped(recs))
		return 0
	}
	var b strings.Builder
	for _, rc := range recs {
		b.WriteString(r.row(rc, interactive))
	}
	write(c.Stdout, b.String())
	return 0
}

// hgrepPick is --pick: the records on fzf's input, the chosen command on
// stdout.
func (m *Module) hgrepPick(c *invoke.Call, p ui.Printer, o hgrepOpts, recs []histRecord) int {
	if c.Env.Get("GASH_HEADLESS") == "1" || o.noHighlight || !c.Term.StdinTTY() {
		p.Error("--pick requires an interactive terminal")
		return 1
	}
	if !found(c, "fzf") {
		p.Error("--pick requires fzf; install it or drop --pick")
		return 1
	}
	var in strings.Builder
	for _, rc := range recs {
		in.WriteString("[" + stamp(rc.epoch, m.loc, "2006-01-02 15:04:05") + "] " + rc.cmd + "\n")
	}
	res, err := m.run.Run(ctx(c), runner.Cmd{Path: "fzf",
		Args:    []string{"--height=40%", "--reverse", "--no-sort", "--prompt=hgrep> ", "--tiebreak=index"},
		Options: runner.Options{Env: c.Env.Child(), Stdin: strings.NewReader(in.String()), Stderr: c.Stderr}})
	if err != nil || res.ExitCode != 0 {
		return 0
	}
	sel := runner.CommandSubst(res.Stdout)
	if sel == "" {
		return 0
	}
	// ${sel#*] }: the "[timestamp] " prefix removed.
	if i := strings.Index(sel, "] "); i >= 0 {
		sel = sel[i+2:]
	}
	write(c.Stdout, sel+"\n")
	return 0
}

// histRecord is one unique command: the timestamp of its last execution
// (the digits of its #EPOCH line), how many times it ran, the command.
type histRecord struct {
	ts    string
	epoch int64
	count int
	cmd   string
	pos   int // line of the last execution
}

// histMatcher is hgrep's pattern test: a case-insensitive substring with
// Unicode simple folding, or with -E a case-insensitive Go regexp on the
// command as written (SYS-28, SYS-32). The regexp is compiled when the
// first command is tested, as awk did; invalid is set when that fails.
type histMatcher struct {
	pattern  string
	regex    bool
	compiled bool
	re       *regexp.Regexp
	ascii    string // a fixed pattern that is not UTF-8: ASCII folding
	invalid  bool
}

func newHistMatcher(pattern string, regex bool) *histMatcher {
	return &histMatcher{pattern: pattern, regex: regex}
}

func (h *histMatcher) compile() {
	if h.compiled {
		return
	}
	h.compiled = true
	switch {
	case h.regex:
		re, err := regexp.Compile("(?i)" + h.pattern)
		if err != nil || wordAnchors(h.pattern) {
			h.invalid = true
			return
		}
		h.re = re
	case utf8.ValidString(h.pattern):
		h.re = regexp.MustCompile("(?i)" + regexp.QuoteMeta(h.pattern))
	default:
		h.ascii = asciiLower(h.pattern)
	}
}

// wordAnchors reports gawk's \< or \> in a regex: Go reads them as the
// characters < and >, so rc.A refuses them as Go regexp syntax it lacks
// (SYS-28).
func wordAnchors(p string) bool {
	for i := 0; i+1 < len(p); i++ {
		if p[i] == '\\' {
			if p[i+1] == '<' || p[i+1] == '>' {
				return true
			}
			i++
		}
	}
	return false
}

// match tests a command.
func (h *histMatcher) match(cmd string) bool {
	h.compile()
	switch {
	case h.invalid:
		return false
	case h.re != nil:
		return h.re.MatchString(cmd)
	default:
		return strings.Contains(asciiLower(cmd), h.ascii)
	}
}

// highlight wraps every match in on/off, as gawk's case-insensitive gsub
// did (leftmost-longest).
func (h *histMatcher) highlight(s, on, off string) string {
	h.compile()
	if h.invalid || h.pattern == "" {
		return s
	}
	if h.re == nil {
		return asciiHighlight(s, h.ascii, on, off)
	}
	re := h.re.Copy() //nolint:staticcheck // Copy is the documented way to vary Longest without mutating the shared h.re
	re.Longest()
	return re.ReplaceAllStringFunc(s, func(m string) string { return on + m + off })
}

func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

func asciiHighlight(s, low, on, off string) string {
	if low == "" {
		return s
	}
	ls := asciiLower(s)
	var b strings.Builder
	for {
		i := strings.Index(ls, low)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i] + on + s[i:i+len(low)] + off)
		s, ls = s[i+len(low):], ls[i+len(low):]
	}
}

// isEpochLine is awk's /^#[0-9]+$/.
func isEpochLine(l string) bool { return len(l) > 1 && l[0] == '#' && isDigits(l[1:]) }

// hgrepRecords is the data plane: every #EPOCH line takes the next line as
// its command (spaces and tabs trimmed, empty ones skipped, the hgrep and
// history_grep ones too unless -a), matched commands are deduplicated with
// the last execution winning and a count kept, then ordered by that
// execution's epoch; equal epochs keep the order of the last executions
// in the file (SYS-31). A CRLF record line is no record (SYS-06).
func hgrepRecords(data string, mt *histMatcher, includeSelf bool) []histRecord {
	lines := strings.Split(data, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	type seen struct {
		ts    string
		count int
		pos   int
	}
	last := map[string]*seen{}
	var order []string
	for i := 0; i < len(lines); i++ {
		if !isEpochLine(lines[i]) {
			continue
		}
		ts := lines[i][1:]
		if i+1 >= len(lines) {
			break
		}
		i++
		cmd := strings.Trim(lines[i], " \t")
		if cmd == "" {
			continue
		}
		if !includeSelf && (strings.Contains(cmd, "hgrep") || strings.Contains(cmd, "history_grep")) {
			continue
		}
		if !mt.match(cmd) {
			if mt.invalid {
				return nil
			}
			continue
		}
		s := last[cmd]
		if s == nil {
			s = &seen{}
			last[cmd] = s
			order = append(order, cmd)
		}
		s.ts, s.pos = ts, i
		s.count++
	}
	recs := make([]histRecord, 0, len(order))
	for _, cmd := range order {
		s := last[cmd]
		// $(...) drops NUL bytes from the records.
		recs = append(recs, histRecord{ts: s.ts, epoch: epochOf(s.ts), count: s.count,
			cmd: strings.ReplaceAll(cmd, "\x00", ""), pos: s.pos})
	}
	slices.SortStableFunc(recs, func(a, b histRecord) int { return a.pos - b.pos })
	slices.SortStableFunc(recs, func(a, b histRecord) int { return cmpDigits(a.ts, b.ts) })
	return recs
}

// cmpDigits compares two runs of digits as numbers (sort -n).
func cmpDigits(a, b string) int {
	a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		return len(a) - len(b)
	}
	return strings.Compare(a, b)
}

// epochOf is the number awk made of the digits (strftime and %d): the
// value, or the largest one for digits beyond 64 bits.
func epochOf(ts string) int64 {
	n, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return 1<<63 - 1
	}
	return n
}

// stamp is strftime(layout, epoch) in loc.
func stamp(epoch int64, loc *time.Location, layout string) string {
	return time.Unix(epoch, 0).In(loc).Format(layout)
}

// jesc is the 2.x JSON escaping of hgrep: backslash, double quote,
// newline and tab only; other control bytes stay raw (SYS-06).
func jesc(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`)
	return r.Replace(s)
}

// hgrepJSON is the --json array on one line.
func hgrepJSON(recs []histRecord, loc *time.Location) string {
	var b strings.Builder
	b.WriteString("[")
	for i, r := range recs {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"timestamp":"` + stamp(r.epoch, loc, "2006-01-02 15:04:05") + `","epoch":` +
			strconv.FormatInt(r.epoch, 10) + `,"count":` + strconv.Itoa(r.count) + `,"command":"` + jesc(r.cmd) + `"}`)
	}
	b.WriteString("]\n")
	return b.String()
}

// hgrepRender holds the presentation of the text modes: the colors (all
// empty unless the interactive presentation has colors), the glyphs and
// the matcher of the highlight (nil without colors).
type hgrepRender struct {
	loc                         *time.Location
	g                           term.Glyphs
	color                       bool
	off, ts, hl, grp, cnt, tree string
	match                       *histMatcher
}

func (r hgrepRender) hilite(s string) string {
	if r.match == nil {
		return s
	}
	return r.match.highlight(s, r.hl, r.off)
}

func (r hgrepRender) badge(n int) string {
	if n > 1 {
		return " " + r.cnt + "(x" + strconv.Itoa(n) + ")" + r.off
	}
	return ""
}

// row is the flat "[date] command" line, with the count badge in the
// interactive presentation.
func (r hgrepRender) row(rc histRecord, showCount bool) string {
	badge := ""
	if showCount {
		badge = r.badge(rc.count)
	}
	return r.ts + "[" + stamp(rc.epoch, r.loc, "2006-01-02 15:04:05") + "]" + r.off + " " + r.hilite(rc.cmd) + badge + "\n"
}

// chainPos is the 1-based position of the first &&, ||, ; or | of s, 0
// when it has none.
func chainPos(s string) int {
	best := 0
	for _, tok := range []string{"&&", "||", ";", "|"} {
		if p := strings.Index(s, tok) + 1; p > 0 && (best == 0 || p < best) {
			best = p
		}
	}
	return best
}

// grouped is --group: commands sharing the text before their first chain
// operator under one header, ranked by count then recency; groups by their
// latest execution, newest first. The order of the 2.x selection sorts is
// kept, starting from the records' order where awk had its hash order.
func (r hgrepRender) grouped(recs []histRecord) string {
	type group struct {
		key     string
		members []int
		max     string
	}
	var groups []*group
	byKey := map[string]*group{}
	for i, rc := range recs {
		key := rc.cmd
		if p := chainPos(rc.cmd); p > 1 {
			key = strings.Trim(rc.cmd[:p-1], " \t")
		}
		g := byKey[key]
		if g == nil {
			g = &group{key: key}
			byKey[key] = g
			groups = append(groups, g)
		}
		g.members = append(g.members, i)
		if rc.ts > g.max { // awk compares the two strings
			g.max = rc.ts
		}
	}
	for a := range groups {
		for b := a + 1; b < len(groups); b++ {
			if groups[b].max > groups[a].max {
				groups[a], groups[b] = groups[b], groups[a]
			}
		}
	}
	var out strings.Builder
	for _, g := range groups {
		mi := g.members
		if len(mi) <= 1 {
			out.WriteString(r.row(recs[mi[0]], true))
			continue
		}
		out.WriteString(r.grp + g.key + r.off + " " + r.cnt + "(" + strconv.Itoa(len(mi)) + " variants)" + r.off + "\n")
		for x := range mi {
			for y := x + 1; y < len(mi); y++ {
				ix, iy := recs[mi[x]], recs[mi[y]]
				if iy.count > ix.count || (iy.count == ix.count && iy.epoch > ix.epoch) {
					mi[x], mi[y] = mi[y], mi[x]
				}
			}
		}
		for x, idx := range mi {
			rc := recs[idx]
			glyph := r.g.TreeMid
			if x == len(mi)-1 {
				glyph = r.g.TreeEnd
			}
			suffix := strings.TrimLeft(rc.cmd[len(g.key):], " \t")
			out.WriteString("  " + r.tree + glyph + r.off + " " + r.ts + stamp(rc.epoch, r.loc, "15:04:05") + r.off + " " +
				r.hilite(suffix) + r.badge(rc.count) + "\n")
		}
	}
	return out.String()
}
