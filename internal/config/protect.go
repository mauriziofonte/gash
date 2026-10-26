// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package config

import "strings"

// Protect is one PROTECT:MODE[@ALIAS]=PATH line: a path the user keeps
// from the llm_* and agent_* functions.
type Protect struct {
	// Mode is "content" (names and metadata are shown, the content is
	// never read, searched or written) or "all" (nothing: the path is
	// refused and left out of every listing).
	Mode string
	// Host is the HOST: alias the line is for; "" applies here and on
	// every host.
	Host string
	// Path is the path as written: absolute, ~ or ~/... (the home of the
	// machine it applies on).
	Path string
	Line int
}

// The PROTECT: modes.
const (
	ProtectContent = "content"
	ProtectAll     = "all"
)

// parseProtect reads the rest of a PROTECT: line (after the prefix).
func (f *File) parseProtect(lineno int, rest string) {
	key, value, ok := strings.Cut(rest, "=")
	if !ok {
		f.warnLine(lineno, "PROTECT: entry has no '=' (expected PROTECT:content=PATH or PROTECT:all=PATH), skipping")
		return
	}
	mode, host, scoped := strings.Cut(key, "@")
	if mode != ProtectContent && mode != ProtectAll {
		f.warnLine(lineno, "PROTECT: mode must be content or all (PROTECT:content=PATH, PROTECT:all@alias=PATH), skipping")
		return
	}
	if scoped && host == "" {
		f.warnLine(lineno, "PROTECT: entry has an empty host alias after @, skipping")
		return
	}
	if value != "/" && strings.HasSuffix(value, "/") {
		value = strings.TrimRight(value, "/")
		if value == "" {
			value = "/"
		}
	}
	if !strings.HasPrefix(value, "/") && value != "~" && !strings.HasPrefix(value, "~/") {
		f.warnLine(lineno, "PROTECT: path must be absolute or start with ~/ (skipping)")
		return
	}
	if strings.Contains("/"+value+"/", "/../") {
		f.warnLine(lineno, "PROTECT: path must not contain .. (skipping)")
		return
	}
	f.Protect = append(f.Protect, Protect{Mode: mode, Host: host, Path: value, Line: lineno})
}

// ProtectFor is the PROTECT: lines that apply on host alias ("" is this
// machine): the lines without a host, plus, for an alias, its own.
func (f *File) ProtectFor(alias string) []Protect {
	var out []Protect
	for _, p := range f.Protect {
		if p.Host == "" || (alias != "" && p.Host == alias) {
			out = append(out, p)
		}
	}
	return out
}

// NoBinary reports whether GASH_REMOTE_NO_BINARY keeps the gash binary
// off host alias: the value names it (aliases separated by spaces, commas
// or colons) or is "*".
func (f *File) NoBinary(alias string) bool {
	v, _ := f.Setting("GASH_REMOTE_NO_BINARY")
	for _, w := range strings.FieldsFunc(v, func(r rune) bool { return r == ' ' || r == '\t' || r == ',' || r == ':' }) {
		if w == "*" || w == alias {
			return true
		}
	}
	return false
}
