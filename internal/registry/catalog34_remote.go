// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

// catalogRemote are the 3.4 agent-toolkit functions of one area, remote:
// llm_remote_run, llm_same, agent_put (R1-R3, R5, E2). Entries follow the
// catalog_gen.go format; mergeCatalog sorts them in.
var catalogRemote = []Function{
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
}
