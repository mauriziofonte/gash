// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package sqlguard is the read-only SQL guard of the llm_db_* functions:
// the 2.x __llm_sql_guard of lib/modules/llm/db.sh, ported state for state.
//
// One lexer, a code/quoted/line/block state machine, walks the query with
// the lexical rules of the engine that will run it and builds a copy in
// which every string, quoted identifier and comment is a single space. The
// keyword checks then run on that copy. A PostgreSQL quote opens an
// E-string, in which a backslash escapes the next character, only after an
// "e" or "E" that starts a token, as the server reads it: 2.x took the last
// letter of an identifier for one too, so the guard and the server
// disagreed on where a string ends (3.0: LEDGER DB-01 fixed). A backslash
// at the start of a line is refused before the lexer runs, and SET is a
// forbidden word.
//
// 2.x runs under the caller's locale, and the checks that use case or
// [[:space:]] read a query differently under LC_ALL=C and under a UTF-8
// locale (towupper folds "ſ" to "S", iswspace takes U+3000 as a blank).
// The guard is the strictest of both readings (LEDGER DB-13): a query is
// checked under the C reading and under UTF-8 readings, and refused when
// any refuses, with the C reading's error when it refuses. The details of
// dot_command_blocked count characters as a UTF-8 locale does (LEDGER
// CORE-16).
package sqlguard

import (
	"strings"
	"unicode/utf8"
)

// The driver names the guard knows. Any other name gets the rules shared
// by every engine and none of the engine-specific ones, as in 2.x.
const (
	MySQL  = "mysql"
	PgSQL  = "pgsql"
	SQLite = "sqlite"
)

// allowedFirstWords is __LLM_SQL_ALLOWED_FIRST: the keywords a query may start
// with.
var allowedFirstWords = []string{"SELECT", "WITH", "VALUES", "TABLE", "SHOW", "DESCRIBE", "DESC", "EXPLAIN", "PRAGMA"}

// forbiddenWords is __LLM_SQL_FORBIDDEN_WORDS, in its order (the first
// match is the one reported), with SET added at the end (3.0: LEDGER DB-01
// fixed).
var forbiddenWords = []string{
	"INSERT", "UPDATE", "DELETE", "MERGE", "UPSERT", "DROP", "CREATE", "ALTER", "TRUNCATE", "GRANT", "REVOKE",
	"RENAME", "ATTACH", "DETACH", "VACUUM", "REINDEX", "COPY", "CALL", "HANDLER", "LOCK", "UNLOCK", "INTO", "OUTFILE",
	"DUMPFILE", "LOAD_FILE", "SYS_EXEC", "SYS_EVAL", "GET_LOCK", "PG_READ_FILE", "PG_READ_BINARY_FILE",
	"PG_LS_DIR", "PG_STAT_FILE", "PG_FILE_WRITE", "PG_FILE_RENAME", "PG_FILE_UNLINK", "LO_IMPORT",
	"LO_EXPORT", "LO_UNLINK", "LO_PUT", "LO_FROM_BYTEA", "DBLINK", "DBLINK_EXEC", "DBLINK_CONNECT",
	"DBLINK_SEND_QUERY", "QUERY_TO_XML", "QUERY_TO_XMLSCHEMA", "QUERY_TO_XML_AND_XMLSCHEMA",
	"SET_CONFIG", "PG_TERMINATE_BACKEND", "PG_CANCEL_BACKEND", "PG_RELOAD_CONF",
	"PG_ROTATE_LOGFILE", "PG_PROMOTE", "PG_NOTIFY", "LOAD_EXTENSION", "READFILE", "WRITEFILE", "EDIT",
	"SLEEP", "PG_SLEEP", "PG_SLEEP_FOR", "PG_SLEEP_UNTIL", "BENCHMARK", "SET",
}

// Result is what a query that passes the guard becomes.
type Result struct {
	// Query is the query as given, cut at its trailing semicolon when it
	// has one (what follows that semicolon, comments and blanks, goes too).
	Query string
	// Kind is the first keyword, in upper case (SELECT, WITH, ...).
	Kind string
}

type lexState int

const (
	stCode lexState = iota
	stQuoted
	stLine
	stBlock
)

// GuardDriver is __llm_db_guard_driver: the guard rules for a resolved
// connection driver.
func GuardDriver(dbDriver string) string {
	switch dbDriver {
	case "mysql", "mariadb":
		return MySQL
	case "pgsql":
		return PgSQL
	}
	return SQLite
}

// Precheck is __llm_sql_precheck: the query passes when one engine's
// rules accept it (mysql, then pgsql, then sqlite). When all reject it the
// sqlite verdict is returned. internal/db uses it only when no connection
// can be resolved; otherwise Check with the connection's engine decides
// (LEDGER DB-12). As Check, it refuses when the precheck of either reading
// refuses, with the C reading's error first (LEDGER DB-13).
func Precheck(query string) error {
	for _, rd := range readings {
		if err := precheck(query, rd); err != nil {
			return err
		}
	}
	return nil
}

func precheck(query string, rd reading) error {
	if _, err := check(MySQL, query, rd); err == nil {
		return nil
	}
	if _, err := check(PgSQL, query, rd); err == nil {
		return nil
	}
	_, err := check(SQLite, query, rd)
	return err
}

// Check is __llm_sql_guard DRIVER QUERY. The error, when not nil, is an
// *Error with the 2.x code, details, action and hint.
//
// The query is refused when the C reading or a UTF-8 reading refuses it
// (see reading); the error is the first refusing reading's, the C reading
// first, so every refusal of 2.x under LC_ALL=C stays the same (LEDGER
// DB-13). An accepted query gets the C reading's result.
func Check(driver, query string) (Result, error) {
	var res Result
	for k, rd := range readings {
		r, err := check(driver, query, rd)
		if err != nil {
			return Result{}, err
		}
		if k == 0 {
			res = r
		}
	}
	return res, nil
}

// check is __llm_sql_guard under one reading of the locale classes.
func check(driver, query string, rd reading) (Result, error) {
	n := len(query)
	at := func(i int) byte { // ${query:i:1}; 0 stands for "no character"
		if i < n {
			return query[i]
		}
		return 0
	}
	// A client reads a line that starts with a backslash as one of its
	// own commands (psql's \! runs a shell): refused whatever the lexer
	// makes of the line (3.0: LEDGER DB-01 fixed, defense in depth).
	if backslashLine(query) {
		return Result{}, clone(errClientCommand)
	}
	code := make([]byte, 0, n+2)
	state := stCode
	var quote byte
	esc := false
	lastSemicolon := -1

	for i := 0; i < n; i++ {
		c := query[i]
		next := at(i + 1)
		if state == stCode {
			switch c {
			case '\'', '"', '`':
				state = stQuoted
				quote = c
				esc = false
				if driver == PgSQL && c == '\'' && eStringAt(query, i) {
					esc = true
				}
				code = append(code, ' ')
			case '[':
				if driver == SQLite {
					state = stQuoted
					quote = ']'
					esc = false
					code = append(code, ' ')
				} else {
					code = append(code, c)
				}
			case '$':
				// PostgreSQL dollar quoting: $$..$$ or $tag$..$tag$, not
				// after an identifier character (a$b, $1).
				tag := ""
				if driver == PgSQL && (i == 0 || !isTagPrev(query[i-1])) {
					tag = dollarTag(query[i:])
				}
				if tag == "" {
					code = append(code, c)
					break
				}
				rest := query[i+len(tag):]
				end := strings.Index(rest, tag)
				if end < 0 {
					return Result{}, unterminatedDollar(tag)
				}
				i += len(tag) + end + len(tag) - 1
				code = append(code, ' ')
			case '-':
				if next == '-' && (driver != MySQL || rd.commentBlankAt(query, i+2)) {
					state = stLine
					code = append(code, ' ')
				} else {
					code = append(code, c)
				}
			case '#':
				if driver == MySQL {
					state = stLine
					code = append(code, ' ')
				} else {
					code = append(code, c)
				}
			case '/':
				if next != '*' {
					code = append(code, c)
					break
				}
				if at(i+2) == '!' || ((at(i+2) == 'M' || at(i+2) == 'm') && at(i+3) == '!') {
					return Result{}, clone(errExecutableComment)
				}
				state = stBlock
				code = append(code, ' ')
				i++
			case '\\':
				return Result{}, clone(errClientCommand)
			case ';':
				lastSemicolon = i
				code = append(code, ';')
			default:
				code = append(code, c)
			}
			continue
		}

		switch state {
		case stQuoted:
			switch {
			case c == '\\' && driver == MySQL:
				return Result{}, clone(errBackslashInLiteral)
			case c == '\\' && esc:
				i++
			case c == quote:
				if next == quote {
					i++
				} else {
					state = stCode
				}
			}
		case stLine:
			if c == '\n' {
				state = stCode
				code = append(code, ' ')
			}
		case stBlock:
			if c == '*' && next == '/' {
				state = stCode
				i++
			}
		}
	}

	if state == stQuoted || state == stBlock {
		return Result{}, clone(errUnterminated)
	}

	// One statement: a single ";" is accepted only at the very end.
	trimmed := rd.trimRight(string(code))
	if strings.HasSuffix(trimmed, ";") {
		trimmed = trimmed[:len(trimmed)-1]
		query = query[:lastSemicolon]
	}
	if strings.Contains(trimmed, ";") {
		return Result{}, clone(errMultipleStatements)
	}

	// sqlite3 runs lines starting with "." as dot-commands, and sqlite3 and
	// mysql end a statement at a line holding only "go" or "/". The lines
	// are those of the query itself, strings and comments included.
	for _, line := range strings.Split(query, "\n") {
		line = rd.trimLeft(line)
		if strings.HasPrefix(line, ".") {
			return Result{}, dotCommand(firstChars(line, 24))
		}
		line = rd.trimRight(line)
		if rd.isGo(line) || line == "/" {
			return Result{}, statementEndLine(line)
		}
	}

	upper := rd.upper(trimmed)
	words := make([]byte, 0, len(upper)+2)
	words = append(words, ' ')
	for i := 0; i < len(upper); i++ {
		if c := upper[i]; isWordChar(c) {
			words = append(words, c)
		} else {
			words = append(words, ' ')
		}
	}
	words = append(words, ' ')

	first := rd.trimLeftParen(upper)
	for i := 0; i < len(first); i++ {
		if c := first[i]; !(c >= 'A' && c <= 'Z' || c == '_') {
			first = first[:i]
			break
		}
	}
	if !allowedFirst(first) {
		return Result{}, startsWith(first)
	}

	w := string(words)
	for _, f := range forbiddenWords {
		if strings.Contains(w, " "+f+" ") {
			return Result{}, containsWord(f)
		}
	}
	if rd.hasLoadData(upper) {
		return Result{}, clone(errLoadData)
	}
	if first == "PRAGMA" {
		if err := pragmaRead(upper); err != nil {
			return Result{}, err
		}
	}
	return Result{Query: query, Kind: first}, nil
}

// pragmaNoArg are the PRAGMAs that, given no value, only report one: the
// statement "PRAGMA [schema.]name" reads them. The others (optimize,
// wal_checkpoint, incremental_vacuum, shrink_memory, ...) act when they run.
var pragmaNoArg = map[string]bool{
	"ANALYSIS_LIMIT": true, "APPLICATION_ID": true, "AUTO_VACUUM": true, "AUTOMATIC_INDEX": true,
	"BUSY_TIMEOUT": true, "CACHE_SIZE": true, "CACHE_SPILL": true, "CELL_SIZE_CHECK": true,
	"CHECKPOINT_FULLFSYNC": true, "COLLATION_LIST": true, "COMPILE_OPTIONS": true, "DATA_VERSION": true,
	"DATABASE_LIST": true, "DEFER_FOREIGN_KEYS": true, "ENCODING": true, "FOREIGN_KEY_CHECK": true,
	"FOREIGN_KEYS": true, "FREELIST_COUNT": true, "FULLFSYNC": true, "FUNCTION_LIST": true,
	"HARD_HEAP_LIMIT": true, "IGNORE_CHECK_CONSTRAINTS": true, "INTEGRITY_CHECK": true, "JOURNAL_MODE": true,
	"JOURNAL_SIZE_LIMIT": true, "LEGACY_ALTER_TABLE": true, "LOCKING_MODE": true, "MAX_PAGE_COUNT": true,
	"MMAP_SIZE": true, "MODULE_LIST": true, "PAGE_COUNT": true, "PAGE_SIZE": true, "PRAGMA_LIST": true,
	"QUERY_ONLY": true, "QUICK_CHECK": true, "READ_UNCOMMITTED": true, "RECURSIVE_TRIGGERS": true,
	"REVERSE_UNORDERED_SELECTS": true, "SCHEMA_VERSION": true, "SECURE_DELETE": true, "SOFT_HEAP_LIMIT": true,
	"SYNCHRONOUS": true, "TABLE_LIST": true, "TEMP_STORE": true, "THREADS": true, "TRUSTED_SCHEMA": true,
	"USER_VERSION": true, "WAL_AUTOCHECKPOINT": true,
}

// pragmaWithArg are the PRAGMAs whose argument names what to read (a
// table, an index, a limit), not a value to set.
var pragmaWithArg = map[string]bool{
	"TABLE_INFO": true, "TABLE_XINFO": true, "TABLE_LIST": true, "INDEX_INFO": true, "INDEX_XINFO": true,
	"INDEX_LIST": true, "FOREIGN_KEY_LIST": true, "FOREIGN_KEY_CHECK": true, "INTEGRITY_CHECK": true,
	"QUICK_CHECK": true,
}

// pragmaRead accepts a PRAGMA statement only in a reading form: "PRAGMA
// [schema.]name" for a name of pragmaNoArg, or "PRAGMA [schema.]name(arg)"
// for one of pragmaWithArg, the argument a word or a string. "PRAGMA x = v"
// and "PRAGMA x(v)" set x, and some PRAGMAs act when run without a value;
// 2.x accepted them all (3.1: refused). code is the statement in upper
// case with strings and comments blanked and the final semicolon removed.
func pragmaRead(code string) error {
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(code), "PRAGMA"))
	name, rest := pragmaWord(rest)
	if r := strings.TrimLeft(rest, " \t\r\n"); strings.HasPrefix(r, ".") {
		name, rest = pragmaWord(strings.TrimLeft(r[1:], " \t\r\n"))
	}
	rest = strings.TrimSpace(rest)
	switch {
	case name == "":
		return pragmaRefused("PRAGMA without a name")
	case rest == "":
		if pragmaNoArg[name] {
			return nil
		}
		return pragmaRefused("PRAGMA " + strings.ToLower(name) + " is not a read")
	case strings.HasPrefix(rest, "("):
		arg, ok := strings.CutSuffix(rest[1:], ")")
		if ok && pragmaWithArg[name] {
			if w, tail := pragmaWord(strings.TrimSpace(arg)); strings.TrimSpace(tail) == "" && (w != "" || strings.TrimSpace(arg) == "") {
				return nil
			}
		}
	}
	return pragmaRefused("PRAGMA " + strings.ToLower(name) + " with a value")
}

// pragmaWord splits a leading name ([A-Z0-9_]) off s.
func pragmaWord(s string) (string, string) {
	i := 0
	for i < len(s) && (s[i] >= 'A' && s[i] <= 'Z' || s[i] >= '0' && s[i] <= '9' || s[i] == '_') {
		i++
	}
	return s[:i], s[i:]
}

func pragmaRefused(details string) *Error {
	return &Error{CodeWriteOperation, details, ActionFatal,
		"Only PRAGMA reads are allowed: PRAGMA name, or PRAGMA table_info(t) and the other schema lookups"}
}

func allowedFirst(first string) bool {
	if first == "" {
		return false
	}
	for _, a := range allowedFirstWords {
		if a == first {
			return true
		}
	}
	return false
}

// eStringAt reports whether the quote at query[i] opens a PostgreSQL
// E-string: it follows an "e" or "E" that starts a token. An "e" that ends
// an identifier ([A-Za-z_] or a byte above 0x7f, then [A-Za-z0-9_$] or such
// bytes) does not; one that ends a number or a parameter ($1) does, as the
// lexer of PostgreSQL 14 and older reads it (newer servers refuse the
// number, so no reading lets text through). 3.0: LEDGER DB-01 fixed.
func eStringAt(query string, i int) bool {
	if i == 0 || (query[i-1] != 'E' && query[i-1] != 'e') {
		return false
	}
	j := i - 1
	for j > 0 && isIdentCont(query[j-1]) {
		j--
	}
	if j == i-1 {
		return true // the e starts the token
	}
	c := query[j]
	return !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' || c >= 0x80)
}

// isIdentCont is the identifier continuation of the PostgreSQL lexer,
// [A-Za-z0-9_$] and every byte above 0x7f.
func isIdentCont(c byte) bool { return isTagPrev(c) || c >= 0x80 }

// backslashLine reports whether a line of the query starts with a
// backslash, after blanks.
func backslashLine(query string) bool {
	for _, line := range strings.Split(query, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " \t\r\f\v"), "\\") {
			return true
		}
	}
	return false
}

// isTagPrev is the character class [A-Za-z0-9_$] that keeps a "$" from
// opening a dollar quote.
func isTagPrev(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '$'
}

// dollarTag matches ^(\$([A-Za-z_][A-Za-z0-9_]*)?\$) and returns the tag,
// or "" when s does not start with one.
func dollarTag(s string) string {
	if len(s) < 2 || s[0] != '$' {
		return ""
	}
	j := 1
	if c := s[1]; c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' {
		j = 2
		for j < len(s) && (isTagPrev(s[j]) && s[j] != '$') {
			j++
		}
	}
	if j < len(s) && s[j] == '$' {
		return s[:j+1]
	}
	return ""
}

func isWordChar(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

// firstChars is ${s:0:n} in a UTF-8 locale: the first n characters, an
// invalid byte counting as one character and kept as it is.
func firstChars(s string, n int) string {
	i := 0
	for ; n > 0 && i < len(s); n-- {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return s[:i]
}
