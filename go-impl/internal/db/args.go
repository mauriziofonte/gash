// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"os"
	"regexp"
	"strings"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/pathguard"
)

// dbArgs are the locals __llm_db_parse_args fills.
type dbArgs struct {
	positional, database, connection, sqliteFile, rows string
	analyze                                            bool
}

// parseArgs is __llm_db_parse_args "EXTRA" ARGS: -d/-c/-f plus the flags
// in extra; one positional (a second one is refused only while the first
// is non-empty, as 2.x tested -n "$positional").
func parseArgs(args []string, extra string, a *dbArgs) *contract.Envelope {
	allowed := " -d --database -c --connection -f --file " + extra + " "
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			if !strings.Contains(allowed, " "+arg+" ") {
				e := contract.New("unknown_option", arg, "Valid options:"+allowed)
				return &e
			}
			if arg == "--analyze" {
				a.analyze = true
				continue
			}
			if i+1 >= len(args) {
				e := contract.New("missing_value", arg, arg+" needs a value")
				return &e
			}
			v := args[i+1]
			switch arg {
			case "-d", "--database":
				a.database = v
			case "-c", "--connection":
				a.connection = v
			case "-f", "--file":
				a.sqliteFile = v
			case "-r", "--rows", "-n", "--limit":
				a.rows = v
			}
			i++
			continue
		}
		if a.positional != "" {
			e := contract.New("unexpected_argument", arg, "Quote the whole query as one argument")
			return &e
		}
		a.positional = arg
	}
	return nil
}

var (
	uintRE   = regexp.MustCompile(`^[0-9]{1,9}$`)
	dbNameRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_$.-]{0,63}$`)
	tableRE  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// validUint is __llm_validate_uint NAME VALUE.
func validUint(name, value string) *contract.Envelope {
	if uintRE.MatchString(value) {
		return nil
	}
	e := contract.New("invalid_"+name, "Value '"+value+"' is not a positive integer below 1000000000",
		"Provide a plain integer for "+name)
	return &e
}

// validDBName is __llm_validate_db_name NAME SOURCE.
func validDBName(name, source string) *contract.Envelope {
	if dbNameRE.MatchString(name) {
		return nil
	}
	e := contract.New("invalid_database_name", source+": '"+name+"'",
		"A database name holds letters, digits, _ $ . and - (64 at most, not starting with -); pass a valid -d NAME")
	return &e
}

// checkTable is __llm_db_check_table.
func checkTable(name string) *contract.Envelope {
	if name == "" {
		e := contract.New("missing_table", "", "Provide table name as first argument")
		return &e
	}
	if !tableRE.MatchString(name) {
		e := contract.New("invalid_table_name", "Table name must be alphanumeric with underscores",
			"Use only letters, numbers, and underscores")
		return &e
	}
	return nil
}

// target is a resolved llm_db_* target: an SQLite file or a connection.
type target struct {
	driver                               string // sqlite, mysql, mariadb or pgsql
	file                                 string
	user, pass, host, port, database string
}

// target is __llm_db_target: the SQLite file of -f, or the named
// connection of ~/.gash_env with its database (-d, the URL, then the
// first word of ./.target-database).
func (m *Module) target(c *invoke.Call, a *dbArgs) (target, *output) {
	if a.sqliteFile != "" {
		path, fail := m.sqliteFile(c, a.sqliteFile)
		if fail != nil {
			return target{}, fail
		}
		return target{driver: "sqlite", file: path}, nil
	}
	f := config.LoadCaller(c.Env.Get)
	url, ok := f.DBURL(a.connection)
	if !ok {
		o := failed(contract.New("no_db_config", "Connection '"+a.connection+"' not found",
			"Ask user which database connection to use. Connections are configured in ~/.gash_env"))
		return target{}, &o
	}
	u := config.ParseDBURL(url)
	t := target{driver: u.Driver, user: u.User, pass: config.DecodePassword(u.Password), host: u.Host, port: u.Port, database: u.Database}
	switch {
	case a.database != "":
		if e := validDBName(a.database, "-d"); e != nil {
			o := failed(*e)
			return target{}, &o
		}
		t.database = a.database
	case t.database == "":
		if name := targetDatabase(); name != "" {
			if e := validDBName(name, ".target-database"); e != nil {
				o := failed(*e)
				return target{}, &o
			}
			t.database = name
		}
	}
	if t.database == "" {
		o := failed(contract.New("no_database", "Database name not specified",
			"Ask user for database name. Use -d option or specify in connection URL"))
		return target{}, &o
	}
	switch t.driver {
	case "mysql", "mariadb", "pgsql":
	default:
		o := failed(contract.New("unknown_db_type", t.driver,
			"Unsupported database driver. Only mysql, mariadb, pgsql are supported"))
		return target{}, &o
	}
	return t, nil
}

// targetDatabase is the first word of ./.target-database ("" when the
// file is not a regular file or holds only blanks). NUL bytes are dropped,
// as $(<file) dropped them.
func targetDatabase() string {
	st, err := os.Stat(".target-database")
	if err != nil || !st.Mode().IsRegular() {
		return ""
	}
	data, err := os.ReadFile(".target-database")
	if err != nil {
		return ""
	}
	s := strings.ReplaceAll(string(data), "\x00", "")
	s = strings.TrimLeft(s, " \t\n\v\f\r")
	if i := strings.IndexAny(s, " \t\n\v\f\r"); i >= 0 {
		s = s[:i]
	}
	return s
}

// sqliteFile is __llm_sqlite_file: the path guard, a regular file, the
// "SQLite format 3" header. 2.x first required the sqlite3 client; the
// native driver needs none.
func (m *Module) sqliteFile(c *invoke.Call, file string) (string, *output) {
	cwd, err := m.getwd()
	if err != nil {
		cwd = c.Env.Get("PWD")
	}
	safe, err := pathguard.NewGuard(pathguard.OSFS{}, cwd, c.Env.Get("HOME"), m.rootHome()).ValidatePath(file)
	if err != nil {
		o := refusal(err)
		return "", &o
	}
	if st, err := os.Stat(safe); err != nil || !st.Mode().IsRegular() {
		o := failed(contract.New("sqlite_file_not_found", safe, "SQLite file does not exist. Check the path"))
		return "", &o
	}
	if readMagic(safe) != "SQLite format 3" {
		o := failed(contract.New("invalid_sqlite_file", safe+" is not a valid SQLite database",
			"Provide a valid SQLite database file"))
		return "", &o
	}
	return safe, nil
}

// readMagic is `IFS= read -r -n 15 magic <FILE`: up to 15 characters,
// stopping at a newline, NUL bytes skipped.
func readMagic(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, 512)
	var out []byte
	for len(out) < 15 {
		n, err := f.Read(buf)
		for _, b := range buf[:n] {
			if b == '\n' {
				return string(out)
			}
			if b != 0 {
				out = append(out, b)
				if len(out) == 15 {
					return string(out)
				}
			}
		}
		if err != nil {
			break
		}
	}
	return string(out)
}
