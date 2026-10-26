// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/remote"
	"github.com/mauriziofonte/gash/internal/remote/sshtest"
)

// sshDBFixture is a fake remote (sshtest) whose PATH starts with stub
// mysql, psql and docker clients that log their arguments, the password
// variable and their stdin under the remote home; sqlite3 is the real one.
type sshDBFixture struct {
	cwd, home, rhome string
	env              map[string]string
	ssh              *sshtest.SSH
	m                *Module
}

const stubMySQL = `#!/bin/sh
printf '%s\n' "$@" >"$HOME/mysql.args"
printf '%s' "${MYSQL_PWD-unset}" >"$HOME/mysql.pw"
cat >"$HOME/mysql.stdin"
case " $* " in *" fail "*) echo "ERROR 1064 (42000) at line 3: You have an error in your SQL syntax" >&2; exit 1 ;; esac
case " $* " in *" --raw "*) printf '10.11.6-MariaDB\n'; exit 0 ;; esac
printf 'id\tname\tnote\n1\tAlice\tNULL\n2\tBob\ta\\tb\\nc\n'
`

const stubPsql = `#!/bin/sh
printf '%s\n' "$@" >"$HOME/psql.args"
printf '%s|%s' "${PGPASSWORD-unset}" "${PGOPTIONS-unset}" >"$HOME/psql.env"
cat >"$HOME/psql.stdin"
case " $* " in
  *" --csv "*" null=__gash_null__ "*) if grep -q nulls "$HOME/psql.stdin"; then printf 'a,b\n__gash_null__,""\n'; else printf 'search_path\n"""$user"", public"\n'; fi ;;
  *) printf '[{"id": 1, "name": "Alice"}, {"id": 2, "name": null}]\n' ;;
esac
`

const stubDocker = `#!/bin/sh
printf '%s\n' "$@" >"$HOME/docker.args"
[ "$1" = exec ] || exit 2
shift
while [ $# -gt 0 ]; do
  case $1 in -i) shift ;; -e) shift 2 ;; *) break ;; esac
done
[ "$1" = gone ] && { echo "Error response from daemon: No such container: gone" >&2; exit 1; }
shift
exec "$@"
`

func newSSHDBFixture(t *testing.T) *sshDBFixture {
	t.Helper()
	f := &sshDBFixture{cwd: physTemp(t), home: physTemp(t), rhome: physTemp(t)}
	stub := physTemp(t)
	mustWrite(t, filepath.Join(stub, "mysql"), stubMySQL, 0o755)
	mustWrite(t, filepath.Join(stub, "psql"), stubPsql, 0o755)
	mustWrite(t, filepath.Join(stub, "docker"), stubDocker, 0o755)
	mustWrite(t, filepath.Join(f.home, ".gash_env"), strings.Join([]string{
		"HOST:web=deploy@web.example",
		"HOST:db=deploy@db.example",
		"DB:shop=mysql+ssh://shop:s3cr%40t@web/shop",
		"DB:port=mariadb+ssh://shop@web:3307/shop",
		"DB:pg=pgsql+ssh://app:pw@db/app+docker:pg",
		"DB:gone=pgsql+ssh://app:pw@db/app+docker:gone",
		"DB:lite=sqlite+ssh://web/data/app.db",
		"DB:nohost=mysql+ssh://shop@nope/shop",
		"DB:badport=mysql+ssh://shop@web:x/shop",
		"",
	}, "\n"), 0o600)
	require.NoError(t, os.MkdirAll(filepath.Join(f.rhome, "data"), 0o755))
	sqliteFixture(t, filepath.Join(f.rhome, "data"), "CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT); INSERT INTO users VALUES (1,'Alice'),(2,NULL);")
	require.NoError(t, os.Rename(filepath.Join(f.rhome, "data", "t.db"), filepath.Join(f.rhome, "data", "app.db")))
	f.ssh = &sshtest.SSH{Home: f.rhome, Path: stub + ":" + os.Getenv("PATH")}
	f.m = frozen(New())
	f.m.ssh = f.ssh
	f.env = baseEnv(f.home, nil)
	return f
}

func (f *sshDBFixture) run(t *testing.T, fn string, args ...string) result {
	t.Helper()
	return runGo(t, f.m, f.cwd, f.env, fn, args...)
}

func (f *sshDBFixture) remote(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.rhome, name))
	require.NoError(t, err)
	return string(b)
}

func TestSSHMySQLOneInvocation(t *testing.T) {
	f := newSSHDBFixture(t)
	r := f.run(t, "llm_db_query", "SELECT id, name, note FROM users", "-c", "shop")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Regexp(t, `^\{"data":\[\{"id":"1","name":"Alice","note":"NULL"\},\{"id":"2","name":"Bob","note":"a\\tb\\nc"\}\],"rows":2,"query_time":"[^"]+"\}$`, strings.TrimSpace(r.Stdout))
	require.Equal(t, 1, f.ssh.N(), "one ssh invocation per query")
	require.NotContains(t, f.ssh.Scripts[0], "s3cr@t", "the password never goes in the remote command")
	require.Equal(t, "s3cr@t", f.remote(t, "mysql.pw"), "it reaches the client as MYSQL_PWD")
	require.Equal(t, "--batch\n--default-character-set=utf8mb4\n-u\nshop\nshop\n", f.remote(t, "mysql.args"), "socket by default")
	require.Equal(t, "SET SESSION TRANSACTION READ ONLY;\nSET SESSION sql_select_limit = 100;\nSELECT id, name, note FROM users\n", f.remote(t, "mysql.stdin"))

	r = f.run(t, "llm_db_query", "SELECT id, name, note FROM users", "-c", "shop", "--format", "jsonl")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Equal(t, `{"id":"1","name":"Alice","note":null}`+"\n"+`{"id":"2","name":"Bob","note":"a\tb\nc"}`+"\n", r.Stdout, "NULL is null in the row formats")

	r = f.run(t, "llm_db_tables", "-c", "port")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Contains(t, f.remote(t, "mysql.args"), "-h\n127.0.0.1\n-P\n3307\n--protocol=TCP\n")
	require.Equal(t, "unset", f.remote(t, "mysql.pw"))

	r = f.run(t, "llm_db_explain", "SELECT 1", "-c", "shop", "--analyze")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Contains(t, f.remote(t, "mysql.stdin"), "ANALYZE FORMAT=JSON SELECT 1", "MariaDB found through the raw VERSION()")

	r = f.run(t, "llm_db_query", "SELECT fail", "-c", "shop", "-d", "fail")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Stderr, `"error":"mysql_error","details":"Error 1064 (42000): You have an error in your SQL syntax"`)
}

func TestSSHGuardRunsBeforeConnecting(t *testing.T) {
	f := newSSHDBFixture(t)
	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"DELETE FROM users", "-c", "shop"}, `"action":"FATAL"`},
		{[]string{"UPDATE users SET name = 'x'", "-c", "pg"}, `"action":"FATAL"`},
		{[]string{"ATTACH DATABASE 'x' AS y", "-c", "lite"}, `"action":"FATAL"`},
		{[]string{"SELECT 1", "-c", "nohost"}, `"error":"unknown_host"`},
		{[]string{"SELECT 1", "-c", "badport"}, `"error":"invalid_db_url"`},
		{[]string{"SELECT 1", "-c", "shop", "-d", "bad name"}, `"error":"invalid_database_name"`},
	} {
		r := f.run(t, "llm_db_query", tc.args...)
		require.Equal(t, 1, r.RC, "%q", tc.args)
		require.Contains(t, r.Stderr, tc.code, "%q", tc.args)
	}
	require.Zero(t, f.ssh.N(), "nothing left the machine")
}

func TestSSHPostgresInAContainer(t *testing.T) {
	f := newSSHDBFixture(t)
	r := f.run(t, "llm_db_query", "SELECT id, name FROM users", "-c", "pg")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Regexp(t, `^\{"data":\[\{"id":1,"name":"Alice"\},\{"id":2,"name":null\}\],"rows":2,`, r.Stdout)
	require.Equal(t, "exec\n-i\n-e\nPGPASSWORD\n-e\nPGOPTIONS\npg\npsql\n-X\n-q\n-v\nON_ERROR_STOP=1\n-A\n-t\n-U\napp\n-d\napp\n-f\n-\n", f.remote(t, "docker.args"))
	require.Equal(t, "pw|-c default_transaction_read_only=on -c standard_conforming_strings=on -c statement_timeout=30s", f.remote(t, "psql.env"))
	require.Contains(t, f.remote(t, "psql.stdin"), "jsonb_agg(to_jsonb(__gash_row))")

	r = f.run(t, "llm_db_query", "SHOW search_path", "-c", "pg")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Contains(t, r.Stdout, `{"data":[{"search_path":"\"$user\", public"}],"rows":1,`)

	r = f.run(t, "llm_db_query", "SHOW nulls", "-c", "pg")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Contains(t, r.Stdout, `{"data":[{"a":"","b":""}],"rows":1,`, "a NULL is \"\" in the envelope")
	r = f.run(t, "llm_db_query", "SHOW nulls", "-c", "pg", "--format", "jsonl")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Equal(t, `{"a":null,"b":""}`+"\n", r.Stdout, "and null in the row formats")

	r = f.run(t, "llm_db_query", "SELECT 1", "-c", "gone")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Stderr, `"error":"container_unavailable","details":"db: Error response from daemon: No such container: gone"`)
}

func TestSSHSQLiteIsTheLocalAnswer(t *testing.T) {
	f := newSSHDBFixture(t)
	for _, tc := range [][]string{
		{"llm_db_query", "SELECT * FROM users ORDER BY id"},
		{"llm_db_tables"},
		{"llm_db_schema", "users"},
		{"llm_db_sample", "users"},
		{"llm_db_explain", "SELECT * FROM users WHERE id = 1"},
	} {
		local := f.run(t, tc[0], append(tc[1:], "-f", filepath.Join(f.rhome, "data", "app.db"))...)
		require.Equal(t, 0, local.RC, local.Stderr)
		n := f.ssh.N()
		got := f.run(t, tc[0], append(tc[1:], "-c", "lite")...)
		got.Stdout = queryTimeRE.ReplaceAllString(got.Stdout, `"query_time":"0µs"`)
		require.Equal(t, local, got, "%q", tc)
		require.Equal(t, n+1, f.ssh.N(), "%q", tc)
	}
	r := f.run(t, "llm_db_query", "SELECT nope FROM users", "-c", "lite")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Stderr, `"error":"sqlite_error"`)
	require.Contains(t, r.Stderr, "no such column: nope")
}

func TestSSHURLParsing(t *testing.T) {
	via := &sshVia{host: remoteHost("web"), container: "api"}
	got, p, ok := parseSQLiteSSH(sqliteSSHFile(via, "/var/lib/app.db"))
	require.True(t, ok)
	require.Equal(t, via, got)
	require.Equal(t, "/var/lib/app.db", p)
	_, _, ok = parseSQLiteSSH("/tmp/x.db")
	require.False(t, ok)
	require.Equal(t, "Error 1146 (42S02): Table 'x' doesn't exist", mysqlMessage("ERROR 1146 (42S02) at line 3: Table 'x' doesn't exist\n"))
	require.Equal(t, "a\tb\nc\\d\x00", mysqlUnescape(`a\tb\nc\\d\0`))
}

// queryTimeRE is the query_time the host measured.
var queryTimeRE = regexp.MustCompile(`"query_time":"[^"]*"`)

func remoteHost(alias string) remote.Host {
	return remote.Host{Alias: alias, Target: "deploy@" + alias + ".example"}
}
