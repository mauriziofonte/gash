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
				"           [--width N] [--head N] [--tail N]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --path P               The value at P: a.b[0].c, paths[\"/api/x\"].post, [-1]\n" +
				"  --keys                 The keys of an object (or of the objects of an array)\n" +
				"  --shape                Types and lengths, two levels deep\n" +
				"  --format F             json (default), jsonl (one element per line), tsv or\n" +
				"                         table (an array of objects as rows)\n" +
				"  --width N              Cut each string value at N columns (ends with …)\n" +
				"  --head N, --tail N     Keep the first / last N elements of an array\n" +
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
				"          [--format json|jsonl|tsv|table]\n" +
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
}
