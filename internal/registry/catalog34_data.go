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
}
