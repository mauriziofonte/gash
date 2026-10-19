// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"database/sql/driver"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLedger_DB16: host localhost connects through the Unix socket the
// mysql client used: socket= of [client] in the option files (the last one
// read wins, includes followed), else the first existing default; nothing
// else from the option files applies, and other hosts use TCP.
func TestLedger_DB16(t *testing.T) {
	d := physTemp(t)
	home := filepath.Join(d, "home")
	mustWrite(t, filepath.Join(home, ".gash_env"),
		"DB:default=mysql://u:p@localhost/app\nDB:tcp=mysql://u:p@127.0.0.1:3307/app\nDB:maria=mariadb://u:p@localhost:3308/m\n", 0o600)
	env := baseEnv(home, nil)
	etc1, etc2 := filepath.Join(d, "etc", "my.cnf"), filepath.Join(d, "etc", "mysql", "my.cnf")
	sock1, sock2 := filepath.Join(d, "run", "mysqld.sock"), filepath.Join(d, "tmp", "mysql.sock")
	f := &fakeMySQL{answers: map[string]fakeAnswer{"SELECT 1": {cols: []string{"1"}, rows: [][]driver.Value{{int64(1)}}}}}
	m := frozen(New())
	m.openMySQL = f.open
	m.myCnfFiles = []string{etc1, etc2}
	m.socketDefaults = []string{sock1, sock2}
	addr := func(conn string) (string, string) {
		t.Helper()
		got := runGo(t, m, d, env, "llm_db_query", "SELECT 1", "-c", conn)
		require.Equal(t, 0, got.RC, got.Stderr)
		return f.cfg.Net, f.cfg.Addr
	}

	// No option file, no default exists: the first default (the connection
	// fails there, as the client's did).
	net, a := addr("default")
	require.Equal(t, "unix", net)
	require.Equal(t, sock1, a)
	// The first default that exists.
	mustWrite(t, sock2, "", 0o644)
	_, a = addr("default")
	require.Equal(t, sock2, a)
	// Other hosts: TCP to HOST:PORT.
	net, a = addr("tcp")
	require.Equal(t, "tcp", net)
	require.Equal(t, "127.0.0.1:3307", a)

	// The [client] group of the system file; other groups do not count.
	mustWrite(t, etc1, "# system\n[mysqld]\nsocket=/srv/server.sock\n[client]\nport = 1\nsocket = \"/x/my sock\" # comment\n", 0o644)
	_, a = addr("default")
	require.Equal(t, "/x/my sock", a)
	// The second file overrides, with its includes in order; a
	// world-writable file is skipped.
	inc := filepath.Join(d, "etc", "mysql", "conf.d")
	mustWrite(t, filepath.Join(inc, "b.cnf"), "[client]\nsocket=/from/b\n", 0o644)
	mustWrite(t, filepath.Join(inc, "a.cnf"), "[Client]\nsocket=/from/a\n", 0o644)
	mustWrite(t, filepath.Join(inc, "z.cnf"), "[client]\nsocket=/from/world-writable\n", 0o666)
	mustWrite(t, filepath.Join(inc, "ignored.txt"), "[client]\nsocket=/from/txt\n", 0o644)
	mustWrite(t, etc2, "!includedir "+inc+"\n", 0o644)
	_, a = addr("default")
	require.Equal(t, "/from/b", a)
	mustWrite(t, filepath.Join(d, "extra.cnf"), "[client]\nsocket='/from/include\\sfile'\n", 0o644)
	mustWrite(t, etc2, "!includedir "+inc+"\n!include "+filepath.Join(d, "extra.cnf")+"\n", 0o644)
	_, a = addr("default")
	require.Equal(t, "/from/include file", a)

	// ~/.my.cnf is read last; its user, password, host and port do not
	// apply: the DB: URL is the connection.
	mustWrite(t, filepath.Join(home, ".my.cnf"),
		"[client]\nuser=other\npassword=zzz\nhost=evil.example\nport=1\nsocket=/home/sock\n[mysql]\nsocket=/mysql/group\n", 0o600)
	_, a = addr("default")
	require.Equal(t, "/home/sock", a)
	require.Equal(t, "u", f.cfg.User)
	require.Equal(t, "p", f.cfg.Passwd)
	require.Equal(t, "app", f.cfg.DBName)
	// MariaDB URLs and gash_db_test go the same way; the port is not used.
	_, a = addr("maria")
	require.Equal(t, "/home/sock", a)
	got := runGo(t, m, d, env, "gash_db_test")
	require.Equal(t, 0, got.RC, got.Stderr)
	require.Equal(t, "unix", f.cfg.Net)
	require.Equal(t, "/home/sock", f.cfg.Addr)
	// TCP hosts still ignore every option file.
	net, a = addr("tcp")
	require.Equal(t, "tcp", net)
	require.Equal(t, "127.0.0.1:3307", a)
}

// The option-file syntax of the client: comments, quotes, escapes, names
// with _ for -, broken files.
func TestOptionFileSyntax(t *testing.T) {
	d := t.TempDir()
	read := func(content string) map[string]string {
		t.Helper()
		p := filepath.Join(d, "x.cnf")
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
		got := map[string]string{}
		readOptionFile(p, 0, func(group, name, value string) { got[group+"."+name] = value })
		return got
	}
	require.Equal(t, map[string]string{"client.socket": "/a#b", "client.default-character-set": "utf8", "client.skip-ssl": ""},
		read("  ; comment\n[client ]\nsocket = \"/a#b\"  # trailing\ndefault_character_set=utf8\nskip-ssl\n"))
	require.Equal(t, map[string]string{"client.socket": "/t\tn\nq\"x\\y\\z"},
		read("[client]\nsocket=/t\\tn\\nq\\\"x\\\\y\\z\n"))
	// An option before any group, or a broken group line, ends the file.
	require.Empty(t, read("socket=/x\n[client]\nsocket=/y\n"))
	require.Equal(t, map[string]string{"client.socket": "/y"}, read("[client]\nsocket=/y\n[broken\nsocket=/z\n"))
	// Includes nest at most ten levels deep (a loop ends).
	loop := filepath.Join(d, "loop.cnf")
	require.NoError(t, os.WriteFile(loop, []byte("[client]\nsocket=/loop\n!include "+loop+"\n"), 0o644))
	got := map[string]string{}
	readOptionFile(loop, 0, func(group, name, value string) { got[group+"."+name] = value })
	require.Equal(t, map[string]string{"client.socket": "/loop"}, got)
}
