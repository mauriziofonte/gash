// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sqlguard

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// vector is one guard input and its 2.x verdict: Code is "" when the query
// passes, with Query and Kind as stored; otherwise Details (when not "")
// is checked too.
type vector struct {
	name    string
	driver  string
	query   string
	code    string
	details string
	stored  string // the stored query when it differs from the input
	kind    string
}

func runVectors(t *testing.T, vectors []vector) {
	t.Helper()
	for _, v := range vectors {
		res, err := Check(v.driver, v.query)
		if v.code == "" {
			if err != nil {
				t.Errorf("%s: Check(%s, %q) = %v, want it accepted", v.name, v.driver, v.query, err)
				continue
			}
			stored := v.stored
			if stored == "" {
				stored = v.query
			}
			if res.Query != stored || res.Kind != v.kind {
				t.Errorf("%s: Check(%s, %q) = %+v, want query %q kind %q", v.name, v.driver, v.query, res, stored, v.kind)
			}
			continue
		}
		var e *Error
		if !errors.As(err, &e) {
			t.Errorf("%s: Check(%s, %q) = %+v, %v; want %s", v.name, v.driver, v.query, res, err, v.code)
			continue
		}
		if e.Code != v.code || (v.details != "" && e.Details != v.details) {
			t.Errorf("%s: Check(%s, %q) = %s %q, want %s %q", v.name, v.driver, v.query, e.Code, e.Details, v.code, v.details)
		}
	}
}

// llm_db_security_spec.sh:9-35: client commands, comment tricks and second
// statements are refused.
func TestSpecRefusals(t *testing.T) {
	runVectors(t, []vector{
		{name: "mysql client command", driver: MySQL, query: `SELECT 1 \! id`, code: CodeClientCommand},
		{name: "backslash in a backtick identifier", driver: MySQL, query: "SELECT `a\\` , 'x` \\! id #'", code: CodeBackslashInLiteral},
		{name: "executable comment", driver: MySQL, query: "/*!50000 DROP TABLE t */ SELECT 1", code: CodeExecutableComment, details: "/*! ... */"},
		{name: "MariaDB executable comment", driver: MySQL, query: "/*M!100101 DROP TABLE t */ SELECT 1", code: CodeExecutableComment},
		{name: "second statement", driver: MySQL, query: "SELECT 1; SELECT 2", code: CodeMultipleStatements, details: "Only single SQL statements allowed"},
		{name: "INTO OUTFILE", driver: MySQL, query: "SELECT * FROM t INTO OUTFILE '/tmp/x'", code: CodeWriteOperation, details: "Statement contains 'INTO'"},
		{name: "FOR UPDATE", driver: MySQL, query: "SELECT * FROM t FOR UPDATE", code: CodeWriteOperation, details: "Statement contains 'UPDATE'"},
		{name: "dollar quotes hide nothing", driver: PgSQL, query: "SELECT $$'$$, pg_read_file('/etc/passwd') -- '", code: CodeWriteOperation, details: "Statement contains 'PG_READ_FILE'"},
		{name: "server control function", driver: PgSQL, query: "SELECT pg_terminate_backend(1)", code: CodeWriteOperation, details: "Statement contains 'PG_TERMINATE_BACKEND'"},
		{name: "unterminated dollar quote", driver: PgSQL, query: "SELECT $t$ unterminated", code: CodeUnterminated, details: "Dollar-quoted string $t$ is not closed"},
		{name: "sqlite dot-command", driver: SQLite, query: "SELECT 1\n.shell id", code: CodeDotCommand, details: ".shell id"},
		{name: "go line", driver: SQLite, query: "SELECT 1\ngo\nSELECT 2", code: CodeMultipleStatements, details: "Line 'go' ends a statement in the database client"},
		{name: "ATTACH", driver: SQLite, query: "ATTACH '/tmp/x.db' AS x", code: CodeWriteOperation, details: "Statement starts with 'ATTACH'"},
	})
}

// llm_db_security_spec.sh:37-52: ordinary read queries pass.
func TestSpecAccepts(t *testing.T) {
	runVectors(t, []vector{
		{name: "doubled quote and comment", driver: MySQL, query: "SELECT 'it''s' -- a comment", kind: "SELECT"},
		{name: "trailing semicolon", driver: MySQL, query: "SELECT 1;", stored: "SELECT 1", kind: "SELECT"},
		{name: "E-string and plain string", driver: PgSQL, query: `SELECT E'a\'b', 'C:\'`, kind: "SELECT"},
		{name: "dollar-quoted literal", driver: PgSQL, query: "SELECT $$ DELETE ; $$ AS literal", kind: "SELECT"},
		{name: "bracket identifier", driver: SQLite, query: "SELECT replace(name, 'a', 'b') FROM [odd'name]", kind: "SELECT"},
		{name: "WITH", driver: SQLite, query: "WITH a AS (SELECT 1) SELECT * FROM a", kind: "WITH"},
		{name: "trailing semicolon dropped from the stored query", driver: SQLite, query: "SELECT 1 ;  ", stored: "SELECT 1 ", kind: "SELECT"},
	})
}

// llm_spec.sh:493-672, 833-850 and error_contract_spec.sh:31: llm_db_query
// and llm_db_explain refuse writes before any connection (Precheck).
func TestSpecPrecheck(t *testing.T) {
	tests := []struct{ query, code, details string }{
		{"INSERT INTO users VALUES (1)", CodeWriteOperation, "Statement starts with 'INSERT'"},
		{"DELETE FROM users", CodeWriteOperation, "Statement starts with 'DELETE'"},
		{"DROP TABLE users", CodeWriteOperation, "Statement starts with 'DROP'"},
		{"SELECT 1; DROP TABLE users", CodeMultipleStatements, "Only single SQL statements allowed"},
		{"UPDATE users SET name = 1", CodeWriteOperation, "Statement starts with 'UPDATE'"},
		{"TRUNCATE TABLE users", CodeWriteOperation, "Statement starts with 'TRUNCATE'"},
		{"DROP TABLE x", CodeWriteOperation, "Statement starts with 'DROP'"},
	}
	for _, tt := range tests {
		err := Precheck(tt.query)
		var e *Error
		if !errors.As(err, &e) || e.Code != tt.code || e.Details != tt.details {
			t.Errorf("Precheck(%q) = %v, want %s %q", tt.query, err, tt.code, tt.details)
			continue
		}
		// error_contract_spec.sh: recoverable is false exactly for FATAL.
		if e.Action != ActionFatal || e.Recoverable() {
			t.Errorf("Precheck(%q): action %s recoverable %v", tt.query, e.Action, e.Recoverable())
		}
	}
	for _, q := range []string{"SELECT 1", "SELECT 'a\\' FROM t", "SELECT `a\\`", "SELECT 1 # x; DELETE"} {
		if err := Precheck(q); err != nil {
			t.Errorf("Precheck(%q) = %v, want accepted by one engine", q, err)
		}
	}
}

// LEDGER DB-12: when every engine rejects the query, Precheck reports the
// sqlite verdict. 3.0: DB-12 fixed in internal/db, which uses Precheck only
// when no connection resolves (see internal/db TestLedger_DB12).
func TestLedger_DB12(t *testing.T) {
	q := "SELECT [a'] ; x"
	for _, d := range []string{MySQL, PgSQL} {
		if _, err := Check(d, q); err == nil || err.(*Error).Code != CodeUnterminated {
			t.Fatalf("Check(%s, %q) = %v, want %s", d, q, err, CodeUnterminated)
		}
	}
	err := Precheck(q)
	if e, ok := err.(*Error); !ok || e.Code != CodeMultipleStatements {
		t.Errorf("Precheck(%q) = %v, want the sqlite verdict %s", q, err, CodeMultipleStatements)
	}
}

// LEDGER DB-01: the 2.x vectors. 2.x opened an E-string at any quote after
// "e" or "E", also the last letter of an identifier, so a backslash kept
// the guard's string open past the quote that PostgreSQL closes. 3.0:
// DB-01 fixed: the identifier's quote is a plain string, a line that
// starts with a backslash is refused before lexing, and SET is a forbidden
// word, so none of these shapes passes any more.
func TestLedger_DB01_2x(t *testing.T) {
	runVectors(t, []vector{
		{name: "DB-01 identifier ending in e hides a backslash line", driver: PgSQL,
			query: "SELECT 1 WHERE somee'\\'\n\\1\n'", code: CodeClientCommand, details: "Backslash outside a string literal"},
		{name: "DB-01 identifier ending in E", driver: PgSQL,
			query: "SELECT 1 WHERE somE'\\'\n\\1\n'", code: CodeClientCommand},
		{name: "DB-01 control: another letter closes the string", driver: PgSQL,
			query: "SELECT 1 WHERE somex'\\'\n\\1\n'", code: CodeClientCommand, details: "Backslash outside a string literal"},
		{name: "DB-01 identifier ending in e hides a forbidden word", driver: PgSQL,
			query: "SELECT 1 WHERE somee'\\' DELETE FROM t --'", code: CodeWriteOperation, details: "Statement contains 'DELETE'"},
		{name: "DB-01 control: the forbidden word is seen", driver: PgSQL,
			query: "SELECT 1 WHERE somex'\\' DELETE FROM t --'", code: CodeWriteOperation, details: "Statement contains 'DELETE'"},
		{name: "DB-01 SET after the identifier's string", driver: PgSQL,
			query: "SELECT 1 WHERE somee'\\' SET x = 1 '", code: CodeUnterminated},
		{name: "DB-01 mysql refuses the backslash line", driver: MySQL,
			query: "SELECT 1 WHERE somee'\\'\n\\1\n'", code: CodeClientCommand},
		{name: "DB-01 sqlite closes the string", driver: SQLite,
			query: "SELECT 1 WHERE somee'\\'\n\\1\n'", code: CodeClientCommand},
		{name: "DB-01 a real E-string escapes the quote", driver: PgSQL,
			query: "SELECT E'\\'' FROM t", kind: "SELECT"},
	})
}

// LEDGER DB-01, 3.0 fix: a quote is an E-string opener only after an "e"
// that starts a token; a backslash line is a client command wherever the
// lexer stands; SET is a forbidden word. The vectors are harmless shapes
// (a backslash line that is no psql command, a forbidden word, SET).
func TestLedger_DB01_fix(t *testing.T) {
	runVectors(t, []vector{
		// The identifier-then-quote-then-backslash-line shape.
		{name: "identifier e, backslash line", driver: PgSQL,
			query: "SELECT 1 WHERE somee'\\'\n\\1\n'", code: CodeClientCommand},
		{name: "identifier with a non-ASCII letter", driver: PgSQL,
			query: "SELECT caf\u00e9e'\\' DELETE FROM t --'", code: CodeWriteOperation, details: "Statement contains 'DELETE'"},
		{name: "identifier with digits and $", driver: PgSQL,
			query: "SELECT a1$e'\\' DELETE FROM t --'", code: CodeWriteOperation, details: "Statement contains 'DELETE'"},
		// An e that starts a token still opens an E-string.
		{name: "standalone E", driver: PgSQL, query: "SELECT E'\\' DELETE ' FROM t", kind: "SELECT"},
		{name: "E after a parenthesis", driver: PgSQL, query: "SELECT (E'\\'')", kind: "SELECT"},
		{name: "E after a quoted identifier", driver: PgSQL, query: "SELECT \"x\"E'\\' DELETE '", kind: "SELECT"},
		{name: "E after a number", driver: PgSQL, query: "SELECT 1E'\\' DELETE '", kind: "SELECT"},
		{name: "E after a parameter", driver: PgSQL, query: "SELECT $1E'\\' DELETE '", kind: "SELECT"},
		{name: "E after a dollar quote", driver: PgSQL, query: "SELECT $$a$$E'\\' DELETE '", kind: "SELECT"},
		// A backslash line is refused in a string, in a comment, after blanks.
		{name: "backslash line in a string", driver: PgSQL, query: "SELECT 'a\n\\b'", code: CodeClientCommand},
		{name: "backslash line in a comment", driver: PgSQL, query: "SELECT 1 /*\n  \\x */", code: CodeClientCommand},
		{name: "backslash line after blanks", driver: SQLite, query: "SELECT 1\n \t\\x", code: CodeClientCommand},
		{name: "backslash inside a line is still a literal", driver: PgSQL, query: "SELECT 'C:\\x'", kind: "SELECT"},
		// SET is a forbidden word.
		{name: "SET anywhere", driver: PgSQL, query: "SELECT 1 AS set", code: CodeWriteOperation, details: "Statement contains 'SET'"},
		{name: "SET first", driver: PgSQL, query: "SET search_path = x", code: CodeWriteOperation, details: "Statement starts with 'SET'"},
		{name: "SET in a string is fine", driver: PgSQL, query: "SELECT 'SET x'", kind: "SELECT"},
		{name: "SETOF is another word", driver: PgSQL, query: "SELECT 1 AS setof", kind: "SELECT"},
	})
}

// SET is refused as the first keyword and, since 3.0, anywhere as a word
// (3.0: DB-01 fixed; 2.x let it pass after the first keyword).
func TestSetNotForbidden(t *testing.T) {
	runVectors(t, []vector{
		{name: "SET first", driver: PgSQL, query: "SET search_path = x", code: CodeWriteOperation, details: "Statement starts with 'SET'"},
		{name: "SET later", driver: PgSQL, query: "SELECT 1 AS set", code: CodeWriteOperation, details: "Statement contains 'SET'"},
		{name: "SET_CONFIG is forbidden", driver: PgSQL, query: "SELECT set_config('a', 'b', false)", code: CodeWriteOperation, details: "Statement contains 'SET_CONFIG'"},
	})
}

// The lexer rules engine by engine, each verdict checked against the 2.x
// function.
func TestLexer(t *testing.T) {
	runVectors(t, []vector{
		// -- needs a blank or the end after it in mysql only.
		{name: "mysql --x is code", driver: MySQL, query: "SELECT 1 --x", kind: "SELECT"},
		{name: "mysql --; is code", driver: MySQL, query: "SELECT 1 --; DELETE", code: CodeMultipleStatements},
		{name: "mysql -- at the end", driver: MySQL, query: "SELECT 1 --", kind: "SELECT"},
		{name: "mysql --tab", driver: MySQL, query: "SELECT 1 --\t; DELETE", kind: "SELECT"},
		{name: "pgsql --; is a comment", driver: PgSQL, query: "SELECT 1 --; DELETE", kind: "SELECT"},
		{name: "other driver --; is a comment", driver: "other", query: "SELECT 1 --; DELETE", kind: "SELECT"},
		{name: "comment ends at the newline", driver: PgSQL, query: "SELECT 1 -- x\n; DELETE", code: CodeMultipleStatements},
		// # is a comment in mysql only.
		{name: "mysql #", driver: MySQL, query: "SELECT 1 # ; DELETE", kind: "SELECT"},
		{name: "pgsql #", driver: PgSQL, query: "SELECT 1 # ; DELETE", code: CodeMultipleStatements},
		// [..] quotes in sqlite only.
		{name: "sqlite [..]", driver: SQLite, query: "SELECT [a;b]", kind: "SELECT"},
		{name: "sqlite doubled ]]", driver: SQLite, query: "SELECT [a]]b]", kind: "SELECT"},
		{name: "mysql [..] is code", driver: MySQL, query: "SELECT [a;b]", code: CodeMultipleStatements},
		{name: "other driver [..] is code", driver: "other", query: "SELECT [a;b]", code: CodeMultipleStatements},
		// Dollar quotes: pgsql only, not after an identifier character.
		{name: "pgsql $$", driver: PgSQL, query: "SELECT $$; DELETE $$", kind: "SELECT"},
		{name: "pgsql $tag$", driver: PgSQL, query: "SELECT $tag$ a $ b $tag$", kind: "SELECT"},
		{name: "pgsql a$$ is code", driver: PgSQL, query: "SELECT a$$; DELETE", code: CodeMultipleStatements},
		{name: "pgsql a$$ is code, a later $$ opens", driver: PgSQL, query: "SELECT a$$; DELETE $$", code: CodeUnterminated, details: "Dollar-quoted string $$ is not closed"},
		{name: "pgsql $1 is code", driver: PgSQL, query: "SELECT $1", kind: "SELECT"},
		{name: "pgsql $ after $ is code", driver: PgSQL, query: "SELECT 1$$; x", code: CodeMultipleStatements},
		{name: "pgsql unterminated $tag$", driver: PgSQL, query: "SELECT $tag$ a $tag", code: CodeUnterminated, details: "Dollar-quoted string $tag$ is not closed"},
		{name: "mysql $$ is code", driver: MySQL, query: "SELECT $$; DELETE $$", code: CodeMultipleStatements},
		{name: "query starting with a dollar quote", driver: PgSQL, query: "$$x$$", code: CodeWriteOperation, details: "Statement starts with '?'"},
		// Quotes.
		{name: "mysql backslash in quotes", driver: MySQL, query: `SELECT 'a\'`, code: CodeBackslashInLiteral, details: "Backslash inside a quoted string or identifier"},
		{name: "pgsql backslash in a plain string", driver: PgSQL, query: `SELECT 'a\'`, kind: "SELECT"},
		{name: "pgsql E-string escaped quote", driver: PgSQL, query: `SELECT E'a\'`, code: CodeUnterminated, details: "A string, identifier or comment is not closed"},
		{name: "pgsql E-string doubled quote", driver: PgSQL, query: "SELECT E'it''s'", kind: "SELECT"},
		{name: "pgsql E-string escaped backslash", driver: PgSQL, query: `SELECT E'\\'`, kind: "SELECT"},
		{name: "pgsql E before a double quote", driver: PgSQL, query: `SELECT E"a\" FROM t`, kind: "SELECT"},
		{name: "mysql doubled double quote", driver: MySQL, query: `SELECT "a""b"`, kind: "SELECT"},
		{name: "backtick in pgsql", driver: PgSQL, query: "SELECT `a;b`", kind: "SELECT"},
		{name: "unterminated quote", driver: SQLite, query: "SELECT 'a", code: CodeUnterminated},
		// Block comments and executable comments.
		{name: "block comment", driver: MySQL, query: "SELECT 1 /* x ; */", kind: "SELECT"},
		{name: "/*/ does not close", driver: MySQL, query: "SELECT 1 /*/ x */", kind: "SELECT"},
		{name: "unterminated block", driver: PgSQL, query: "SELECT 1 /* x", code: CodeUnterminated},
		{name: "executable comment in every driver", driver: SQLite, query: "SELECT /*!*/ 1", code: CodeExecutableComment},
		{name: "lower-case m!", driver: PgSQL, query: "SELECT /*m!x*/1", code: CodeExecutableComment},
		{name: "M without !", driver: MySQL, query: "SELECT /*M x*/ 1", kind: "SELECT"},
		{name: "executable comment inside a string", driver: MySQL, query: "SELECT '/*!'", kind: "SELECT"},
		// Backslash in code.
		{name: "backslash in code, sqlite", driver: SQLite, query: `SELECT 1 \`, code: CodeClientCommand},
		{name: "backslash in code, other driver", driver: "other", query: `SELECT 1 \g`, code: CodeClientCommand},
		{name: "backslash in a comment", driver: PgSQL, query: "SELECT 1 -- \\!", kind: "SELECT"},
	})
}

func TestStatements(t *testing.T) {
	runVectors(t, []vector{
		{name: "two semicolons", driver: SQLite, query: "SELECT 1;;", code: CodeMultipleStatements},
		{name: "cut after the semicolon", driver: SQLite, query: "SELECT 1; -- c", stored: "SELECT 1", kind: "SELECT"},
		{name: "semicolon then newline", driver: SQLite, query: "SELECT 1;\n", stored: "SELECT 1", kind: "SELECT"},
		{name: "semicolon in a string", driver: SQLite, query: "SELECT ';'", kind: "SELECT"},
		{name: "lines after the cut are not checked", driver: PgSQL, query: "SELECT 1; /*\n.x */", stored: "SELECT 1", kind: "SELECT"},
		// Lines.
		{name: "indented dot line", driver: MySQL, query: "SELECT 1\n \t.shell id", code: CodeDotCommand, details: ".shell id"},
		{name: "dot line inside a string", driver: SQLite, query: "SELECT 'a\n.b'", code: CodeDotCommand, details: ".b'"},
		{name: "dot first", driver: PgSQL, query: ".tables", code: CodeDotCommand, details: ".tables"},
		{name: "Go line, trimmed", driver: MySQL, query: "SELECT 1\n  Go  ", code: CodeMultipleStatements, details: "Line 'Go' ends a statement in the database client"},
		{name: "slash line", driver: PgSQL, query: "SELECT 1\n/", code: CodeMultipleStatements, details: "Line '/' ends a statement in the database client"},
		{name: "slash line with CR", driver: PgSQL, query: "SELECT 1\r\n/\r", code: CodeMultipleStatements, details: "Line '/' ends a statement in the database client"},
		{name: "go inside a comment", driver: MySQL, query: "SELECT 1 /*\ngo\n*/", code: CodeMultipleStatements},
		{name: "go with more text", driver: MySQL, query: "SELECT 1\n go x", kind: "SELECT"},
		// First keyword.
		{name: "lower case", driver: SQLite, query: "select 1", kind: "SELECT"},
		{name: "parentheses and blanks", driver: SQLite, query: " \t((select 1))", kind: "SELECT"},
		{name: "keyword run", driver: SQLite, query: "SELECTX 1", code: CodeWriteOperation, details: "Statement starts with 'SELECTX'"},
		{name: "keyword stops at a digit", driver: SQLite, query: "SELECT1", kind: "SELECT"},
		{name: "no keyword", driver: SQLite, query: "1", code: CodeWriteOperation, details: "Statement starts with '?'"},
		{name: "empty", driver: SQLite, query: "", code: CodeWriteOperation, details: "Statement starts with '?'"},
		{name: "comment first", driver: SQLite, query: "/* c */ SELECT 1", kind: "SELECT"},
		{name: "line comment first", driver: PgSQL, query: "-- c\nSELECT 1", kind: "SELECT"},
		{name: "DESC", driver: MySQL, query: "DESC t", kind: "DESC"},
		{name: "PRAGMA", driver: SQLite, query: "PRAGMA table_info(t)", kind: "PRAGMA"},
		{name: "non-ASCII blank before the keyword", driver: SQLite, query: "\u3000SELECT 1", code: CodeWriteOperation, details: "Statement starts with '?'"},
		// Forbidden words.
		{name: "word boundary", driver: SQLite, query: "SELECT deleted FROM t", kind: "SELECT"},
		{name: "word in a string", driver: SQLite, query: "SELECT * FROM t WHERE a = 'DELETE'", kind: "SELECT"},
		{name: "word in a comment", driver: SQLite, query: "SELECT 1 -- DELETE", kind: "SELECT"},
		{name: "list order decides", driver: SQLite, query: "SELECT 1 INTO x FROM t FOR UPDATE", code: CodeWriteOperation, details: "Statement contains 'UPDATE'"},
		{name: "underscore kept", driver: PgSQL, query: "SELECT pg_sleep(1)", code: CodeWriteOperation, details: "Statement contains 'PG_SLEEP'"},
		{name: "LOAD DATA", driver: MySQL, query: "SELECT 1 LOAD  DATA", code: CodeWriteOperation, details: "Statement contains 'LOAD DATA'"},
		{name: "LOAD XML", driver: MySQL, query: "SELECT 1 load\nxml", code: CodeWriteOperation, details: "Statement contains 'LOAD DATA'"},
		{name: "LOAD DATA is not anchored", driver: MySQL, query: "SELECT download  xml FROM t", code: CodeWriteOperation, details: "Statement contains 'LOAD DATA'"},
		{name: "LOAD DATA across a comment", driver: MySQL, query: "SELECT 1 LOAD/**/DATA", code: CodeWriteOperation},
		{name: "LOAD_DATA is one word", driver: MySQL, query: "SELECT load_data", kind: "SELECT"},
	})
}

func TestErrorFields(t *testing.T) {
	tests := []struct {
		driver, query, action, hint string
	}{
		{MySQL, "SELECT 1 /*! x */", ActionRetry, "Remove MySQL/MariaDB executable comments (/*! ... */, /*M! ... */)"},
		{MySQL, `SELECT \`, ActionFatal, `Client commands such as \! are not allowed; use plain SQL`},
		{MySQL, `SELECT '\'`, ActionRetry, "MySQL clients and servers disagree on backslash escapes: write the value without backslashes (e.g. CHAR(92))"},
		{SQLite, "SELECT '", ActionRetry, "Close every quote and /* comment */"},
		{PgSQL, "SELECT $x$", ActionRetry, "Close every quote and /* comment */"},
		{SQLite, "SELECT 1; 2", ActionFatal, "Send one statement per call (a trailing semicolon is fine)"},
		{SQLite, "SELECT 1\n/", ActionFatal, "Send one statement per call"},
		{SQLite, ".x", ActionFatal, "sqlite3 dot-commands are not allowed; use plain SQL"},
		{SQLite, "DROP t", ActionFatal, "Only SELECT, WITH, VALUES, TABLE, SHOW, DESCRIBE, EXPLAIN and PRAGMA are allowed"},
		{SQLite, "SELECT 1 FROM t FOR UPDATE", ActionFatal, "Only read-only queries are allowed; data-changing and file/program functions are refused"},
		{SQLite, "SELECT 1 LOAD DATA", ActionFatal, "Only read-only queries are allowed"},
	}
	for _, tt := range tests {
		_, err := Check(tt.driver, tt.query)
		e, ok := err.(*Error)
		if !ok {
			t.Errorf("Check(%s, %q) = %v", tt.driver, tt.query, err)
			continue
		}
		if e.Action != tt.action || e.Hint != tt.hint || e.Recoverable() != (tt.action != ActionFatal) {
			t.Errorf("Check(%s, %q) = %+v, want action %s hint %q", tt.driver, tt.query, e, tt.action, tt.hint)
		}
	}
	// A caller may change the error it got without changing the next one.
	_, err := Check(MySQL, `SELECT \`)
	err.(*Error).Details = "changed"
	if _, err = Check(MySQL, `SELECT \`); err.(*Error).Details != "Backslash outside a string literal" {
		t.Error("errors share state")
	}
	if got := (&Error{Code: "c", Details: "d"}).Error(); got != "c: d" {
		t.Errorf("Error() = %q", got)
	}
}

func TestGuardDriver(t *testing.T) {
	for in, want := range map[string]string{"mysql": MySQL, "mariadb": MySQL, "pgsql": PgSQL, "sqlite": SQLite, "": SQLite, "postgres": SQLite} {
		if got := GuardDriver(in); got != want {
			t.Errorf("GuardDriver(%q) = %q, want %q", in, got, want)
		}
	}
}

// The word lists are those of lib/modules/llm/db.sh, in their order.
func TestWordListsAre2x(t *testing.T) {
	src := repoFile(t, filepath.Join("lib", "modules", "llm", "db.sh"))
	list := func(name string) []string {
		m := regexp.MustCompile(`(?m)^` + name + `="([^"]*)"$`).FindStringSubmatch(src)
		if m == nil {
			t.Fatalf("%s not found in db.sh", name)
		}
		return strings.Fields(m[1])
	}
	if got := list("__LLM_SQL_ALLOWED_FIRST"); !reflect.DeepEqual(got, allowedFirstWords) {
		t.Errorf("allowed first words differ from db.sh: %q", got)
	}
	if got := list("__LLM_SQL_FORBIDDEN_WORDS"); !reflect.DeepEqual(got, forbiddenWords) {
		t.Errorf("forbidden words differ from db.sh: %q", got)
	}
}

// repoFile reads a file of the 2.x tree, found by walking up from the
// package directory.
func repoFile(t *testing.T, rel string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err == nil {
			return string(data)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("the 2.x sources are not in this tree")
		}
		dir = parent
	}
}
