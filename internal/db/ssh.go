// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

// llm_db_* over ssh (plan D2): a DB: connection whose scheme ends in +ssh
// runs the engine's own client on a registered host, in ONE ssh
// invocation per query, optionally inside a container:
//
//	DB:shop=mysql+ssh://user:pass@web/shop        mysql on host web
//	DB:shop=mariadb+ssh://user@web:3307/shop      127.0.0.1:3307 there
//	DB:app=pgsql+ssh://app:pw@db/app+docker:pg    psql inside container pg
//	DB:local=sqlite+ssh://web/data/app.db         ~/data/app.db on web
//	DB:abs=sqlite+ssh://web//var/lib/app.db       /var/lib/app.db
//
// The host part is a HOST: alias, never a raw host. The client connects
// to the server local to the host (its socket, or 127.0.0.1:PORT when the
// URL names a port). The SQL guard has run on the query before any of
// this (guardedTarget): nothing leaves the machine for a refused query.
// The password and the SQL go on the ssh stdin (the password as the
// MYSQL_PWD or PGPASSWORD of the client, never an argument); the session
// is read-only as the native drivers make it.

import (
	"bytes"
	"encoding/csv"
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/remote"
)

// sshVia is where a +ssh target runs.
type sshVia struct {
	host      remote.Host
	container string
}

// sqliteSSHMark starts the file of a sqlite+ssh target, which carries
// its host: no local path holds a NUL, so sqliteRun (which gets the file
// only) knows the file is remote and where.
const sqliteSSHMark = "\x00ssh\x00"

// sqliteSSHFile is the file of a sqlite+ssh target.
func sqliteSSHFile(via *sshVia, p string) string {
	return sqliteSSHMark + strings.Join([]string{via.host.Alias, via.host.Target, via.container, p}, "\x00")
}

// parseSQLiteSSH reads a sqliteSSHFile back.
func parseSQLiteSSH(file string) (*sshVia, string, bool) {
	rest, ok := strings.CutPrefix(file, sqliteSSHMark)
	f := strings.SplitN(rest, "\x00", 4)
	if !ok || len(f) != 4 {
		return nil, "", false
	}
	return &sshVia{host: remote.Host{Alias: f[0], Target: f[1]}, container: f[2]}, f[3], true
}

var (
	dockerSuffixRE = lazyre.MustCompile(`\+docker:([A-Za-z0-9][A-Za-z0-9_.-]{0,127})$`)
)

// sshTarget resolves a +ssh connection (ok false for any other target,
// which target resolves as before).
func (m *Module) sshTarget(c *invoke.Call, a *dbArgs) (target, *output, bool) {
	if a.sqliteFile != "" {
		return target{}, nil, false
	}
	f := config.LoadCaller(c.Env.Get)
	url, ok := f.DBURL(a.connection)
	scheme, rest, _ := strings.Cut(url, "://")
	engine, isSSH := strings.CutSuffix(scheme, "+ssh")
	if !ok || !isSSH {
		return target{}, nil, false
	}
	fail := func(code, details, hint string) (target, *output, bool) {
		o := failed(contract.New(code, details, hint))
		return target{}, &o, true
	}
	container := ""
	if mm := dockerSuffixRE.FindStringSubmatch(rest); mm != nil {
		container = mm[1]
		rest = rest[:len(rest)-len(mm[0])]
	}
	if engine == "sqlite" {
		alias, p, _ := strings.Cut(rest, "/")
		if alias == "" || p == "" {
			return fail("invalid_db_url", "Connection '"+a.connection+"' has an invalid URL", "sqlite+ssh://ALIAS/relative/file.db or sqlite+ssh://ALIAS//absolute/file.db")
		}
		h, e := remote.Resolve(c.Env.Get, alias)
		if e != nil {
			o := failed(*e)
			return target{}, &o, true
		}
		if e := remote.CheckRead(p); e != nil {
			o := failed(*e)
			return target{}, &o, true
		}
		if e := remote.SSHMissing(c, c.Name); e != nil {
			o := failed(*e)
			return target{}, &o, true
		}
		via := &sshVia{host: h, container: container}
		return target{driver: "sqlite", file: sqliteSSHFile(via, p), via: via}, nil, true
	}
	switch engine {
	case "mysql", "mariadb", "pgsql":
	default:
		return fail("unknown_db_type", scheme, "Unsupported database driver. Only mysql, mariadb, pgsql and sqlite go over ssh")
	}
	u := config.ParseDBURL(engine + "://" + rest)
	pass := config.DecodePassword(u.Password)
	if strings.ContainsAny(pass, "\n\x00") {
		return fail("invalid_db_url", "Connection '"+a.connection+"' has an invalid URL", "A +ssh connection's password cannot hold a newline")
	}
	h, e := remote.Resolve(c.Env.Get, u.Host)
	if e != nil {
		o := failed(*e)
		return target{}, &o, true
	}
	t := target{driver: engine, user: u.User, pass: pass, database: u.Database, via: &sshVia{host: h, container: container}}
	hostPart := rest
	if i := strings.LastIndexByte(hostPart, '@'); i >= 0 {
		hostPart = hostPart[i+1:]
	}
	if i := strings.IndexByte(hostPart, '/'); i >= 0 {
		hostPart = hostPart[:i]
	}
	if _, port, ok := strings.Cut(hostPart, ":"); ok {
		if !digitsRE.MatchString(port) {
			return fail("invalid_db_url", "Connection '"+a.connection+"' has an invalid URL", "The port of a +ssh URL is the server's port on the host, digits only")
		}
		t.host, t.port = "127.0.0.1", port
	}
	switch {
	case a.database != "":
		if e := validDBName(a.database, "-d"); e != nil {
			o := failed(*e)
			return target{}, &o, true
		}
		t.database = a.database
	case t.database == "":
		if name := targetDatabase(); name != "" {
			if e := validDBName(name, ".target-database"); e != nil {
				o := failed(*e)
				return target{}, &o, true
			}
			t.database = name
		}
	}
	if t.database == "" {
		return fail("no_database", "Database name not specified", "Ask user for database name. Use -d option or specify in connection URL")
	}
	if e := validDBName(t.database, "URL"); e != nil {
		o := failed(*e)
		return target{}, &o, true
	}
	// ssh is looked up once the URL is known good, before anything
	// connects: without it, the prerequisite report, not host_unreachable.
	if e := remote.SSHMissing(c, c.Name); e != nil {
		o := failed(*e)
		return target{}, &o, true
	}
	return t, nil, true
}

// sshClient runs one client invocation on the host: argv is the client
// command (quoted here), env the variables it takes from the ssh stdin's
// first line (the password) or sets, sql the rest of stdin. It returns the
// client's stdout, its stderr and exit status, or a failure output. When
// the client fails, the same invocation asks it for its version ("version"
// frame, returned as version): an old psql or sqlite3 rejects the options
// of this code, which is not the SQL's fault. Without docker on the host
// (+docker:CTR) it answers "missing docker".
func (m *Module) sshClient(c *invoke.Call, via *sshVia, argv []string, pwVar string, env map[string]string, pass, sql string) (out []byte, errText string, rc int, version string, fail *output) {
	nonce := remote.NewNonce()
	var s strings.Builder
	s.WriteString("N=" + remote.Quote(nonce) + "\n")
	s.WriteString("IFS= read -r GASH_PW || GASH_PW=\n")
	keys := []string{}
	if pwVar != "" {
		s.WriteString(pwVar + "=$GASH_PW; export " + pwVar + "\n")
		keys = append(keys, pwVar)
	}
	for _, k := range []string{"PGOPTIONS"} {
		if v, ok := env[k]; ok {
			s.WriteString(k + "=" + remote.Quote(v) + "; export " + k + "\n")
			keys = append(keys, k)
		}
	}
	s.WriteString("unset GASH_PW\n")
	cmd := remote.QuoteAll(argv)
	vcmd := remote.QuoteAll([]string{argv[0], "--version"})
	if via.container != "" {
		pre := []string{"docker", "exec", "-i"}
		for _, k := range keys {
			pre = append(pre, "-e", k)
		}
		cmd = remote.QuoteAll(append(append(pre, via.container), argv...))
		vcmd = remote.QuoteAll([]string{"docker", "exec", via.container, argv[0], "--version"})
		s.WriteString(`command -v docker >/dev/null 2>&1 || { printf '%s missing docker\n' "$N" >&2; ` + remote.Trailer(nonce, "rc", "127") + "; exit 0; }\n")
	}
	s.WriteString(`gash_t() { date +%s%N 2>/dev/null; }` + "\n")
	s.WriteString(`s=$(gash_t); ` + cmd + `; r=$?; e=$(gash_t)` + "\n")
	s.WriteString(`printf '%s time %s %s\n' "$N" "$s" "$e" >&2` + "\n")
	s.WriteString(`if [ "$r" != 0 ] && [ "$r" != 127 ]; then ` + vcmd + ` 2>/dev/null </dev/null | { IFS= read -r gv || :; printf '%s version %s\n' "$N" "$gv" >&2; }; fi` + "\n")
	s.WriteString(remote.Trailer(nonce, "rc", `"$r"`) + "\nexit 0\n")
	tr := remote.Transport{Run: m.ssh, Env: c.Env}
	timeout := dbTimeout(c) + 15*time.Second
	t0 := m.now()
	r := tr.Exec(ctxOf(c), via.host, nonce, s.String(), strings.NewReader(pass+"\n"+sql), timeout)
	elapsed := m.now().Sub(t0)
	if e := r.Failure(via.host); e != nil {
		o := failed(*e)
		return nil, "", 0, "", &o
	}
	if tool, ok := remote.MissingFrame(r.Stderr, nonce); ok {
		o := failed(remote.MissingOnHost(c.Name, tool, via.host, ""))
		return nil, "", 0, "", &o
	}
	version, _ = remote.Frame(r.Stderr, nonce, "version")
	took := elapsed
	if tm, ok := remote.Frame(r.Stderr, nonce, "time"); ok {
		a, b, _ := strings.Cut(tm, " ")
		if d, ok := nanoSpan(a, b); ok {
			took = d
		}
	}
	m.addQueryTime(c, took)
	errText = string(r.Stderr)
	if i := strings.Index(errText, nonce+" time "); i >= 0 {
		errText = errText[:i]
	}
	if r.Status == 127 && via.container == "" {
		return nil, errText, r.Status, version, nil
	}
	if via.container != "" && r.Status != 0 && containerFailed(errText) {
		o := failed(contract.New("container_unavailable", via.host.Alias+": "+cleanError(lastLine(errText)), "Check the container name and that it runs (docker ps on the host)"))
		return nil, "", 0, "", &o
	}
	return r.Stdout, errText, r.Status, version, nil
}

// clientVersionRE is the version in a client's --version line ("psql
// (PostgreSQL) 11.22", "3.31.1 2020-01-27 ...").
var clientVersionRE = lazyre.MustCompile(`[0-9]+(\.[0-9]+)+`)

// oldClient reports a client too old for an option this code passes
// (psql --csv, sqlite3 -json), from the version it printed or, when it
// printed none, from its "unknown option" error; found is the version.
func oldClient(version, errText, option string, major, minor int) (found string, old bool) {
	found = clientVersionRE.FindString(version)
	if found != "" {
		maj, rest, _ := strings.Cut(found, ".")
		mnr, _, _ := strings.Cut(rest, ".")
		a, _ := strconv.Atoi(maj)
		b, _ := strconv.Atoi(mnr)
		return found, a < major || (a == major && b < minor)
	}
	if !strings.Contains(errText, option) {
		return "", false
	}
	for _, s := range []string{"unrecognized option", "unknown option", "illegal option", "invalid option"} {
		if strings.Contains(errText, s) {
			return "", true
		}
	}
	return "", false
}

// containerFailed reports a docker exec that did not reach the client.
func containerFailed(stderr string) bool {
	for _, s := range []string{"No such container", "is not running", "docker: not found", "docker: command not found", "Cannot connect to the Docker daemon", "permission denied while trying to connect"} {
		if strings.Contains(stderr, s) {
			return true
		}
	}
	return false
}

func lastLine(s string) string {
	s = strings.TrimRight(s, "\n\r ")
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// nanoSpan is the time between two `date +%s%N` readings.
func nanoSpan(a, b string) (time.Duration, bool) {
	if len(a) < 16 || len(b) < 16 {
		return 0, false
	}
	x, err1 := strconv.ParseInt(a, 10, 64)
	y, err2 := strconv.ParseInt(b, 10, 64)
	if err1 != nil || err2 != nil || y < x {
		return 0, false
	}
	return time.Duration(y - x), true
}

// addQueryTime counts a query that ran remotely in the envelope's
// query_time (the time the host measured around the client).
func (m *Module) addQueryTime(c *invoke.Call, d time.Duration) {
	if qc, ok := ctxOf(c).Value(queryClockKey{}).(*queryClock); ok {
		qc.took += d
		qc.runs++
	}
}

// clientMissing is the envelope of a client the host (or the container
// on it) does not have, with the members of a missing prerequisite
// ("missing", "host", "container").
func clientMissing(code, client string, via *sshVia) output {
	if via.container != "" {
		return failed(contract.New(code, client+" is not installed in container "+via.container+" on "+via.host.Alias,
			"Install the "+client+" client in the container, or name another one with +docker:CONTAINER in the URL").
			With("missing", client).With("host", via.host.Alias).With("container", via.container))
	}
	return failed(contract.New(code, client+" is not installed on "+via.host.Alias, "Install the "+client+" client on the host, or name its container with +docker:CONTAINER in the URL").
		With("missing", client).With("host", via.host.Alias))
}

// sshMySQL is mysqlRun on a +ssh target: the mysql client in batch mode,
// the same read-only session and sql_select_limit, the rows as objects
// of the client's text (NULL as "NULL"), or the raw text.
func (m *Module) sshMySQL(c *invoke.Call, t target, raw bool, query, rowCap string) output {
	argv := []string{"mysql", "--batch", "--default-character-set=utf8mb4"}
	if raw {
		argv = append(argv, "--raw", "--skip-column-names")
	}
	if t.port != "" {
		argv = append(argv, "-h", t.host, "-P", t.port, "--protocol=TCP")
	}
	if t.user != "" {
		argv = append(argv, "-u", t.user)
	}
	argv = append(argv, t.database)
	sql := "SET SESSION TRANSACTION READ ONLY;\n"
	if rowCap != "" {
		sql += "SET SESSION sql_select_limit = " + rowCap + ";\n"
	}
	sql += query + "\n"
	pw := ""
	if t.pass != "" {
		pw = "MYSQL_PWD" //nolint:gosec // G101: the name of the client variable, not a credential
	}
	out, errText, rc, _, fail := m.sshClient(c, t.via, argv, pw, nil, t.pass, sql)
	switch {
	case fail != nil:
		return *fail
	case rc == 127:
		return clientMissing("mysql_not_found", "mysql", t.via)
	case rc != 0:
		return failed(contract.New("mysql_error", cleanError(mysqlMessage(errText)), "Fix the SQL and retry"))
	}
	if raw {
		return textOut(trimNewlines(string(out)))
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return values(jvArray(nil))
	}
	cols := strings.Split(lines[0], "\t")
	for i := range cols {
		cols[i] = mysqlUnescape(cols[i])
	}
	var rows []jv
	for _, l := range lines[1:] {
		f := strings.Split(l, "\t")
		var o object
		for i, col := range cols {
			v := ""
			if i < len(f) {
				v = mysqlUnescape(f[i])
			}
			// The client prints a SQL NULL as NULL (and the string
			// 'NULL' the same way): null in the row formats, as the
			// driver path gives it.
			if t.rowFormat && i < len(f) && f[i] == "NULL" {
				o.set(col, jvNull)
				continue
			}
			o.set(col, jvString(v))
		}
		rows = append(rows, o.value())
	}
	res := values(jvArray(rows))
	res.cols = cols
	return res
}

// mysqlMessage is the client's error without its "ERROR 1064 (42000) at
// line 3: " prefix, as the driver gives it ("Error 1064 (42000): ...").
func mysqlMessage(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "ERROR ") {
		if i := strings.Index(s, ": "); i >= 0 {
			head := strings.TrimPrefix(s[:i], "ERROR ")
			if j := strings.Index(head, " at line "); j >= 0 {
				head = head[:j]
			}
			return "Error " + head + ": " + s[i+2:]
		}
	}
	return s
}

// mysqlUnescape undoes the batch mode escapes: \0 \b \n \r \t \\.
func mysqlUnescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case '0':
			b.WriteByte(0)
		case 'b':
			b.WriteByte('\b')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// sshPG is pgsqlRun on a +ssh target: psql with the read-only session,
// standard strings and the statement timeout in PGOPTIONS; json is the
// unaligned tuples-only text, otherwise the rows (CSV) as objects of
// strings, NULL as "".
func (m *Module) sshPG(c *invoke.Call, t target, json bool, query string) output {
	timeout := c.Env.Get("GASH_DB_TIMEOUT")
	if !digitsRE.MatchString(timeout) {
		timeout = "30"
	}
	argv := []string{"psql", "-X", "-q", "-v", "ON_ERROR_STOP=1"}
	if json {
		argv = append(argv, "-A", "-t")
	} else {
		argv = append(argv, "--csv", "-P", "null="+pgNullMark)
	}
	if t.port != "" {
		argv = append(argv, "-h", t.host, "-p", t.port)
	}
	if t.user != "" {
		argv = append(argv, "-U", t.user)
	}
	argv = append(argv, "-d", t.database, "-f", "-")
	env := map[string]string{"PGOPTIONS": "-c default_transaction_read_only=on -c standard_conforming_strings=on -c statement_timeout=" + timeout + "s"}
	pw := ""
	if t.pass != "" {
		pw = "PGPASSWORD" //nolint:gosec // G101: the name of the client variable, not a credential
	}
	out, errText, rc, version, fail := m.sshClient(c, t.via, argv, pw, env, t.pass, query+"\n")
	switch {
	case fail != nil:
		return *fail
	case rc == 127:
		return clientMissing("psql_not_found", "psql", t.via)
	case rc != 0 && !json:
		// --csv came with psql 12: an older one refuses it before it
		// reads any SQL.
		if found, old := oldClient(version, errText, "--csv", 12, 0); old {
			return failed(remote.OldOnHost("psql", "12", found, t.via.host, t.via.container))
		}
		fallthrough
	case rc != 0:
		msg := strings.TrimSpace(errText)
		msg = strings.TrimPrefix(msg, "psql:<stdin>:")
		if i := strings.Index(msg, "ERROR:"); i >= 0 {
			msg = msg[i:]
		}
		return failed(contract.New("pgsql_error", cleanError(msg), "Fix the SQL and retry"))
	}
	if json {
		s := trimNewlines(string(out))
		if s == "" {
			s = "[]"
		}
		return textOut(s)
	}
	recs, err := csv.NewReader(bytes.NewReader(out)).ReadAll()
	if err != nil || len(recs) == 0 {
		return values(jvArray(nil))
	}
	var rows []jv
	for _, rec := range recs[1:] {
		var o object
		for i, col := range recs[0] {
			v := ""
			if i < len(rec) {
				v = rec[i]
			}
			if v == pgNullMark {
				// A SQL NULL: "" in the envelope's text rows, null in
				// the row formats (the driver path's answer).
				if t.rowFormat {
					o.set(col, jvNull)
					continue
				}
				v = ""
			}
			o.set(col, jvString(v))
		}
		rows = append(rows, o.value())
	}
	res := values(jvArray(rows))
	res.cols = recs[0]
	return res
}

// pgNullMark is what psql prints for a SQL NULL in the text rows (psql
// -P null=...), so that it is told apart from an empty string.
const pgNullMark = "__gash_null__"

// sshSQLite is sqliteRun on a sqlite+ssh file: sqlite3 -readonly -json,
// the SQL on stdin (the guard has refused ATTACH, load_extension and the
// dot commands); an EXPLAIN QUERY PLAN comes back as the shell's text.
func (m *Module) sshSQLite(c *invoke.Call, via *sshVia, p, query string, mode int) output {
	argv := []string{"sqlite3", "-readonly", "-bail", "-json", p}
	out, errText, rc, version, fail := m.sshClient(c, via, argv, "", nil, "", query+";\n")
	switch {
	case fail != nil:
		return *fail
	case rc == 127:
		return clientMissing("sqlite_not_found", "sqlite3", via)
	case rc != 0:
		// -json came with sqlite3 3.33: an older one refuses it before
		// it reads any SQL.
		if found, old := oldClient(version, errText, "-json", 3, 33); old {
			return failed(remote.OldOnHost("sqlite3", "3.33", found, via.host, via.container))
		}
		msg := strings.TrimSpace(errText)
		msg = strings.TrimPrefix(msg, "Error: ")
		if i := strings.Index(msg, "Parse error"); i >= 0 {
			msg = msg[i:]
		}
		return failed(contract.New("sqlite_error", cleanError(msg), "Fix the SQL and retry"))
	}
	s := trimNewlines(string(out))
	if mode == sqliteEQP || (mode == sqliteExplain && strings.HasPrefix(s, "QUERY PLAN")) {
		return textOut(s)
	}
	if s == "" {
		return values(jvArray(nil))
	}
	vals, err := parseJSON(s)
	if err != nil {
		return textOut(s)
	}
	return values(vals...)
}

// sqliteOnHost is the sqliteRun hook: a sqlite+ssh file runs on its host.
func (m *Module) sqliteOnHost(c *invoke.Call, file, query string, mode int) (output, bool) {
	via, p, ok := parseSQLiteSSH(file)
	if !ok {
		return output{}, false
	}
	return m.sshSQLite(c, via, p, query, mode), true
}
