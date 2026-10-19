// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// mysqlSystemFiles are the option files the mysql client reads before the
// user's ~/.my.cnf, in its order.
var mysqlSystemFiles = []string{"/etc/my.cnf", "/etc/mysql/my.cnf"}

// mysqlSocketDefaults are the built-in socket paths of the platform
// packages, the first existing one used when no option file names one.
var mysqlSocketDefaults = []string{"/run/mysqld/mysqld.sock", "/var/run/mysqld/mysqld.sock", "/tmp/mysql.sock", "/var/lib/mysql/mysql.sock"}

// maxIncludeDepth is how deep !include and !includedir nest, as in the
// client.
const maxIncludeDepth = 10

// mysqlSocket is the Unix socket the mysql client used for host localhost
// (LEDGER DB-16): the last socket= of the [client] group of the option
// files it reads (the system files with their !include and !includedir,
// then HOME/.my.cnf), else the first existing default, else the first
// default (the connection then fails as the client's did). No other
// option-file setting applies: the DB: URL is the connection.
func (m *Module) mysqlSocket(home string) string {
	files := append([]string(nil), m.myCnfFiles...)
	if home != "" {
		files = append(files, filepath.Join(home, ".my.cnf"))
	}
	socket := ""
	for _, f := range files {
		readOptionFile(f, 0, func(group, name, value string) {
			if group == "client" && name == "socket" {
				socket = value
			}
		})
	}
	if socket != "" {
		return socket
	}
	for _, s := range m.socketDefaults {
		if _, err := os.Stat(s); err == nil {
			return s
		}
	}
	if len(m.socketDefaults) > 0 {
		return m.socketDefaults[0]
	}
	return ""
}

// readOptionFile reads a MySQL option file the way the client's
// my_default.cc does and calls set for each option of each group
// (lowercase group, option name with _ read as -, value unquoted and
// unescaped). A missing or world-writable file is skipped, as the client
// skipped it.
func readOptionFile(path string, depth int, set func(group, name, value string)) {
	if depth > maxIncludeDepth {
		return
	}
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0o002 != 0 {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	group := ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	for sc.Scan() {
		line := strings.TrimLeft(sc.Text(), " \t\r\v\f")
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if line[0] == '!' {
			include(line, depth, set)
			continue
		}
		if line[0] == '[' {
			end := strings.IndexByte(line, ']')
			if end < 0 {
				// The client stopped reading the file at a broken group line.
				return
			}
			group = strings.ToLower(strings.TrimRight(line[1:end], " \t\r\v\f"))
			continue
		}
		if group == "" {
			// An option before any group: the client refused the file.
			return
		}
		line = removeEndComment(line)
		name, value, _ := strings.Cut(line, "=")
		name = strings.ReplaceAll(strings.TrimRight(name, " \t\r\v\f"), "_", "-")
		set(group, name, optionValue(value))
	}
}

// include runs !include FILE and !includedir DIR (its .cnf files in name
// order).
func include(line string, depth int, set func(group, name, value string)) {
	rest := line[1:]
	i := strings.IndexAny(rest, " \t")
	if i < 0 {
		return
	}
	directive, arg := rest[:i], strings.TrimSpace(rest[i:])
	if arg == "" {
		return
	}
	switch directive {
	case "include":
		readOptionFile(arg, depth+1, set)
	case "includedir":
		entries, err := os.ReadDir(arg)
		if err != nil {
			return
		}
		var names []string
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".cnf") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, n := range names {
			readOptionFile(filepath.Join(arg, n), depth+1, set)
		}
	}
}

// removeEndComment cuts the line at a # outside quotes.
func removeEndComment(s string) string {
	var quote byte
	escape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c == '\'' || c == '"') && !escape {
			switch quote {
			case 0:
				quote = c
			case c:
				quote = 0
			}
		}
		if quote == 0 && c == '#' {
			return s[:i]
		}
		escape = quote != 0 && c == '\\' && !escape
	}
	return s
}

// optionValue is the value after =: blanks trimmed, one pair of matching
// quotes removed, the escapes \n \t \r \b \s \" \' \\ decoded.
func optionValue(v string) string {
	v = strings.Trim(v, " \t\r\v\f")
	if len(v) > 1 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		v = v[1 : len(v)-1]
	}
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		if v[i] != '\\' || i == len(v)-1 {
			b.WriteByte(v[i])
			continue
		}
		i++
		switch v[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case 'b':
			b.WriteByte('\b')
		case 's':
			b.WriteByte(' ')
		case '"', '\'', '\\':
			b.WriteByte(v[i])
		default:
			b.WriteByte('\\')
			b.WriteByte(v[i])
		}
	}
	return b.String()
}
