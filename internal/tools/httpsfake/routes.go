// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// A route is one line of a MOCK_CURL_ROUTES file: the answer a test chose
// for a method and a URL glob. tests/mocks/bin/curl documents the format
// and serves the same file in Bash mode.
type route struct {
	line            int
	method, pattern string
	status          int // 0 when unreachable
	unreachable     bool
	header          [][2]string
	body            string // the literal body, or @PATH
}

var routeLine = regexp.MustCompile(`^([^ \t]+)[ \t]+([^ \t]+)[ \t]+([^ \t]+)(?:[ \t]+(.*))?$`)

var routeStatus = regexp.MustCompile(`^[1-9][0-9][0-9]$`)

// errNoRoutes is a MOCK_CURL_ROUTES file that cannot be read: no routes,
// as in the mock.
var errNoRoutes = errors.New("no routes")

// loadRoutes parses the route file at path. A line that does not parse is
// an error naming it (the mock exits 2 on it).
func loadRoutes(path string) ([]route, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("MOCK_CURL_ROUTES must be an absolute path")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errNoRoutes
	}
	defer func() { _ = f.Close() }()
	var routes []route
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16<<20)
	// cur is the route that the indented lines below it belong to; -1
	// after a line that is not a route.
	n, cur := 0, -1
	for sc.Scan() {
		n++
		line := strings.TrimSuffix(sc.Text(), "\r")
		if line != "" && (line[0] == ' ' || line[0] == '\t') {
			h := strings.TrimLeft(line, " \t")
			if cur < 0 || h == "" {
				continue
			}
			name, value, ok := strings.Cut(h, ":")
			if !ok {
				return nil, fmt.Errorf("MOCK_CURL_ROUTES line %d: a header is Name: value", n)
			}
			routes[cur].header = append(routes[cur].header, [2]string{name, strings.TrimLeft(value, " \t")})
			continue
		}
		cur = -1
		if line == "" || line[0] == '#' {
			continue
		}
		m := routeLine.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("MOCK_CURL_ROUTES line %d: expected METHOD URL-PATTERN STATUS [BODY]", n)
		}
		rt := route{line: n, method: m[1], pattern: m[2], body: m[4]}
		switch {
		case m[3] == "unreachable":
			rt.unreachable = true
		case routeStatus.MatchString(m[3]):
			rt.status, _ = strconv.Atoi(m[3])
		default:
			return nil, fmt.Errorf("MOCK_CURL_ROUTES line %d: STATUS is a three-digit code or unreachable", n)
		}
		routes = append(routes, rt)
		cur = len(routes) - 1
	}
	if err := sc.Err(); err != nil {
		return nil, errNoRoutes
	}
	return routes, nil
}

// matches reports whether the route answers method and url.
func (rt route) matches(method, url string) bool {
	if rt.method != "*" && rt.method != method && (rt.method != "GET" || method != "HEAD") {
		return false
	}
	return globMatch(rt.pattern, url)
}

// findRoute is the first route of path that answers the request.
func findRoute(path, method, url string) (route, bool, error) {
	routes, err := loadRoutes(path)
	if err != nil {
		if errors.Is(err, errNoRoutes) {
			return route{}, false, nil
		}
		return route{}, false, err
	}
	for _, rt := range routes {
		if rt.matches(method, url) {
			return rt, true, nil
		}
	}
	return route{}, false, nil
}

// refusesTunnel reports whether the routes of path make every request to
// the origin (https://HOST[:PORT]) unreachable: the first route that can
// match a URL of the origin is an unreachable route for any method that
// matches all of them. The fake then refuses the CONNECT; when only the
// path decides, it drops the connection after the request instead.
func refusesTunnel(path, origin string) bool {
	routes, err := loadRoutes(path)
	if err != nil {
		return false
	}
	for _, rt := range routes {
		could, all := globPrefix(rt.pattern, origin+"/")
		if !could {
			continue
		}
		return rt.unreachable && rt.method == "*" && all
	}
	return false
}

// reply is the answer of the route to a request with method.
func (rt route) reply(path, method string) reply {
	if rt.unreachable {
		return reply{drop: true}
	}
	body := rt.body
	if file, ok := strings.CutPrefix(body, "@"); ok {
		if !filepath.IsAbs(file) {
			file = filepath.Join(filepath.Dir(path), file)
		}
		b, err := os.ReadFile(file)
		if err != nil {
			return routeFailure(fmt.Errorf("MOCK_CURL_ROUTES line %d: cannot read %s", rt.line, file))
		}
		body = string(b)
	}
	r := reply{status: rt.status, length: int64(len(body))}
	for _, h := range rt.header {
		if strings.EqualFold(h[0], "Content-Length") {
			if n, err := strconv.ParseInt(strings.TrimSpace(h[1]), 10, 64); err == nil && n >= 0 {
				r.length = n
			}
			continue
		}
		r.header = append(r.header, h)
	}
	if method != "HEAD" {
		r.body = body
		r.length = int64(len(body))
	}
	return r
}

// routeFailure is the answer to a request that a bad route file met: the
// mock exits 2 with the message; the fake logs it and answers 500 with it.
func routeFailure(err error) reply {
	fmt.Fprintln(os.Stderr, "httpsfake:", err)
	return reply{status: 500, header: [][2]string{{"Content-Type", "text/plain"}}, body: "httpsfake: " + err.Error() + "\n"}
}

// A glob is a Bash pattern of [[ STRING == PATTERN ]]: * (any string, /
// included), ? (any character), [...] (a set: ranges, [:class:], ! or ^
// to negate) and \ quoting the next character. Extended globs are not
// recognized.
type globTok struct {
	kind byte // 'c' a character, '?', '*', '[' a set
	r    rune
	set  func(rune) bool
}

func parseGlob(p string) []globTok {
	rs := []rune(p)
	var toks []globTok
	for i := 0; i < len(rs); i++ {
		switch c := rs[i]; c {
		case '*':
			toks = append(toks, globTok{kind: '*'})
		case '?':
			toks = append(toks, globTok{kind: '?'})
		case '\\':
			if i+1 < len(rs) {
				i++
			}
			toks = append(toks, globTok{kind: 'c', r: rs[i]})
		case '[':
			if set, n := parseSet(rs[i+1:]); set != nil {
				toks = append(toks, globTok{kind: '[', set: set})
				i += n
				continue
			}
			toks = append(toks, globTok{kind: 'c', r: c})
		default:
			toks = append(toks, globTok{kind: 'c', r: c})
		}
	}
	return toks
}

// posixClasses are the [:class:] of a set, as in a UTF-8 locale.
var posixClasses = map[string]func(rune) bool{
	"alpha":  unicode.IsLetter,
	"digit":  func(r rune) bool { return r >= '0' && r <= '9' },
	"alnum":  func(r rune) bool { return unicode.IsLetter(r) || r >= '0' && r <= '9' },
	"upper":  unicode.IsUpper,
	"lower":  unicode.IsLower,
	"space":  unicode.IsSpace,
	"blank":  func(r rune) bool { return r == ' ' || r == '\t' },
	"punct":  func(r rune) bool { return unicode.IsPunct(r) || unicode.IsSymbol(r) },
	"xdigit": func(r rune) bool { return strings.ContainsRune("0123456789abcdefABCDEF", r) },
}

// parseSet reads the set after a "[": its test and how many runes it
// took, the closing "]" included; nil when there is no closing "]" (the
// "[" is then a plain character).
func parseSet(rs []rune) (func(rune) bool, int) {
	i, neg := 0, false
	if i < len(rs) && (rs[i] == '!' || rs[i] == '^') {
		neg = true
		i++
	}
	var tests []func(rune) bool
	first := true
	for i < len(rs) {
		c := rs[i]
		if c == ']' && !first {
			return func(r rune) bool {
				for _, t := range tests {
					if t(r) {
						return !neg
					}
				}
				return neg
			}, i + 1
		}
		first = false
		if c == '[' && i+1 < len(rs) && rs[i+1] == ':' {
			if end := strings.Index(string(rs[i+2:]), ":]"); end >= 0 {
				name := string(rs[i+2:])[:end]
				if t, ok := posixClasses[name]; ok {
					tests = append(tests, t)
					i += 2 + len([]rune(name)) + 2
					continue
				}
			}
		}
		if c == '\\' && i+1 < len(rs) {
			i++
			c = rs[i]
		}
		if i+2 < len(rs) && rs[i+1] == '-' && rs[i+2] != ']' {
			lo, hi := c, rs[i+2]
			if hi == '\\' && i+3 < len(rs) {
				hi = rs[i+3]
				i++
			}
			tests = append(tests, func(r rune) bool { return r >= lo && r <= hi })
			i += 3
			continue
		}
		tests = append(tests, func(r rune) bool { return r == c })
		i++
	}
	return nil, 0
}

// closure adds to states the positions reachable by skipping stars.
func closure(toks []globTok, states []bool) {
	for i := 0; i < len(toks); i++ {
		if states[i] && toks[i].kind == '*' {
			states[i+1] = true
		}
	}
}

// globRun consumes s from the start of the pattern: the reachable
// positions (len(toks) is the end of the pattern), nil when none is left.
func globRun(toks []globTok, s string) []bool {
	states := make([]bool, len(toks)+1)
	states[0] = true
	closure(toks, states)
	for _, r := range s {
		next := make([]bool, len(toks)+1)
		any := false
		for i, on := range states[:len(toks)] {
			if !on {
				continue
			}
			t := toks[i]
			switch {
			case t.kind == '*':
				next[i] = true
			case t.kind == '?', t.kind == 'c' && t.r == r, t.kind == '[' && t.set(r):
				next[i+1] = true
			default:
				continue
			}
			any = true
		}
		if !any {
			return nil
		}
		closure(toks, next)
		states = next
	}
	return states
}

// globMatch reports whether pattern matches all of s.
func globMatch(pattern, s string) bool {
	toks := parseGlob(pattern)
	states := globRun(toks, s)
	return states != nil && states[len(toks)]
}

// globPrefix reports whether pattern matches some string that starts
// with prefix (could), and whether it matches every one of them (all).
func globPrefix(pattern, prefix string) (could, all bool) {
	toks := parseGlob(pattern)
	states := globRun(toks, prefix)
	if states == nil {
		return false, false
	}
	for i, on := range states {
		if !on {
			continue
		}
		could = true
		if i < len(toks) && toks[i].kind == '*' && onlyStars(toks[i:]) {
			all = true
		}
	}
	return could, all
}

func onlyStars(toks []globTok) bool {
	for _, t := range toks {
		if t.kind != '*' {
			return false
		}
	}
	return true
}
