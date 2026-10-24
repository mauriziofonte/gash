// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"bytes"
	"context"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/pathguard"
	"github.com/mauriziofonte/gash/internal/runner"
)

// Entity patterns of __sysinfo_collect_question_context, byte-for-byte from
// the 2.x regexes.
var (
	// Pattern 1: word.service/timer/socket/... systemd units.
	reUnitEntity = regexp.MustCompile(`[a-zA-Z0-9_@.-]+\.(service|timer|socket|mount|path|target|slice|scope)`)
	// Pattern 2: /etc|var|usr|opt|srv|home|run paths (the body excludes the
	// shell-metacharacter set of the 2.x class).
	rePathEntity = regexp.MustCompile("/(etc|var|usr|opt|srv|home|run)/[^ \"'\x60|;&<>(){}]+")
	// Pattern 3: ports (LEDGER AI-08: the bare ":" alternative also takes
	// clock times such as 12:30). The digits are submatch 2.
	rePortEntity = regexp.MustCompile(`(port[: ]+|:)([0-9]{1,5})([^0-9]|$)`)
	// Pattern 5: hyphenated/underscored names that could be unit names.
	reSvcNameEntity = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9]*[-_][a-zA-Z0-9][-a-zA-Z0-9_]*`)
)

// Pattern 4 service keywords (matched case-insensitively against the
// lowercased question).
var (
	reKwApache   = regexp.MustCompile(`apache|httpd`)
	reKwNginx    = regexp.MustCompile(`nginx`)
	reKwMysql    = regexp.MustCompile(`mysql|mariadb`)
	reKwRedis    = regexp.MustCompile(`redis`)
	reKwPhp      = regexp.MustCompile(`php|fpm`)
	reKwPostfix  = regexp.MustCompile(`postfix|mail`)
	reKwSsh      = regexp.MustCompile(`ssh|sshd`)
	reKwFail2ban = regexp.MustCompile(`fail2ban`)
	reKwDocker   = regexp.MustCompile(`docker`)
	reKwPostgres = regexp.MustCompile(`postgres`)
	reKwCron     = regexp.MustCompile(`cron`)
)

// grep -Ev filters of the config reads.
var (
	reCommentBlank     = regexp.MustCompile(`^(#|$)`)   // ^(#|$)
	reCommentSemiBlank = regexp.MustCompile(`^(#|;|$)`) // ^(#|;|$)
	reSemiBlank        = regexp.MustCompile(`^(;|$)`)   // ^(;|$)
)

// stripTrailingPunct is the 2.x `${fpath%%[set]+([set])}; ${fpath%[set]}`
// pair (LEDGER AI-18): the whole trailing run of the punctuation set is
// removed.
func stripTrailingPunct(s string) string { return strings.TrimRight(s, "?.!,:;)\"'") }

// collectQuestionContext is __sysinfo_collect_question_context: it detects
// units, file paths, ports and known services in the question and gathers
// read-only data for each, deduplicating across patterns. sudo is the
// session's __sysinfo_sudo (journalctl and the sshd_config read). Non-sudo
// commands go through the runner; file contents and directory listings
// through the sysroot.
func (m *Module) collectQuestionContext(c *invoke.Call, sudo func(string, ...string) []byte, question string) string {
	text, _ := m.questionContext(c, sudo, question)
	return text
}

// questionContext is collectQuestionContext that also returns the kinds of
// items it collected (files, directories, units, ports, services), in that
// order, for the per-question confirmation (LEDGER AI-01).
func (m *Module) questionContext(c *invoke.Call, sudo func(string, ...string) []byte, question string) (string, []string) {
	if question == "" {
		return "", nil
	}
	q := &qctx{m: m, c: c, sudo: sudo}
	lowerQ := strings.ToLower(question)
	seen := map[string]bool{}

	// Pattern 1: systemd units.
	remaining := question
	for {
		loc := reUnitEntity.FindStringIndex(remaining)
		if loc == nil {
			break
		}
		unit := remaining[loc[0]:loc[1]]
		remaining = remaining[loc[1]:]
		if seen[unit] {
			continue
		}
		seen[unit] = true
		q.unitBlock(unit)
	}

	// Pattern 2: file paths.
	remaining = question
	for {
		loc := rePathEntity.FindStringIndex(remaining)
		if loc == nil {
			break
		}
		fpath := remaining[loc[0]:loc[1]]
		remaining = remaining[loc[1]:]
		fpath = stripTrailingPunct(fpath)
		if seen[fpath] {
			continue
		}
		seen[fpath] = true
		// LEDGER AI-01: the path guard of the llm_* readers, on the
		// lexical and the resolved path; a refused path sends no content.
		target, ok := q.guardPath(fpath)
		if !ok {
			q.add("=== FILE: " + fpath + " === (skipped: protected path)\n")
			q.add("\n")
			continue
		}
		switch {
		case m.sys.IsRegular(target):
			q.kind("files")
			q.add("=== FILE: " + fpath + " ===\n")
			q.add(q.headFile(target, 200) + "\n")
			q.add("\n")
		case m.sys.IsDir(target):
			q.kind("directories")
			q.add("=== DIRECTORY: " + fpath + " ===\n")
			q.add(sub(headN(q.dropSecretEntries(target, m.cmd(c, "ls", "-la", target)), 50)) + "\n")
			q.add("\n")
		}
	}

	// Pattern 3: port numbers (on the lowercased question).
	remaining = lowerQ
	for {
		loc := rePortEntity.FindStringSubmatchIndex(remaining)
		if loc == nil {
			break
		}
		port := remaining[loc[4]:loc[5]]
		remaining = remaining[loc[1]:]
		if port == "" || seen["port:"+port] {
			continue
		}
		seen["port:"+port] = true
		q.kind("ports")
		q.add("=== PORT: " + port + " ===\n")
		q.add(sub(headN(grepF(m.cmd(c, "ss", "-tlnp"), ":"+port), 10)) + "\n")
		q.add("\n")
	}

	// Pattern 4: known service keywords.
	q.serviceKeywords(lowerQ, seen)

	// Pattern 5: hyphenated service names (collected only when the unit
	// actually exists).
	remaining = question
	for {
		loc := reSvcNameEntity.FindStringIndex(remaining)
		if loc == nil {
			break
		}
		svc := remaining[loc[0]:loc[1]]
		remaining = remaining[loc[1]:]
		if seen[svc+".service"] || seen["svc:"+strings.ToLower(svc)] {
			continue
		}
		unitOut := q.cmdHead(60, "systemctl", "cat", svc+".service")
		if unitOut == "" {
			continue
		}
		seen[svc+".service"] = true
		q.kind("units")
		q.add("=== SYSTEMD UNIT: " + svc + ".service ===\n")
		q.add("--- Unit file ---\n")
		q.add(unitOut + "\n")
		q.add("--- Status ---\n")
		q.add(q.cmdHead(30, "systemctl", "status", svc+".service", "--no-pager") + "\n")
		q.add("--- Journal (last 50 lines) ---\n")
		q.add(sub(headN(sudo("journalctl", "-u", svc+".service", "-n", "50", "--no-pager"), 50)) + "\n")
		q.add("\n")
	}

	var kinds []string
	for _, k := range []string{"files", "directories", "units", "ports", "services"} {
		if q.kinds[k] {
			kinds = append(kinds, k)
		}
	}
	return q.b.String(), kinds
}

// guardPath applies the path rules of the llm_* file readers to a path
// named in a question (LEDGER AI-01): a ".." component is refused, then
// the lexically cleaned path and its symlink-resolved form must be outside
// the forbidden list (root's home included) and must not be a secret file
// or lie under a secret directory (base names case-insensitive). It
// returns the resolved path to read.
func (q *qctx) guardPath(p string) (string, bool) {
	for _, comp := range strings.Split(p, "/") {
		if comp == ".." {
			return "", false
		}
	}
	clean := path.Clean(p)
	g := pathguard.NewGuard(q.m.sys, "/", q.c.Env.Get("HOME"), pathguard.RootHome())
	resolved, err := g.Realpath(clean, false)
	if err != nil || resolved == "" {
		resolved = clean
	}
	if g.PathForbidden(clean, resolved) || pathguard.SecretName(clean) || pathguard.SecretName(resolved) {
		return "", false
	}
	return resolved, true
}

// reLsLong splits an `ls -la` line into its eight leading fields and the
// name (with " -> TARGET" for a symlink).
var reLsLong = regexp.MustCompile(`^\S+\s+\S+\s+\S+\s+\S+\s+\S+\s+\S+\s+\S+\s+\S+\s+(.*)$`)

// dropSecretEntries removes from an `ls -la DIR` listing the entries the
// path guard refuses: a secret name, or a symlink into a protected path or
// to a secret file (LEDGER AI-01).
func (q *qctx) dropSecretEntries(dir string, out []byte) []byte {
	var kept []byte
	for _, l := range bytes.SplitAfter(out, []byte("\n")) {
		if len(l) == 0 {
			continue
		}
		if mm := reLsLong.FindSubmatch(bytes.TrimRight(l, "\n")); mm != nil {
			name, _, _ := strings.Cut(string(mm[1]), " -> ")
			if name != "." && name != ".." {
				if _, ok := q.guardPath(strings.TrimSuffix(dir, "/") + "/" + name); !ok {
					continue
				}
			}
		}
		kept = append(kept, l...)
	}
	return kept
}

// serviceKeywords runs the pattern-4 blocks in the 2.x order.
func (q *qctx) serviceKeywords(lowerQ string, seen map[string]bool) {
	m := q.m
	if reKwApache.MatchString(lowerQ) && !seen["svc:apache"] {
		seen["svc:apache"] = true
		q.kind("services")
		q.add("=== SERVICE: Apache2 ===\n")
		q.add("--- Status ---\n")
		q.add(q.cmdHead(20, "systemctl", "status", "apache2", "--no-pager") + "\n")
		if m.sys.IsRegular("/etc/apache2/apache2.conf") {
			q.add("--- /etc/apache2/apache2.conf (key lines) ---\n")
			q.add(q.grepFile("/etc/apache2/apache2.conf", reCommentBlank, 40) + "\n")
		}
		if m.sys.IsDir("/etc/apache2/sites-enabled") {
			q.add("--- sites-enabled ---\n")
			q.add(q.lsOne("/etc/apache2/sites-enabled/") + "\n")
		}
		if m.sys.IsDir("/etc/apache2/mods-enabled") {
			q.add("--- mods-enabled ---\n")
			q.add(q.modsLoad("/etc/apache2/mods-enabled") + "\n")
		}
		q.add("\n")
	}

	if reKwNginx.MatchString(lowerQ) && !seen["svc:nginx"] {
		seen["svc:nginx"] = true
		q.kind("services")
		q.add("=== SERVICE: Nginx ===\n")
		q.add(q.cmdHead(20, "systemctl", "status", "nginx", "--no-pager") + "\n")
		if m.sys.IsRegular("/etc/nginx/nginx.conf") {
			q.add("--- /etc/nginx/nginx.conf ---\n")
			q.add(q.grepFile("/etc/nginx/nginx.conf", reCommentBlank, 40) + "\n")
		}
		q.add("\n")
	}

	if reKwMysql.MatchString(lowerQ) && !seen["svc:mysql"] {
		seen["svc:mysql"] = true
		q.kind("services")
		q.add("=== SERVICE: MariaDB/MySQL ===\n")
		q.add(q.cmdHead(20, "systemctl", "status", "mariadb", "--no-pager") + "\n")
		if m.sys.IsDir("/etc/mysql") {
			q.add("--- /etc/mysql/ config files ---\n")
			for _, dir := range []string{"/etc/mysql/mariadb.conf.d", "/etc/mysql/mysql.conf.d", "/etc/mysql/conf.d"} {
				for _, name := range q.globSuffix(dir, ".cnf") {
					p := dir + "/" + name
					if !m.sys.IsRegular(p) {
						continue
					}
					q.add("--- " + p + " ---\n")
					q.add(q.grepFile(p, reCommentSemiBlank, 30) + "\n")
				}
			}
		}
		q.add("\n")
	}

	if reKwRedis.MatchString(lowerQ) && !seen["svc:redis"] {
		seen["svc:redis"] = true
		q.kind("services")
		q.add("=== SERVICE: Redis ===\n")
		q.add(q.cmdHead(20, "systemctl", "status", "redis-server", "--no-pager") + "\n")
		if m.sys.IsRegular("/etc/redis/redis.conf") {
			q.add("--- /etc/redis/redis.conf (key lines) ---\n")
			q.add(q.grepFile("/etc/redis/redis.conf", reCommentBlank, 30) + "\n")
		}
		q.add("\n")
	}

	if reKwPhp.MatchString(lowerQ) && !seen["svc:php"] {
		seen["svc:php"] = true
		q.kind("services")
		q.add("=== SERVICE: PHP-FPM ===\n")
		for _, ver := range q.phpVersions() {
			q.add("--- PHP " + ver + " FPM ---\n")
			q.add(q.cmdHead(10, "systemctl", "status", "php"+ver+"-fpm", "--no-pager") + "\n")
			pool := "/etc/php/" + ver + "/fpm/pool.d"
			if m.sys.IsDir(pool) {
				for _, name := range q.globSuffix(pool, ".conf") {
					p := pool + "/" + name
					if !m.sys.IsRegular(p) {
						continue
					}
					q.add("--- " + name + " ---\n")
					q.add(q.grepFile(p, reSemiBlank, 20) + "\n")
				}
			}
		}
		q.add("\n")
	}

	if reKwPostfix.MatchString(lowerQ) && !seen["svc:postfix"] {
		seen["svc:postfix"] = true
		q.kind("services")
		q.add("=== SERVICE: Postfix ===\n")
		q.add(q.cmdHead(20, "systemctl", "status", "postfix", "--no-pager") + "\n")
		if m.sys.IsRegular("/etc/postfix/main.cf") {
			q.add("--- /etc/postfix/main.cf (key lines) ---\n")
			q.add(q.grepFile("/etc/postfix/main.cf", reCommentBlank, 40) + "\n")
		}
		q.add("\n")
	}

	if reKwSsh.MatchString(lowerQ) && !seen["svc:ssh"] {
		seen["svc:ssh"] = true
		q.kind("services")
		q.add("=== SERVICE: SSH ===\n")
		q.add(q.cmdHead(20, "systemctl", "status", "sshd", "--no-pager") + "\n")
		if m.sys.IsRegular("/etc/ssh/sshd_config") {
			q.add("--- /etc/ssh/sshd_config (key lines) ---\n")
			q.add(sub(headN(q.sudo("grep", "-Ev", "^(#|$)", "/etc/ssh/sshd_config"), 30)) + "\n")
		}
		if m.sys.IsDir("/etc/ssh/sshd_config.d") {
			q.add("--- sshd_config.d/ ---\n")
			q.add(q.lsOne("/etc/ssh/sshd_config.d/") + "\n")
		}
		q.add("\n")
	}

	if reKwFail2ban.MatchString(lowerQ) && !seen["svc:fail2ban"] {
		seen["svc:fail2ban"] = true
		q.kind("services")
		q.add("=== SERVICE: Fail2ban ===\n")
		q.add(q.cmdHead(20, "systemctl", "status", "fail2ban", "--no-pager") + "\n")
		if m.sys.IsRegular("/etc/fail2ban/jail.local") {
			q.add("--- /etc/fail2ban/jail.local ---\n")
			q.add(q.grepFile("/etc/fail2ban/jail.local", reCommentSemiBlank, 40) + "\n")
		}
		q.add("\n")
	}

	if reKwDocker.MatchString(lowerQ) && !seen["svc:docker"] {
		seen["svc:docker"] = true
		q.kind("services")
		q.add("=== SERVICE: Docker ===\n")
		q.add(q.cmdHead(20, "systemctl", "status", "docker", "--no-pager") + "\n")
		if m.sys.IsRegular("/etc/docker/daemon.json") {
			q.add("--- /etc/docker/daemon.json ---\n")
			q.add(q.catFile("/etc/docker/daemon.json") + "\n")
		}
		q.add("\n")
	}

	if reKwPostgres.MatchString(lowerQ) && !seen["svc:postgres"] {
		seen["svc:postgres"] = true
		q.kind("services")
		q.add("=== SERVICE: PostgreSQL ===\n")
		q.add(q.cmdHead(20, "systemctl", "status", "postgresql", "--no-pager") + "\n")
		pgconf := firstLine(m.cmd(q.c, "find", "/etc/postgresql", "-name", "pg_hba.conf"))
		if pgconf != "" {
			q.add("--- " + pgconf + " ---\n")
			q.add(q.grepFile(pgconf, reCommentBlank, 20) + "\n")
		}
		q.add("\n")
	}

	if reKwCron.MatchString(lowerQ) && !seen["svc:cron"] {
		seen["svc:cron"] = true
		q.kind("services")
		q.add("=== SERVICE: Cron ===\n")
		if out, rc := m.cmdRC(q.c, "crontab", "-l"); rc != 0 {
			q.add("(no user crontab)\n")
		} else {
			q.add(sub(out) + "\n")
		}
		if m.sys.IsDir("/etc/cron.d") {
			q.add("--- /etc/cron.d/ ---\n")
			q.add(q.lsOne("/etc/cron.d/") + "\n")
		}
		q.add("\n")
	}
}

// qctx accumulates the question-context text with the module's seams.
type qctx struct {
	m     *Module
	c     *invoke.Call
	sudo  func(string, ...string) []byte
	b     strings.Builder
	kinds map[string]bool
}

func (q *qctx) add(s string) { q.b.WriteString(s) }

// kind records that an item of kind k was collected.
func (q *qctx) kind(k string) {
	if q.kinds == nil {
		q.kinds = map[string]bool{}
	}
	q.kinds[k] = true
}

// unitBlock is the per-unit collection of patterns 1 and 5's head.
func (q *qctx) unitBlock(unit string) {
	q.kind("units")
	q.add("=== SYSTEMD UNIT: " + unit + " ===\n")
	q.add("--- Unit file ---\n")
	q.add(q.cmdHead(60, "systemctl", "cat", unit) + "\n")
	q.add("--- Status ---\n")
	q.add(q.cmdHead(30, "systemctl", "status", unit, "--no-pager") + "\n")
	q.add("--- Journal (last 50 lines) ---\n")
	q.add(sub(headN(q.sudo("journalctl", "-u", unit, "-n", "50", "--no-pager"), 50)) + "\n")
	q.add("\n")
}

// cmdHead is `NAME ARGS 2>/dev/null | head -N` captured in $(...).
func (q *qctx) cmdHead(n int, name string, args ...string) string {
	return sub(headN(q.m.cmd(q.c, name, args...), n))
}

// headFile is `head -N FILE` captured in $(...), read through the sysroot.
func (q *qctx) headFile(path string, n int) string {
	b, _ := q.m.sys.ReadFile(path)
	return sub(headN(b, n))
}

// catFile is `cat FILE` captured in $(...).
func (q *qctx) catFile(path string) string {
	b, _ := q.m.sys.ReadFile(path)
	return sub(b)
}

// grepFile is `grep -Ev RE FILE 2>/dev/null | head -N` captured in $(...).
func (q *qctx) grepFile(path string, re *regexp.Regexp, n int) string {
	b, _ := q.m.sys.ReadFile(path)
	return grepVHead(b, re, n)
}

// lsOne is `ls -1 DIR`: the non-hidden entries, sorted.
func (q *qctx) lsOne(dir string) string {
	ents, _ := q.m.sys.ReadDir(dir)
	var names []string
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return strings.Join(names, "\n")
}

// modsLoad is the `for __f in DIR/*.load; basename "$__f" .load` loop.
func (q *qctx) modsLoad(dir string) string {
	var names []string
	for _, name := range q.globSuffix(dir, ".load") {
		names = append(names, strings.TrimSuffix(name, ".load"))
	}
	return strings.Join(names, "\n")
}

// globSuffix is the sorted names in dir ending with suffix (one glob level).
func (q *qctx) globSuffix(dir, suffix string) []string {
	ents, _ := q.m.sys.ReadDir(dir)
	var names []string
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), suffix) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// phpVersions is the sorted `*` of the /etc/php/*/fpm glob.
func (q *qctx) phpVersions() []string {
	ents, _ := q.m.sys.ReadDir("/etc/php")
	var vers []string
	for _, e := range ents {
		if q.m.sys.IsDir("/etc/php/" + e.Name() + "/fpm") {
			vers = append(vers, e.Name())
		}
	}
	sort.Strings(vers)
	return vers
}

// cmd runs NAME ARGS and returns its stdout (errors discarded, as the 2.x
// `2>/dev/null` and empty output on failure).
func (m *Module) cmd(c *invoke.Call, name string, args ...string) []byte {
	out, _ := m.cmdRC(c, name, args...)
	return out
}

// cmdRC runs NAME ARGS and returns its stdout and $?.
func (m *Module) cmdRC(c *invoke.Call, name string, args ...string) ([]byte, int) {
	ctx := c.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := m.run.Run(ctx, runner.Cmd{Path: name, Args: args,
		Options: runner.Options{Env: c.Env.Child()}})
	if err != nil {
		return nil, 127
	}
	return res.Stdout, res.ExitCode
}

// sub is command substitution `$(...)`: trailing newlines are stripped.
func sub(b []byte) string { return strings.TrimRight(string(b), "\n") }

// headN is `head -N`: the first n lines (with the nth's newline), or all
// of b when it has fewer. n <= 0 yields nothing.
func headN(b []byte, n int) []byte {
	if n <= 0 {
		return nil
	}
	cnt := 0
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' {
			if cnt++; cnt == n {
				return b[:i+1]
			}
		}
	}
	return b
}

// grepF is `grep -F SUBSTR`: the lines containing substr, each kept with
// its newline.
func grepF(b []byte, substr string) []byte {
	var out []byte
	for _, l := range bytes.SplitAfter(b, []byte("\n")) {
		if bytes.Contains(l, []byte(substr)) {
			out = append(out, l...)
		}
	}
	return out
}

// grepVHead is `grep -Ev RE | head -N` captured in $(...): the first n
// lines that do not match re, joined by newlines (no trailing newline).
func grepVHead(b []byte, re *regexp.Regexp, n int) string {
	var kept []string
	for _, l := range contentLines(b) {
		if re.MatchString(l) {
			continue
		}
		kept = append(kept, l)
		if len(kept) == n {
			break
		}
	}
	return strings.Join(kept, "\n")
}

// contentLines splits a file's bytes into lines as grep reads them: the
// final newline does not make a trailing empty line.
func contentLines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// firstLine is `head -1` captured in $(...): the first line without its
// newline, "" for empty input.
func firstLine(b []byte) string {
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}
