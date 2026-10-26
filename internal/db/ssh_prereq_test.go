// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/prereq"
)

// The 3.4 prerequisites of the +ssh databases: ssh here, the client (and
// docker) on the host, and a client new enough for the options this code
// passes.

// oldPsql is a PostgreSQL 11 psql: no --csv.
const oldPsql = `#!/bin/sh
[ "$1" = --version ] && { echo 'psql (PostgreSQL) 11.22 (Debian 11.22-0+deb10u1)'; exit 0; }
cat >/dev/null
for a; do [ "$a" = --csv ] && { echo "psql: unrecognized option '--csv'" >&2; echo 'Try "psql --help" for more information.' >&2; exit 1; }; done
echo '[]'
`

// oldSqlite is a sqlite3 3.31: no -json.
const oldSqlite = `#!/bin/sh
[ "$1" = --version ] && { echo '3.31.1 2020-01-27 19:55:54 3bfa9cc97da10598521b342961df8f5f68c7388fa117345eec516236d1aa2ce5'; exit 0; }
cat >/dev/null
echo 'sqlite3: Error: unknown option: -json' >&2; echo 'Use -help for a list of options.' >&2; exit 1
`

// firstOnRemote puts a directory holding the named stub scripts first on
// the fake remote PATH.
func firstOnRemote(t *testing.T, f *sshDBFixture, stubs map[string]string) {
	t.Helper()
	dir := physTemp(t)
	for name, body := range stubs {
		mustWrite(t, filepath.Join(dir, name), body, 0o755)
	}
	f.ssh.Path = dir + ":" + f.ssh.Path
}

// TestSSHOldClients: an old psql (no --csv) or sqlite3 (no -json) was
// pgsql_error/sqlite_error "Fix the SQL and retry"; it is named now, with
// the version the host has, and a real SQL error still blames the SQL.
func TestSSHOldClients(t *testing.T) {
	f := newSSHDBFixture(t)
	f.env["GASH_ENV_FILE"] = filepath.Join(f.home, ".gash_env")
	env, err := os.ReadFile(f.env["GASH_ENV_FILE"])
	require.NoError(t, err)
	mustWrite(t, f.env["GASH_ENV_FILE"], string(env)+"DB:pgweb=pgsql+ssh://app:pw@web/app\n", 0o600)
	firstOnRemote(t, f, map[string]string{"psql": oldPsql, "sqlite3": oldSqlite})

	r := f.run(t, "llm_db_query", "SHOW search_path", "-c", "pgweb")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Stderr, `{"error":"prerequisite_missing","details":"psql 12 or newer is needed on web (found 11.22)","action":"STOP","recoverable":true,`+
		`"hint":"Upgrade psql on web (usually the postgresql-client package), then retry","missing":"psql","host":"web"`)
	require.NotContains(t, r.Stderr, "Fix the SQL")

	r = f.run(t, "llm_db_query", "SHOW search_path", "-c", "pg")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Stderr, `"details":"psql 12 or newer is needed in container pg on db (found 11.22)"`)
	require.Contains(t, r.Stderr, `"missing":"psql","host":"db","container":"pg"`)

	// The JSON path takes no --csv: psql 11 answers it.
	r = f.run(t, "llm_db_query", "SELECT 1", "-c", "pgweb")
	require.Equal(t, 0, r.RC, r.Stderr)

	r = f.run(t, "llm_db_query", "SELECT * FROM users", "-c", "lite")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Stderr, `{"error":"prerequisite_missing","details":"sqlite3 3.33 or newer is needed on web (found 3.31.1)","action":"STOP","recoverable":true,`+
		`"hint":"Upgrade sqlite3 on web (usually the sqlite3 package), then retry","missing":"sqlite3","host":"web"`)

	// No version printed: the "unknown option" error tells.
	firstOnRemote(t, f, map[string]string{"sqlite3": strings.Replace(oldSqlite, "echo '3.31.1", "exit 1; echo '3.31.1", 1)})
	r = f.run(t, "llm_db_query", "SELECT * FROM users", "-c", "lite")
	require.Contains(t, r.Stderr, `"details":"sqlite3 3.33 or newer is needed on web (found an older version)"`)
}

// TestSSHNewClientSQLErrorBlamesTheSQL: with a client new enough, a failed
// query is still the engine's error.
func TestSSHNewClientSQLErrorBlamesTheSQL(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("no sqlite3")
	}
	f := newSSHDBFixture(t)
	r := f.run(t, "llm_db_query", "SELECT nope FROM users", "-c", "lite")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Stderr, `"error":"sqlite_error"`)
	require.Contains(t, r.Stderr, "Fix the SQL and retry")
	require.Equal(t, []string{}, oldVersions(oldClient("psql (PostgreSQL) 16.4", "ERROR: syntax error", "--csv", 12, 0)))
	require.Equal(t, []string{"9.6.24"}, oldVersions(oldClient("psql (PostgreSQL) 9.6.24", "", "--csv", 12, 0)))
	require.Equal(t, []string{""}, oldVersions(oldClient("", "psql: illegal option -- --csv", "--csv", 12, 0)))
	require.Equal(t, []string{}, oldVersions(oldClient("", "ERROR: column \"--csv\" does not exist", "--csv", 12, 0)))
}

func oldVersions(found string, old bool) []string {
	if !old {
		return []string{}
	}
	return []string{found}
}

// TestSSHMissingOnTheHost: without docker on the host a +docker URL was
// container_unavailable ("check the container name"); without the client
// the envelope now carries "missing" and "host".
func TestSSHMissingOnTheHost(t *testing.T) {
	f := newSSHDBFixture(t)
	dir := physTemp(t)
	for _, name := range []string{"sh", "date", "cat"} {
		p, err := exec.LookPath(name)
		require.NoError(t, err)
		require.NoError(t, os.Symlink(p, filepath.Join(dir, name)))
	}
	f.ssh.Path = dir
	r := f.run(t, "llm_db_query", "SELECT 1", "-c", "pg")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Stderr, `{"error":"prerequisite_missing","details":"llm_db_query on db needs docker, which is not installed there","action":"STOP","recoverable":true,`+
		`"hint":"Install docker on db (usually the docker package), then retry","missing":"docker","host":"db"`)

	r = f.run(t, "llm_db_query", "SELECT 1", "-c", "shop")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Stderr, `{"error":"mysql_not_found","details":"mysql is not installed on web","action":"FATAL","recoverable":false,`+
		`"hint":"Install the mysql client on the host, or name its container with +docker:CONTAINER in the URL","missing":"mysql","host":"web"`)

	// Inside a container (the stub docker runs the client on the host).
	mustWrite(t, filepath.Join(dir, "docker"), stubDocker, 0o755)
	r = f.run(t, "llm_db_query", "SELECT 1", "-c", "pg")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Stderr, `{"error":"psql_not_found","details":"psql is not installed in container pg on db","action":"FATAL","recoverable":false,`+
		`"hint":"Install the psql client in the container, or name another one with +docker:CONTAINER in the URL","missing":"psql","host":"db","container":"pg"`)
}

// TestSSHWithoutLocalSSH: host_unreachable (RETRY) "check the host is up"
// became the prerequisite report, after the SQL guard and before anything
// connects.
func TestSSHWithoutLocalSSH(t *testing.T) {
	defer prereq.SetSystem(&prereq.System{OS: platform.OS{GOOS: "linux", ID: "debian", Name: "Debian GNU/Linux 12 (bookworm)"}, Manager: prereq.Apt, Sudo: true,
		LookPath: func(name string, _ []string) (string, bool) { return "/usr/bin/" + name, name != "ssh" }})()
	f := newSSHDBFixture(t)
	for _, conn := range []string{"shop", "pg", "lite"} {
		r := f.run(t, "llm_db_query", "SELECT 1", "-c", conn)
		require.Equal(t, 1, r.RC, conn)
		require.Contains(t, r.Stderr, `{"error":"prerequisite_missing","details":"llm_db_query needs ssh, which is not installed","action":"STOP","recoverable":true,`+
			`"hint":"Install openssh-client: sudo apt-get install -y openssh-client","missing":"ssh","install":"sudo apt-get install -y openssh-client"`, conn)
	}
	r := f.run(t, "llm_db_query", "DELETE FROM users", "-c", "shop")
	require.Contains(t, r.Stderr, `"action":"FATAL"`, "the guard answers first")
	require.NotContains(t, r.Stderr, "prerequisite_missing")
	require.Zero(t, f.ssh.N())
}
