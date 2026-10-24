// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package config

import "testing"

// config_spec.sh:7-67 (__gash_url_decode, __gash_url_encode) plus the edge
// cases checked against the 2.x functions.
func TestURLDecode(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"config_spec:7 simple %XX", "hello%20world", "hello world"},
		{"config_spec:17 @ symbol", "p%40ssword", "p@ssword"},
		{"config_spec:27 colon", "pass%3Aword", "pass:word"},
		{"config_spec:37 multiple", "p%40ss%3Aw%2Frd", "p@ss:w/rd"},
		{"config_spec:47 plain text", "plaintext123", "plaintext123"},
		{"lower-case hex", "%5c%5C", `\\`},
		{"%25 is not decoded twice", "a%25b%2541", "a%b%41"},
		{"invalid and short escapes kept", "%zz%4", "%zz%4"},
		{"lone percent", "%", "%"},
		{"%00 decodes to nothing", "a%00b", "ab"},
		{"inner newline kept", "a%0Ab", "a\nb"},
		{"trailing newline kept by the function", "ab%0A", "ab\n"},
		{"high byte", "%FF", "\xff"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		if got := URLDecode(tt.in); got != tt.want {
			t.Errorf("%s: URLDecode(%q) = %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

// LEDGER CORE-13, reproduced: the callers capture __gash_url_decode with
// $(...) (db.sh:246, config.sh:545), so a password loses its trailing
// newlines, and %00 decodes to nothing (a Bash string holds no NUL).
func TestLedger_CORE13_2x(t *testing.T) {
	tests := []struct{ in, want string }{
		{"ab%0A%0A", "ab"},
		{"a%0Ab", "a\nb"},
		{"p%40ss", "p@ss"},
		{"%0A", ""},
		{"pw%00x", "pwx"},
		{"pw%0A%00", "pw"},
		{"%00%0A%00", ""},
	}
	for _, tt := range tests {
		if got := DecodePassword(tt.in); got != tt.want {
			t.Errorf("DecodePassword(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestURLEncode(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"config_spec:57 @ symbol", "p@ssword", "p%40ssword"},
		{"config_spec:67 alphanumeric", "abc123XYZ", "abc123XYZ"},
		{"unreserved kept", "a.b_c~d-e", "a.b_c~d-e"},
		{"one %XX per byte", "è", "%C3%A8"},
		{"space and percent", "a b%", "a%20b%25"},
		{"high byte", "\xff", "%FF"},
	}
	for _, tt := range tests {
		if got := URLEncode(tt.in); got != tt.want {
			t.Errorf("%s: URLEncode(%q) = %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

// config_spec.sh:79-207 and :552-595 (__gash_parse_db_url), LEDGER CORE-11
// and CORE-12, and edge cases checked against the 2.x function.
func TestParseDBURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want DBURL
	}{
		{"config_spec:79 mysql", "mysql://root:secret@localhost:3306/myapp",
			DBURL{"mysql", "root", "secret", "localhost", "3306", "myapp"}},
		{"config_spec:100 pgsql", "pgsql://pguser:pgpass@dbhost:5432/analytics",
			DBURL{"pgsql", "pguser", "pgpass", "dbhost", "5432", "analytics"}},
		{"config_spec:120 mariadb", "mariadb://admin:pw123@192.168.1.100:3307/prod",
			DBURL{"mariadb", "admin", "pw123", "192.168.1.100", "3307", "prod"}},
		{"config_spec:140 encoded password stays encoded", "mysql://root:p%40ss%3Aword@localhost:3306/app",
			DBURL{"mysql", "root", "p%40ss%3Aword", "localhost", "3306", "app"}},
		{"config_spec:160 no database", "mysql://root:pass@localhost:3306",
			DBURL{"mysql", "root", "pass", "localhost", "3306", ""}},
		{"config_spec:179 mysql default port", "mysql://root:pass@localhost/mydb",
			DBURL{"mysql", "root", "pass", "localhost", "3306", "mydb"}},
		{"config_spec:193 pgsql default port", "pgsql://user:pass@localhost/mydb",
			DBURL{"pgsql", "user", "pass", "localhost", "5432", "mydb"}},
		{"config_spec:552 encoded @ and :", "mysql://admin:p%40ss%3Aword@dbhost:3306/mydb",
			DBURL{"mysql", "admin", "p%40ss%3Aword", "dbhost", "3306", "mydb"}},
		{"config_spec:574 raw @ split on last @", "mysql://user:p@ss@localhost:3306/db",
			DBURL{"mysql", "user", "p@ss", "localhost", "3306", "db"}},
		{"CORE-12 raw @ : / # in password", "mysql://u:p@:/#x@h/d",
			DBURL{"mysql", "u", "p@:/#x", "h", "3306", "d"}},
		{"CORE-11 IPv6 host split at its first colon (reproduced)", "pgsql://u:p@[::1]:5432/d",
			DBURL{"pgsql", "u", "p", "[", ":1]:5432", "d"}},
		{"user only", "mysql://u@h/d", DBURL{"mysql", "u", "", "h", "3306", "d"}},
		{"empty user", "mysql://:pw@h/d", DBURL{"mysql", "", "pw", "h", "3306", "d"}},
		{"empty port: no default", "mysql://u:p@h:/db", DBURL{"mysql", "u", "p", "h", "", "db"}},
		{"empty host: no default port", "mysql://u:p@/db", DBURL{"mysql", "u", "p", "", "", "db"}},
		{"no user part", "pgsql://h", DBURL{"pgsql", "", "", "h", "5432", ""}},
		{"unknown driver: no default port", "sqlite://h/x", DBURL{"sqlite", "", "", "h", "", "x"}},
		{"no scheme separator", "u:p@h/d", DBURL{"u:p@h/d", "u", "p", "h", "", "d"}},
		{"database keeps slashes and query", "mysql://u:p@h/a/b?x=1", DBURL{"mysql", "u", "p", "h", "3306", "a/b?x=1"}},
		{"empty", "", DBURL{}},
	}
	for _, tt := range tests {
		if got := ParseDBURL(tt.in); got != tt.want {
			t.Errorf("%s: ParseDBURL(%q) = %+v, want %+v", tt.name, tt.in, got, tt.want)
		}
	}
}

// LEDGER CORE-12: raw and encoded special characters give the same password.
func TestLedger_CORE12(t *testing.T) {
	raw := DecodePassword(ParseDBURL("mysql://u:p@:/#x@h/d").Password)
	enc := DecodePassword(ParseDBURL("mysql://u:p%40%3A%2F%23x@h/d").Password)
	if raw != "p@:/#x" || enc != raw {
		t.Errorf("raw %q, encoded %q, want both %q", raw, enc, "p@:/#x")
	}
	if got := DecodePassword(ParseDBURL("mysql://u:%2540@h/d").Password); got != "%40" {
		t.Errorf("%%2540 decodes to %q, want %%40", got)
	}
	if got := ParseDBURL("mysql://u%40x:p@h/d").User; got != "u%40x" {
		t.Errorf("user %q, want it as written", got)
	}
}

// LEDGER CORE-03: the password is masked whether or not the user name is
// empty (3.0: CORE-03 fixed; 2.x showed mysql://:PW@h/d unmasked).
func TestLedger_CORE03(t *testing.T) {
	tests := []struct{ in, want string }{
		{"mysql://u:p@h/d", "mysql://u:****@h/d"},
		{"mysql://:SECRETPW@h/d", "mysql://:****@h/d"},
		{"redis://:pw@h:6379", "redis://:****@h:6379"},
		{"mysql://u:p@@h", "mysql://u:****@h"},
		{"mysql://u:p@ss@host/db", "mysql://u:****@host/db"},
		{"mysql://u@h/d", "mysql://u@h/d"},
		{"mysql://u:@h/d", "mysql://u:@h/d"},
		{"mysql://u@h:3306/d", "mysql://u@h:3306/d"},
		{"mysql://u@h:3306/d@x", "mysql://u@h:3306/d@x"}, // no password in the userinfo
		{"pgsql://u:p%40ss@h:5432/x", "pgsql://u:****@h:5432/x"},
	}
	for _, tt := range tests {
		if got := MaskDBURL(tt.in); got != tt.want {
			t.Errorf("MaskDBURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// LEDGER LLM-02, reproduced: the llm_env URL mask is anchored and stops the
// password at the first "/" or "@".
func TestLedger_LLM02_2x(t *testing.T) {
	tests := []struct{ in, want string }{
		{"mysql://user:p@ss@host/db", "mysql://user:***@ss@host/db"},
		{"mysql://u:pa/ss@h/d", "mysql://u:pa/ss@h/d"},
		{"x https://u:pw@h/", "x https://u:pw@h/"},
		{"https://u:pw@h/", "https://u:***@h/"},
		{"redis://:pw@h:6379", "redis://:***@h:6379"},
		{"postgres://u@h/d", "postgres://u@h/d"},
		{"a+b.c-d://u:x@h", "a+b.c-d://u:***@h"},
		{"1http://u:p@h", "1http://u:p@h"},
	}
	for _, tt := range tests {
		if got := MaskEnvURL(tt.in); got != tt.want {
			t.Errorf("MaskEnvURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// LEDGER CORE-16: ${token:0:8} counts characters as a UTF-8 locale does
// (LC_ALL=C would count bytes and could cut a character); an invalid byte
// counts as one character and is kept.
func TestLedger_CORE16(t *testing.T) {
	tests := []struct{ in, want string }{
		{"sk-ant-api03-xxxx", "sk-ant-a..."},
		{"short", "short..."},
		{"", "..."},
		{"ééééééééé", "éééééééé..."},
		{"ab\xffcdefghij", "ab\xffcdefg..."},
		{"日本語のトークンです", "日本語のトークン..."},
	}
	for _, tt := range tests {
		if got := MaskAIToken(tt.in); got != tt.want {
			t.Errorf("MaskAIToken(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDisplay(t *testing.T) {
	got := ParseDBURL("mysql://u:secret@db/app").Display()
	if got != "mysql://u@db:3306/app" {
		t.Errorf("Display = %q", got)
	}
}
