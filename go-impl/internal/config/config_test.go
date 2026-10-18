// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
		"Line 4 of " + shown + ": unrecognized entry (expected SSH:, DB:, AI:, HOST: or GASH_NAME=), skipping",
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

// LEDGER SYS-44: the whole line is trimmed before SSH: is split, so a
// passphrase loses its trailing whitespace (2.x kept).
func TestLedger_SYS44_2x(t *testing.T) {
	key := write(t, "k", "", 0o600)
	f := load(t, "SSH:"+key+"=pass  \t\n", "/h")
	if len(f.SSHKeys) != 1 || f.SSHKeys[0].Value != "pass" {
		t.Errorf("got %+v", f.SSHKeys)
	}
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

// LEDGER CORE-04, reproduced: duplicate DB: names are all listed, the first
// one is used, and nothing is said about the second.
func TestLedger_CORE04_2x(t *testing.T) {
	f := load(t, "DB:default=mysql://a:p1@h1/d\nDB:default=pgsql://b:p2@h2/e\n", "/h")
	rows := f.DBList()
	want := []DBListRow{{"default", "mysql://a:****@h1/d"}, {"default", "pgsql://b:****@h2/e"}}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows %+v", rows)
	}
	if u, _ := f.DBURL("default"); u != "mysql://a:p1@h1/d" {
		t.Errorf("first entry wins: %q", u)
	}
	if len(f.Warnings) != 0 {
		t.Errorf("warnings %q", texts(f.Warnings))
	}
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
	// LEDGER REM-02, reproduced: an option-looking target is accepted.
	if got, _ := f.HostTarget("opt"); got != "-oProxyCommand=x@h" {
		t.Errorf("REM-02: %q", got)
	}
	if _, ok := f.HostTarget(""); ok {
		t.Error("empty alias must find nothing")
	}
	if len(f.Hosts) != 4 || len(f.Warnings) != 1 {
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
		{name: "bad DB scheme", line: "DB:prod=ftp://u:pw@h/x", warn: "DB URL must start with mysql://, mariadb:// or pgsql:// (skipping)"},
		{name: "DB scheme is case-sensitive", line: "DB:x=MYSQL://h", warn: "DB URL must start with mysql://, mariadb:// or pgsql:// (skipping)"},
		{name: "space before the URL", line: "DB:x= mysql://h", warn: "DB URL must start with mysql://, mariadb:// or pgsql:// (skipping)"},
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
		{name: "unrecognized", line: "PASSWORD=hunter2", warn: "unrecognized entry (expected SSH:, DB:, AI:, HOST: or GASH_NAME=), skipping"},
		{name: "prefix is case-sensitive", line: "ssh:x=y", warn: "unrecognized entry (expected SSH:, DB:, AI:, HOST: or GASH_NAME=), skipping"},
		{name: "GASH_ without =", line: "GASH_PROMPT", warn: "unrecognized entry (expected SSH:, DB:, AI:, HOST: or GASH_NAME=), skipping"},
		{name: "space before the colon", line: "DB :x=y", warn: "unrecognized entry (expected SSH:, DB:, AI:, HOST: or GASH_NAME=), skipping"},
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
	if got := texts(f.Warnings); !reflect.DeepEqual(got, []string{"Line 3 of /x/env: unrecognized entry (expected SSH:, DB:, AI:, HOST: or GASH_NAME=), skipping"}) {
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
	for _, leak := range []string{"GASH_NOPE", "topsecret", "PASSWORD", "hunter2", "sk-ant", "PASTED", "prod", "ftp", "pw@", "openai", "sk-zzz", "box", "nohost", "missing_key", "pass"} {
		if strings.Contains(out, leak) {
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
		"Line 2 of ~/env: unrecognized entry (expected SSH:, DB:, AI:, HOST: or GASH_NAME=), skipping",
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
			f.Warnings[0].Text != "Line 1 of /e: unrecognized entry (expected SSH:, DB:, AI:, HOST: or GASH_NAME=), skipping" {
			t.Errorf("%q padding: %+v", pad, f)
		}
	}
	f := Parse([]byte(" \t\v\fGASH_DB_TIMEOUT=7 \t\v\f\r\n"), "/e", "/h")
	if v, _ := f.Setting("GASH_DB_TIMEOUT"); v != "7" || len(f.Warnings) != 0 {
		t.Errorf("ASCII padding: %q %q", v, texts(f.Warnings))
	}
}

func TestShownPath(t *testing.T) {
	tests := []struct{ path, home, want string }{
		{"/home/u/.gash_env", "/home/u", "~/.gash_env"},
		{"/home/u", "/home/u", "~"},
		{"/home/user2/x", "/home/u", "~ser2/x"}, // not limited to path components
		{"/etc/gash_env", "/home/u", "/etc/gash_env"},
		{"/a", "", "~/a"}, // empty HOME matches the empty prefix
		{"/home/u/x", "/home/u/", "~x"},
		{"/h/abc/def", "/h/*", "~"},    // HOME is a glob pattern
		{"/h/a/x", "/h/[ab]", "~/x"},   // bracket
		{"/h/a/x", "/h/a?", "~x"},      // ?
		{"/h/*/x", "/h/\\*", "~/x"},    // escaped *
		{"/h/a/x", "/h/\\*", "/h/a/x"}, // escaped * is literal
		{"bZ", "[!]x]", "bZ"},          // umatchlen and the matcher disagree, as in Bash
		{"]b", "[]x]", "~b"},           // leading ] is a member
		{"\\~", "\\", "\\~"},           // a lone backslash matches nothing here
		{"-b\t\\", "*\\", "-b\t\\"},    // quick check pattern *\* needs a final *
		{"a\\x", "a\\", "a\\x"},        // trailing backslash
		{"*a-z", "**\\*", "*a-z"},      // pattern starting and ending with *
		{"x*", "**\\*", "~"},           // ... matches a final *
		{"ab", "?(", "ab"},             // ?( is not extglob here
		{"/home/u/x", "/home/[[:alpha:]]", "~/x"},
	}
	for _, tt := range tests {
		if got := ShownPath(tt.path, tt.home); got != tt.want {
			t.Errorf("ShownPath(%q, HOME=%q) = %q, want %q", tt.path, tt.home, got, tt.want)
		}
	}
}

// LEDGER CORE-14, reproduced: ${file/#$HOME/\~} takes HOME as an unquoted
// glob pattern matched against any prefix, not a path component.
func TestLedger_CORE14_2x(t *testing.T) {
	tests := []struct{ path, home, want string }{
		{"/home/ab/.gash_env", "/home/a", "~b/.gash_env"},
		{"/home/a/.gash_env", "/home/a", "~/.gash_env"},
		{"/home/a", "/home/a", "~"},
		{"/home/bob/.gash_env", "/home/*", "~"},
		{"/home/b/.gash_env", "/home/[ab]", "~/.gash_env"},
	}
	for _, tt := range tests {
		if got := ShownPath(tt.path, tt.home); got != tt.want {
			t.Errorf("ShownPath(%q, HOME=%q) = %q, want %q", tt.path, tt.home, got, tt.want)
		}
	}
}

// LEDGER CORE-15, reproduced: ${key/#\~/$HOME} replaces only the "~", so
// ~user/k becomes $HOME followed by "user/k", and under Bash 5.2
// patsub_replacement an "&" in HOME stands for the matched "~".
func TestLedger_CORE15_2x(t *testing.T) {
	tests := []struct{ key, home, want string }{
		{"~user/k", "/home/u", "/home/uuser/k"},
		{"~root/.ssh/id", "/home/u", "/home/uroot/.ssh/id"},
		{"~/k", "/srv/a&b", "/srv/a~b/k"},
		{"~/k", "/srv/a\\&b", "/srv/a&b/k"},
		{"~/k", "/home/u", "/home/u/k"},
	}
	for _, tt := range tests {
		if got := ExpandTilde(tt.key, tt.home); got != tt.want {
			t.Errorf("ExpandTilde(%q, HOME=%q) = %q, want %q", tt.key, tt.home, got, tt.want)
		}
	}
}

func TestExpandTilde(t *testing.T) {
	tests := []struct{ key, home, want string }{
		{"~/.ssh/id", "/home/u", "/home/u/.ssh/id"},
		{"~", "/home/u", "/home/u"},
		{"~root/.ssh/id", "/home/u", "/home/uroot/.ssh/id"},
		{"/abs/~/id", "/home/u", "/abs/~/id"},
		{"~/x", "/h/a&b", "/h/a~b/x"}, // Bash 5.2 patsub_replacement
		{"~/x", "/h/a\\&b", "/h/a&b/x"},
		{"~/x", "/h/a\\\\b", "/h/a\\b/x"},
		{"~/x", "/h/a\\b", "/h/a\\b/x"},
		{"~/x", "/h/\\", "/h/\\/x"},
		{"~/x", "/h/a\\\\&b", "/h/a\\~b/x"},
		{"~/x", "", "/x"},
	}
	for _, tt := range tests {
		if got := ExpandTilde(tt.key, tt.home); got != tt.want {
			t.Errorf("ExpandTilde(%q, HOME=%q) = %q, want %q", tt.key, tt.home, got, tt.want)
		}
	}
}

// ValidHostTarget answers as the 2.x regex does in bash.
func TestValidHostTargetMatches2x(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	for _, v := range []string{"u@h", "root@web.example.com", "u@h:22", "-oProxyCommand=x@h", "@h", "u@", "@",
		"u@@h", "u@h@x", "u h@x", "u@h\tx", "u@h\n", "\vu@h", "u@h\r", "u@h\f", "uh", "", "ü@h", "u@h x"} {
		cmd := exec.Command(bash, "--noprofile", "--norc", "-c", `[[ $1 =~ ^[^@[:space:]]+@[^@[:space:]]+$ ]]`, "_", v)
		cmd.Env = []string{"LC_ALL=C"}
		want := cmd.Run() == nil
		if got := ValidHostTarget(v); got != want {
			t.Errorf("ValidHostTarget(%q) = %v, bash says %v", v, got, want)
		}
	}
}
