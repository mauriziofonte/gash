// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

// catalogAgent are the 3.4 agent-toolkit functions of one area, edits:
// agent_replace (E1). Entries follow the catalog_gen.go format;
// mergeCatalog sorts them in.
var catalogAgent = []Function{
	{
		Name:     "agent_replace",
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
			Short:  "Replace text in explicit files, all or nothing",
			Module: "agent",
			Body: "" +
				"USAGE\n" +
				"  agent_replace FILE... (--old A --new B)... [--count N | --min N] [--regex] [--dry-run] [--force] [--format F] [--width N] [--host ALIAS]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --old A --new B        One replacement; repeat the pair for more, applied in\n" +
				"                         order (each to the result of the one before)\n" +
				"  --count N              Every pair must match exactly N times in every file\n" +
				"                         (default: exactly 1)\n" +
				"  --min N                Every pair must match at least N times (replaces all)\n" +
				"  --regex                --old is a Go (RE2) regular expression; $1 in --new is\n" +
				"                         a group\n" +
				"  --dry-run              Report the counts and the lines each file would gain\n" +
				"                         and lose; write nothing\n" +
				"  --force                Allow a sensitive path (/etc, /var, /usr/local, /root,\n" +
				"                         .ssh) outside your HOME and working directory: only\n" +
				"                         after the user agreed\n" +
				"  --format F             json (default), jsonl, tsv or table: one row per file\n" +
				"                         and pair\n" +
				"  --width N              Cut output lines at N columns\n" +
				"  --host ALIAS           Edit the files on a registered HOST: alias (the gash\n" +
				"                         binary runs there; one ssh round trip; AGENTS.md 3.12)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  # Rename a class in two files; fails, writing nothing, unless each has it once\n" +
				"  agent_replace app/A.php app/B.php --old 'OldName' --new 'NewName'\n" +
				"\n" +
				"  # Every occurrence, at least one per file, preview first\n" +
				"  agent_replace src/x.js src/y.js --old 'var ' --new 'let ' --min 1 --dry-run\n" +
				"\n" +
				"  # Two pairs, a regular expression with a group\n" +
				"  agent_replace conf.ini --old 'debug=1' --new 'debug=0' --old 'v([0-9]+)' --new 'v$1-lts' --regex\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces python3 open().read().replace() + write scripts and sed -i, with\n" +
				"  the count assertion built in. All or nothing: every pair's count is checked\n" +
				"  in every file in memory first; one mismatch is replace_count_mismatch\n" +
				"  (RETRY) and no file changes. Writes go to a temporary file renamed over the\n" +
				"  target (mode kept); a file that changed meanwhile is file_changed and\n" +
				"  nothing is written. Targets are explicit: no glob is expanded, each file is\n" +
				"  named once. Secret files, protected paths and system paths are refused\n" +
				"  (FATAL); a sensitive path is sensitive_path (STOP) without --force; binary\n" +
				"  files and files over 64 MiB are refused. Output:\n" +
				"  {\"files\":N,\"replaced\":{\"FILE\":[n1,n2]},\"written\":true}.",
		},
		ShellVars: []string{"HOME"},
		Env:       []string{"HOME", "PWD"},
		Replaces:  "python3 - <<EOF ... s.replace(...) ... EOF; sed -i",
	},
}
