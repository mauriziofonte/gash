// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

// catalogProc are the 3.4 agent-toolkit functions of one area, processes,
// waiting and services: llm_wait, llm_service, llm_docker_ps,
// llm_docker_logs, llm_docker_exec, llm_sysinfo, agent_kill,
// agent_docker_exec (P1-P5, R4). Entries follow the catalog_gen.go format;
// mergeCatalog sorts them in.
var catalogProc = []Function{
	{
		Name:     "llm_wait",
		Module:   "llm",
		Topic:    "llm",
		File:     "lib/modules/llm.sh",
		StubFile: "lib/modules/llm.sh",
		Class:    Class("pure output"),
		Decision: "new",
		Impl:     Impl("binary"),
		Headless: true,
		HasHelp:  true,
		Help: Help{
			Short:   "Wait for a log line, a path, a process exit, a URL, a port or a command (JSON)",
			Module:  "llm",
			SeeAlso: "llm_procs",
			Body: "" +
				"USAGE\n" +
				"  llm_wait --timeout S [--interval S] CONDITION [--tail N] [--width N]\n" +
				"  CONDITION is one of:\n" +
				"    --file-match FILE RE   a line of FILE matches RE\n" +
				"    --exists PATH          PATH exists\n" +
				"    --gone PID|NAME        the process ended / no command line matches NAME\n" +
				"    --http URL [--code N] [--json-path P --equals V]\n" +
				"    --port HOST:PORT       a TCP connection opens\n" +
				"    --cmd-ok CMD           CMD (llm_exec rules) exits 0\n" +
				"\n" +
				"OPTIONS\n" +
				"  --timeout S            Give up after S (seconds, or 500ms, 2m; at most 1h);\n" +
				"                         required\n" +
				"  --interval S           Poll interval (default 0.2s for files and paths and a\n" +
				"                         PID, 0.5s for a NAME, a URL and a port, 1s for a command)\n" +
				"  --file-match FILE RE   Scan FILE once, then only what is appended (inotify on\n" +
				"                         Linux); a missing FILE is waited for. RE is RE2,\n" +
				"                         matched against each line\n" +
				"  --exists PATH          Met when PATH exists\n" +
				"  --gone PID|NAME        A PID: met when it ends (a zombie counts as ended). A\n" +
				"                         NAME: a regular expression matched against command\n" +
				"                         lines, as llm_procs --name; the caller is left out\n" +
				"  --http URL             GET until the status is 2xx\n" +
				"  --code N               ... until it is N (100-599) instead\n" +
				"  --json-path P          ... and the JSON body holds --equals V at path P\n" +
				"  --equals V             (a.b[0].c; a string compares as is, a number in its\n" +
				"                         shortest form, true, false, null)\n" +
				"  --port HOST:PORT       Met when a TCP connection to HOST:PORT opens\n" +
				"  --cmd-ok CMD           Re-run the command line CMD (checked once under the\n" +
				"                         llm_exec rules) until it exits 0\n" +
				"  --tail N               Add the last N lines of FILE, or of CMD's last output\n" +
				"  --width N              Cut the match and tail lines at N columns (default 300)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  # Instead of: while ! grep -q DONE out.log; do sleep 10; done\n" +
				"  llm_wait --timeout 600 --file-match out.log '^DONE' --tail 5\n" +
				"\n" +
				"  # A server comes up\n" +
				"  llm_wait --timeout 60 --http http://127.0.0.1:8080/health --json-path status --equals ok\n" +
				"  llm_wait --timeout 30 --port 127.0.0.1:5432\n" +
				"\n" +
				"  # A background job ends\n" +
				"  llm_wait --timeout 300 --gone \"$pid\"\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces sleep-and-check loops (while ! grep -q X f; do sleep N; done,\n" +
				"  sleep N; tail f, until curl ...; do sleep N; done) with one call that\n" +
				"  returns as soon as the condition holds. Met: {\"met\":true,\"after_s\":2.01,\n" +
				"  \"match\":\"<line>\",\"tail\":[...]} and exit 0 (--http adds \"code\", --cmd-ok\n" +
				"  \"rc\" and \"runs\"). Timeout: {\"met\":false,\"after_s\":S} and exit 1, with\n" +
				"  no error envelope (--http, --port and --cmd-ok add what the last look saw:\n" +
				"  \"last\", \"code\", \"rc\"). One condition per call. FILE follows the path\n" +
				"  rules and secret files are refused. Reads only: nothing is signaled,\n" +
				"  written or started but CMD.",
		},
		ShellVars: []string{"HOME"},
		Env:       []string{"HOME", "PWD"},
		Replaces:  "while ! grep -q X f; do sleep N; done; sleep N; tail f",
	},
	{
		Name:     "llm_service",
		Module:   "llm",
		Topic:    "llm",
		File:     "lib/modules/llm.sh",
		StubFile: "lib/modules/llm.sh",
		Class:    Class("pure output"),
		Decision: "new",
		Impl:     Impl("binary"),
		Headless: true,
		HasHelp:  true,
		Help: Help{
			Short:   "State and last journal lines of systemd units (JSON)",
			Module:  "llm",
			SeeAlso: "llm_procs",
			Body: "" +
				"USAGE\n" +
				"  llm_service UNIT... [--log N] [--format F] [--width N] [--max-lines N] [--head N] [--tail N]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --log N                Last N journal lines of each unit (default 10; 0: none)\n" +
				"  --format F             json (default), jsonl, tsv or table\n" +
				"  --width N              Cut each journal line at N columns (default 300)\n" +
				"  --max-lines N, --head N, --tail N\n" +
				"                         Keep the first/last N units\n" +
				"\n" +
				"EXAMPLES\n" +
				"  llm_service nginx php8.4-fpm\n" +
				"  llm_service mariadb --log 30\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces systemctl is-active x; journalctl -u x -n 20 --no-pager | cut\n" +
				"  -c1-200, for several units in one call. One row per unit, in the order\n" +
				"  given: [{\"unit\",\"load\",\"active\",\"sub\",\"since\",\"pid\",\"restarts\",\n" +
				"  \"log\":[...]}]; an unknown unit is load \"not-found\". Journal lines are\n" +
				"  short-iso without the host name; the journal may need the user in the\n" +
				"  systemd-journal group (no lines otherwise). Read-only (systemctl show,\n" +
				"  journalctl). Without systemd: systemd_unavailable (CONTINUE).",
		},
		Env:      []string{"HOME", "PWD"},
		Replaces: "systemctl is-active x; journalctl -u x -n 20 --no-pager | cut -c1-200",
	},
	{
		Name:     "llm_docker_ps",
		Module:   "llm",
		Topic:    "llm",
		File:     "lib/modules/llm.sh",
		StubFile: "lib/modules/llm.sh",
		Class:    Class("pure output"),
		Decision: "new",
		Impl:     Impl("binary"),
		Headless: true,
		HasHelp:  true,
		Help: Help{
			Short:   "Docker containers with state, health and ports (JSON)",
			Module:  "llm",
			SeeAlso: "llm_docker_logs",
			Body: "" +
				"USAGE\n" +
				"  llm_docker_ps [--running] [--format F] [--width N] [--max-lines N] [--head N] [--tail N]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --running              Running containers only (default: all)\n" +
				"  --format F             json (default), jsonl, tsv or table\n" +
				"  --width N              Cut each value at N columns\n" +
				"  --max-lines N, --head N, --tail N\n" +
				"                         Keep the first/last N containers\n" +
				"\n" +
				"EXAMPLES\n" +
				"  llm_docker_ps\n" +
				"  llm_docker_ps --running --format tsv\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces docker ps --format '{{.Names}} {{.Status}}' and docker ps -a.\n" +
				"  [{\"name\",\"image\",\"state\",\"health\",\"ports\",\"up\"}] by name: state\n" +
				"  is docker's (running, exited, ...), health healthy, unhealthy, starting\n" +
				"  or \"\", ports \"8080->80/tcp\" (the all-address bindings shortened), up\n" +
				"  docker's status without the health (\"Up 2 hours\", \"Exited (1) 3\n" +
				"  minutes ago\"). Without docker: docker_not_found; daemon down:\n" +
				"  docker_unavailable (both CONTINUE).",
		},
		Env:      []string{"DOCKER_CONTEXT", "DOCKER_HOST", "HOME"},
		Replaces: "docker ps --format '{{.Names}} {{.Status}}'",
	},
	{
		Name:     "llm_docker_logs",
		Module:   "llm",
		Topic:    "llm",
		File:     "lib/modules/llm.sh",
		StubFile: "lib/modules/llm.sh",
		Class:    Class("pure output"),
		Decision: "new",
		Impl:     Impl("binary"),
		Headless: true,
		HasHelp:  true,
		Help: Help{
			Short:   "Last log lines of a container, optionally filtered (JSON)",
			Module:  "llm",
			SeeAlso: "llm_docker_ps",
			Body: "" +
				"USAGE\n" +
				"  llm_docker_logs CTR [--tail N] [--since T] [--grep RE] [--width N]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --tail N               Read the last N lines (default 100)\n" +
				"  --since T              Only lines since T (10m, 2h, 2026-10-25T10:00:00)\n" +
				"  --grep RE              Keep the lines RE matches (RE2; (?i) for any case)\n" +
				"  --width N              Cut each line at N columns (default 300)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  llm_docker_logs web --tail 50 --grep '(?i)error'\n" +
				"  llm_docker_logs db --since 10m\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces docker logs --tail 50 ctr 2>&1 | grep -i error | cut -c1-300.\n" +
				"  {\"ctr\",\"read\":N,\"log\":[...]}: stdout and stderr in docker's order,\n" +
				"  read is how many lines docker returned before --grep. Unknown\n" +
				"  container: container_not_found (RETRY); docker absent or down:\n" +
				"  docker_not_found, docker_unavailable (CONTINUE).",
		},
		Env:      []string{"DOCKER_CONTEXT", "DOCKER_HOST", "HOME"},
		Replaces: "docker logs --tail 50 ctr 2>&1 | grep -i error",
	},
	{
		Name:     "llm_docker_exec",
		Module:   "llm",
		Topic:    "llm",
		File:     "lib/modules/llm.sh",
		StubFile: "lib/modules/llm.sh",
		Class:    Class("pure output"),
		Decision: "new",
		Impl:     Impl("binary"),
		Headless: true,
		HasHelp:  true,
		Help: Help{
			Short:   "Run a read-only command in a container, no shell quoting (JSON)",
			Module:  "llm",
			SeeAlso: "llm_exec",
			Body: "" +
				"USAGE\n" +
				"  llm_docker_exec CTR [--user U] [--width N] [--max-lines N] [--head N] [--tail N] -- CMD...\n" +
				"\n" +
				"OPTIONS\n" +
				"  --user U, -u U         Run as user U (name or uid[:gid])\n" +
				"  --width N              Cut each output line at N columns\n" +
				"  --max-lines N, --head N, --tail N\n" +
				"                         Keep the first/last N lines of each stream\n" +
				"  -- CMD...              The command, one word per argument\n" +
				"\n" +
				"EXAMPLES\n" +
				"  # Instead of: docker exec -u node ctr sh -c 'grep -rl \"x\" /app | head -3'\n" +
				"  llm_docker_exec ctr --user node --head 3 -- grep -rl x /app\n" +
				"  llm_docker_exec db -- cat /etc/mysql/my.cnf\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces docker exec CTR sh -c '...' with its quoting levels: CMD's words\n" +
				"  reach docker exec as an array, no shell runs. Read-only: CMD passes the\n" +
				"  llm_exec rules first (allowlist and option rules, secret files,\n" +
				"  forbidden paths, blocklist; GASH_LLM_EXEC_ALLOW extends them); sed and\n" +
				"  awk run as sed --sandbox and gawk --sandbox, grep and rg skip secret\n" +
				"  files, git is refused. One command (no pipes): shape the output with\n" +
				"  --head/--tail. {\"rc\",\"out\",\"err\"} and \"truncated\" when lines were\n" +
				"  cut; the exit status is 0 when the command ran (read rc). Arbitrary\n" +
				"  commands: agent_docker_exec.",
		},
		ShellVars: []string{"HOME"},
		Env:       []string{"DOCKER_CONTEXT", "DOCKER_HOST", "GASH_ENV_FILE", "HOME", "PWD"},
		Replaces:  "docker exec -u USER ctr sh -c '...'",
	},
	{
		Name:     "llm_sysinfo",
		Module:   "llm",
		Topic:    "llm",
		File:     "lib/modules/llm.sh",
		StubFile: "lib/modules/llm.sh",
		Class:    Class("pure output"),
		Decision: "new",
		Impl:     Impl("binary"),
		Headless: true,
		HasHelp:  true,
		Help: Help{
			Short:   "Load, uptime, CPUs, memory, disks, GPUs and OS in one line (JSON)",
			Module:  "llm",
			SeeAlso: "sysinfo",
			Body: "" +
				"USAGE\n" +
				"  llm_sysinfo [--mount PATH]...\n" +
				"\n" +
				"OPTIONS\n" +
				"  --mount PATH           Report the filesystem of PATH (repeatable) instead\n" +
				"                         of / and the other local disks\n" +
				"\n" +
				"EXAMPLES\n" +
				"  llm_sysinfo\n" +
				"  llm_sysinfo --mount /var/lib/mysql\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces uptime; free -h; df -h /; nproc; uname -a; nvidia-smi\n" +
				"  --query-gpu=... in about 200 bytes: {\"host\",\"os\",\"kernel\",\"uptime_h\",\n" +
				"  \"load\":[1,5,15],\"cpus\",\"cpu\",\"mem_mb\":{\"total\",\"avail\"},\n" +
				"  \"disks\":[{\"mount\",\"size_gb\",\"used_pct\"}],\"gpu\":[{\"name\",\"mem_mb\":\n" +
				"  [used,total],\"util_pct\"}]}; gpu only when nvidia-smi is installed. The\n" +
				"  disks are / and the writable local disks (one mount per device; not\n" +
				"  /boot, /snap, network or pseudo filesystems). Linux reads /proc and\n" +
				"  statfs, no program runs; macOS asks sysctl.",
		},
		ShellVars: []string{"HOME"},
		Env:       []string{"HOME", "PWD"},
		Replaces:  "uptime; free -h; df -h /; nproc; nvidia-smi --query-gpu=...",
	},
}
