# Security policy

## Supported versions

| Version | Supported |
|---|---|
| 2.x | Yes |
| 1.x | No: upgrade with `gash_upgrade` |

## Reporting a vulnerability

Please report vulnerabilities privately through
[GitHub security advisories](https://github.com/mauriziofonte/gash/security/advisories/new),
not in a public issue. Include the Gash version (`echo $GASH_VERSION`), the
Bash version and OS, the function involved and the smallest input that shows
the problem.

We aim to acknowledge reports within a week. Once a fix is ready we agree
on a disclosure date, publish a patch release and credit you in the advisory
unless you prefer otherwise.

## Scope

Gash's guardrails (`llm_exec` allowlist, read-only `llm_db_*` sessions,
secret-file and forbidden-path checks, `remote_*` deny lists, AI redaction)
are defence in depth for an agent or a person working in their own account,
not a sandbox against a hostile user of that account. A way around any of
them is in scope; so is anything that leaks secrets or runs code an input
did not ask for. [docs/security.md](docs/security.md) describes the threat
model and what is out of scope.
