# Databases

The MySQL, MariaDB, PostgreSQL and SQLite connections of ~/.gash_env: list, test and query them read-only.

You name each database connection once, in `~/.gash_env`, then list the
connections, test one, and read from it with the `llm_db_*` functions. Gash
talks to MySQL, MariaDB and PostgreSQL through drivers built into its binary,
so no `mysql` or `psql` client is needed, and it opens SQLite files directly.
Every query is read-only: Gash refuses a statement that could write, and runs
the others in a read-only session.

Platforms: Linux, macOS and WSL. A connection to a database on a remote host
(a `+ssh` URL) runs that engine's client on the host, over `ssh`.

<!-- BEGIN generated: prerequisites (go test ./internal/docgen -update) -->
Nothing beyond what Gash itself needs ([requirements](../requirements.md)).
<!-- END generated: prerequisites -->

Every function, alias and command of this area, with all its options: [reference.md](reference.md).

## Add a connection

Add one `DB:` line per connection to `~/.gash_env` (`gash_env_init` creates
the file from a template):

```text
DB:default=mysql://app:s3cret@localhost:3306/shop
DB:reports=pgsql://report:p%40ss@db.example.com/analytics
```

The format is `DB:name=driver://user:password@host:port/database`:

- The driver is `mysql`, `mariadb` or `pgsql`. A SQLite database needs no
  line: you pass its file with `-f` (below).
- The port is optional: 3306 for MySQL and MariaDB, 5432 for PostgreSQL.
- The database is optional. When the URL has none, the `llm_db_*` functions
  take it from `-d NAME`, else from the first word of a `.target-database`
  file in the current directory.
- Only the password is percent-decoded: write a `%` in it as `%25`; `@` may be
  written as is or as `%40`.
- An IPv6 host goes in brackets: `[::1]:5432`.
- For MySQL and MariaDB, the host `localhost` means the local Unix socket (the
  one the `mysql` client would use); write `127.0.0.1` to connect over TCP.
- The connection named `default` is the one used when you name none.

The full grammar is in [configuration](../configuration.md#databases). Keep
the file private (`chmod 600 ~/.gash_env`): it holds passwords, and Gash
warns at each read when other users can open it.

[`gash_db_list`](reference.md#gash_db_list) shows the connections in effect,
with the passwords masked:

<!-- example
printf 'DB:default=mysql://app:s3cret@localhost:3306/shop\nDB:reports=pgsql://report:p%%40ss@db.example.com/analytics\n' >.gash_env
chmod 600 .gash_env
-->
```console
$ gash_db_list
Info: Available database connections:

  default         mysql://app:****@localhost:3306/shop
  reports         pgsql://report:****@db.example.com/analytics
```

## Test a connection

[`gash_db_test`](reference.md#gash_db_test) connects and runs `SELECT 1`, on
the connection you name or on `default`:

```bash
gash_db_test            # the default connection
gash_db_test reports
```

It prints `Info: Testing connection 'reports' (pgsql://report@db.example.com:5432/analytics)...`
(the password left out), then `OK: Connection successful!` and status 0, or
`Error: Connection failed` with the driver's own message on a `Hint:` line
and status 1. It gives up after `GASH_DB_TIMEOUT` seconds (30 by default).

## Query a database, read-only

Six functions read a database. They are built for AI agents (one JSON answer
per call), and they work the same for you at the prompt:

| To | Use |
|---|---|
| List the databases of a server | [`llm_db_databases`](../agents/reference.md#llm_db_databases) |
| List the tables, with `--counts` their row counts | [`llm_db_tables`](../agents/reference.md#llm_db_tables) |
| Show the columns of a table, or of all with `--all` | [`llm_db_schema`](../agents/reference.md#llm_db_schema) |
| See a few rows of a table | [`llm_db_sample`](../agents/reference.md#llm_db_sample) |
| Run a query | [`llm_db_query`](../agents/reference.md#llm_db_query) |
| See how a query runs (`EXPLAIN`, `--analyze`) | [`llm_db_explain`](../agents/reference.md#llm_db_explain) |

Pick the database with `-c NAME` (a `DB:` connection; `default` when you
leave it out) or `-f FILE` (a SQLite file), and override the database name
with `-d NAME`. The answer is a JSON object by default; `--format table` or
`--format tsv` prints the rows for a person (the timing goes to stderr as one
JSON line), and `--scalar` prints the single value of a one-row, one-column
result. `llm_db_query` returns at most 100 rows unless `-n N` says otherwise.

<!-- example
sqlite3 shop.db "CREATE TABLE customers(id INTEGER PRIMARY KEY, email TEXT NOT NULL); CREATE TABLE orders(id INTEGER PRIMARY KEY, customer_id INTEGER NOT NULL, amount INTEGER NOT NULL, status TEXT NOT NULL); INSERT INTO customers VALUES (1,'ada@example.com'),(2,'linus@example.com'); INSERT INTO orders VALUES (1,1,1990,'paid'),(2,1,500,'pending'),(3,2,4200,'paid');"
mask: "query_time":"[^"]*"
-->
```console
$ llm_db_tables -f shop.db --counts --format table
name       rows
customers  2
orders     3
{"query_time":"787µs","rows":2}
$ llm_db_query "SELECT c.email, count(*) AS orders, sum(o.amount) AS spent FROM orders o JOIN customers c ON c.id = o.customer_id GROUP BY c.email ORDER BY spent DESC" -f shop.db --format table
email              orders  spent
linus@example.com  1       4200
ada@example.com    2       2490
{"query_time":"246µs","rows":2}
$ llm_db_query "SELECT count(*) FROM orders WHERE status = 'pending'" -f shop.db --scalar
1
```

Against a server the call is the same with `-c`:

```bash
llm_db_tables -c reports --format table
llm_db_query "SELECT id, email FROM users ORDER BY id DESC" -c default -n 20 --format table
llm_db_schema users -c default --format table
```

## What read-only means

Every `llm_db_*` query passes two checks:

1. **The SQL guard.** The query must be a single statement that starts
   with a reading keyword (`SELECT`, `WITH`, `SHOW`, `EXPLAIN` and a few
   others, listed in the [`llm_db_query`](../agents/reference.md#llm_db_query)
   notes), with no write keyword (such as `INSERT`, `UPDATE` or `SET`) and
   no function that reads or writes files or runs programs. The guard reads
   strings and comments the way the engine does, so quoting tricks cannot
   hide a second statement or a write. A refused query never reaches the
   database.
2. **A read-only session.** SQLite files open read-only, in defensive mode,
   with `ATTACH` disabled; MySQL and MariaDB run the query in a
   `READ ONLY` transaction; PostgreSQL sessions have
   `default_transaction_read_only` on and a statement timeout.

A refusal is a JSON error with status 1:

<!-- example
sqlite3 shop.db "CREATE TABLE orders(id INTEGER PRIMARY KEY, status TEXT NOT NULL); INSERT INTO orders VALUES (1,'pending');"
mask: "query_time":"[^"]*"
-->
```console
$ llm_db_query "UPDATE orders SET status = 'paid'" -f shop.db
{"error":"write_operation_blocked","details":"Statement starts with 'UPDATE'","action":"FATAL","recoverable":false,"hint":"Only SELECT, WITH, VALUES, TABLE, SHOW, DESCRIBE, EXPLAIN and PRAGMA are allowed","query_time":"437µs"}
$ llm_db_query "SELECT 1; DROP TABLE orders" -f shop.db
{"error":"multiple_statements_blocked","details":"Only single SQL statements allowed","action":"FATAL","recoverable":false,"hint":"Send one statement per call (a trailing semicolon is fine)","query_time":"27µs"}
```

The guard is not a substitute for database permissions: for a production
database, give Gash a user that can only read. [security](../security.md)
describes what the guard covers and what it does not.

## Reach a database on a remote host

A scheme that ends in `+ssh` names a database on a host of the
[remote hosts](../remote/README.md) area. The `@` part is then a `HOST:`
alias, not a network address:

```text
HOST:web=deploy@web.example.com
DB:live=mysql+ssh://app:s3cret@web/shop
DB:pglive=pgsql+ssh://report:s3cret@web:5432/analytics
DB:cache=sqlite+ssh://web//var/lib/app/cache.db
DB:dock=mysql+ssh://app:s3cret@web/shop+docker:mysql
```

The `llm_db_*` functions then run the engine's own client on the host
(`mysql`, `psql` 12 or later, `sqlite3`), one `ssh` round trip per query,
after the same SQL guard, in the same read-only session. The client connects
to the server local to that host: its socket, or `127.0.0.1:PORT` when the
URL names a port. The password and the query travel on the standard input
of `ssh`, never on a command line. `sqlite+ssh://web/data/app.db` is
relative to the remote home (`~/data/app.db`), `sqlite+ssh://web//var/...`
is absolute. A `+docker:NAME` suffix runs the client inside that container
on the host. The client must be installed where it runs. [configuration](../configuration.md#databases) has
the full syntax.

`gash_db_test` does not test these connections (it says so, and names the
query to run instead):

```bash
llm_db_query "SELECT 1" -c live --scalar
```

## Timeouts

`GASH_DB_TIMEOUT` (seconds, default 30, set in `~/.gash_env` or the
environment) is:

- for `gash_db_test`, how long the whole test may take, on every engine;
- for the `llm_db_*` functions on PostgreSQL, the statement timeout of the
  session.

Gash sets no time limit on the MySQL, MariaDB and SQLite queries of
`llm_db_*`.

## When something goes wrong

| Message | What it means and what to do |
|---|---|
| `Error: Connection 'NAME' not found in ~/.gash_env` | No `DB:NAME=` line is in effect. `gash_db_list` shows the names; without a name, the test uses `default`. |
| `Warning: Line N of ~/.gash_env: DB URL must start with mysql://, mariadb://, pgsql:// or a +ssh form (mysql+ssh://, mariadb+ssh://, pgsql+ssh://, sqlite+ssh://) (skipping)` | The scheme is not one Gash knows. A local SQLite file has no `DB:` line: use `-f FILE`. |
| `Warning: Line N of ~/.gash_env: duplicate DB:NAME, ignored (first is at line M)` | Two lines share a name; the first one is used. |
| `Error: Connection 'NAME' has an invalid URL` | The `Hint:` line says why: an IPv6 host without its closing `]`, or a password holding `%00`. Fix the line. |
| `Error: Connection failed` and a `Hint:` | The driver's message: `connection refused` (no server on that host and port), `Access denied` or `password authentication failed` (user or password; check the percent-encoding), `no such file or directory` for `localhost` (no MySQL socket: try `127.0.0.1`). |
| `Error: gash_db_test tests mysql, mariadb and pgsql connections from this machine; 'NAME' is a mysql+ssh connection` | `gash_db_test` cannot test a `sqlite`, `+ssh` or `+docker` connection; use `llm_db_query "SELECT 1" -c NAME --scalar`. |
| `{"error":"no_db_config",...}` | An `llm_db_*` call named a connection (or `default`) that is not in the file. |
| `{"error":"no_database",...}` | The connection has no database: add it to the URL, pass `-d NAME`, or write it in `.target-database`. |
| `{"error":"write_operation_blocked",...}`, `multiple_statements_blocked` and the other `_blocked` codes | The SQL guard refused the statement. Send one read-only statement per call. |
| `{"error":"mysql_error",...}`, `pgsql_error`, `sqlite_error` | The database refused the query or the connection; `details` holds its message. |
| `{"error":"sqlite_file_not_found",...}` | The `-f` path does not exist. |

The JSON errors follow the error envelope of [AGENTS.md](../../AGENTS.md):
`error` (the code), `details`, `action`, `hint`.

## See also

- [reference.md](reference.md): `gash_db_list` and `gash_db_test` with all their options.
- [The `llm_db_*` reference](../agents/reference.md#llm_db_query): every option of the query functions.
- [configuration](../configuration.md#databases): the `DB:` grammar, and [`GASH_DB_TIMEOUT`](../configuration.md#settings-reference).
- [Remote hosts](../remote/README.md): the `HOST:` aliases that `+ssh` connections use.
- [security](../security.md): the SQL guard in detail.
