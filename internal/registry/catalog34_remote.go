// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

// catalogRemote are the 3.4 agent-toolkit functions of one area, remote:
// llm_remote_run, llm_same, agent_put (R1-R3, R5, E2). Entries follow the
// catalog_gen.go format; mergeCatalog sorts them in.
var catalogRemote = []Function{
	{
		Name:     "agent_put",
		Module:   "agent",
		Topic:    "agent",
		File:     "lib/modules/llm.sh",
		StubFile: "lib/modules/llm.sh",
		Class:    Class("shell-mutating"),
		Decision: "new",
		Impl:     Impl("binary"),
		Headless: true,
		HasHelp:  true,
		Help: Help{
			Short:  "Upload a file to a registered host: atomic, optional backup and hash check",
			Module: "agent",
			Body: "" +
				"USAGE\n" +
				"  agent_put ALIAS:PATH --from FILE|- [--mode M] [--backup] [--verify] [--force] [--dry-run] [--timeout S]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --from FILE|-          The content: a local file, or - for stdin (64 MiB max)\n" +
				"  --mode M               Octal mode of the written file (default: the old\n" +
				"                         file's, or the host's umask for a new one)\n" +
				"  --backup               Copy the old file to PATH.bak-<UTC time> first\n" +
				"  --verify               Check the sha256 on the host before the rename\n" +
				"  --force                Allow an ASK path (/etc, /var, /usr/local, /root, any\n" +
				"                         .ssh): only after the user agreed\n" +
				"  --dry-run              Check the target on the host and report; write nothing\n" +
				"  --timeout S            Bound the call (default 300)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  # Deploy one file, keep the old one, check what arrived\n" +
				"  agent_put web:/var/www/app/config.php --from config.php --backup --verify --force\n" +
				"\n" +
				"  # Generated content from stdin, into the remote home\n" +
				"  printf '%s\\n' \"$TOKEN_LINE\" | agent_put web:app/.version --from -\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces scp f h:/tmp/x; ssh h \"install -m 644 /tmp/x /dest\"; sha256sum on\n" +
				"  both sides. One ssh round trip: the content goes on stdin to a temporary\n" +
				"  file next to PATH (the old file's mode kept), its size is checked (and its\n" +
				"  sha256 with --verify), then it is renamed over PATH; a failed check\n" +
				"  replaces nothing (write_failed, put_verify_failed: STOP). PATH's directory\n" +
				"  must exist; a relative PATH is below the remote home. ALIAS is a\n" +
				"  registered HOST: alias. The transfer path policy applies before connecting\n" +
				"  and again on the host on the resolved path: DENY paths are\n" +
				"  write_path_denied (FATAL, even with --force), ASK paths sensitive_path\n" +
				"  (STOP) without --force; secret files are never written or uploaded\n" +
				"  (secret_file_blocked). Output: {\"host\",\"path\",\"bytes\",\"sha256\",\n" +
				"  \"replaced\",\"backup\",\"verified\",\"written\":true}; --dry-run gives\n" +
				"  \"exists\" and \"dry_run\":true,\"written\":false.",
		},
		LoadsEnv:  true,
		ShellVars: []string{"HOME"},
		Env:       []string{"GASH_ENV_FILE", "HOME", "PWD"},
		Replaces:  "scp f h:/tmp/x && ssh h \"install -m 644 /tmp/x /dest && sha256sum /dest\"",
	},
	{
		Name:     "llm_remote_run",
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
			Short:  "Run a read-only command on registered hosts, in parallel or in steps",
			Module: "llm",
			Body: "" +
				"USAGE\n" +
				"  llm_remote_run HOST... [-j N] [--timeout S] [--format F] -- CMD\n" +
				"  llm_remote_run HOST --step CMD... [--stop-on-error] [--timeout S] [--format F]\n" +
				"  (budget: [--width N] [--max-lines N] [--head N] [--tail N])\n" +
				"\n" +
				"OPTIONS\n" +
				"  -- CMD                 The command line, run on every HOST in parallel: one\n" +
				"                         word is the whole line (quote pipes), several words\n" +
				"                         are one command\n" +
				"  --step CMD             One step of a session on one HOST; repeat it: the\n" +
				"                         steps run in order in ONE ssh round trip\n" +
				"  --stop-on-error        End the session at the first step that fails\n" +
				"  -j, --jobs N           Hosts at a time (default 8)\n" +
				"  --timeout S            Per host (with --step: the session; default 120)\n" +
				"  --format F             json (default), jsonl, tsv or table\n" +
				"  --width N              Cut output lines at N columns\n" +
				"  --max-lines N          Keep the first N lines of each out/err (--head N)\n" +
				"  --head N, --tail N     Keep the first / last N lines of each out/err\n" +
				"                         (default: 100 and 100, an omitted marker between)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  # The same check on three hosts at once\n" +
				"  llm_remote_run web1 web2 db -- 'df -h / | tail -n 1'\n" +
				"\n" +
				"  # Three steps, one round trip\n" +
				"  llm_remote_run web --step 'systemctl is-active nginx' --step 'tail -n 20 /var/log/nginx/error.log' --step 'uptime'\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces for h in ...; do ssh $h CMD; done and ssh h a; ssh h b; ssh h c.\n" +
				"  HOST is a registered HOST: alias of ~/.gash_env (unknown_host, STOP,\n" +
				"  otherwise). CMD passes the llm_exec rules here before anything connects\n" +
				"  (read-only commands, secret files and protected paths refused); a glob or\n" +
				"  ~ is refused (remote_glob_unsupported): it would expand on this machine.\n" +
				"  Each command runs in bash on the host, stdin closed, from the remote home.\n" +
				"  Steps do not share a shell (no cd or variables between steps).\n" +
				"  Output: [{\"host\",\"rc\",\"ms\",\"out\",\"err\",\"error\"}] in HOST order, error\n" +
				"  null or timeout, host_unreachable, remote_protocol_error; with --step\n" +
				"  [{\"step\",\"rc\",\"ms\",\"out\"}] (out holds stdout and stderr; ms null when\n" +
				"  the host's date has no %N), and {\"stopped\":true,\"skipped\":K} or\n" +
				"  {\"timed_out\":true} on stderr. A failing command is data (rc), not an\n" +
				"  error of the call.",
		},
		LoadsEnv:  true,
		ShellVars: []string{"HOME"},
		Env:       []string{"GASH_ENV_FILE", "HOME"},
		Replaces:  "for h in a b; do ssh $h CMD; done; ssh h a; ssh h b",
	},
	{
		Name:     "llm_same",
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
			Short:  "Is the deployed copy the local one? Hash both sides, list the differences",
			Module: "llm",
			Body: "" +
				"USAGE\n" +
				"  llm_same LOCAL ALIAS:REMOTE... [--container CTR] [--exclude NAME]... [-j N] [--timeout S]\n" +
				"  (budget: [--width N] [--max-lines N] [--head N] [--tail N])\n" +
				"\n" +
				"OPTIONS\n" +
				"  --container CTR        REMOTE is a path inside container CTR on the host\n" +
				"                         (docker exec)\n" +
				"  --exclude NAME         Leave out files and directories with this name glob\n" +
				"                         (repeatable; .git is always left out)\n" +
				"  -j, --jobs N           Targets at a time (default 8)\n" +
				"  --timeout S            Per target (default 120)\n" +
				"  --width N              Cut each p at N columns\n" +
				"  --max-lines N          Keep the first N diff entries (--head N; default 100)\n" +
				"  --head N, --tail N     Keep the first / last N diff entries\n" +
				"\n" +
				"EXAMPLES\n" +
				"  # One file on two hosts\n" +
				"  llm_same config/app.php web1:/srv/app/config/app.php web2:/srv/app/config/app.php\n" +
				"\n" +
				"  # A whole tree, without node_modules\n" +
				"  llm_same dist web:/var/www/app/dist --exclude node_modules\n" +
				"\n" +
				"  # Inside a container\n" +
				"  llm_same src/handler.py web:/app/handler.py --container api\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces sha256sum f | cut -c1-12; ssh h \"docker exec c sha256sum f\" and\n" +
				"  the eyeball comparison. One ssh round trip per target, however many files.\n" +
				"  Output: {\"same\":false,\"files\":N,\"diff\":[{\"host\",\"p\",\"local\",\"remote\"}]}\n" +
				"  with the first 12 hex digits of each sha256, null where a side lacks the\n" +
				"  path; p is relative to the directory (LOCAL as given for a file, \".\" when\n" +
				"  a side is missing or is a directory against a file); files counts the\n" +
				"  local files. Directories are compared recursively over regular files\n" +
				"  (symlinks not followed); names holding a newline or a backslash are left\n" +
				"  out. Secret files are not compared (secrets_skipped: K); a secret LOCAL\n" +
				"  file is secret_file_blocked. ALIAS is a registered HOST: alias; REMOTE\n" +
				"  follows the remote path rules (AGENTS.md 3.12). The host needs\n" +
				"  sha256sum, shasum or openssl (hash_tool_not_found).",
		},
		LoadsEnv:  true,
		ShellVars: []string{"HOME"},
		Env:       []string{"GASH_ENV_FILE", "HOME", "PWD"},
		Replaces:  "sha256sum f | cut -c1-12; ssh h \"sha256sum f\" | cut -c1-12",
	},
}
