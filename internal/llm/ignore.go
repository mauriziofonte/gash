// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/mauriziofonte/gash/internal/lazyre"
)

// The content search of llm_grep and llm_find --contains has ripgrep
// semantics (LLM-03): rg's override globs (--glob, --iglob), its ignore
// files (.rgignore, .ignore, and in a git repository .gitignore,
// .git/info/exclude and the global excludes file) and its hidden-file rule.
// The glob dialect is the one of rg's globset crate with the gitignore
// rewrites of its ignore crate.

var errGlob = errors.New("invalid glob")

// globToken is one token of a globset pattern.
type globToken struct {
	kind   int // tokLiteral...
	lit    rune
	neg    bool
	ranges [][2]rune
	alts   [][]globToken
}

const (
	tokLiteral = iota
	tokAny
	tokZeroOrMore
	tokRecursivePrefix
	tokRecursiveSuffix
	tokRecursiveZeroOrMore
	tokClass
	tokAlternates
)

// globParser is globset's Parser (backslash escapes on).
type globParser struct {
	chars []rune
	pos   int
	prev  rune
	has   bool // prev is set
	stack [][]globToken
	alts  [][][]globToken // the alternatives of each open {...}
}

func (p *globParser) peek() (rune, bool) {
	if p.pos < len(p.chars) {
		return p.chars[p.pos], true
	}
	return 0, false
}

func (p *globParser) bump() (rune, bool) {
	if p.pos >= len(p.chars) {
		return 0, false
	}
	c := p.chars[p.pos]
	p.pos++
	return c, true
}

func (p *globParser) push(t globToken) {
	p.stack[len(p.stack)-1] = append(p.stack[len(p.stack)-1], t)
}

func (p *globParser) popToken() (globToken, bool) {
	top := p.stack[len(p.stack)-1]
	if len(top) == 0 {
		return globToken{}, false
	}
	t := top[len(top)-1]
	p.stack[len(p.stack)-1] = top[:len(top)-1]
	return t, true
}

func (p *globParser) haveTokens() bool { return len(p.stack[len(p.stack)-1]) > 0 }

// parseGlob tokenizes a globset pattern.
func parseGlob(glob string) ([]globToken, error) {
	p := &globParser{chars: []rune(glob), stack: [][]globToken{nil}}
	for {
		c, ok := p.bump()
		if !ok {
			break
		}
		switch c {
		case '?':
			p.push(globToken{kind: tokAny})
		case '*':
			p.parseStar()
		case '[':
			if err := p.parseClass(); err != nil {
				return nil, err
			}
		case '{':
			if len(p.alts) > 0 {
				return nil, errGlob // nested alternates
			}
			p.alts = append(p.alts, nil)
			p.stack = append(p.stack, nil)
		case '}':
			if len(p.alts) == 0 {
				return nil, errGlob
			}
			alts := append(p.alts[len(p.alts)-1], p.stack[len(p.stack)-1])
			p.alts = p.alts[:len(p.alts)-1]
			p.stack = p.stack[:len(p.stack)-1]
			p.push(globToken{kind: tokAlternates, alts: alts})
		case ',':
			if len(p.alts) > 0 {
				p.alts[len(p.alts)-1] = append(p.alts[len(p.alts)-1], p.stack[len(p.stack)-1])
				p.stack[len(p.stack)-1] = nil
			} else {
				p.push(globToken{kind: tokLiteral, lit: ','})
			}
		case '\\':
			e, ok := p.bump()
			if !ok {
				return nil, errGlob
			}
			p.push(globToken{kind: tokLiteral, lit: e})
		default:
			p.push(globToken{kind: tokLiteral, lit: c})
		}
		p.prev, p.has = c, true
	}
	if len(p.alts) > 0 {
		return nil, errGlob
	}
	return p.stack[0], nil
}

func (p *globParser) parseStar() {
	prev, hasPrev := p.prev, p.has
	if c, ok := p.peek(); !ok || c != '*' {
		p.push(globToken{kind: tokZeroOrMore})
		return
	}
	p.bump()
	if !p.haveTokens() {
		if c, ok := p.peek(); ok && c != '/' {
			p.push(globToken{kind: tokZeroOrMore})
			p.push(globToken{kind: tokZeroOrMore})
		} else {
			p.push(globToken{kind: tokRecursivePrefix})
			p.bump()
		}
		return
	}
	if !hasPrev || prev != '/' {
		if len(p.stack) <= 1 || (prev != ',' && prev != '{') {
			p.push(globToken{kind: tokZeroOrMore})
			p.push(globToken{kind: tokZeroOrMore})
			return
		}
	}
	isSuffix := false
	c, ok := p.peek()
	switch {
	case !ok:
		isSuffix = true
	case (c == ',' || c == '}') && len(p.stack) >= 2:
		isSuffix = true
	case c == '/':
		p.bump()
	default:
		p.push(globToken{kind: tokZeroOrMore})
		p.push(globToken{kind: tokZeroOrMore})
		return
	}
	t, _ := p.popToken()
	switch t.kind {
	case tokRecursivePrefix:
		p.push(globToken{kind: tokRecursivePrefix})
	case tokRecursiveSuffix:
		p.push(globToken{kind: tokRecursiveSuffix})
	default:
		if isSuffix {
			p.push(globToken{kind: tokRecursiveSuffix})
		} else {
			p.push(globToken{kind: tokRecursiveZeroOrMore})
		}
	}
}

func (p *globParser) parseClass() error {
	var ranges [][2]rune
	neg := false
	if c, ok := p.peek(); ok && (c == '!' || c == '^') {
		p.bump()
		neg = true
	}
	first, inRange := true, false
	for {
		c, ok := p.bump()
		if !ok {
			return errGlob
		}
		switch {
		case c == ']':
			if !first {
				if inRange {
					ranges = append(ranges, [2]rune{'-', '-'})
				}
				p.push(globToken{kind: tokClass, neg: neg, ranges: ranges})
				return nil
			}
			ranges = append(ranges, [2]rune{']', ']'})
		case c == '-':
			switch {
			case first:
				ranges = append(ranges, [2]rune{'-', '-'})
			case inRange:
				r := &ranges[len(ranges)-1]
				r[1] = '-'
				if r[1] < r[0] {
					return errGlob
				}
				inRange = false
			default:
				inRange = true
			}
		default:
			if inRange {
				r := &ranges[len(ranges)-1]
				r[1] = c
				if r[1] < r[0] {
					return errGlob
				}
			} else {
				ranges = append(ranges, [2]rune{c, c})
			}
			inRange = false
		}
		first = false
	}
}

// tokensRegexp is globset's regex for tokens with literal_separator on.
func tokensRegexp(toks []globToken, b *strings.Builder) {
	for _, t := range toks {
		switch t.kind {
		case tokLiteral:
			b.WriteString(regexp.QuoteMeta(string(t.lit)))
		case tokAny:
			b.WriteString("[^/]")
		case tokZeroOrMore:
			b.WriteString("[^/]*")
		case tokRecursivePrefix:
			b.WriteString("(?:/?|.*/)")
		case tokRecursiveSuffix:
			b.WriteString("/.*")
		case tokRecursiveZeroOrMore:
			b.WriteString("(?:/|/.*/)")
		case tokClass:
			b.WriteByte('[')
			if t.neg {
				b.WriteByte('^')
			}
			for _, r := range t.ranges {
				b.WriteString(classLiteral(r[0]))
				if r[0] != r[1] {
					b.WriteByte('-')
					b.WriteString(classLiteral(r[1]))
				}
			}
			b.WriteByte(']')
		case tokAlternates:
			var parts []string
			for _, a := range t.alts {
				var ab strings.Builder
				tokensRegexp(a, &ab)
				if ab.Len() > 0 {
					parts = append(parts, ab.String())
				}
			}
			if len(parts) > 0 {
				b.WriteString("(?:" + strings.Join(parts, "|") + ")")
			}
		}
	}
}

func classLiteral(r rune) string {
	if r < 0x80 && !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9') {
		return fmt.Sprintf(`\x{%x}`, r)
	}
	return string(r)
}

// compiledGlob is a glob's regexp, and its baseGlob when it has one.
type compiledGlob struct {
	re   *lazyre.Regexp
	fast *baseGlob
}

// globCache keeps the compiled globs of the process: a search compiles
// the same sixty twice (with and without the secret excludes), and the
// per-shell server runs search after search.
var globCache = struct {
	sync.Mutex
	m map[string]compiledGlob
}{m: map[string]compiledGlob{}}

// globCacheMax bounds globCache (ignore files hold any number of lines).
const globCacheMax = 4096

// compileGlobFast compiles a globset pattern into an anchored regexp, with
// the glob's baseGlob when it has one. The regexp of a **/P glob, which only a name that is not ASCII
// needs, is compiled at its first use.
func compileGlobFast(glob string, icase bool) (compiledGlob, error) {
	key := "c\x00" + glob
	if icase {
		key = "i\x00" + glob
	}
	globCache.Lock()
	c, ok := globCache.m[key]
	globCache.Unlock()
	if ok {
		return c, nil
	}
	toks, err := parseGlob(glob)
	if err != nil {
		return compiledGlob{}, err
	}
	var b strings.Builder
	if icase {
		b.WriteString("(?i)")
	}
	b.WriteByte('^')
	tokensRegexp(toks, &b)
	b.WriteByte('$')
	if c.fast = newBaseGlob(toks, icase); c.fast != nil {
		c.re = lazyre.MustCompile(b.String())
	} else {
		re, err := regexp.Compile(b.String())
		if err != nil {
			return compiledGlob{}, err
		}
		c.re, c.fast = lazyre.Compiled(re), needGlob(toks, icase)
	}
	globCache.Lock()
	if len(globCache.m) < globCacheMax {
		globCache.m[key] = c
	}
	globCache.Unlock()
	return c, nil
}

// needGlob is the baseGlob of a glob that is not of that form but holds a
// run of literal ASCII characters: a path without that text cannot match
// (need), and the regexp decides for the others.
func needGlob(toks []globToken, icase bool) *baseGlob {
	run, best := "", ""
	for _, t := range toks {
		if t.kind == tokLiteral && t.lit < utf8.RuneSelf {
			c := byte(t.lit)
			if icase && 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}
			run += string(c)
		} else {
			run = ""
		}
		if len(run) > len(best) {
			best = run
		}
	}
	if best == "" {
		return nil
	}
	return &baseGlob{need: best, icase: icase}
}

// baseGlob is a glob of the form **/P, P made of ASCII characters other
// than '/' and of '*': the path matches when its last component matches P,
// which a wildcard comparison tells far sooner than the regexp (a search
// matches every path against some sixty globs).
type baseGlob struct {
	parts []string // P split at each '*', lower case when icase
	icase bool
	// need, set instead of parts, is text the whole path must hold (lower
	// case when icase) for the regexp to match it.
	need string
}

func newBaseGlob(toks []globToken, icase bool) *baseGlob {
	if len(toks) == 0 || toks[0].kind != tokRecursivePrefix {
		return nil
	}
	parts := []string{""}
	for _, t := range toks[1:] {
		switch {
		case t.kind == tokZeroOrMore:
			parts = append(parts, "")
		case t.kind == tokLiteral && t.lit != '/' && t.lit < utf8.RuneSelf:
			c := byte(t.lit)
			if icase && 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}
			parts[len(parts)-1] += string(c)
		default:
			return nil
		}
	}
	return &baseGlob{parts: parts, icase: icase}
}

// match reports whether base, the last component of a path, matches; ok
// is false when only the regexp can tell (a case-insensitive glob and a
// name that is not ASCII, whose runes may fold to ASCII ones).
func (g *baseGlob) match(base string, ascii bool) (matched, ok bool) {
	if g.need != "" {
		return false, false
	}
	if g.icase {
		if !ascii {
			return false, false
		}
		base = strings.ToLower(base)
	}
	first, last := g.parts[0], g.parts[len(g.parts)-1]
	if len(g.parts) == 1 {
		return base == first, true
	}
	if len(base) < len(first)+len(last) || !strings.HasPrefix(base, first) || !strings.HasSuffix(base, last) {
		return false, true
	}
	mid := base[len(first) : len(base)-len(last)]
	for _, p := range g.parts[1 : len(g.parts)-1] {
		i := strings.Index(mid, p)
		if i < 0 {
			return false, true
		}
		mid = mid[i+len(p):]
	}
	return true, true
}

func candASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// ignoreGlob is one line of an ignore file (or one --glob).
type ignoreGlob struct {
	re        *lazyre.Regexp
	fast      *baseGlob // nil: the regexp decides
	whitelist bool
	onlyDir   bool
}

// match is a matcher verdict: none, ignore or whitelist.
type match int

const (
	matchNone match = iota
	matchIgnore
	matchWhitelist
)

// gitignore is the ignore crate's Gitignore: the globs of one file (or of
// the override set), matched relative to root.
type gitignore struct {
	root  string
	globs []ignoreGlob
}

// add is GitignoreBuilder::add_line; it reports whether the line held a
// valid glob (comments and blank lines are not errors).
func (g *gitignore) add(line string, icase bool) error {
	if strings.HasPrefix(line, "#") {
		return nil
	}
	if !strings.HasSuffix(line, `\ `) {
		line = strings.TrimRightFunc(line, unicode.IsSpace)
	}
	if line == "" {
		return nil
	}
	var gl ignoreGlob
	absolute := false
	if strings.HasPrefix(line, `\!`) || strings.HasPrefix(line, `\#`) {
		line = line[1:]
		absolute = strings.HasPrefix(line, "/")
	} else {
		if strings.HasPrefix(line, "!") {
			gl.whitelist = true
			line = line[1:]
		}
		if strings.HasPrefix(line, "/") {
			line = line[1:]
			absolute = true
		}
	}
	if strings.HasSuffix(line, "/") {
		gl.onlyDir = true
		line = line[:len(line)-1]
		line = strings.TrimSuffix(line, `\`)
	}
	actual := line
	if !absolute && !strings.Contains(line, "/") && !strings.HasPrefix(actual, "**/") {
		actual = "**/" + actual
	}
	if strings.HasSuffix(actual, "/**") {
		actual += "/*"
	}
	cg, err := compileGlobFast(actual, icase)
	if err != nil {
		return err
	}
	gl.re, gl.fast = cg.re, cg.fast
	g.globs = append(g.globs, gl)
	return nil
}

// strip is Gitignore::strip: the path relative to the matcher's root.
func (g *gitignore) strip(path string) string {
	path = strings.TrimPrefix(path, "./")
	if g.root != "." && strings.Contains(path, "/") {
		if strings.HasPrefix(path, g.root) {
			path = strings.TrimPrefix(path[len(g.root):], "/")
		}
	}
	return path
}

// matched is Gitignore::matched: the last glob that matches decides.
func (g *gitignore) matched(path string, isDir bool) match {
	if g == nil || len(g.globs) == 0 {
		return matchNone
	}
	cand := g.strip(path)
	base := cand[strings.LastIndexByte(cand, '/')+1:]
	ascii := candASCII(base)
	for i := len(g.globs) - 1; i >= 0; i-- {
		gl := &g.globs[i]
		if gl.onlyDir && !isDir {
			continue
		}
		hit, ok := false, false
		if gl.fast != nil {
			hit, ok = gl.fast.match(base, ascii)
			if need := gl.fast.need; need != "" {
				switch {
				case !gl.fast.icase:
					ok = !strings.Contains(cand, need)
				case candASCII(cand):
					ok = !strings.Contains(strings.ToLower(cand), need)
				}
			}
		}
		if !ok {
			hit = gl.re.MatchString(cand)
		}
		if hit {
			if gl.whitelist {
				return matchWhitelist
			}
			return matchIgnore
		}
	}
	return matchNone
}

// readIgnoreFile builds the matcher of one ignore file in dir; nil when the
// file is missing or holds no glob. Invalid lines are skipped, as rg does.
func readIgnoreFile(dir, file string) *gitignore {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	g := &gitignore{root: dir}
	text := strings.TrimPrefix(string(data), "\ufeff")
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		_ = g.add(line, false)
	}
	if len(g.globs) == 0 {
		return nil
	}
	return g
}

// overrides is rg's override matcher (--glob then --iglob, in that order).
type overrides struct {
	g          *gitignore
	whitelists int
}

// newOverrides builds the matcher; ok is false when a glob is invalid (rg
// then stops with an error and prints nothing on stdout).
func newOverrides(cwd string, globs, iglobs []string) (*overrides, bool) {
	o := &overrides{g: &gitignore{root: cwd}}
	for i, list := range [][]string{globs, iglobs} {
		for _, gl := range list {
			if err := o.g.add(gl, i == 1); err != nil {
				return nil, false
			}
		}
	}
	for _, gl := range o.g.globs {
		if !gl.whitelist {
			o.whitelists++
		}
	}
	return o, true
}

// matched is Override::matched: the gitignore verdict inverted, and a file
// that no glob matched is ignored when there is a whitelist glob.
func (o *overrides) matched(path string, isDir bool) match {
	if o == nil || len(o.g.globs) == 0 {
		return matchNone
	}
	switch o.g.matched(path, isDir) {
	case matchIgnore:
		return matchWhitelist
	case matchWhitelist:
		return matchIgnore
	}
	if o.whitelists > 0 && !isDir {
		return matchIgnore
	}
	return matchNone
}

// ignoreDir holds the ignore files of one directory.
type ignoreDir struct {
	dir                   string
	rgignore, ignore      *gitignore
	gitignore, gitExclude *gitignore
	hasGit                bool
	parent                *ignoreDir
}

// ignoreRules is the per-search configuration of the ignore stack.
type ignoreRules struct {
	noIgnore bool // --no-ignore: no ignore file is read
	hidden   bool // hidden files are skipped (no --hidden)
	global   *gitignore
}

// newIgnoreDir reads the ignore files of dir.
func (r *ignoreRules) newIgnoreDir(parent *ignoreDir, dir string) *ignoreDir {
	d := &ignoreDir{dir: dir, parent: parent}
	if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
		d.hasGit = true
	}
	if r.noIgnore {
		return d
	}
	d.rgignore = readIgnoreFile(dir, filepath.Join(dir, ".rgignore"))
	d.ignore = readIgnoreFile(dir, filepath.Join(dir, ".ignore"))
	d.gitignore = readIgnoreFile(dir, filepath.Join(dir, ".gitignore"))
	if d.hasGit {
		if gd := gitDir(dir); gd != "" {
			d.gitExclude = readIgnoreFile(dir, filepath.Join(gd, "info", "exclude"))
		}
	}
	return d
}

// gitDir resolves the git directory of a working tree root: .git itself, or
// the common directory a "gitdir:" file points to.
func gitDir(dir string) string {
	p := filepath.Join(dir, ".git")
	st, err := os.Stat(p)
	if err != nil {
		return ""
	}
	if st.IsDir() {
		return p
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	gd, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
	if !ok {
		return ""
	}
	gd = strings.TrimSpace(gd)
	if !filepath.IsAbs(gd) {
		gd = filepath.Join(dir, gd)
	}
	if c, err := os.ReadFile(filepath.Join(gd, "commondir")); err == nil {
		cd := strings.TrimSpace(string(c))
		if !filepath.IsAbs(cd) {
			cd = filepath.Join(gd, cd)
		}
		return cd
	}
	return gd
}

// matchedIgnore is Ignore::matched_ignore: .rgignore before .ignore before
// .gitignore before .git/info/exclude before the global excludes, each from
// the closest directory that has a verdict; the git files only up to the
// repository root.
func (r *ignoreRules) matchedIgnore(d *ignoreDir, path string, isDir bool) match {
	if r.noIgnore {
		return matchNone
	}
	anyGit := false
	for p := d; p != nil; p = p.parent {
		if p.hasGit {
			anyGit = true
			break
		}
	}
	var mCustom, mIgnore, mGi, mEx match
	sawGit := false
	for p := d; p != nil; p = p.parent {
		if mCustom == matchNone {
			mCustom = p.rgignore.matched(path, isDir)
		}
		if mIgnore == matchNone {
			mIgnore = p.ignore.matched(path, isDir)
		}
		if anyGit && !sawGit && mGi == matchNone {
			mGi = p.gitignore.matched(path, isDir)
		}
		if anyGit && !sawGit && mEx == matchNone {
			mEx = p.gitExclude.matched(path, isDir)
		}
		sawGit = sawGit || p.hasGit
	}
	mGlobal := matchNone
	if anyGit {
		mGlobal = r.global.matched(path, isDir)
	}
	for _, m := range []match{mCustom, mIgnore, mGi, mEx, mGlobal} {
		if m != matchNone {
			return m
		}
	}
	return matchNone
}

// excludesFileRE is the ignore crate's core.excludesFile line reader.
var excludesFileRE = lazyre.MustCompile(`(?im)^\s*excludesfile\s*=\s*"?\s*(\S+?)\s*"?\s*$`)

// globalExcludes is the ignore crate's global gitignore: core.excludesFile
// from ~/.gitconfig or $XDG_CONFIG_HOME/git/config, else
// $XDG_CONFIG_HOME/git/ignore (~/.config/git/ignore).
func globalExcludes(home, xdg string) *gitignore {
	cfgDir := xdg
	if cfgDir == "" && home != "" {
		cfgDir = filepath.Join(home, ".config")
	}
	var path string
	var cands []string
	if home != "" {
		cands = append(cands, filepath.Join(home, ".gitconfig"))
	}
	if cfgDir != "" {
		cands = append(cands, filepath.Join(cfgDir, "git", "config"))
	}
	for _, f := range cands {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if m := excludesFileRE.FindStringSubmatch(string(data)); m != nil {
			path = m[1]
			if strings.HasPrefix(path, "~/") && home != "" {
				path = filepath.Join(home, path[2:])
			}
			break
		}
	}
	if path == "" && cfgDir != "" {
		path = filepath.Join(cfgDir, "git", "ignore")
	}
	if path == "" {
		return nil
	}
	// GitignoreBuilder::new(""): the root is empty, so a candidate is the
	// path without its leading slash.
	return readIgnoreFile("", path)
}
