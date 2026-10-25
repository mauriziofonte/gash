// Gash 3.1. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package lazyre is a regular expression compiled on first use. A package
// level regexp.MustCompile runs at process start, in every invocation of
// the binary, whether or not the function that needs it runs; these
// compile once, the first time a method is called, and then behave as the
// *regexp.Regexp they wrap.
package lazyre

import (
	"regexp"
	"sync"
)

// Regexp is a regexp.Regexp compiled on first use.
type Regexp struct {
	src   string
	posix bool
	once  sync.Once
	re    *regexp.Regexp
}

// MustCompile is regexp.MustCompile, deferred to the first use (a bad
// pattern panics there instead of at start).
func MustCompile(src string) *Regexp { return &Regexp{src: src} }

// MustCompilePOSIX is regexp.MustCompilePOSIX, deferred to the first use.
func MustCompilePOSIX(src string) *Regexp { return &Regexp{src: src, posix: true} }

// Compiled wraps an expression already compiled.
func Compiled(re *regexp.Regexp) *Regexp {
	l := &Regexp{src: re.String(), re: re}
	l.once.Do(func() {})
	return l
}

// R returns the compiled expression.
func (l *Regexp) R() *regexp.Regexp {
	l.once.Do(func() {
		if l.posix {
			l.re = regexp.MustCompilePOSIX(l.src)
		} else {
			l.re = regexp.MustCompile(l.src)
		}
	})
	return l.re
}

func (l *Regexp) String() string                             { return l.src }
func (l *Regexp) Match(b []byte) bool                        { return l.R().Match(b) }
func (l *Regexp) MatchString(s string) bool                  { return l.R().MatchString(s) }
func (l *Regexp) Find(b []byte) []byte                       { return l.R().Find(b) }
func (l *Regexp) FindIndex(b []byte) []int                   { return l.R().FindIndex(b) }
func (l *Regexp) FindString(s string) string                 { return l.R().FindString(s) }
func (l *Regexp) FindStringIndex(s string) []int             { return l.R().FindStringIndex(s) }
func (l *Regexp) FindSubmatch(b []byte) [][]byte             { return l.R().FindSubmatch(b) }
func (l *Regexp) FindSubmatchIndex(b []byte) []int           { return l.R().FindSubmatchIndex(b) }
func (l *Regexp) FindStringSubmatch(s string) []string       { return l.R().FindStringSubmatch(s) }
func (l *Regexp) FindStringSubmatchIndex(s string) []int     { return l.R().FindStringSubmatchIndex(s) }
func (l *Regexp) FindAll(b []byte, n int) [][]byte           { return l.R().FindAll(b, n) }
func (l *Regexp) FindAllIndex(b []byte, n int) [][]int       { return l.R().FindAllIndex(b, n) }
func (l *Regexp) FindAllString(s string, n int) []string     { return l.R().FindAllString(s, n) }
func (l *Regexp) FindAllStringIndex(s string, n int) [][]int { return l.R().FindAllStringIndex(s, n) }
func (l *Regexp) FindAllSubmatch(b []byte, n int) [][][]byte { return l.R().FindAllSubmatch(b, n) }
func (l *Regexp) FindAllSubmatchIndex(b []byte, n int) [][]int {
	return l.R().FindAllSubmatchIndex(b, n)
}
func (l *Regexp) FindAllStringSubmatch(s string, n int) [][]string {
	return l.R().FindAllStringSubmatch(s, n)
}
func (l *Regexp) FindAllStringSubmatchIndex(s string, n int) [][]int {
	return l.R().FindAllStringSubmatchIndex(s, n)
}
func (l *Regexp) ReplaceAll(src, repl []byte) []byte { return l.R().ReplaceAll(src, repl) }
func (l *Regexp) ReplaceAllLiteral(src, repl []byte) []byte {
	return l.R().ReplaceAllLiteral(src, repl)
}
func (l *Regexp) ReplaceAllString(src, repl string) string { return l.R().ReplaceAllString(src, repl) }
func (l *Regexp) ReplaceAllLiteralString(src, repl string) string {
	return l.R().ReplaceAllLiteralString(src, repl)
}
func (l *Regexp) ReplaceAllFunc(src []byte, repl func([]byte) []byte) []byte {
	return l.R().ReplaceAllFunc(src, repl)
}
func (l *Regexp) ReplaceAllStringFunc(src string, repl func(string) string) string {
	return l.R().ReplaceAllStringFunc(src, repl)
}
func (l *Regexp) Split(s string, n int) []string { return l.R().Split(s, n) }
func (l *Regexp) SubexpNames() []string          { return l.R().SubexpNames() }
func (l *Regexp) SubexpIndex(name string) int    { return l.R().SubexpIndex(name) }
func (l *Regexp) NumSubexp() int                 { return l.R().NumSubexp() }
