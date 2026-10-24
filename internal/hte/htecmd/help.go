// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package htecmd

import "github.com/spf13/cobra"

// helpLong is the --help text of each visible command of gash hte, by
// command path, printed above the usage and the flags (F-02). It names
// only flags the command has (TestFollowup_F02 checks that).
var helpLong = map[string]string{
	"hte": `gash hte runs a userland Apache and PHP-FPM for local sites: one sudo at
install for the few system steps, everything else as you. The Apache and
PHP-FPM binaries are the ones the system already has.

Every command takes --json (one JSON object on stdout). Errors are the Gash
error envelope on stderr with exit status 1. Without a terminal nothing
prompts: a missing required flag is missing_value, and a destructive step
without --yes is confirmation_required.

Start with: gash hte install, then gash hte migrate (a dry run of what
would move from hte-cli), then gash hte site create. The guide is
docs/hte.md; WSL notes are in its "WSL" section.`,

	"hte install": `Install the stack. install prints its plan, then runs the system steps in
one interactive sudo: the hosts helper and its sudoers line, the low ports
(--ports standard), linger for the systemd unit, and stopping the system
Apache when the plan needs it. Then, as you, it writes the userland tree,
starts the stack and sets up autostart. Running it again changes only what
is missing.

--ports standard binds 80 and 443 (on Linux by lowering
net.ipv4.ip_unprivileged_port_start, which lets every local user bind ports
80 to 1023; on macOS directly or through a pf redirect); --ports high uses
8080 and 8443 and needs no port step.
--autostart auto picks launchd on macOS, the systemd user unit where
systemd runs, else none; shell starts the stack from the first interactive
shell that loads gash.
--yes allows stopping the system Apache when the plan does that; while an
enabled system vhost is neither migrated nor excluded, install leaves the
stack stopped and names gash hte migrate.
An uninstall stage is restored when one is found: --restore-stage PATH
names it, --no-restore skips it.`,

	"hte uninstall": `Stop the stack, remove autostart, and restore what install changed, as
its receipt records it (the hosts block, the port setting, linger, the
helper and its sudoers line). Your sites, their certificates and settings
move to an hte-stage-<timestamp> directory that a later install restores.
Document roots are never touched. Needs --yes.`,

	"hte doctor": `Check the machine and the stack: binaries and modules, ports and who holds
them, autostart, the configuration test, every site (HTTP and HTTPS
answers, certificate validity and names), name resolution, the receipt and
the PHP versions. Each failure comes with its remedy. A hosts block that
drifted is written again through the helper. On WSL it also checks that
Windows resolves the site names (see gash hte hosts windows).
Exits 1 when a check fails.`,

	"hte migrate": `Move the hte-cli and other system Apache vhosts to the stack. Without
--apply it prints the plan (a dry run); --apply runs it and switches the
sites over, and --yes allows stopping the system Apache and archiving the
old vhost files.

--site D migrates only that domain (repeatable); --exclude D or PATH leaves
a domain or a vhost file out (repeatable). --keep-pool-values keeps the old
PHP-FPM pool's PHP values. Certificates are copied; --reissue-certs makes
new ones instead. Each old site keeps its forced-HTTPS behavior.`,

	"hte site": `Create, change and remove sites. A site is known by its domain; its state,
rendered configuration, PHP-FPM pool, certificate and hosts names follow
from it. Hand edits go in the site's overlay file, which gash never
rewrites.`,

	"hte site create": `Create a site. --domain and --docroot are required (on a terminal a missing
one is asked for); a relative document root is resolved against the current
directory. A flag left out gets its default: HTTPS on, forced HTTPS off,
the loopback bypass off, HTTP/2 on, the dev profile, a www. alias and the
default PHP version of hte.toml (else the newest installed). A value that
is not valid is an error, never replaced by a default.

Each on/off setting has both forms (--https/--no-https, --force-https,
--force-https-bypass, --http2); passing both is conflicting_options.
--force-https-bypass lets plain HTTP through for requests from 127.0.0.1
and ::1. --alias adds a name (repeatable); --extra-http-port and
--extra-https-port add loopback ports to the vhosts (repeatable).`,

	"hte site modify": `Change a site: only the flags you pass change, the others keep their
values. The flags are those of site create (--docroot is optional here).
Changing the aliases makes a new certificate; --no-https keeps the
certificate so that --https later reuses it. The site's overlay file stays
untouched.`,

	"hte site remove": `Remove a site: its state, rendered configuration, PHP-FPM pool,
certificate and hosts names. The document root, the site's overlay file
and its logs are never touched. Needs --yes.`,

	"hte site list": `List the sites: domain, PHP version, HTTPS (on, off or forced), whether
they are enabled, and the document root.`,

	"hte site show": `Show one site: its settings, aliases, extra ports, php.ini values and
certificate.`,

	"hte site enable": `Serve a disabled site again, with the settings it had.`,

	"hte site disable": `Stop serving a site. Its state, certificate and overlay file stay, and
site enable serves it again.`,

	"hte php": `PHP versions and php.ini values: per site (ini, rendered into the site's
pool) and per PHP version (global, an overlay for every site of that
version).`,

	"hte php versions": `List the PHP-FPM versions found on the system, with their release, FPM
binary and php.ini.`,

	"hte php ini": `The php.ini values of one site (--domain), kept in its state and
rendered into its PHP-FPM pool. A change reloads the stack.`,

	"hte php ini get":   `Print one php.ini value of the site; set is false when it is not set.`,
	"hte php ini set":   `Set one php.ini value of the site, then reload.`,
	"hte php ini unset": `Remove one php.ini value of the site, then reload.`,

	"hte php global": `The global php.ini overlay of one PHP version (--version X.Y), read by
every site of that version. A change reloads that version's master; a
master that does not come back gets the previous file.`,

	"hte php global get":   `Print one value of the version's overlay; set is false when it is not set.`,
	"hte php global set":   `Set one value of the version's overlay, then reload that master.`,
	"hte php global unset": `Remove one value of the version's overlay, then reload that master.`,

	"hte apache": `The global Apache overlay and the extra modules.`,

	"hte apache config": `The global Apache overlay: a file of yours that gash includes and never
rewrites. Run gash hte service reload after editing it.`,

	"hte apache config path": `Print the path of the global Apache overlay file.`,

	"hte apache modules": `The Apache modules the stack loads: the base set gash needs, plus extra
modules you enable from the system module directory.`,

	"hte apache modules list": `List the base, enabled and available modules.`,

	"hte apache modules enable": `Load one more module (NAME, as in the system module directory) and reload;
when the new configuration fails, the change is rolled back. A module the
directory lacks is apache_module_missing.`,

	"hte apache modules disable": `Stop loading an extra module you enabled, then reload. A module of the
base set cannot be disabled.`,

	"hte service": `Start, stop and inspect the stack (the Apache and PHP-FPM masters). With
the systemd user unit, start, stop and restart go through the unit.`,

	"hte service start": `Start the stack. The hosts block is written again first when it drifted
(WSL rewrites /etc/hosts at boot).`,

	"hte service stop":    `Stop the stack.`,
	"hte service restart": `Stop and start the stack.`,

	"hte service reload": `Render the configuration again, test it, and reload gracefully. When the
test or the reload fails, the previous configuration comes back.`,

	"hte service status": `Show the masters and whether each site answers.`,

	"hte service logs": `Print the last lines of the logs: the global Apache error log and every
PHP-FPM log, or with --site D that site's access, error and PHP error logs.
--lines N sets the lines per log. There is no follow mode.`,

	"hte cert": `The sites' self-signed certificates (ECDSA P-256, valid for 10 years,
for the domain, *.domain and every alias).`,

	"hte cert show": `Show a site's certificate: file, subject, names, validity, algorithm,
serial and SHA-256 fingerprint.`,

	"hte cert renew": `Make a new certificate for a site, then reload. This also replaces a pair
that migrate copied from hte-cli.`,

	"hte hosts": `The managed block of the hosts file: the names of the enabled sites
between "# >>> gash hte >>>" and "# <<< gash hte <<<", written through the
hosts helper. Lines outside the block are never changed.`,

	"hte hosts list":  `List the entries of the managed block.`,
	"hte hosts check": `Compare the block with the sites. Exits 1 when they differ.`,
	"hte hosts apply": `Write the block for the enabled sites. Every site change, service start
and doctor do this too.`,

	"hte hosts windows": `On WSL, print the lines to add to C:\Windows\System32\drivers\etc\hosts
for the site names Windows cannot resolve (asked through powershell.exe
when interop is available, else every name). gash never edits the Windows
file. Elsewhere this is platform_unsupported. See the "WSL" section of
docs/hte.md.`,
}

// setHelp gives every visible command of the tree its long help.
func setHelp(c *cobra.Command) {
	if c.Hidden {
		return
	}
	if long, ok := helpLong[c.CommandPath()]; ok {
		c.Long = long
	}
	for _, s := range c.Commands() {
		setHelp(s)
	}
}
