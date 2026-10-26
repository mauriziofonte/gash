// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

// catalogGit are the 3.4 agent-toolkit functions of one area, git:
// llm_git_overview, llm_git_show, agent_git_commit (G1, G2, G4). Entries
// follow the catalog_gen.go format; mergeCatalog sorts them in.
var catalogGit = []Function{
	{
		Name:     "agent_git_commit",
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
			Short:   "Stage explicit paths, commit, optionally push, in one call",
			Module:  "agent",
			SeeAlso: "llm_git_overview llm_git_status",
			Body: "" +
				"USAGE\n" +
				"  agent_git_commit -m SUBJECT [-m BODY]... (--add PATH... | -A) [--author \"N <e>\"]\n" +
				"                   [--push [REMOTE]] [--dry-run] [--force] [--timeout S] [--format F]\n" +
				"                   [--max-lines N] [--head N] [--tail N] [--width N] [PATH]\n" +
				"\n" +
				"OPTIONS\n" +
				"  -m, --message TEXT     The subject; each further -m is a body paragraph\n" +
				"  --add PATH...          The paths to stage (files or directories, relative to\n" +
				"                         PATH), every word up to the next option\n" +
				"  -A, --all              Stage every change of the work tree (never implied)\n" +
				"  --author \"N <e>\"       The commit's author\n" +
				"  --push [REMOTE]        Push the branch afterwards: to its upstream, or to\n" +
				"                         REMOTE under the branch's name; never forced\n" +
				"  --dry-run              Report the files, the branch and the push target;\n" +
				"                         stage, commit and push nothing\n" +
				"  --force                Allow a repository on a sensitive path (/etc, /var,\n" +
				"                         /usr/local, /root) outside your HOME and working\n" +
				"                         directory: only after the user agreed. It never forces\n" +
				"                         a push\n" +
				"  --timeout S            Bound the commit (hooks) and the push (default 120s)\n" +
				"  --format F             json (default) or jsonl: the report is one object\n" +
				"  --max-lines N          Paths listed by --dry-run (default 100; also --head N,\n" +
				"                         --tail N); --width N cuts each path\n" +
				"\n" +
				"EXAMPLES\n" +
				"  # Commit two files and push to the upstream\n" +
				"  agent_git_commit -m 'fix: parser' -m 'Handles empty input.' --add src/p.go src/p_test.go --push\n" +
				"  # {\"hash\":\"a1b2c3d\",\"branch\":\"main\",\"files\":2,\"pushed\":true,\"remote\":\"origin\",\"ahead\":0}\n" +
				"\n" +
				"  # Preview a commit of every change in another repository\n" +
				"  agent_git_commit -m 'chore: update' -A --dry-run ~/src/site\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces git add A B && git commit -m S -m B && git push && git log -1.\n" +
				"  Checked before anything is written: a detached HEAD (detached_head) and\n" +
				"  unresolved conflicts (merge_conflict) are STOP; nothing to commit\n" +
				"  (nothing_to_commit), a missing path (file_not_found), a push without an\n" +
				"  upstream (no_upstream) or to an unknown remote (unknown_remote), and a\n" +
				"  branch behind its upstream (push_rejected) are RETRY; a secret file in the\n" +
				"  commit (staged already or under an --add path) is secret_file_blocked\n" +
				"  (FATAL). The commit holds what is staged plus the --add paths, as git add\n" +
				"  && git commit does. git runs with your configuration and hooks and never\n" +
				"  prompts. A failed commit (a hook, the identity) is commit_failed (RETRY)\n" +
				"  with the paths left staged; a push the remote refuses is push_rejected\n" +
				"  (RETRY) with the remote's message, any other push failure push_failed\n" +
				"  (STOP); both carry \"hash\" and \"branch\" of the commit, which exists.\n" +
				"  ahead is null without an upstream to compare with.",
		},
		ShellVars: []string{"HOME"},
		Env:       []string{"HOME", "PWD"},
		Replaces:  "git add A B && git commit -m S && git push && git log -1",
	},
	{
		Name:     "llm_git_overview",
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
			Short:   "Repository state in one small object (JSON)",
			Module:  "llm",
			SeeAlso: "llm_git_status llm_git_log llm_git_diff",
			Body: "" +
				"USAGE\n" +
				"  llm_git_overview [PATH] [--log N] [--files N] [--host ALIAS]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --log N                Last N commit subjects (default 3; 0 for none)\n" +
				"  --files N              Also list the paths of each state, at most N each\n" +
				"  --host ALIAS           Run on a registered HOST: alias over ssh, one round\n" +
				"                         trip (repeatable; see AGENTS.md 3.12)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  llm_git_overview\n" +
				"  # {\"branch\":\"main\",\"upstream\":\"origin/main\",\"ahead\":1,\"behind\":0,\n" +
				"  #  \"counts\":{\"staged\":0,\"modified\":2,\"untracked\":1,\"conflicted\":0},\n" +
				"  #  \"log\":[{\"hash\":\"a1b2c3d\",\"subject\":\"fix: parser\"}],\n" +
				"  #  \"diffstat\":{\"files\":2,\"insertions\":10,\"deletions\":3}}\n" +
				"\n" +
				"  # With up to 20 paths per state\n" +
				"  llm_git_overview . --files 20\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces git status --short | wc -l; git log --oneline -3; git diff --stat\n" +
				"  | tail -1. branch is \"\" on a detached HEAD, upstream \"\" without one (ahead\n" +
				"  and behind are then 0). counts follow llm_git_status: a path both staged\n" +
				"  and modified counts in both. diffstat counts the tracked changes against\n" +
				"  HEAD, staged or not (untracked files are only in counts). With --files,\n" +
				"  \"files\" holds the four lists (paths relative to the repository root) and\n" +
				"  \"truncated\" the number of paths left out. Not a repository:\n" +
				"  not_a_git_repo (CONTINUE).",
		},
		ShellVars: []string{"HOME"},
		Env:       []string{"HOME", "PWD"},
		Replaces:  "git status --short | wc -l; git log --oneline -3; git diff --stat | tail -1",
	},
	{
		Name:     "llm_git_show",
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
			Short:   "A commit's metadata and files, its patch, or a file at a revision",
			Module:  "llm",
			SeeAlso: "llm_git_log llm_git_diff",
			Body: "" +
				"USAGE\n" +
				"  llm_git_show [REV] [--stat] [--files] [PATH]\n" +
				"  llm_git_show [REV] --patch [--max-lines N] [--head N] [--tail N] [--width N] [PATH]\n" +
				"  llm_git_show REV:FILE [--max-lines N] [--head N] [--tail N] [--width N] [PATH]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --stat                 Add the totals: {\"files\",\"insertions\",\"deletions\"}\n" +
				"  --files                Add each file's status letter (A, M, D, R, C, T)\n" +
				"  --patch                The commit's patch as text, capped per file\n" +
				"  --max-lines N          Lines kept per file with --patch (default 200), of the\n" +
				"                         content with REV:FILE (default 2000), files in the\n" +
				"                         JSON otherwise; also --head N and --tail N\n" +
				"  --width N              Cut each line (JSON: each path and the subject) at N\n" +
				"                         columns\n" +
				"\n" +
				"EXAMPLES\n" +
				"  llm_git_show HEAD~1 --stat\n" +
				"  # {\"hash\":\"6595b2c\",\"subject\":\"docs: change 118\",\"author\":\"Ann\",\n" +
				"  #  \"date\":\"2026-10-25 22:44:46 +0000\",\"files\":[{\"p\":\"CHANGES.md\",\"add\":1,\n" +
				"  #  \"del\":0}],\"stat\":{\"files\":1,\"insertions\":1,\"deletions\":0}}\n" +
				"\n" +
				"  # The patch, 80 lines per file\n" +
				"  llm_git_show a1b2c3d --patch --max-lines 80\n" +
				"\n" +
				"  # A file as it was two commits ago, first 100 lines\n" +
				"  llm_git_show HEAD~2:src/app.js --max-lines 100\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces git show REV --stat, git show REV --name-status and git show\n" +
				"  REV:path | sed -n 1,80p. REV defaults to HEAD; a tag shows its commit; a\n" +
				"  merge is shown against its first parent. File paths are relative to the\n" +
				"  repository root; add and del are null for a binary file. A FILE as PATH\n" +
				"  limits the files and the patch to it. --patch keeps each file's header and\n" +
				"  puts [... K lines omitted] where it cuts; the patch of a secret file is\n" +
				"  replaced by a marker line. REV:FILE paths are relative to the repository\n" +
				"  root (REV:./FILE to PATH); a secret FILE is secret_file_blocked (FATAL), a\n" +
				"  binary one binary_file. An unknown REV is unknown_revision (RETRY), a FILE\n" +
				"  missing at REV file_not_found, not a repository not_a_git_repo (CONTINUE).",
		},
		ShellVars: []string{"HOME"},
		Env:       []string{"HOME", "PWD"},
		Replaces:  "git show REV --stat; git show REV:path | sed -n 1,80p",
	},
}
