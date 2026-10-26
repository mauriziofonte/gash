// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

// catalogData are the 3.4 agent-toolkit functions of one area, data:
// llm_db_databases, llm_json, llm_csv, llm_dotenv, llm_date (D3-D6, V3, V4).
// Entries follow the catalog_gen.go format; mergeCatalog sorts them in.
var catalogData = []Function{
	{
		Name:     "llm_db_databases",
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
			Short:   "List the databases of a connection (replaces SHOW DATABASES, psql -l)",
			Module:  "llm",
			SeeAlso: "llm_db_tables llm_db_query",
			Body: "" +
				"USAGE\n" +
				"  llm_db_databases [-c CONNECTION | -f SQLITE_FILE] [-d DB] [--format json|jsonl|tsv|table] [--scalar]\n" +
				"\n" +
				"OPTIONS\n" +
				"  -c, --connection NAME  Connection from ~/.gash_env (default: default)\n" +
				"  -d, --database NAME    Database to connect to (any one works)\n" +
				"  -f, --file PATH        SQLite database file\n" +
				"  --format F             json (the envelope, default), jsonl, tsv or table\n" +
				"  --scalar               Print the one value of a 1x1 result\n" +
				"\n" +
				"EXAMPLES\n" +
				"  llm_db_databases -c default\n" +
				"  llm_db_databases -c postgres --format tsv\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces mysql -e 'SHOW DATABASES' and psql -l. Output:\n" +
				"  {data: [names], rows, query_time}, sorted: every schema MySQL/MariaDB\n" +
				"  shows (system ones included), every PostgreSQL database that is not a\n" +
				"  template, \"main\" for an SQLite file. The connection needs no\n" +
				"  database here (PostgreSQL then connects to \"postgres\"). With\n" +
				"  --format jsonl, tsv or table the names are rows of a \"name\" column\n" +
				"  and the metadata is one JSON line on stderr.",
		},
		Settings:  []string{"GASH_DB_TIMEOUT"},
		LoadsEnv:  true,
		ShellVars: []string{"HOME"},
		Env:       []string{"GASH_ENV_FILE", "GASH_HEADLESS", "GASH_NO_COLOR", "HOME", "NO_COLOR", "PWD"},
		Replaces:  "mysql -e 'SHOW DATABASES'; psql -l",
	},
	{
		Name:     "llm_json",
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
			Short:   "One value, the keys or the shape of a JSON file (replaces python3 json.load, jq)",
			Module:  "llm",
			SeeAlso: "llm_config llm_csv",
			Body: "" +
				"USAGE\n" +
				"  llm_json FILE|- [--path P] [--keys | --shape] [--format json|jsonl|tsv|table]\n" +
				"           [--width N] [--head N] [--tail N] [--host ALIAS]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --path P               The value at P: a.b[0].c, paths[\"/api/x\"].post, [-1]\n" +
				"  --keys                 The keys of an object (or of the objects of an array)\n" +
				"  --shape                Types and lengths, two levels deep\n" +
				"  --format F             json (default), jsonl (one element per line), tsv or\n" +
				"                         table (an array of objects as rows)\n" +
				"  --width N              Cut each string value at N columns (ends with …)\n" +
				"  --head N, --tail N     Keep the first / last N elements of an array\n" +
				"  --host ALIAS           Run on a registered HOST: alias over ssh, one round\n" +
				"                         trip (repeatable; see AGENTS.md 3.12)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  llm_json package.json --path scripts\n" +
				"  llm_json openapi.json --path 'paths[\"/users\"].get.responses' --keys\n" +
				"  llm_json data.json --shape\n" +
				"  llm_json users.json --format tsv --head 20\n" +
				"  curl -s https://api.example/x | llm_json - --path items[0]\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces python3 -c \"import json; print(json.load(open(f))[...])\" and\n" +
				"  jq '.a', jq 'length, (.[0]|keys)'. The whole text is validated first\n" +
				"  (invalid_json, RETRY, with line and column); the value prints as compact\n" +
				"  JSON with its numbers and escapes as written; a repeated key: the last\n" +
				"  one wins. A path that does not exist is json_path_not_found (RETRY) with\n" +
				"  the nearest existing prefix in the hint; --keys on a value without\n" +
				"  objects is not_an_object. --shape: {\"type\":\"array[100]\",\"items\":\n" +
				"  {\"type\":\"object{3}\",\"keys\":{\"id\":\"number\",...},\"optional\":[...]}};\n" +
				"  lengths are N or MIN..MAX, mixed types are joined with |. A cut array\n" +
				"  adds {\"truncated\":K} on stderr. Secret files (.env, keys, ...) are\n" +
				"  refused; - reads the standard input.",
		},
		ShellVars: []string{"HOME"},
		Env:       []string{"GASH_HEADLESS", "GASH_NO_COLOR", "HOME", "NO_COLOR", "PWD"},
		Replaces:  "python3 -c \"import json; print(json.load(open(f))[...])\"; jq '.a' f",
	},
	{
		Name:     "llm_csv",
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
			Short:   "Header, sample and row count of a CSV file (replaces python3 csv.reader, wc -l)",
			Module:  "llm",
			SeeAlso: "llm_json llm_config",
			Body: "" +
				"USAGE\n" +
				"  llm_csv FILE [--head N] [--cols] [--count] [--delim C] [--where COL=V]\n" +
				"          [--format json|jsonl|tsv|table] [--host ALIAS]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --head N               Sample rows (default 5; 0 for none)\n" +
				"  --cols                 The header only, no sample\n" +
				"  --count                Count every row (reads the whole file)\n" +
				"  --delim C              The delimiter (default: sniffed among , ; tab |);\n" +
				"                         tab or \\t for a tab\n" +
				"  --where COL=V          Only rows whose column COL (a header name or a\n" +
				"                         1-based number) is exactly V\n" +
				"  --format F             json (default), or the sample as jsonl, tsv or table\n" +
				"  --host ALIAS           Run on a registered HOST: alias over ssh, one round\n" +
				"                         trip (repeatable; see AGENTS.md 3.12)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  llm_csv export.csv\n" +
				"  llm_csv export.csv --head 2 --count\n" +
				"  llm_csv data.csv --where status=failed --count --head 10\n" +
				"  llm_csv report.txt --delim ';' --cols\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces python3 -c \"import csv; r=list(csv.reader(open(f))); ...\" and\n" +
				"  wc -l f; head -3 f (which miscounts quoted newlines). Output:\n" +
				"  {\"rows\":N,\"matches\":M,\"cols\":K,\"header\":[...],\"sample\":[[...]],\n" +
				"  \"delimiter\":\",\",\"errors\":[{\"line\":L,\"error\":\"...\"}],\"error_count\":E}\n" +
				"  (rows, matches and error_count with --count; matches with --where).\n" +
				"  A streaming RFC 4180 parser: quoted fields hold delimiters, \"\" and\n" +
				"  newlines; a record is not a line; blank lines are skipped. errors lists\n" +
				"  the first 10 malformed records (field count differing from the header,\n" +
				"  a stray quote, an unterminated quote) by their first line. Without\n" +
				"  --count the read stops once the header and the sample are complete.\n" +
				"  With --format jsonl, tsv or table the sample rows print under the\n" +
				"  header and the rest is one JSON line on stderr.",
		},
		ShellVars: []string{"HOME"},
		Env:       []string{"GASH_HEADLESS", "GASH_NO_COLOR", "HOME", "NO_COLOR", "PWD"},
		Replaces:  "python3 -c \"import csv; r=list(csv.reader(open(f))); print(len(r)-1, r[0])\"; wc -l f; head -3 f",
	},
	{
		Name:     "llm_dotenv",
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
			Short:   "Whether keys are set in a .env file, secrets masked (replaces grep KEY= .env)",
			Module:  "llm",
			SeeAlso: "llm_env llm_config",
			Body: "" +
				"USAGE\n" +
				"  llm_dotenv FILE [KEY...] [--present] [--host ALIAS]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --present              true or false per KEY, never a value\n" +
				"  --host ALIAS           Run on a registered HOST: alias over ssh, one round\n" +
				"                         trip (repeatable; see AGENTS.md 3.12)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  llm_dotenv .env DB_HOST DB_PORT DB_PASSWORD APP_URL\n" +
				"  llm_dotenv .env.production APP_KEY --present\n" +
				"  llm_dotenv .env\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces grep -E '^(DB_HOST|APP_URL)=' .env, which prints secrets into\n" +
				"  the transcript. Output: {\"DB_HOST\":\"127.0.0.1\",\"DB_PASSWORD\":\n" +
				"  \"***(set)\",\"MISSING\":null}. A key whose name llm_env hides\n" +
				"  (PASSWORD, TOKEN, KEY, SECRET, AUTH, ...) shows \"***(set)\" (\"\" when\n" +
				"  empty); other values pass llm_env's redaction (URL passwords, private\n" +
				"  keys, known token formats masked). Without KEY: the names the file\n" +
				"  sets, never a value. Reads .env, .env.*, *.env and .envrc files (the\n" +
				"  only reader of them; key and credential files stay refused): KEY=VALUE,\n" +
				"  export KEY=VALUE, quoted values (also on several lines), # comments;\n" +
				"  ${VAR} is not expanded; a key set twice keeps its last value.",
		},
		ShellVars: []string{"HOME"},
		Env:       []string{"GASH_HEADLESS", "GASH_NO_COLOR", "HOME", "NO_COLOR", "PWD"},
		Replaces:  "grep -E '^(DB_HOST|APP_URL)=' .env",
	},
	{
		Name:     "llm_date",
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
			Short:   "Portable date arithmetic: differences, additions, Unix times (replaces date -d, python datetime)",
			Module:  "llm",
			SeeAlso: "llm_env",
			Body: "" +
				"USAGE\n" +
				"  llm_date [--diff A B | --add DATE DURATION | --epoch X] [--tz ZONE]\n" +
				"           [--fmt FORMAT]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --diff A B             B - A: {\"result\":\"67d\",\"days\":67,\"seconds\":N}\n" +
				"  --add DATE DURATION    DATE + DURATION (y, mo, w, d, h, m, s, ms: 3d,\n" +
				"                         -600s, 1h30m, 1y2mo)\n" +
				"  --epoch X              A Unix time as a date (13 digits: milliseconds),\n" +
				"                         or a date as a Unix time\n" +
				"  --tz ZONE              Europe/Rome, UTC, +02:00 (default: TZ, else the\n" +
				"                         system zone)\n" +
				"  --fmt FORMAT           rfc3339 (default), date, time, epoch, rfc2822, or\n" +
				"                         a date(1) format: '%Y-%m-%d %H:%M:%S'\n" +
				"\n" +
				"EXAMPLES\n" +
				"  llm_date --diff 2026-10-25 2026-12-31\n" +
				"  llm_date --add now -600s --fmt '%Y-%m-%dT%H:%M:%S'\n" +
				"  llm_date --add 2026-01-31 1mo\n" +
				"  llm_date --epoch 1700000000 --tz UTC\n" +
				"  llm_date --tz America/New_York\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces date -d '-600 seconds' +FMT (GNU and BSD date differ) and\n" +
				"  python3 -c \"from datetime import date; print((date(...)-date(...)).days)\".\n" +
				"  Output: {\"result\":\"...\",\"epoch\":N}; without a mode, now. A date is\n" +
				"  now, today, yesterday, tomorrow, YYYY-MM-DD, YYYY-MM-DD[T ]HH:MM[:SS]\n" +
				"  [.frac][Z|+HH:MM], an RFC 2822 date or @EPOCH; without an offset it is\n" +
				"  in --tz. The result keeps the input's offset unless --tz is given; a\n" +
				"  date plus calendar units stays a date. Months clamp to the month's\n" +
				"  last day (Jan 31 + 1mo = Feb 28); days follow the calendar across DST.\n" +
				"  --diff of two dates counts calendar days; days truncates toward zero.\n" +
				"  Bad input is invalid_date, invalid_duration, invalid_timezone or\n" +
				"  invalid_date_format (all RETRY). The same output on Linux and macOS.",
		},
		ShellVars: []string{"HOME"},
		Env:       []string{"GASH_HEADLESS", "GASH_NO_COLOR", "HOME", "NO_COLOR", "PWD", "TZ"},
		Replaces:  "date -d '-600 seconds' '+%Y-%m-%dT%H:%M:%S'; python3 -c \"from datetime import date; print((date(2026,12,31)-date(2026,10,25)).days)\"",
	},
}
