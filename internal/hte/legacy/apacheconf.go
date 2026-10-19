// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package legacy reads what hte-cli and the system Apache left on the
// machine (design/hte.md section 10): vhost files, FPM pools, the hte-cli
// certificate directory, the hosts file and Debian's envvars, through a
// platform.Sysroot. It only reads and classifies; package migrate turns
// the result into a plan. Nothing here writes, and every input is parsed
// strictly: what does not parse is reported, never guessed.
package legacy

import (
	"fmt"
	"strings"
)

// Directive is one configuration line: a directive, or the opening line of
// a section (Section true, Name without "<").
type Directive struct {
	Name    string   // as written; compare with strings.EqualFold
	Args    []string // the words, unquoted the way ap_getword_conf does
	ArgText string   // the raw text after the name (for a section, up to ">")
	Line    int      // 1-based line of the directive
	Section bool
	// Path holds the enclosing sections inside the vhost (or the file, for
	// directives outside every vhost), outermost first: "Directory",
	// "IfModule", ...
	Path []string
}

// Is reports whether d is the directive name (case-insensitive).
func (d Directive) Is(name string) bool { return strings.EqualFold(d.Name, name) }

// VirtualHost is one <VirtualHost> section.
type VirtualHost struct {
	Addrs      []string // the addresses of the opening line ("127.0.0.1:80")
	Line       int      // line of the opening tag
	EndLine    int      // line of </VirtualHost>
	Directives []Directive
}

// Find returns the directives named name, in order.
func (v VirtualHost) Find(name string) []Directive {
	var out []Directive
	for _, d := range v.Directives {
		if d.Is(name) {
			out = append(out, d)
		}
	}
	return out
}

// TLS reports whether the section turns SSLEngine on.
func (v VirtualHost) TLS() bool {
	for _, d := range v.Directives {
		if d.Is("SSLEngine") && len(d.Args) == 1 && strings.EqualFold(d.Args[0], "on") {
			return true
		}
	}
	return false
}

// Ports returns the port of each address; an address without a port is 0.
func (v VirtualHost) Ports() []int {
	out := make([]int, 0, len(v.Addrs))
	for _, a := range v.Addrs {
		out = append(out, addrPort(a))
	}
	return out
}

func addrPort(a string) int {
	i := strings.LastIndex(a, ":")
	if i < 0 || strings.HasSuffix(a, "]") {
		return 0
	}
	n := 0
	for _, c := range a[i+1:] {
		if c < '0' || c > '9' || n > 65535 {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// ConfFile is a parsed vhost file.
type ConfFile struct {
	Vhosts  []VirtualHost
	Outside []Directive // directives and sections outside every vhost
}

// ParseError locates a syntax error.
type ParseError struct {
	Line int
	Msg  string
}

func (e *ParseError) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Msg) }

// ParseConf parses an Apache configuration file the way httpd reads one:
// lines (a trailing backslash continues a line), "#" comment lines,
// <Section> nesting, and words split by ap_getword_conf. An unbalanced or
// unterminated section, a nested <VirtualHost> or an unterminated quote is
// an error.
func ParseConf(data []byte) (*ConfFile, error) {
	var (
		cf    ConfFile
		stack []string // open sections
		vh    *VirtualHost
		vhAt  int // stack depth of the open vhost
	)
	lines := strings.Split(string(data), "\n")
	for i := 0; i < len(lines); i++ {
		lineNo := i + 1
		text := strings.TrimRight(lines[i], " \t\r")
		for strings.HasSuffix(text, `\`) && i+1 < len(lines) {
			i++
			text = strings.TrimSuffix(text, `\`) + strings.TrimRight(lines[i], " \t\r")
		}
		text = strings.TrimLeft(text, " \t")
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		path := func() []string {
			if vh != nil {
				return append([]string(nil), stack[vhAt+1:]...)
			}
			return append([]string(nil), stack...)
		}
		if strings.HasPrefix(text, "</") {
			name, ok := strings.CutSuffix(strings.TrimPrefix(text, "</"), ">")
			name = strings.TrimSpace(name)
			if !ok || name == "" {
				return nil, &ParseError{lineNo, "malformed closing tag " + text}
			}
			if len(stack) == 0 || !strings.EqualFold(stack[len(stack)-1], name) {
				return nil, &ParseError{lineNo, "</" + name + "> without a matching opening section"}
			}
			stack = stack[:len(stack)-1]
			if vh != nil && len(stack) == vhAt {
				vh.EndLine = lineNo
				cf.Vhosts = append(cf.Vhosts, *vh)
				vh = nil
			}
			continue
		}
		if strings.HasPrefix(text, "<") {
			inner, ok := strings.CutSuffix(text[1:], ">")
			if !ok {
				return nil, &ParseError{lineNo, "section line does not end with >"}
			}
			name, argText, _ := strings.Cut(strings.TrimSpace(inner), " ")
			if j := strings.IndexAny(name, "\t"); j >= 0 {
				name, argText = name[:j], name[j+1:]+" "+argText
			}
			argText = strings.TrimSpace(argText)
			args, err := words(argText)
			if err != nil {
				return nil, &ParseError{lineNo, err.Error()}
			}
			d := Directive{Name: name, Args: args, ArgText: argText, Line: lineNo, Section: true, Path: path()}
			if strings.EqualFold(name, "VirtualHost") {
				if vh != nil {
					return nil, &ParseError{lineNo, "<VirtualHost> inside another <VirtualHost>"}
				}
				vh = &VirtualHost{Addrs: args, Line: lineNo}
				vhAt = len(stack)
			} else if vh != nil {
				vh.Directives = append(vh.Directives, d)
			} else {
				cf.Outside = append(cf.Outside, d)
			}
			stack = append(stack, name)
			continue
		}
		name, argText, _ := strings.Cut(text, " ")
		if j := strings.IndexAny(name, "\t"); j >= 0 {
			name, argText = name[:j], name[j+1:]+" "+argText
		}
		argText = strings.TrimSpace(argText)
		args, err := words(argText)
		if err != nil {
			return nil, &ParseError{lineNo, err.Error()}
		}
		d := Directive{Name: name, Args: args, ArgText: argText, Line: lineNo, Path: path()}
		if vh != nil {
			vh.Directives = append(vh.Directives, d)
		} else {
			cf.Outside = append(cf.Outside, d)
		}
	}
	if len(stack) > 0 {
		return nil, &ParseError{len(lines), "<" + stack[len(stack)-1] + "> is never closed"}
	}
	return &cf, nil
}

// words splits directive arguments as ap_getword_conf does: a word in
// double or single quotes may hold spaces, and inside it \\ and the
// escaped quote are unescaped; an unquoted word ends at whitespace.
func words(s string) ([]string, error) {
	var out []string
	for {
		s = strings.TrimLeft(s, " \t")
		if s == "" {
			return out, nil
		}
		if q := s[0]; q == '"' || q == '\'' {
			var b strings.Builder
			i := 1
			for ; i < len(s) && s[i] != q; i++ {
				if s[i] == '\\' && i+1 < len(s) && (s[i+1] == q || s[i+1] == '\\') {
					i++
				}
				b.WriteByte(s[i])
			}
			if i >= len(s) {
				return nil, fmt.Errorf("unterminated quote in %q", s)
			}
			out = append(out, b.String())
			s = s[i+1:]
			continue
		}
		end := strings.IndexAny(s, " \t")
		if end < 0 {
			end = len(s)
		}
		out = append(out, strings.ReplaceAll(s[:end], `\\`, `\`))
		s = s[end:]
	}
}
