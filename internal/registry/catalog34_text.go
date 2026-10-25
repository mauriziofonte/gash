// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

// catalogText are the 3.4 agent-toolkit functions of one area, text reads:
// llm_stat, llm_count, llm_assert, llm_read, llm_log, llm_hash,
// llm_textstats, llm_textcheck (F1, F6-F12). Entries follow the
// catalog_gen.go format; mergeCatalog sorts them in.
var catalogText = []Function{
	{
		Name:     "llm_assert",
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
			Short:   "Check that files hold, lack or count patterns; exit 0 only if all pass",
			Module:  "llm",
			SeeAlso: "llm_count",
			Body: "" +
				"USAGE\n" +
				"  llm_assert FILE... (--has P | --lacks P | --count P=N)... [--regex] [--lines | --occurrences] [-j N] [--timeout S] [--absolute]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --has P                Every FILE holds P (repeatable, as the two below)\n" +
				"  --lacks P              No FILE holds P\n" +
				"  --count P=N            Every FILE holds P exactly N times (the last = splits)\n" +
				"  --regex                Patterns are regular expressions (ripgrep's syntax);\n" +
				"                         literal text by default\n" +
				"  --lines                --count counts the lines that hold P (default)\n" +
				"  --occurrences          --count counts every occurrence of P\n" +
				"  -j N, --jobs N         Files read at a time (default: the CPU count)\n" +
				"  --timeout S            Time limit per file (seconds, 500ms, 2s, 1m)\n" +
				"  --absolute, --relative Absolute paths, or as given (the default)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  # After an edit: the new name is in, the old one is gone, one route\n" +
				"  llm_assert app/A.php app/B.php --has 'NewName' --lacks 'OldName'\n" +
				"  llm_assert routes/web.php --count \"Route::get('/health'=1\"\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces chains of grep -q / grep -c used as checks, read back by eye.\n" +
				"  Every assertion is checked on every FILE, with the llm_count scanner (one\n" +
				"  read per file). All pass: {\"pass\":true,\"checked\":N}, exit 0. Otherwise\n" +
				"  exit 1 and {\"pass\":false,\"checked\":N,\"failed\":[{\"file\",\"check\",\"found\"}]},\n" +
				"  check as has:P, lacks:P or count:P=N; a FILE that cannot be read fails as\n" +
				"  {\"file\",\"error\"} (file_not_found, not_a_file, timeout). Secret files are\n" +
				"  refused (secret_file_blocked, FATAL); a bad --count is invalid_assertion.",
		},
		ShellVars: []string{"HOME"},
		Env:       []string{"HOME", "PWD"},
		Replaces:  "grep -q P f && echo ok; grep -c P f (as checks)",
	},
	{
		Name:     "llm_count",
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
			Short:   "Count several patterns in several files in one pass per file",
			Module:  "llm",
			SeeAlso: "llm_assert llm_grep",
			Body: "" +
				"USAGE\n" +
				"  llm_count FILE... --pat P... [--regex] [--lines | --occurrences] [--format F] [-j N] [--timeout S] [--absolute]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --pat P                A pattern to count (repeatable)\n" +
				"  --regex                Patterns are regular expressions (ripgrep's syntax);\n" +
				"                         literal text by default\n" +
				"  --lines                Count the lines that hold the pattern, as grep -c\n" +
				"                         (default)\n" +
				"  --occurrences          Count every occurrence, as grep -o | wc -l\n" +
				"  --format F             tsv, jsonl or table: one row per file and pattern\n" +
				"                         (file, pat, count); json is the default object\n" +
				"  -j N, --jobs N         Files read at a time (default: the CPU count)\n" +
				"  --timeout S            Time limit per file (seconds, 500ms, 2s, 1m)\n" +
				"  --absolute, --relative Absolute paths, or as given (the default)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  llm_count storage/logs/laravel.log --pat ERROR --pat WARNING --pat 'Timeout'\n" +
				"  llm_count a.md b.md --pat '## ' --format tsv\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces grep -c A f; grep -c B f; grep -c C g (one pass per pattern).\n" +
				"  Every pattern is counted in one read of each file (a large file in\n" +
				"  parallel parts). Output: {\"FILE\":{\"P\":N,...},...}, files and patterns in\n" +
				"  the order given (repeats dropped). A FILE that cannot be read is\n" +
				"  {\"FILE\":{\"error\":\"file_not_found\"}} (also not_a_file, timeout). A pattern\n" +
				"  never matches across lines; an empty one is missing_pattern, one with a\n" +
				"  line break invalid_filter. Secret files are refused (secret_file_blocked,\n" +
				"  FATAL). For a case-insensitive count use --regex with (?i).",
		},
		ShellVars: []string{"HOME"},
		Env:       []string{"HOME", "PWD"},
		Replaces:  "grep -c A f; grep -c B f; grep -c C g",
	},
	{
		Name:     "llm_hash",
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
			Short:   "Hash files in parallel (sha256 or md5)",
			Module:  "llm",
			SeeAlso: "llm_stat",
			Body: "" +
				"USAGE\n" +
				"  llm_hash PATH... [--algo sha256|md5] [--short N] [--format F] [-j N] [--timeout S] [--absolute]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --algo sha256|md5      The digest (default sha256)\n" +
				"  --short N              Keep the first N hex digits\n" +
				"  --format F             json (default), jsonl, tsv or table\n" +
				"  --head N, --tail N     Keep the first / last N rows (--max-lines N is --head)\n" +
				"  --width N              Cut values at N columns\n" +
				"  -j N, --jobs N         Files hashed at a time (default: the CPU count)\n" +
				"  --timeout S            Time limit per file (seconds, 500ms, 2s, 1m)\n" +
				"  --absolute, --relative Absolute paths, or as given (the default)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  llm_hash dist/app.js dist/app.css --short 12\n" +
				"  llm_hash backup.tar --algo md5\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces sha256sum f | cut -c1-12 and md5sum f, file by file.\n" +
				"  Output: [{\"p\",\"sha256\",\"size\"}] in input order (the key is \"md5\" with\n" +
				"  --algo md5). A PATH that cannot be hashed keeps its row with null digest\n" +
				"  and size and an \"error\" (file_not_found, not_a_file, path_unreadable,\n" +
				"  timeout); the error column appears only then. Secret files are hashed: a\n" +
				"  digest shows no content.",
		},
		ShellVars: []string{"HOME"},
		Env:       []string{"HOME", "PWD"},
		Replaces:  "sha256sum f | cut -c1-12; md5sum f",
	},
	{
		Name:     "llm_log",
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
			Short:   "Last lines of a log, read from the end, by pattern, time and level",
			Module:  "llm",
			SeeAlso: "llm_read",
			Body: "" +
				"USAGE\n" +
				"  llm_log FILE [--tail N] [--last-match RE -n N] [--grep RE] [--since T] [--level L] [--width N] [--format F]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --tail N, -n N         The last N selected lines (default 50)\n" +
				"  --last-match RE        Lines that match RE (ripgrep's syntax); with -n N, the\n" +
				"                         last N matches. Same as --grep RE\n" +
				"  --grep RE              Keep the lines that match RE (repeatable: any one)\n" +
				"  --since T              Entries at or after T: a duration back from now (30s,\n" +
				"                         10m, 2h, 1d, 1w) or an ISO 8601 date or time\n" +
				"  --level L              Entries at level L or more severe: trace, debug,\n" +
				"                         info, notice, warning, error, critical, alert,\n" +
				"                         emergency (warn, err, fatal, ... are read too)\n" +
				"  --width N              Cut each line at N columns\n" +
				"  --format F             json, jsonl, tsv or table: rows {\"line\",\"ts\",\n" +
				"                         \"level\",\"text\"} (line numbers cost a count pass)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  llm_log storage/logs/laravel.log --last-match ERROR -n 20\n" +
				"  llm_log /var/log/nginx/error.log --since 1h --level error --width 300\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces tail -n N log | cut -c1-W and grep X log | tail -N.\n" +
				"  The file is read backwards in 64 KiB blocks, so the cost is the size of\n" +
				"  the answer, not of the log. Lines print in file order, as they are. An\n" +
				"  entry is a line with a timestamp or a level and the lines below it with\n" +
				"  neither (a stack trace): --since and --level keep or drop whole entries,\n" +
				"  --grep single lines. Timestamps read: ISO 8601 (also [bracketed], as\n" +
				"  Laravel and Monolog write it), syslog (Oct 25 14:02:01), nginx and Apache\n" +
				"  access ([25/Oct/2026:14:02:01 +0000]) and error logs, and an ISO time in\n" +
				"  the first 64 bytes (JSON lines). A time without a zone is local (TZ).\n" +
				"  With --since the walk stops at the first older entry and a cut by --tail\n" +
				"  is marked \"[... K lines omitted]\". Secret files are refused (FATAL),\n" +
				"  binary ones binary_file; a bad T is invalid_duration.",
		},
		ShellVars: []string{"HOME"},
		Env:       []string{"HOME", "PWD", "TZ"},
		Replaces:  "tail -n 50 app.log | cut -c1-200; grep ERROR app.log | tail -20",
	},
	{
		Name:     "llm_read",
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
			Short:   "Read files, line ranges or the lines around a match, numbered",
			Module:  "llm",
			SeeAlso: "llm_grep llm_log",
			Body: "" +
				"USAGE\n" +
				"  llm_read PATH[:A-B]... [--lines A:B] [--around RE -C N] [--max-bytes N] [--head N] [--tail N] [--max-lines N] [--width N] [--format F] [--absolute]\n" +
				"\n" +
				"OPTIONS\n" +
				"  PATH:A-B               Lines A to B of that file (also PATH:A:B, PATH:A-)\n" +
				"  --lines A:B            Lines A to B of every PATH without its own range\n" +
				"                         (repeatable; A-B and A,B too, A: and :B leave an end\n" +
				"                         open)\n" +
				"  --around RE            The lines that match RE (ripgrep's syntax) and their\n" +
				"                         context (repeatable)\n" +
				"  -C N, --context N      Context lines around each match (default 3)\n" +
				"  --max-bytes N          Content bytes printed per file (default 262144)\n" +
				"  --head N, --tail N     The first / last N selected lines of each file\n" +
				"                         (--max-lines N is --head N)\n" +
				"  --width N              Cut each line at N columns\n" +
				"  --format F             json, jsonl, tsv or table: rows {\"file\",\"line\",\"text\"}\n" +
				"  --absolute, --relative Absolute paths, or as given (the default)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  # Locate and read in one call\n" +
				"  llm_read app/Http/Kernel.php --around 'protected \\$middleware' -C 10\n" +
				"\n" +
				"  # Two ranges of two files\n" +
				"  llm_read src/a.php:120-180 src/b.php:1-40\n" +
				"\n" +
				"  # The end of a file\n" +
				"  llm_read CHANGELOG.md --tail 30\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces grep -n X f, then sed -n A,Bp f (two calls), and cat a b.\n" +
				"  Each run of consecutive lines is a block \"==> FILE:A-B <==\", each line N:\n" +
				"  then its text (N- for a context line of --around); a cut is marked\n" +
				"  \"[... K lines omitted]\". A file with nothing selected prints its header\n" +
				"  and why ([no match in N lines], [empty file]). PATH:A-B is read as a range\n" +
				"  only when PATH:A-B itself does not exist. Secret files are refused\n" +
				"  (secret_file_blocked, FATAL, as llm_config), binary files binary_file, a\n" +
				"  missing file file_not_found, a bad range invalid_range, before anything is\n" +
				"  printed.",
		},
		ShellVars: []string{"HOME"},
		Env:       []string{"HOME", "PWD"},
		Replaces:  "grep -n X f; sed -n A,Bp f; cat -n f | sed -n A,Bp; cat a b",
	},
	{
		Name:     "llm_stat",
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
			Short:   "Metadata of several paths: type, size, mode, owner, mtime, lines",
			Module:  "llm",
			SeeAlso: "llm_hash llm_textstats",
			Body: "" +
				"USAGE\n" +
				"  llm_stat PATH... [--lines] [--follow] [--format F] [-j N] [--timeout S] [--absolute]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --lines                Count the lines of regular files under 64 MiB\n" +
				"  --follow               Describe what symlinks point to; target is the\n" +
				"                         resolved path\n" +
				"  --format F             tsv (default), json, jsonl or table\n" +
				"  --head N, --tail N     Keep the first / last N rows (--max-lines N is --head)\n" +
				"  --width N              Cut output lines at N columns\n" +
				"  -j N, --jobs N         Paths read at a time (default: the CPU count)\n" +
				"  --timeout S            Time limit per path (seconds, 500ms, 2s, 1m)\n" +
				"  --absolute, --relative Absolute paths, or as given (the default)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  llm_stat composer.json vendor storage/logs/laravel.log --lines\n" +
				"  llm_stat public/storage --follow --format json\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces ls -la a b; [ -f c ] && echo yes; wc -l d; readlink -f e.\n" +
				"  One row per PATH in input order: p, t (f d l p s c b, or missing), size,\n" +
				"  mode (octal, as chmod reads it), owner (user:group), mtime (UTC, ISO\n" +
				"  8601), lines with --lines (null for a directory, a link, a file of 64 MiB\n" +
				"  or more). target is the link text of a symlink, or with --follow the\n" +
				"  resolved path (t \"l\" then means it could not be followed). A missing path\n" +
				"  is a row, not an error. Secret files: metadata yes, lines no (error\n" +
				"  secret_file_blocked in that row). Columns lines, target and error appear\n" +
				"  only when they apply. TSV by default: the JSON objects of --format json\n" +
				"  repeat every key on every row.",
		},
		ShellVars: []string{"HOME"},
		Env:       []string{"HOME", "PWD"},
		Replaces:  "ls -la a b; [ -f c ] && echo yes; wc -l d; readlink -f e",
	},
	{
		Name:     "llm_textcheck",
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
			Short:   "Text hygiene checks: non-ASCII, CRLF, BOM, trailing blanks, tabs, long lines",
			Module:  "llm",
			SeeAlso: "llm_textstats",
			Body: "" +
				"USAGE\n" +
				"  llm_textcheck FILE... [--rules non-ascii,crlf,bom,trailing-ws,tabs,long-lines=N,final-newline,mixed-indent] [--max-lines N] [--format F] [-j N] [--timeout S] [--absolute]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --rules LIST           The rules to run, comma-separated (default:\n" +
				"                         non-ascii,crlf,bom,trailing-ws,final-newline,\n" +
				"                         mixed-indent)\n" +
				"  --max-lines N          Issues listed (default 100; --head N is the same,\n" +
				"                         --tail N keeps the last N)\n" +
				"  --width N              Cut samples at N columns\n" +
				"  --format F             jsonl, tsv or table: one row per issue; json is the\n" +
				"                         default object\n" +
				"  -j N, --jobs N         Files read at a time (default: the CPU count)\n" +
				"  --timeout S            Time limit per file (seconds, 500ms, 2s, 1m)\n" +
				"  --absolute, --relative Absolute paths, or as given (the default)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  llm_textcheck README.md docs/*.md\n" +
				"  llm_textcheck src/*.php --rules trailing-ws,tabs,long-lines=120\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces grep -nP '[^\\x00-\\x7F]' f, file -i f and python unicodedata\n" +
				"  scripts. One read per file. Output: {\"files\":N,\"issues\":[{\"file\",\"line\",\n" +
				"  \"rule\",\"sample\"}]}, plus \"counts\" by rule when there are issues,\n" +
				"  \"truncated\" when the list was cut and \"skipped\" ([{\"file\",\"reason\"}]:\n" +
				"  binary_file, file_not_found, not_a_file) when files were skipped. No\n" +
				"  issue: {\"files\":N,\"issues\":[]}. non-ascii names the code points\n" +
				"  (U+200B, U+00E9 e-acute); crlf is one issue per file (\"K of N lines\");\n" +
				"  bom, mixed-indent (the first line indented unlike the first indented\n" +
				"  line) and final-newline are once per file. Secret files are refused\n" +
				"  (FATAL); an unknown rule is invalid_rule.",
		},
		ShellVars: []string{"HOME"},
		Env:       []string{"HOME", "PWD"},
		Replaces:  "grep -nP '[^\\x00-\\x7F]' f; file -i f",
	},
	{
		Name:     "llm_textstats",
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
			Short:   "Lines, words, characters, bytes, gzip size and longest line of files",
			Module:  "llm",
			SeeAlso: "llm_stat llm_textcheck",
			Body: "" +
				"USAGE\n" +
				"  llm_textstats FILE... [--format F] [-j N] [--timeout S] [--absolute]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --format F             json (default), jsonl, tsv or table\n" +
				"  --head N, --tail N     Keep the first / last N rows (--max-lines N is --head)\n" +
				"  --width N              Cut values at N columns\n" +
				"  -j N, --jobs N         Files read at a time (default: the CPU count)\n" +
				"  --timeout S            Time limit per file (seconds, 500ms, 2s, 1m)\n" +
				"  --absolute, --relative Absolute paths, or as given (the default)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  llm_textstats README.md docs/guide.md\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces wc -w f; wc -c f; gzip -c f | wc -c (three processes per file).\n" +
				"  One read per file. Output: [{\"p\",\"lines\",\"words\",\"chars\",\"bytes\",\"gzip\",\n" +
				"  \"max_line\"}]: lines counts a last line without a newline, words as wc -w\n" +
				"  (in a Markdown file without fenced code blocks and front matter), chars\n" +
				"  are UTF-8 characters, gzip is the gzip -c size at level 6 (null from 64\n" +
				"  MiB), max_line the longest line in characters. A file that is binary,\n" +
				"  secret, missing or not a file keeps its row with null values and an\n" +
				"  \"error\" (binary_file, secret_file_blocked, file_not_found, not_a_file).",
		},
		ShellVars: []string{"HOME"},
		Env:       []string{"HOME", "PWD"},
		Replaces:  "wc -w f; wc -c f; gzip -c f | wc -c",
	},
}
