// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package config reads ~/.gash_env (or GASH_ENV_FILE) with the 2.x grammar
// of lib/core/config.sh, ported rule for rule, and resolves GASH_* settings
// with the 2.x precedence: the caller's shell value (passed by the compat
// stub through GASH__SET), then the file, then the default.
//
// The package prints nothing. The warnings 2.x printed while loading are
// returned as data (File.Warnings), with the message text exactly as 2.x
// passed it to __gash_warning; the caller decides whether to show them
// (2.x discarded them in headless mode unless GASH_DEBUG=1).
//
// Whitespace is ASCII whitespace and character classes are those of the C
// locale everywhere (LEDGER CORE-08).
package config

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
)

// Options says which file to read and how 2.x would have read it.
type Options struct {
	Path     string // the file: FilePath(HOME, GASH_ENV_FILE)
	Home     string // $HOME: "~" in messages, "~" in SSH key paths
	Headless bool   // GASH_HEADLESS=1: the permission check is skipped...
	Debug    bool   // ...unless GASH_DEBUG=1
	EUID     int    // the effective uid the owner check compares with
}

// Entry is one accepted SSH:, DB:, AI: or HOST: line.
type Entry struct {
	Name  string // SSH key path (after "~" expansion), DB name, AI provider, HOST alias
	Value string // passphrase, URL, token, user@host
	Line  int    // 1-based line number
}

// Setting is one accepted GASH_NAME=value line, value unquoted.
type Setting struct {
	Name  string
	Value string
	Line  int
}

// Warning is one diagnostic 2.x printed with __gash_warning while loading.
type Warning struct {
	Line int    // line number, 0 for the permission and owner checks
	Text string // the message, e.g. "Line 3 of ~/.gash_env: unknown GASH_ setting, skipping"
	// Hint is an optional "Hint:" line printed after the warning, e.g.
	// "put the port in a Host entry of ~/.ssh/config".
	Hint string
}

// String is the warning as 2.x prints it without colors.
func (w Warning) String() string { return "Warning: " + w.Text }

// File is the parsed configuration, in file order. A repeated SSH:, DB: or
// HOST: name keeps only its first entry, with a warning for each later one
// (3.0: CORE-04 fixed), so the lists hold the entries in effect.
type File struct {
	Path    string
	Shown   string // Path as 2.x names it in messages (see ShownPath)
	Exists  bool   // a regular file (after symlinks) is there
	SSHKeys []Entry
	DB      []Entry
	AI      []Entry
	Hosts   []Entry
	// RejectedHosts are the HOST: entries skipped because their target
	// fails ValidHostTarget, so remote_* can say why an alias is unknown.
	RejectedHosts []Entry
	Settings      []Setting
	Warnings      []Warning
	// ReadErr is set when the file exists but cannot be read. 2.x then
	// printed Bash's own redirection error (text not reproducible) and
	// went on with an empty configuration, as this File does.
	ReadErr error
}

// Load reads and parses the file named by opts.Path, as __gash_load_env.
// A missing file is an empty configuration, not an error.
func Load(opts Options) *File {
	f := &File{Path: opts.Path, Shown: ShownPath(opts.Path, opts.Home)}
	fi, err := os.Stat(opts.Path)
	if err != nil || !fi.Mode().IsRegular() {
		return f
	}
	f.Exists = true
	if !opts.Headless || opts.Debug {
		f.Warnings = append(f.Warnings, permWarnings(fi, f.Shown, opts.EUID)...)
	}
	data, err := os.ReadFile(opts.Path)
	if err != nil {
		f.ReadErr = err
		return f
	}
	f.parse(data, opts.Home)
	return f
}

// Parse parses file content as __gash_load_env does, without touching the
// file system except for the SSH key existence test. path and home only
// shape the messages and the "~" expansion.
func Parse(data []byte, path, home string) *File {
	f := &File{Path: path, Shown: ShownPath(path, home), Exists: true}
	f.parse(data, home)
	return f
}

func (f *File) warn(lineno int, text string) {
	f.Warnings = append(f.Warnings, Warning{Line: lineno, Text: text})
}

func (f *File) warnLine(lineno int, text string) {
	f.warn(lineno, "Line "+strconv.Itoa(lineno)+" of "+f.Shown+": "+text)
}

func (f *File) parse(data []byte, home string) {
	r := bufio.NewReader(bytes.NewReader(data))
	lineno := 0
	for {
		raw, err := r.ReadString('\n')
		if raw == "" && err != nil {
			break
		}
		lineno++
		f.parseLine(lineno, raw, home)
		if err == io.EOF {
			break
		}
	}
}

func (f *File) parseLine(lineno int, raw, home string) {
	// read -r drops the newline and NUL bytes; then one trailing CR goes.
	line := strings.TrimSuffix(raw, "\n")
	line = strings.ReplaceAll(line, "\x00", "")
	line = strings.TrimSuffix(line, "\r")
	line = trimSpace(line)
	if line == "" || line[0] == '#' {
		return
	}

	var prefix, key, value string
	switch {
	case hasPrefixAny(line, "SSH:", "DB:", "AI:", "HOST:"):
		prefix = line[:strings.IndexByte(line, ':')]
		rest := line[len(prefix)+1:]
		eq := strings.IndexByte(rest, '=')
		if eq < 0 {
			f.warnLine(lineno, prefix+": entry has no '=' (expected "+prefix+":name=value), skipping")
			return
		}
		key, value = rest[:eq], rest[eq+1:]
		if key == "" {
			f.warnLine(lineno, prefix+": entry has an empty name, skipping")
			return
		}
	case strings.HasPrefix(line, "GASH_") && strings.Contains(line, "="):
		eq := strings.IndexByte(line, '=')
		key, value = line[:eq], settingValue(line[eq+1:])
		if !validSettingName(key) || !IsKnown(key) {
			f.warnLine(lineno, "unknown GASH_ setting, skipping")
			return
		}
		f.Settings = append(f.Settings, Setting{Name: key, Value: value, Line: lineno})
		return
	default:
		f.warnLine(lineno, "unrecognized entry (expected SSH:, DB:, AI:, HOST: or GASH_NAME=), skipping")
		return
	}

	switch prefix {
	case "SSH":
		// SSH:keypath=passphrase (an empty passphrase is allowed).
		key, ok := ExpandTilde(key, home)
		if !ok {
			f.warnLine(lineno, "SSH key path starts with ~user; write the full path (skipping)")
			return
		}
		if fi, err := os.Stat(key); err != nil || !fi.Mode().IsRegular() {
			f.warnLine(lineno, "SSH key file not found (skipping)")
			return
		}
		f.SSHKeys = f.add(f.SSHKeys, "SSH", entry(key, value, lineno))
	case "DB":
		if !hasPrefixAny(value, "mysql://", "mariadb://", "pgsql://") {
			f.warnLine(lineno, "DB URL must start with mysql://, mariadb:// or pgsql:// (skipping)")
			return
		}
		f.DB = f.add(f.DB, "DB", entry(key, value, lineno))
	case "AI":
		if key != "claude" && key != "gemini" {
			f.warnLine(lineno, "unknown AI provider (supported: claude, gemini), skipping")
			return
		}
		if value == "" {
			f.warnLine(lineno, "AI provider "+key+" has an empty token (skipping)")
			return
		}
		f.AI = append(f.AI, entry(key, value, lineno))
	case "HOST":
		if !ValidHostTarget(value) {
			f.warnLine(lineno, "HOST target must be user@host (skipping)")
			if strings.Contains(value, ":") {
				f.Warnings[len(f.Warnings)-1].Hint = "put the port in a Host entry of ~/.ssh/config"
			}
			f.RejectedHosts = append(f.RejectedHosts, entry(key, value, lineno))
			return
		}
		f.Hosts = f.add(f.Hosts, "HOST", entry(key, value, lineno))
	}
}

// add appends e to list unless an entry of that name is there already: the
// later one is then ignored, with "Line N of FILE: duplicate DB:NAME,
// ignored (first is at line M)" (LEDGER CORE-04). 2.x kept both, used the
// first and said nothing.
func (f *File) add(list []Entry, prefix string, e Entry) []Entry {
	for _, o := range list {
		if o.Name == e.Name {
			f.warnLine(e.Line, "duplicate "+prefix+":"+e.Name+", ignored (first is at line "+strconv.Itoa(o.Line)+")")
			return list
		}
	}
	return append(list, e)
}

// entry stores NAME and VALUE the way 2.x did, as one "NAME<TAB>VALUE" line
// read back by splitting at the first tab: a tab inside NAME moves the rest
// of NAME into the value.
func entry(name, value string, lineno int) Entry {
	if i := strings.IndexByte(name, '\t'); i >= 0 {
		value = name[i+1:] + "\t" + value
		name = name[:i]
	}
	return Entry{Name: name, Value: value, Line: lineno}
}

// settingValue unquotes the value of a GASH_NAME= line as 2.x does.
//
// Quoted ("..." or '...'): the text up to the first matching quote, when
// only whitespace or a "# comment" follows it; otherwise the raw text,
// quotes included. Unquoted: cut at the first whitespace followed by "#",
// then trailing whitespace trimmed.
func settingValue(value string) string {
	if value != "" && (value[0] == '"' || value[0] == '\'') {
		q := value[0]
		inner := value[1:]
		if i := strings.IndexByte(inner, q); i >= 0 {
			rest := trimLeftSpace(inner[i+1:])
			if rest == "" || rest[0] == '#' {
				return inner[:i]
			}
		}
		return value
	}
	for i := 0; i+1 < len(value); i++ {
		if isSpace(value[i]) && value[i+1] == '#' {
			value = value[:i]
			break
		}
	}
	return trimRightSpace(value)
}

// validSettingName is ^GASH_[A-Z0-9_]+$.
func validSettingName(key string) bool {
	if !strings.HasPrefix(key, "GASH_") || len(key) == len("GASH_") {
		return false
	}
	for i := len("GASH_"); i < len(key); i++ {
		c := key[i]
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

// ValidHostTarget reports whether v is a HOST: target remote_* accepts,
// the rule of remote.validTarget: user@host with a user of
// [A-Za-z0-9._][A-Za-z0-9._-]* and a host name of
// [A-Za-z0-9][A-Za-z0-9.-]* or a bracketed IPv6 literal ([::1]). A port
// (host:22), an option-like word or any other character is refused, so a
// HOST: line the remote functions would reject is skipped at load with
// its warning (LEDGER REM-02, REM-08).
func ValidHostTarget(v string) bool {
	user, host, ok := strings.Cut(v, "@")
	if !ok || !targetWord(user, "._", "._-") {
		return false
	}
	if targetWord(host, "", ".-") {
		return true
	}
	if len(host) > 2 && host[0] == '[' && host[len(host)-1] == ']' {
		return net.ParseIP(host[1:len(host)-1]) != nil && strings.Contains(host, ":")
	}
	return false
}

// targetWord reports whether s is one ASCII letter or digit (or a byte of
// first) followed by letters, digits and bytes of rest: no regexp, so the
// package compiles nothing at start.
func targetWord(s, first, rest string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		alnum := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
		extra := rest
		if i == 0 {
			extra = first
		}
		if !alnum && strings.IndexByte(extra, c) < 0 {
			return false
		}
	}
	return true
}

func hasPrefixAny(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func trimLeftSpace(s string) string {
	i := 0
	for i < len(s) && isSpace(s[i]) {
		i++
	}
	return s[i:]
}

func trimRightSpace(s string) string {
	i := len(s)
	for i > 0 && isSpace(s[i-1]) {
		i--
	}
	return s[:i]
}

func trimSpace(s string) string { return trimRightSpace(trimLeftSpace(s)) }

func first(list []Entry, name string) (string, bool) {
	for _, e := range list {
		if e.Name == name {
			return e.Value, true
		}
	}
	return "", false
}

// DBURL is __gash_get_db_url: the URL of the first DB: entry named name
// ("" means "default").
func (f *File) DBURL(name string) (string, bool) {
	if name == "" {
		name = "default"
	}
	return first(f.DB, name)
}

// AIToken is __gash_get_ai_token: the token of the first AI: entry for
// provider (an empty provider finds nothing).
func (f *File) AIToken(provider string) (string, bool) {
	if provider == "" {
		return "", false
	}
	return first(f.AI, provider)
}

// FirstAIProvider is __gash_get_first_ai_provider.
func (f *File) FirstAIProvider() (string, bool) {
	if len(f.AI) == 0 {
		return "", false
	}
	return f.AI[0].Name, true
}

// HostTarget is __gash_get_host_target: user@host of the first HOST: entry
// named alias (an empty alias finds nothing).
func (f *File) HostTarget(alias string) (string, bool) {
	if alias == "" {
		return "", false
	}
	return first(f.Hosts, alias)
}

// RejectedHost is the target of the first HOST: line for alias that was
// skipped because it fails ValidHostTarget (LEDGER REM-02, REM-08).
func (f *File) RejectedHost(alias string) (string, bool) {
	if alias == "" {
		return "", false
	}
	return first(f.RejectedHosts, alias)
}

// Setting is __gash_get_setting: the value of the first GASH_NAME= line
// for name, read from the file only.
func (f *File) Setting(name string) (string, bool) {
	for _, s := range f.Settings {
		if s.Name == name {
			return s.Value, true
		}
	}
	return "", false
}

// DBListRow is one row of gash_db_list.
type DBListRow struct {
	Name string
	URL  string // masked with MaskDBURL
}

// DBList is what gash_db_list prints, one row per DB: entry in effect
// (a repeated name is listed once, 3.0: CORE-04 fixed).
func (f *File) DBList() []DBListRow {
	rows := make([]DBListRow, 0, len(f.DB))
	for _, e := range f.DB {
		rows = append(rows, DBListRow{Name: e.Name, URL: MaskDBURL(e.Value)})
	}
	return rows
}
