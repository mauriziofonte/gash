// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// write creates a file with the given mode in a fresh temporary directory.
func write(t *testing.T, name, content string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func load(t *testing.T, content, home string) *File {
	t.Helper()
	p := write(t, "env", content, 0o600)
	return Load(Options{Path: p, Home: home, EUID: os.Geteuid()})
}

func texts(ws []Warning) []string {
	out := []string{}
	for _, w := range ws {
		out = append(out, w.Text)
	}
	return out
}

func names(es []Entry) []string {
	out := []string{}
	for _, e := range es {
		out = append(out, e.Name+"="+e.Value)
	}
	return out
}

func TestFilePath(t *testing.T) {
	if got := FilePath("/h", ""); got != "/h/.gash_env" {
		t.Errorf("default: %q", got)
	}
	if got := FilePath("/h", "/x/y"); got != "/x/y" {
		t.Errorf("GASH_ENV_FILE: %q", got)
	}
	if got := FilePath("", ""); got != "/.gash_env" {
		t.Errorf("no HOME: %q", got)
	}
}

// config_spec.sh:209: a missing file is an empty configuration.
func TestLoadMissingFile(t *testing.T) {
	f := Load(Options{Path: "/nonexistent/.gash_env", Home: "/nonexistent"})
	if f.Exists || len(f.DB) != 0 || len(f.SSHKeys) != 0 || len(f.Warnings) != 0 || f.ReadErr != nil {
		t.Errorf("missing file: %+v", f)
	}
	dir := t.TempDir()
	if f := Load(Options{Path: dir}); f.Exists || len(f.Warnings) != 0 {
		t.Errorf("a directory is not a configuration file: %+v", f)
	}
}

// config_spec.sh:228 and ssh_auto_unlock_spec.sh:10.
func TestLoadSSHEntries(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"key1", "key2"} {
		if err := os.WriteFile(filepath.Join(home, ".ssh", k), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	key1 := filepath.Join(home, ".ssh", "key1")

	f := load(t, "# Test config\nSSH:"+key1+"=mypassphrase\n", home)
	if got := names(f.SSHKeys); !reflect.DeepEqual(got, []string{key1 + "=mypassphrase"}) {
		t.Errorf("config_spec:228: %q", got)
	}

	// Tolerant parsing: CRLF, whitespace, comments, invalid lines. The
	// passphrase keeps its leading spaces; a missing key is skipped.
	f = load(t, "# comment\r\n  SSH:~/.ssh/key1=P@ss:with:colons\r\nSSH:~/.ssh/key2=  leading-space-password\nbadlinewithoutprefix\nSSH:~/.ssh/missing=pw\n", home)
	want := []string{key1 + "=P@ss:with:colons", filepath.Join(home, ".ssh", "key2") + "=  leading-space-password"}
	if got := names(f.SSHKeys); !reflect.DeepEqual(got, want) {
		t.Errorf("ssh_auto_unlock_spec:10: %q", got)
	}
	shown := ShownPath(f.Path, home)
	wantW := []string{
		"Line 4 of " + shown + ": unrecognized entry (expected SSH:, DB:, AI:, HOST:, PROTECT: or GASH_NAME=), skipping",
		"Line 5 of " + shown + ": SSH key file not found (skipping)",
	}
	if got := texts(f.Warnings); !reflect.DeepEqual(got, wantW) {
		t.Errorf("warnings: %q", got)
	}
}

// config_spec.sh:405: an SSH key that does not exist is skipped.
func TestLoadSSHMissingKey(t *testing.T) {
	f := load(t, "SSH:/nonexistent/key=passphrase\n", "/h")
	if len(f.SSHKeys) != 0 || len(f.Warnings) != 1 {
		t.Errorf("got %+v", f)
	}
}

// LEDGER SYS-44: an SSH: passphrase is the value verbatim but for a final
// CR: trailing whitespace is kept, with one "passphrase ends with
// whitespace" warning for the line (3.0: SYS-44 fixed; 2.x trimmed the
// whole line, so "pass  \t" became "pass"). The left side and the other
// entries are trimmed as before.
func TestLedger_SYS44(t *testing.T) {
	key := write(t, "k", "", 0o600)
	f := load(t, "  SSH:"+key+"=pass  \t\r\nSSH:"+key+"2=x\n", "/h")
	require.Len(t, f.SSHKeys, 1)
	require.Equal(t, "pass  \t", f.SSHKeys[0].Value)
	require.Equal(t, key, f.SSHKeys[0].Name)
	ws := texts(f.Warnings)
	require.Len(t, ws, 2, "%q", ws)
	require.True(t, strings.HasSuffix(ws[0], "Line 1 of "+f.Shown+": passphrase ends with whitespace"), ws[0])
	require.Contains(t, ws[1], "SSH key file not found")

	f = load(t, "SSH:"+key+"=trail   \n", "/h")
	require.Equal(t, "trail   ", f.SSHKeys[0].Value)
	require.Len(t, f.Warnings, 1)

	// No warning without trailing whitespace, and an empty passphrase stays
	// allowed; a DB: value is still trimmed.
	f = load(t, "SSH:"+key+"=pass\nDB:x=mysql://u:p@h/d   \n", "/h")
	require.Empty(t, f.Warnings)
	require.Equal(t, "pass", f.SSHKeys[0].Value)
	require.Equal(t, "mysql://u:p@h/d", f.DB[0].Value)
	f = load(t, "SSH:"+key+"=\n", "/h")
	require.Empty(t, f.Warnings)
	require.Equal(t, "", f.SSHKeys[0].Value)
}

// config_spec.sh:258, :287, :315, :345, :430.
func TestLoadDBEntries(t *testing.T) {
	f := load(t, "# Test config\nDB:default=mysql://root:pass@localhost:3306/myapp\nDB:postgres=pgsql://pg:secret@dbhost:5432/analytics\n", "/h")
	if u, ok := f.DBURL("default"); !ok || u != "mysql://root:pass@localhost:3306/myapp" {
		t.Errorf("config_spec:258 default: %q %v", u, ok)
	}
	if u, ok := f.DBURL("postgres"); !ok || u != "pgsql://pg:secret@dbhost:5432/analytics" {
		t.Errorf("config_spec:258 postgres: %q %v", u, ok)
	}
	if u, ok := f.DBURL(""); !ok || u != "mysql://root:pass@localhost:3306/myapp" {
		t.Errorf("empty name means default: %q %v", u, ok)
	}
	if _, ok := f.DBURL("nonexistent"); ok {
		t.Error("config_spec:287: a missing connection must not be found")
	}

	f = load(t, "# This is a comment\n\n   # Indented comment\n\nDB:default=mysql://root:pass@localhost:3306/myapp\n\n# Another comment\n", "/h")
	if u, _ := f.DBURL("default"); u != "mysql://root:pass@localhost:3306/myapp" || len(f.DB) != 1 || len(f.Warnings) != 0 {
		t.Errorf("config_spec:315: %+v", f)
	}

	f = load(t, "   DB:default=mysql://root:pass@localhost:3306/myapp\n", "/h")
	if u, _ := f.DBURL("default"); u != "mysql://root:pass@localhost:3306/myapp" {
		t.Errorf("config_spec:345: %q", u)
	}

	f = load(t, "DB:invalid=sqlite://invalid\nDB:default=mysql://root:pass@localhost:3306/myapp\n", "/h")
	if _, ok := f.DBURL("invalid"); ok {
		t.Error("config_spec:430: invalid URL accepted")
	}
	if u, _ := f.DBURL("default"); u != "mysql://root:pass@localhost:3306/myapp" {
		t.Errorf("config_spec:430: %q", u)
	}
}

// config_spec.sh:368: the cache is per process in 3.x; loading again reads
// the new content.
func TestLoadReload(t *testing.T) {
	p := write(t, "env", "DB:default=mysql://root:pass1@localhost:3306/db1\n", 0o600)
	f1 := Load(Options{Path: p, Home: "/h"})
	if err := os.WriteFile(p, []byte("DB:default=mysql://root:pass2@localhost:3306/db2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if u, _ := f1.DBURL("default"); u != "mysql://root:pass1@localhost:3306/db1" {
		t.Errorf("first load changed: %q", u)
	}
	f2 := Load(Options{Path: p, Home: "/h"})
	if u, _ := f2.DBURL("default"); u != "mysql://root:pass2@localhost:3306/db2" {
		t.Errorf("reload: %q", u)
	}
}

// LEDGER CORE-04: a repeated DB:, SSH: or HOST: name keeps only the first
// entry, which is the one used, and the later one is reported with both
// line numbers (3.0: CORE-04 fixed; 2.x listed both and said nothing).
func TestLedger_CORE04(t *testing.T) {
	f := Parse([]byte("DB:default=mysql://a:p1@h1/d\nDB:other=mysql://o:p@h/o\nDB:default=pgsql://b:p2@h2/e\n"+
		"HOST:web=u@a\nHOST:web=v@b\nAI:claude=sk-1\nAI:claude=sk-2\n"), "/h/.gash_env", "/h")
	rows := f.DBList()
	want := []DBListRow{{"default", "mysql://a:****@h1/d"}, {"other", "mysql://o:****@h/o"}}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows %+v", rows)
	}
	if u, _ := f.DBURL("default"); u != "mysql://a:p1@h1/d" {
		t.Errorf("first entry wins: %q", u)
	}
	if h, _ := f.HostTarget("web"); h != "u@a" || len(f.Hosts) != 1 {
		t.Errorf("host %q %d", h, len(f.Hosts))
	}
	require.Equal(t, []string{
		"Line 3 of ~/.gash_env: duplicate DB:default, ignored (first is at line 1)",
		"Line 5 of ~/.gash_env: duplicate HOST:web, ignored (first is at line 4)",
	}, texts(f.Warnings))
	// AI: lines are outside the rule: the first token is used, as in 2.x.
	if tok, _ := f.AIToken("claude"); tok != "sk-1" {
		t.Errorf("ai %q", tok)
	}

	dir := t.TempDir()
	key := filepath.Join(dir, "id")
	require.NoError(t, os.WriteFile(key, nil, 0o600))
	f = Parse([]byte("SSH:"+key+"=one\nSSH:"+key+"=two\n"), "/h/.gash_env", "/h")
	require.Len(t, f.SSHKeys, 1)
	require.Equal(t, "one", f.SSHKeys[0].Value)
	require.Equal(t, []string{"Line 2 of ~/.gash_env: duplicate SSH:" + key + ", ignored (first is at line 1)"}, texts(f.Warnings))
}

// ai_spec.sh:7-162 (AI: entries and lookups).
func TestLoadAIEntries(t *testing.T) {
	f := load(t, "AI:claude=sk-ant-api03-test-token-123\n", "/h")
	if tok, ok := f.AIToken("claude"); !ok || tok != "sk-ant-api03-test-token-123" {
		t.Errorf("ai_spec:7: %q", tok)
	}
	f = load(t, "AI:gemini=AIzaSy-test-gemini-key-456\n", "/h")
	if tok, _ := f.AIToken("gemini"); tok != "AIzaSy-test-gemini-key-456" {
		t.Errorf("ai_spec:30: %q", tok)
	}
	f = load(t, "AI:openai=sk-test-openai-key\n", "/h")
	if _, ok := f.AIToken("openai"); ok || len(f.Warnings) != 1 {
		t.Errorf("ai_spec:53: %+v", f)
	}
	f = load(t, "AI:claude=claude-token-123\nAI:gemini=gemini-token-456\n", "/h")
	c, _ := f.AIToken("claude")
	g, _ := f.AIToken("gemini")
	if c != "claude-token-123" || g != "gemini-token-456" {
		t.Errorf("ai_spec:82: %q %q", c, g)
	}
	f = load(t, "AI:claude=my-claude-token\n", "/h")
	if tok, ok := f.AIToken("unknown"); ok || tok != "" {
		t.Errorf("ai_spec:134: %q", tok)
	}
	if _, ok := f.AIToken(""); ok {
		t.Error("empty provider must find nothing")
	}
	f = load(t, "AI:gemini=gemini-token\nAI:claude=claude-token\n", "/h")
	if p, ok := f.FirstAIProvider(); !ok || p != "gemini" {
		t.Errorf("ai_spec:162: %q", p)
	}
	if _, ok := (&File{}).FirstAIProvider(); ok {
		t.Error("no provider")
	}
}

func TestLoadHostEntries(t *testing.T) {
	f := load(t, "HOST:web=root@web.example.com\nHOST:db=deploy@10.0.0.5\nHOST:web=other@x\nHOST:bad=nohost\nHOST:opt=-oProxyCommand=x@h\n", "/h")
	if got, _ := f.HostTarget("web"); got != "root@web.example.com" {
		t.Errorf("first wins: %q", got)
	}
	// 3.0: REM-02 fixed: an option-looking target is skipped with a warning
	// (2.x accepted it).
	if got, ok := f.HostTarget("opt"); ok {
		t.Errorf("REM-02: %q", got)
	}
	if _, ok := f.HostTarget(""); ok {
		t.Error("empty alias must find nothing")
	}
	// 3.0: CORE-04 fixed: the second web= line is ignored with a warning.
	if len(f.Hosts) != 2 || len(f.Warnings) != 3 {
		t.Errorf("hosts %q warnings %q", names(f.Hosts), texts(f.Warnings))
	}
}

// Every rule of the line grammar, with the exact 2.x warning texts; the
// expected values were checked against lib/core/config.sh.
func TestGrammar(t *testing.T) {
	home := t.TempDir()
	key := filepath.Join(home, "id")
	if err := os.WriteFile(key, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "a\tb"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	const S = "~/env"
	tests := []struct {
		name     string
		line     string
		db       []string
		ai       []string
		hosts    []string
		ssh      []string
		settings []string
		warn     string
	}{
		{name: "no = after prefix", line: "AI:claude", warn: "AI: entry has no '=' (expected AI:name=value), skipping"},
		{name: "empty name", line: "DB:=mysql://x", warn: "DB: entry has an empty name, skipping"},
		{name: "bad DB scheme", line: "DB:prod=ftp://u:pw@h/x", warn: "DB URL must start with mysql://, mariadb://, pgsql:// or a +ssh form (mysql+ssh://, mariadb+ssh://, pgsql+ssh://, sqlite+ssh://) (skipping)"},
		{name: "DB scheme is case-sensitive", line: "DB:x=MYSQL://h", warn: "DB URL must start with mysql://, mariadb://, pgsql:// or a +ssh form (mysql+ssh://, mariadb+ssh://, pgsql+ssh://, sqlite+ssh://) (skipping)"},
		{name: "space before the URL", line: "DB:x= mysql://h", warn: "DB URL must start with mysql://, mariadb://, pgsql:// or a +ssh form (mysql+ssh://, mariadb+ssh://, pgsql+ssh://, sqlite+ssh://) (skipping)"},
		{name: "name keeps inner spaces", line: "DB: x =mysql://h", db: []string{" x =mysql://h"}},
		{name: "value split at the first =", line: "DB:x=mysql://h/d?a=b", db: []string{"x=mysql://h/d?a=b"}},
		{name: "tab in the name moves into the value", line: "DB:a\tb=mysql://h", db: []string{"a=b\tmysql://h"}},
		{name: "unknown provider", line: "AI:openai=sk-zzz", warn: "unknown AI provider (supported: claude, gemini), skipping"},
		{name: "provider is case-sensitive", line: "AI:Claude=x", warn: "unknown AI provider (supported: claude, gemini), skipping"},
		{name: "empty token", line: "AI:claude=", warn: "AI provider claude has an empty token (skipping)"},
		{name: "token kept as written", line: "AI:claude= tok=x", ai: []string{"claude= tok=x"}},
		{name: "host without @", line: "HOST:box=nohost", warn: "HOST target must be user@host (skipping)"},
		{name: "host with two @", line: "HOST:box=u@h@i", warn: "HOST target must be user@host (skipping)"},
		{name: "host with a space", line: "HOST:box=u @h", warn: "HOST target must be user@host (skipping)"},
		{name: "host empty user", line: "HOST:box=@h", warn: "HOST target must be user@host (skipping)"},
		{name: "host", line: "HOST:box=u@h", hosts: []string{"box=u@h"}},
		{name: "ssh key with ~", line: "SSH:~/id=pw", ssh: []string{key + "=pw"}},
		{name: "ssh key empty passphrase", line: "SSH:~/id=", ssh: []string{key + "="}},
		{name: "ssh key directory", line: "SSH:~=pw", warn: "SSH key file not found (skipping)"},
		{name: "ssh key tab in path", line: "SSH:~/a\tb=pw", ssh: []string{home + "/a=b\tpw"}},
		{name: "unknown setting", line: "GASH_NOPE=topsecret", warn: "unknown GASH_ setting, skipping"},
		{name: "setting name with lower case", line: "GASH_prompt=1", warn: "unknown GASH_ setting, skipping"},
		{name: "setting name with a space", line: "GASH_PROMPT GASH_BANNER=1", warn: "unknown GASH_ setting, skipping"},
		{name: "bare GASH_", line: "GASH_=1", warn: "unknown GASH_ setting, skipping"},
		{name: "unrecognized", line: "PASSWORD=hunter2", warn: "unrecognized entry (expected SSH:, DB:, AI:, HOST:, PROTECT: or GASH_NAME=), skipping"},
		{name: "prefix is case-sensitive", line: "ssh:x=y", warn: "unrecognized entry (expected SSH:, DB:, AI:, HOST:, PROTECT: or GASH_NAME=), skipping"},
		{name: "GASH_ without =", line: "GASH_PROMPT", warn: "unrecognized entry (expected SSH:, DB:, AI:, HOST:, PROTECT: or GASH_NAME=), skipping"},
		{name: "space before the colon", line: "DB :x=y", warn: "unrecognized entry (expected SSH:, DB:, AI:, HOST:, PROTECT: or GASH_NAME=), skipping"},
		{name: "comment", line: "  # DB:x=y"},
		{name: "whitespace only", line: " \t\v\f\r"},
		{name: "CR then trimmed", line: "GASH_PROMPT=0 \r", settings: []string{"GASH_PROMPT=0"}},
		{name: "NUL bytes are dropped", line: "GASH_PRO\x00MPT=0", settings: []string{"GASH_PROMPT=0"}},
		{name: "setting value with =", line: "GASH_GIT_EXCLUDE=a=b", settings: []string{"GASH_GIT_EXCLUDE=a=b"}},
		{name: "space before = is part of the name", line: "GASH_PROMPT =0", warn: "unknown GASH_ setting, skipping"},
	}
	for _, tt := range tests {
		f := Parse([]byte(tt.line+"\n"), home+"/env", home)
		got := func(es []Entry) []string {
			if len(es) == 0 {
				return nil
			}
			return names(es)
		}
		var sets []string
		for _, s := range f.Settings {
			sets = append(sets, s.Name+"="+s.Value)
		}
		var w string
		if len(f.Warnings) > 0 {
			w = f.Warnings[0].Text
		}
		wantW := ""
		if tt.warn != "" {
			wantW = "Line 1 of " + S + ": " + tt.warn
		}
		if !reflect.DeepEqual(got(f.DB), tt.db) || !reflect.DeepEqual(got(f.AI), tt.ai) ||
			!reflect.DeepEqual(got(f.Hosts), tt.hosts) || !reflect.DeepEqual(got(f.SSHKeys), tt.ssh) ||
			!reflect.DeepEqual(sets, tt.settings) || w != wantW || len(f.Warnings) > 1 {
			t.Errorf("%s: %q\n db=%q ai=%q hosts=%q ssh=%q settings=%q\n warnings=%q", tt.name, tt.line,
				got(f.DB), got(f.AI), got(f.Hosts), got(f.SSHKeys), sets, texts(f.Warnings))
		}
	}
}

func TestLineNumbersAndLastLine(t *testing.T) {
	// The last line counts without a newline; empty lines count too.
	f := Parse([]byte("\n\nnope\nGASH_PROMPT=1"), "/x/env", "/h")
	if got := texts(f.Warnings); !reflect.DeepEqual(got, []string{"Line 3 of /x/env: unrecognized entry (expected SSH:, DB:, AI:, HOST:, PROTECT: or GASH_NAME=), skipping"}) {
		t.Errorf("warnings %q", got)
	}
	if v, ok := f.Setting("GASH_PROMPT"); !ok || v != "1" || f.Settings[0].Line != 4 {
		t.Errorf("last line: %q %v %+v", v, ok, f.Settings)
	}
	if w := f.Warnings[0]; w.Line != 3 || w.String() != "Warning: "+w.Text {
		t.Errorf("warning %+v", w)
	}
}

// settings_spec.sh:8.
func TestSettingsQuotesStripped(t *testing.T) {
	f := load(t, "# comment\nGASH_PROMPT=0\nGASH_GIT_EXCLUDE=\"/srv/a b:/srv/c\"\nGASH_AI_MODEL_CLAUDE='claude-x'\n", "/h")
	for name, want := range map[string]string{"GASH_PROMPT": "0", "GASH_GIT_EXCLUDE": "/srv/a b:/srv/c", "GASH_AI_MODEL_CLAUDE": "claude-x"} {
		if v, ok := f.Setting(name); !ok || v != want {
			t.Errorf("%s = %q %v, want %q", name, v, ok, want)
		}
	}
	if _, ok := f.Setting("GASH_BANNER"); ok {
		t.Error("GASH_BANNER must be absent")
	}
}

// settings_spec.sh:74 (second half) and the other unquoting rules.
func TestSettingValues(t *testing.T) {
	tests := []struct{ line, want string }{
		{`GASH_LLM_EXEC_ALLOW="make # npm"   # comment`, "make # npm"},
		{`GASH_GIT_EXCLUDE='/a b'`, "/a b"},
		{`GASH_AI_MODEL_CLAUDE=x#y`, "x#y"},
		{"GASH_DB_TIMEOUT=5\t# tab", "5"},
		{`GASH_DB_TIMEOUT=5   # seconds`, "5"},
		{`GASH_PROMPT=x #y #z`, "x"},
		{`GASH_PROMPT=a b  `, "a b"},
		{`GASH_PROMPT= 7`, " 7"},
		{`GASH_PROMPT=`, ""},
		{`GASH_PROMPT=""`, ""},
		{`GASH_PROMPT="x"#y`, "x"},
		{`GASH_PROMPT="a`, `"a`},
		{`GASH_PROMPT="a" b`, `"a" b`},
		{`GASH_PROMPT="a" b # c`, `"a" b # c`},
		{`GASH_PROMPT='it''s'`, `'it''s'`},
		{`GASH_PROMPT="it's"`, "it's"},
		{`GASH_PROMPT='a"b'`, `a"b`},
	}
	for _, tt := range tests {
		f := Parse([]byte(tt.line+"\n"), "/e", "/h")
		name := tt.line[:strings.IndexByte(tt.line, '=')]
		if v, ok := f.Setting(name); !ok || v != tt.want {
			t.Errorf("%q: got %q %v, want %q", tt.line, v, ok, tt.want)
		}
	}
}

// settings_spec.sh:74 (first half): every commented setting of the
// template, uncommented, parses to the value written before its comment.
func TestTemplateCommentedSettings(t *testing.T) {
	lines := templateLines(t)
	n := 0
	for _, line := range lines {
		// ^# (GASH_[A-Z0-9_]+)=([^[:space:]]*)[[:space:]]+#
		if !strings.HasPrefix(line, "# GASH_") {
			continue
		}
		body := line[2:]
		eq := strings.IndexByte(body, '=')
		if eq < 0 || !validSettingName(body[:eq]) {
			continue
		}
		rest := body[eq+1:]
		end := 0
		for end < len(rest) && !isSpace(rest[end]) {
			end++
		}
		after := trimLeftSpace(rest[end:])
		if end == len(rest) || !strings.HasPrefix(after, "#") {
			continue
		}
		f := Parse([]byte(body+"\n"), "/e", "/h")
		if got, _ := f.Setting(body[:eq]); got != rest[:end] {
			t.Errorf("%s: got %q, want %q", body[:eq], got, rest[:end])
		}
		n++
	}
	if n < 10 {
		t.Errorf("only %d template settings checked", n)
	}
}

// settings_spec.sh:236: every setting documented in the template is known.
func TestTemplateSettingsKnown(t *testing.T) {
	n := 0
	for _, line := range templateLines(t) {
		if !strings.HasPrefix(line, "# GASH_") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 || !validSettingName(line[2:eq]) {
			continue
		}
		n++
		if !IsKnown(line[2:eq]) {
			t.Errorf("not accepted: %s", line[2:eq])
		}
	}
	if n < 9 {
		t.Errorf("only %d settings found", n)
	}
}

func templateLines(t *testing.T) []string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, ".gash_env.template"))
		if err == nil {
			return strings.Split(string(data), "\n")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal(".gash_env.template not found above the package directory")
		}
		dir = parent
	}
}

// settings_spec.sh:46: warnings name the line and never any content.
func TestWarningsNeverShowContent(t *testing.T) {
	dir := t.TempDir()
	content := strings.Join([]string{"GASH_NOPE=topsecret", "AI:claude", "PASSWORD=hunter2", "GASH_PROMPT=1",
		"sk-ant-api03-PASTEDTOKENWITHOUTSEPARATORS", "DB:prod=ftp://u:pw@h/x", "AI:openai=sk-zzz",
		"HOST:box=nohost", "SSH:~/.ssh/missing_key=pass"}, "\n") + "\n"
	p := filepath.Join(dir, "env")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	f := Load(Options{Path: p, Home: "/elsewhere", EUID: os.Geteuid()})
	out := strings.Join(texts(f.Warnings), "\n")
	for _, n := range []string{"1", "2", "3", "5", "6", "7", "8", "9"} {
		if !strings.Contains(out, "Line "+n+" of "+p+":") {
			t.Errorf("no warning for line %s: %s", n, out)
		}
	}
	// The file's own path (a random temp name, which may hold a checked
	// word such as "box") is not content: the words are looked for with
	// it replaced (LEDGER TEST-12).
	scan := strings.ReplaceAll(out, p, "FILE")
	for _, leak := range []string{"GASH_NOPE", "topsecret", "PASSWORD", "hunter2", "sk-ant", "PASTED", "prod", "ftp", "pw@", "openai", "sk-zzz", "box", "nohost", "missing_key", "pass"} {
		if strings.Contains(scan, leak) {
			t.Errorf("leaked %s: %s", leak, out)
		}
	}
	if strings.Contains(out, "~/.gash_env") {
		t.Errorf("names ~/.gash_env: %s", out)
	}
	if v, _ := f.Setting("GASH_PROMPT"); v != "1" {
		t.Errorf("GASH_PROMPT = %q", v)
	}
	// Under $HOME the file is shown as ~/...
	f = Load(Options{Path: p, Home: dir, EUID: os.Geteuid()})
	if !strings.HasPrefix(f.Warnings[0].Text, "Line 1 of ~/env:") {
		t.Errorf("shown path: %q", f.Warnings[0].Text)
	}
}

// settings_spec.sh:106: the permission check warns only when group or
// other bits are set.
func TestCheckPerms(t *testing.T) {
	euid := os.Geteuid()
	for _, mode := range []os.FileMode{0o600, 0o400, 0o700, 0o500} {
		p := write(t, "env", "GASH_PROMPT=1\n", mode)
		if w := CheckPerms(p, "/h", euid); len(w) != 0 {
			t.Errorf("warned on %o: %q", mode, texts(w))
		}
	}
	for _, mode := range []os.FileMode{0o640, 0o604, 0o660, 0o644, 0o710} {
		p := write(t, "env", "GASH_PROMPT=1\n", mode)
		w := CheckPerms(p, filepath.Dir(p), euid)
		want := "~/env is accessible by other users (mode " + strings.TrimLeft(modeString(mode), "0") + "). Run: chmod 600 ~/env"
		if len(w) != 1 || w[0].Text != want {
			t.Errorf("mode %o: %q, want %q", mode, texts(w), want)
		}
	}
	// Owner check against another uid.
	p := write(t, "env", "", 0o600)
	w := CheckPerms(p, filepath.Dir(p), euid+1)
	want := "~/env is owned by uid " + itoa(euid) + ", not by you (uid " + itoa(euid+1) + "). Gash will still read it."
	if len(w) != 1 || w[0].Text != want {
		t.Errorf("owner: %q, want %q", texts(w), want)
	}
	// Symlinks are followed; a missing file says nothing.
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(write(t, "env", "", 0o644), link); err != nil {
		t.Fatal(err)
	}
	if w := CheckPerms(link, "/h", euid); len(w) != 1 {
		t.Errorf("symlink: %q", texts(w))
	}
	if w := CheckPerms("/nonexistent", "/h", euid); w != nil {
		t.Errorf("missing: %q", texts(w))
	}
}

func modeString(m os.FileMode) string {
	const digits = "01234567"
	v := uint32(m.Perm())
	return string([]byte{digits[v>>6&7], digits[v>>3&7], digits[v&7]})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// LEDGER CORE-17: the mode in the warning is always GNU stat -c %a: octal
// without leading zeros, the setuid, setgid and sticky digit included (2.x
// printed BSD stat -f %Lp where GNU stat was missing).
func TestLedger_CORE17(t *testing.T) {
	euid := os.Geteuid()
	tests := []struct {
		mode os.FileMode
		want string
	}{
		{0o644, "644"},
		{0o044, "44"},
		{0o640, "640"},
		{os.ModeSetuid | 0o644, "4644"},
		{os.ModeSetgid | 0o640, "2640"},
		{os.ModeSticky | 0o604, "1604"},
		{os.ModeSetuid | os.ModeSetgid | 0o755, "6755"},
	}
	for _, tt := range tests {
		p := write(t, "env", "", 0o600)
		if err := os.Chmod(p, tt.mode); err != nil {
			t.Fatal(err)
		}
		w := CheckPerms(p, filepath.Dir(p), euid)
		want := "~/env is accessible by other users (mode " + tt.want + "). Run: chmod 600 ~/env"
		if len(w) != 1 || w[0].Text != want {
			t.Errorf("mode %v: %q, want %q", tt.mode, texts(w), want)
		}
	}
}

// The permission check runs before the line warnings, and is skipped in
// headless mode unless GASH_DEBUG=1 (settings_spec.sh:125, hygiene_spec.sh:124).
func TestLoadPermsHeadless(t *testing.T) {
	p := write(t, "env", "GASH_BOGUS=1\nnonsense line\n", 0o644)
	home := filepath.Dir(p)
	f := Load(Options{Path: p, Home: home, EUID: os.Geteuid()})
	want := []string{
		"~/env is accessible by other users (mode 644). Run: chmod 600 ~/env",
		"Line 1 of ~/env: unknown GASH_ setting, skipping",
		"Line 2 of ~/env: unrecognized entry (expected SSH:, DB:, AI:, HOST:, PROTECT: or GASH_NAME=), skipping",
	}
	if got := texts(f.Warnings); !reflect.DeepEqual(got, want) {
		t.Errorf("interactive: %q", got)
	}
	f = Load(Options{Path: p, Home: home, Headless: true, EUID: os.Geteuid()})
	if got := texts(f.Warnings); !reflect.DeepEqual(got, want[1:]) {
		t.Errorf("headless: %q", got)
	}
	f = Load(Options{Path: p, Home: home, Headless: true, Debug: true, EUID: os.Geteuid()})
	if got := texts(f.Warnings); !reflect.DeepEqual(got, want) {
		t.Errorf("headless debug: %q", got)
	}
}

// headless_contract_spec.sh:236 (C12): warnings never mix with values.
func TestWarningsDoNotContaminateValues(t *testing.T) {
	p := write(t, "env", "garbage line that triggers a warning\nDB:default=mysql://u:p@h:3306/db\n", 0o644)
	f := Load(Options{Path: p, Home: "/h", EUID: os.Geteuid()})
	if u, _ := f.DBURL("default"); u != "mysql://u:p@h:3306/db" {
		t.Errorf("url %q", u)
	}
}

func TestLoadUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads any file")
	}
	p := write(t, "env", "GASH_PROMPT=1\n", 0o000)
	f := Load(Options{Path: p, Home: "/h", EUID: os.Geteuid()})
	if !f.Exists || f.ReadErr == nil || len(f.Settings) != 0 {
		t.Errorf("got %+v", f)
	}
}

// LEDGER CORE-08 (forced divergence): only ASCII whitespace is trimmed, in
// every locale.
func TestLedger_CORE08(t *testing.T) {
	for _, pad := range []string{"\u3000", "\u00a0"} {
		f := Parse([]byte(pad+"GASH_DB_TIMEOUT=7"+pad+"\n"), "/e", "/h")
		if _, ok := f.Setting("GASH_DB_TIMEOUT"); ok || len(f.Warnings) != 1 ||
			f.Warnings[0].Text != "Line 1 of /e: unrecognized entry (expected SSH:, DB:, AI:, HOST:, PROTECT: or GASH_NAME=), skipping" {
			t.Errorf("%q padding: %+v", pad, f)
		}
	}
	f := Parse([]byte(" \t\v\fGASH_DB_TIMEOUT=7 \t\v\f\r\n"), "/e", "/h")
	if v, _ := f.Setting("GASH_DB_TIMEOUT"); v != "7" || len(f.Warnings) != 0 {
		t.Errorf("ASCII padding: %q %q", v, texts(f.Warnings))
	}
}

// 3.0: CORE-14 fixed: HOME is a literal prefix that ends at a "/" or at
// the end of the path. The rows marked 2.x were shown otherwise by
// ${path/#$HOME/\~}, which took HOME as a glob pattern.
func TestShownPath(t *testing.T) {
	tests := []struct{ path, home, want string }{
		{"/home/u/.gash_env", "/home/u", "~/.gash_env"},
		{"/home/u", "/home/u", "~"},
		{"/home/user2/x", "/home/u", "/home/user2/x"}, // 2.x: ~ser2/x
		{"/etc/gash_env", "/home/u", "/etc/gash_env"},
		{"/a", "", "/a"},                     // 2.x: ~/a
		{"/home/u/x", "/home/u/", "~/x"},     // 2.x: ~x
		{"/h/abc/def", "/h/*", "/h/abc/def"}, // 2.x: ~ (HOME was a glob)
		{"/h/a/x", "/h/[ab]", "/h/a/x"},      // 2.x: ~/x
		{"/h/*/x", "/h/*", "~/x"},
		{"/home/u/x", "/home/[[:alpha:]]", "/home/u/x"}, // 2.x: ~/x
		{"/x", "/", "/x"},
	}
	for _, tt := range tests {
		if got := ShownPath(tt.path, tt.home); got != tt.want {
			t.Errorf("ShownPath(%q, HOME=%q) = %q, want %q", tt.path, tt.home, got, tt.want)
		}
	}
}

// LEDGER CORE-14: HOME is matched literally and only up to a "/" or the
// end of the path (3.0: CORE-14 fixed; 2.x took HOME as an unquoted glob
// pattern matched against any prefix).
func TestLedger_CORE14(t *testing.T) {
	tests := []struct{ path, home, want string }{
		{"/home/ab/.gash_env", "/home/a", "/home/ab/.gash_env"}, // 2.x: ~b/.gash_env
		{"/home/a/.gash_env", "/home/a", "~/.gash_env"},
		{"/home/a", "/home/a", "~"},
		{"/home/bob/.gash_env", "/home/*", "/home/bob/.gash_env"}, // 2.x: ~
		{"/home/b/.gash_env", "/home/[ab]", "/home/b/.gash_env"},  // 2.x: ~/.gash_env
	}
	for _, tt := range tests {
		if got := ShownPath(tt.path, tt.home); got != tt.want {
			t.Errorf("ShownPath(%q, HOME=%q) = %q, want %q", tt.path, tt.home, got, tt.want)
		}
	}
	// The loader's messages use it.
	f := Parse([]byte("bogus\n"), "/home/ab/.gash_env", "/home/a")
	require.Equal(t, []string{"Line 1 of /home/ab/.gash_env: unrecognized entry (expected SSH:, DB:, AI:, HOST:, PROTECT: or GASH_NAME=), skipping"}, texts(f.Warnings))
}

// LEDGER CORE-15: only "~" and "~/" stand for HOME, used literally, and a
// "~user" key path is refused with a warning (3.0: CORE-15 fixed; 2.x made
// ~user/k $HOME followed by user/k, and under Bash 5.2 an "&" in HOME stood
// for the matched "~").
func TestLedger_CORE15(t *testing.T) {
	tests := []struct {
		key, home, want string
		ok              bool
	}{
		{"~user/k", "/home/u", "", false},       // 2.x: /home/uuser/k
		{"~root/.ssh/id", "/home/u", "", false}, // 2.x: /home/uroot/.ssh/id
		{"~/k", "/srv/a&b", "/srv/a&b/k", true}, // 2.x (Bash 5.2): /srv/a~b/k
		{"~/k", "/srv/a\\&b", "/srv/a\\&b/k", true},
		{"~/k", "/home/u", "/home/u/k", true},
	}
	for _, tt := range tests {
		if got, ok := ExpandTilde(tt.key, tt.home); got != tt.want || ok != tt.ok {
			t.Errorf("ExpandTilde(%q, HOME=%q) = %q %v, want %q %v", tt.key, tt.home, got, ok, tt.want, tt.ok)
		}
	}
	f := Parse([]byte("SSH:~root/.ssh/id=pw\n"), "/h/.gash_env", "/h")
	require.Empty(t, f.SSHKeys)
	require.Equal(t, []string{"Line 1 of ~/.gash_env: SSH key path starts with ~user; write the full path (skipping)"}, texts(f.Warnings))
}

func TestExpandTilde(t *testing.T) {
	tests := []struct{ key, home, want string }{
		{"~/.ssh/id", "/home/u", "/home/u/.ssh/id"},
		{"~", "/home/u", "/home/u"},
		{"/abs/~/id", "/home/u", "/abs/~/id"},
		{"rel/id", "/home/u", "rel/id"},
		// 3.0: CORE-15 fixed: HOME is used literally (2.x on Bash 5.2 read
		// "&" and "\\" in HOME as replacement syntax).
		{"~/x", "/h/a&b", "/h/a&b/x"},
		{"~/x", "/h/a\\&b", "/h/a\\&b/x"},
		{"~/x", "/h/a\\\\b", "/h/a\\\\b/x"},
		{"~/x", "/h/a\\b", "/h/a\\b/x"},
		{"~/x", "", "/x"},
	}
	for _, tt := range tests {
		if got, ok := ExpandTilde(tt.key, tt.home); got != tt.want || !ok {
			t.Errorf("ExpandTilde(%q, HOME=%q) = %q %v, want %q", tt.key, tt.home, got, ok, tt.want)
		}
	}
}

// ValidHostTarget answers as the 2.x regex does in bash, except for the
// targets 3.0 refuses (3.0: REM-02, REM-08 fixed): a port, an option-like
// user and characters outside the remote_* rule. It never accepts what
// 2.x refused.
func TestValidHostTargetMatches2x(t *testing.T) {
	refused3x := map[string]bool{"u@h:22": true, "-oProxyCommand=x@h": true, "ü@h": true, "u@h\u00a0x": true}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	for _, v := range []string{"u@h", "root@web.example.com", "u@h:22", "-oProxyCommand=x@h", "@h", "u@", "@",
		"u@@h", "u@h@x", "u h@x", "u@h\tx", "u@h\n", "\vu@h", "u@h\r", "u@h\f", "uh", "", "ü@h", "u@h x"} {
		cmd := exec.Command(bash, "--noprofile", "--norc", "-c", `[[ $1 =~ ^[^@[:space:]]+@[^@[:space:]]+$ ]]`, "_", v)
		cmd.Env = []string{"LC_ALL=C"}
		want := cmd.Run() == nil && !refused3x[v]
		if got := ValidHostTarget(v); got != want {
			t.Errorf("ValidHostTarget(%q) = %v, bash says %v", v, got, want)
		}
	}
}

// LEDGER CORE-06: File.Shown, the name the messages use, is the file read:
// GASH_ENV_FILE when set (HOME shown as "~"), else ~/.gash_env.
func TestLedger_CORE06(t *testing.T) {
	get := func(env map[string]string) func(string) string { return func(k string) string { return env[k] } }
	require.Equal(t, "/x/y", LoadCaller(get(map[string]string{"HOME": "/h", "GASH_ENV_FILE": "/x/y"})).Shown)
	require.Equal(t, "~/conf/env", LoadCaller(get(map[string]string{"HOME": "/h", "GASH_ENV_FILE": "/h/conf/env"})).Shown)
	require.Equal(t, "~/.gash_env", LoadCaller(get(map[string]string{"HOME": "/h", "GASH_ENV_FILE": ""})).Shown)
}

// LEDGER REM-02, REM-08: a HOST: target must pass the remote_* rule
// (user [A-Za-z0-9._][A-Za-z0-9._-]*, host [A-Za-z0-9][A-Za-z0-9.-]* or a
// bracketed IPv6 literal), or the line is skipped with its warning; a
// target with a ':' also gets a hint about ~/.ssh/config (2.x accepted
// option-like users and host:port, which remote_* then refused or passed
// to ssh as an option).
func TestLedger_REM02(t *testing.T) {
	for v, ok := range map[string]bool{
		"root@web.example.com": true, "deploy@10.0.0.5": true, "u@[::1]": true, "u@[2001:db8::1]": true,
		"_svc.x@h-1": true, ".u@h": true, "u-x@h": true,
		"-oProxyCommand=x@h": false, "u@h:22": false, "u@-h": false, "u@.h": false, "-u@h": false,
		"u@[10.0.0.1]": false, "u@[::1": false, "u@[]": false, "u@h_x": false, "u@h/x": false, "u@": false, "@h": false,
		"u@h@i": false, "u @h": false, "ü@h": false,
	} {
		require.Equal(t, ok, ValidHostTarget(v), v)
	}

	f := load(t, "HOST:opt=-oProxyCommand=x@h\nHOST:port=u@h:22\nHOST:v6=u@[::1]\n", "/h")
	require.Equal(t, []string{"v6=u@[::1]"}, names(f.Hosts))
	require.Len(t, f.Warnings, 2)
	require.Contains(t, f.Warnings[0].Text, "Line 1 of ")
	require.True(t, strings.HasSuffix(f.Warnings[0].Text, ": HOST target must be user@host (skipping)"))
	require.Empty(t, f.Warnings[0].Hint)
	require.Contains(t, f.Warnings[1].Text, "Line 2 of ")
	require.Equal(t, "put the port in a Host entry of ~/.ssh/config", f.Warnings[1].Hint)
	// remote_* names the skipped entry when its alias is used.
	got, ok := f.RejectedHost("port")
	require.True(t, ok)
	require.Equal(t, "u@h:22", got)
	_, ok = f.HostTarget("port")
	require.False(t, ok)
	_, ok = f.RejectedHost("v6")
	require.False(t, ok)
}
