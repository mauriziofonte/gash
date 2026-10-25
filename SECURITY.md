# Security policy

## Supported versions

| Version | Supported |
|---|---|
| 3.x | Yes: fixes ship in the next 3.x release |
| 2.1.x | Critical and high security fixes only, as 2.1.x patch releases, until 2027-04-30. Upgrade with `gash_upgrade` |
| 2.0.x and older | No: upgrade with `gash_upgrade` (1.x: through 2.1.0, see [docs/migration.md](docs/migration.md)) |

## Reporting a vulnerability

Report vulnerabilities privately through
[GitHub security advisories](https://github.com/mauriziofonte/gash/security/advisories/new),
not in a public issue. Include:

- the output of `gash version` (or `echo $GASH_VERSION` on 2.x), the Bash
  version and the OS;
- the function or `gash hte` command involved;
- the smallest input that shows the problem, and what it lets you do.

We aim to acknowledge a report within a week and to ship the fix of a
critical or high issue within 30 days of the report. Once a fix is ready we agree
on a disclosure date with you, publish the release and credit you in the
advisory unless you prefer otherwise. Until then, the changelog and the
public issue tracker describe the fix without a working exploit.

## Scope

In scope:

- **A way around a guardrail**: the `llm_exec` allowlist and its parser,
  the read-only `llm_db_*` sessions and SQL guard, the secret-file and
  forbidden-path checks, the `remote_*` deny lists, the redaction before an
  AI request, the confirmations of commands that change things.
- **Code execution from data**: a file name, branch name, archive member,
  query result, model answer or configuration value that ends up run as
  code or as an option.
- **Secrets leaking**: a `~/.gash_env` value, an SSH passphrase, a database
  password or an API key that reaches a log, an argument list, another
  process or the network unasked.
- **Install and upgrade**: anything that makes `install.sh`, `gash update`
  or `gash_upgrade` install a binary or a tree that the release tag does not
  pin, or skip a signature check that `GASH_REQUIRE_SIGNED=1` asks for;
  a release asset whose Sigstore signature or build provenance verifies
  although the release workflow did not build it.
- **`gash hte`**: the root-owned `gash-hosts-helper` and its sudoers line
  (anything that writes outside the gash block of `/etc/hosts`, or puts a
  non-loopback address or an unvalidated name in it), the install and
  uninstall steps that run as root, and the sites' certificates, keys and
  configuration files (permissions, other users' access).

Out of scope:

- The guardrails are defence in depth for a person or an agent working in
  their own account, not a sandbox against a hostile user of that account:
  someone who can already run arbitrary commands as you is not stopped by
  them.
- What `gash hte install` documents and asks for:
  `net.ipv4.ip_unprivileged_port_start = 80` lets every local user bind
  ports 80 to 1023 while HTE is installed (`--ports high` avoids it).
- Vulnerabilities in Apache, PHP, OpenSSL, Git, Docker or other programs
  Gash drives, unless Gash calls them unsafely.

[docs/security.md](docs/security.md) describes the threat model and every
guardrail in detail, and how to verify a release.
