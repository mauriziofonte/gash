// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sqlguard

// The actions of the 2.x error contract that the guard uses.
const (
	ActionRetry = "RETRY"
	ActionFatal = "FATAL"
)

// The error codes of the guard.
const (
	CodeWriteOperation     = "write_operation_blocked"
	CodeMultipleStatements = "multiple_statements_blocked"
	CodeClientCommand      = "client_command_blocked"
	CodeDotCommand         = "dot_command_blocked"
	CodeExecutableComment  = "executable_comment_blocked"
	CodeBackslashInLiteral = "backslash_in_literal"
	CodeUnterminated       = "unterminated_literal"
)

// Error is a guard verdict as 2.x reports it through __llm_error: the
// error code, its details, the action and the hint, with the exact 2.x
// strings.
type Error struct {
	Code    string
	Details string
	Action  string
	Hint    string
}

func (e *Error) Error() string {
	if e.Details == "" {
		return e.Code
	}
	return e.Code + ": " + e.Details
}

// Recoverable is the "recoverable" field of the 2.x error line: false
// exactly for FATAL.
func (e *Error) Recoverable() bool { return e.Action != ActionFatal }

const hintClose = "Close every quote and /* comment */"

var (
	errExecutableComment = &Error{CodeExecutableComment, "/*! ... */", ActionRetry,
		"Remove MySQL/MariaDB executable comments (/*! ... */, /*M! ... */)"}
	errClientCommand = &Error{CodeClientCommand, "Backslash outside a string literal", ActionFatal,
		`Client commands such as \! are not allowed; use plain SQL`}
	errBackslashInLiteral = &Error{CodeBackslashInLiteral, "Backslash inside a quoted string or identifier", ActionRetry,
		"MySQL clients and servers disagree on backslash escapes: write the value without backslashes (e.g. CHAR(92))"}
	errUnterminated = &Error{CodeUnterminated, "A string, identifier or comment is not closed", ActionRetry,
		hintClose}
	errMultipleStatements = &Error{CodeMultipleStatements, "Only single SQL statements allowed", ActionFatal,
		"Send one statement per call (a trailing semicolon is fine)"}
	errLoadData = &Error{CodeWriteOperation, "Statement contains 'LOAD DATA'", ActionFatal,
		"Only read-only queries are allowed"}
)

// The verdicts below carry part of the query in their details; each call
// returns a new value, and the shared ones above are copied, so a caller
// may change what it gets.

func clone(e *Error) *Error { c := *e; return &c }

func unterminatedDollar(tag string) *Error {
	return &Error{CodeUnterminated, "Dollar-quoted string " + tag + " is not closed", ActionRetry, hintClose}
}

func dotCommand(head string) *Error {
	return &Error{CodeDotCommand, head, ActionFatal, "sqlite3 dot-commands are not allowed; use plain SQL"}
}

func statementEndLine(line string) *Error {
	return &Error{CodeMultipleStatements, "Line '" + line + "' ends a statement in the database client", ActionFatal,
		"Send one statement per call"}
}

func startsWith(first string) *Error {
	if first == "" {
		first = "?"
	}
	return &Error{CodeWriteOperation, "Statement starts with '" + first + "'", ActionFatal,
		"Only SELECT, WITH, VALUES, TABLE, SHOW, DESCRIBE, EXPLAIN and PRAGMA are allowed"}
}

func containsWord(w string) *Error {
	return &Error{CodeWriteOperation, "Statement contains '" + w + "'", ActionFatal,
		"Only read-only queries are allowed; data-changing and file/program functions are refused"}
}
