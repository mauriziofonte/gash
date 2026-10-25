// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sysinfo

import (
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/runner"
)

// The drill-down collectors of ai_sysinfo (__sysinfo_deep_*): whole
// configs and live state, written at once (no lazy headers), with the
// session's sudo for the reads 2.x ran through __sysinfo_sudo.

// DeepSections are the drill-down collectors, in the order of the
// ai_sysinfo menu (1-6).
var DeepSections = []string{"services", "security", "network", "storage", "performance", "maintenance"}

// Deep writes the collector __sysinfo_deep_NAME to w; false for a name
// that is not one of DeepSections.
func (s *Session) Deep(w io.Writer, name string) bool {
	d := deepOut{w}
	switch name {
	case "services":
		s.deepServices(d)
	case "security":
		s.deepSecurity(d)
	case "network":
		s.deepNetwork(d)
	case "storage":
		s.deepStorage(d)
	case "performance":
		s.deepPerformance(d)
	case "maintenance":
		s.deepMaintenance(d)
	default:
		return false
	}
	return true
}

// Sudo is `__sysinfo_sudo NAME ARGS 2>/dev/null`: the program through
// sudo when the session is elevated, as the user otherwise; its stdout.
// Paths in ARGS are the caller's (system paths outside tests).
func (s *Session) Sudo(name string, args ...string) []byte {
	if s.elevated() {
		out, _ := s.tool("sudo", append([]string{name}, args...)...)
		return out
	}
	out, _ := s.tool(name, args...)
	return out
}

// deepOut writes what the collectors echo and cat.
type deepOut struct{ w io.Writer }

// echo is `echo "S"`.
func (d deepOut) echo(s string) { _, _ = io.WriteString(d.w, s+"\n") }

// lines echoes each line (a grep, ls or awk output).
func (d deepOut) lines(ls []string) {
	for _, l := range ls {
		d.echo(l)
	}
}

// raw is a tool's or cat's bytes, ended by a newline when the last line
// has none, so the next header starts on a line of its own (SYS-52).
func (d deepOut) raw(b []byte) {
	_, _ = d.w.Write(b)
	if len(b) > 0 && b[len(b)-1] != '\n' {
		_, _ = io.WriteString(d.w, "\n")
	}
}

var (
	reNoteBlankIndent = lazyre.MustCompile(`^(#|$|[[:space:]]*#)`)
	reSemiBlank       = lazyre.MustCompile(`^(;|$)`)
	rePHPIni          = lazyre.MustCompile(`^(memory_limit|upload_max|post_max|max_execution|max_input|date.timezone|error_reporting|display_errors|opcache)`)
	reWebminKeys      = lazyre.MustCompile(`^(port|ssl|listen|session)`)
	reTimesyncd       = lazyre.MustCompile(`^(#|$|\[)`)
	// Both WireGuard key kinds, any case and spacing (SYS-50).
	reWireGuardKey = lazyre.MustCompile(`(?i)^([[:space:]]*(?:PrivateKey|PresharedKey)[[:space:]]*=[[:space:]]*).*`)
	// grep -oP '^EnvironmentFile=-?\K.*' and friends: the match is group 1.
	reEnvFile = lazyre.MustCompile(`^EnvironmentFile=-?(.*)`)
	reExec    = lazyre.MustCompile(`^ExecStart=(.*)`)
	reWorkDir = lazyre.MustCompile(`^WorkingDirectory=(.*)`)
	// grep -oE: POSIX leftmost-longest.
	reCfgPath    = lazyre.MustCompilePOSIX(`(/etc/[^ ]+|/opt/[^ ]+\.(conf|yaml|yml|json|ini|cfg))`)
	reLoginDefs  = lazyre.MustCompile(`^(PASS_MAX_DAYS|PASS_MIN_DAYS|PASS_MIN_LEN|PASS_WARN_AGE|LOGIN_RETRIES|ENCRYPT_METHOD|UMASK)`)
	reCgroup     = lazyre.MustCompile(`cgroup`)
	reCPUInfo    = lazyre.MustCompile(`(?i)model name|cpu\(s\)|thread|core|socket|mhz`)
	rePHPVersion = lazyre.MustCompile(`/etc/php/([^/]+)`)
	reSysctlKeys = lazyre.MustCompile(`^(vm\.swappiness|vm\.dirty_ratio|vm\.dirty_background|net\.core\.somaxconn|net\.core\.netdev_max_backlog|net\.ipv4\.tcp_max_syn|net\.ipv4\.tcp_fin_timeout|net\.ipv4\.tcp_tw_reuse|net\.ipv4\.ip_local_port_range|fs\.file-max|fs\.inotify)`)
	reCloudInit  = lazyre.MustCompile(`^(distro|system_info|manage_etc_hosts|preserve_hostname)`)
)

// grepFile is `grep [-v] -E RE FILE 2>/dev/null`.
func (f files) grepFile(p string, re *regexp.Regexp, keep bool) []string {
	return f.grep(f.read(p), re, keep)
}

// toolHead is `NAME ARGS 2>/dev/null | head -N`.
func (s *Session) toolHead(n int, name string, args ...string) []byte {
	out, _ := s.tool(name, args...)
	return headN(out, n)
}

// sudoTail is `__sysinfo_sudo tail -N FILE`.
func (s *Session) sudoTail(p string, n int) []byte {
	if s.elevated() {
		out, _ := s.tool("sudo", "tail", "-"+strconv.Itoa(n), s.m.sys.Path(p))
		return out
	}
	return s.f.tail(p, n)
}

// listening is the two ss blocks that close the services and network
// drill-downs.
func (s *Session) listening(d deepOut) {
	d.echo("--- Listening TCP")
	out, _ := s.tool("ss", "-tlnp")
	d.raw(headN(skipLine(out), 50))
	d.echo("--- Listening UDP")
	out, _ = s.tool("ss", "-ulnp")
	d.raw(headN(skipLine(out), 30))
}

// deepServices is __sysinfo_deep_services.
func (s *Session) deepServices(d deepOut) {
	f := s.f
	d.echo("=== SERVICES DEEP ===")

	if f.isDir("/etc/apache2") {
		d.echo("--- Apache2")
		if f.isFile("/etc/apache2/ports.conf") {
			d.echo("--- ports.conf")
			d.lines(f.grepFile("/etc/apache2/ports.conf", reNoteBlank.R(), false))
		}
		d.echo("--- mods-enabled")
		d.lines(f.basenames("/etc/apache2/mods-enabled", ".load", false))
		if f.isDir("/etc/apache2/sites-enabled") {
			for _, p := range f.globIn("/etc/apache2/sites-enabled/", "*.conf") {
				if f.isFile(p) {
					d.echo("--- vhost: " + path.Base(p))
					d.lines(head(f.grepFile(p, reNoteBlankIndent.R(), false), 80))
				}
			}
		}
	}
	if f.isDir("/etc/nginx") {
		d.echo("--- Nginx")
		if f.isDir("/etc/nginx/sites-enabled") {
			for _, p := range f.globIn("/etc/nginx/sites-enabled/", "*") {
				if f.isFile(p) {
					d.echo("--- site: " + path.Base(p))
					d.lines(head(f.grepFile(p, reNoteBlankIndent.R(), false), 80))
				}
			}
		}
	}
	if f.isDir("/etc/lighttpd") {
		d.echo("--- Lighttpd")
		if f.isFile("/etc/lighttpd/lighttpd.conf") {
			d.lines(head(f.grepFile("/etc/lighttpd/lighttpd.conf", reNoteBlank.R(), false), 50))
		}
		d.lines(f.ls("/etc/lighttpd/conf-enabled"))
	}

	if f.isDir("/etc/php") {
		for _, ver := range f.glob("/etc/php/*/") {
			if !f.isDir(ver) {
				continue
			}
			d.echo("--- PHP " + basenameSuffix(ver, ""))
			if f.isDir(ver + "fpm/pool.d") {
				for _, p := range f.globIn(ver+"fpm/pool.d/", "*.conf") {
					if f.isFile(p) {
						d.echo("--- pool: " + path.Base(p))
						d.lines(head(f.grepFile(p, reSemiBlank.R(), false), 50))
					}
				}
			}
			if f.isFile(ver + "cli/php.ini") {
				d.echo("--- cli/php.ini (key)")
				d.lines(head(f.grepFile(ver+"cli/php.ini", rePHPIni.R(), true), 10))
			}
			if f.isDir(ver + "mods-available") {
				d.echo("--- modules")
				d.lines(pasteS(f.basenames(ver+"mods-available", ".ini", false)))
			}
		}
	}

	if f.isDir("/etc/mysql") {
		d.echo("--- MySQL/MariaDB")
		s.cnfFiles(d, "/etc/mysql/mariadb.conf.d")
		s.cnfFiles(d, "/etc/mysql/conf.d")
	}
	if f.isDir("/etc/postgresql") {
		d.echo("--- PostgreSQL")
		for _, ver := range f.glob("/etc/postgresql/*/") {
			if !f.isDir(ver) {
				continue
			}
			d.echo("--- version: " + basenameSuffix(ver, ""))
			if f.isFile(ver + "main/postgresql.conf") {
				d.lines(head(f.grepFile(ver+"main/postgresql.conf", reNoteBlank.R(), false), 50))
			}
			if f.isFile(ver + "main/pg_hba.conf") {
				d.echo("--- pg_hba.conf")
				d.lines(head(f.grepFile(ver+"main/pg_hba.conf", reNoteBlank.R(), false), 50))
			}
		}
	}
	if f.isFile("/etc/redis/redis.conf") {
		d.echo("--- Redis")
		d.lines(head(f.grepFile("/etc/redis/redis.conf", reNoteBlank.R(), false), 50))
	}

	if f.isDir("/etc/postfix") {
		d.echo("--- Postfix")
		for _, n := range []string{"main.cf", "master.cf"} {
			if f.isFile("/etc/postfix/" + n) {
				d.echo("--- " + n)
				d.lines(head(f.grepFile("/etc/postfix/"+n, reNoteBlank.R(), false), 50))
			}
		}
	}
	if f.isDir("/etc/dovecot") {
		d.echo("--- Dovecot")
		if f.isFile("/etc/dovecot/dovecot.conf") {
			d.lines(head(f.grepFile("/etc/dovecot/dovecot.conf", reNoteBlank.R(), false), 30))
		}
		// find | sort: the paths in byte order (SYS-41).
		for _, p := range sorted(f.find("/etc/dovecot/conf.d", query{maxDepth: -1, names: []string{"*.conf"}})) {
			if content := subst(f.grepFile(p, reNoteBlank.R(), false)); content != "" {
				d.echo("--- " + path.Base(p))
				d.lines(head(strings.Split(content, "\n"), 30))
			}
		}
	}
	if f.isDir("/etc/exim4") {
		d.echo("--- Exim4")
		if f.isFile("/etc/exim4/update-exim4.conf.conf") {
			d.lines(head(f.grepFile("/etc/exim4/update-exim4.conf.conf", reNoteBlank.R(), false), 30))
		}
	}
	if f.isDir("/etc/spamassassin") {
		d.echo("--- SpamAssassin")
		if f.isFile("/etc/spamassassin/local.cf") {
			d.lines(head(f.grepFile("/etc/spamassassin/local.cf", reNoteBlank.R(), false), 30))
		}
		if f.isDir("/etc/spamassassin/razor") {
			d.echo("Razor=configured")
		}
		if f.isDir("/etc/spamassassin/pyzor") {
			d.echo("Pyzor=configured")
		}
	}
	if f.isFile("/etc/default/opendkim") || f.isDir("/etc/opendkim") {
		d.echo("--- OpenDKIM")
		if f.isFile("/etc/default/opendkim") {
			d.lines(f.grepFile("/etc/default/opendkim", reNoteBlank.R(), false))
		}
		for _, n := range []string{"TrustedHosts", "KeyTable", "SigningTable"} {
			if f.isFile("/etc/opendkim/" + n) {
				d.echo("--- " + n)
				d.raw(headN(f.read("/etc/opendkim/"+n), 20))
			}
		}
	}

	if f.isDir("/etc/bind") {
		d.echo("--- BIND")
		if f.isFile("/etc/bind/named.conf.local") {
			d.echo("--- named.conf.local")
			d.raw(headN(f.read("/etc/bind/named.conf.local"), 50))
		}
		if f.isFile("/etc/bind/named.conf.options") {
			d.echo("--- named.conf.options")
			d.lines(head(f.grepFile("/etc/bind/named.conf.options", reSlashBlank.R(), false), 50))
		}
	}
	if f.isDir("/etc/unbound") {
		d.echo("--- Unbound")
		d.raw(headN(f.cat(f.find("/etc/unbound", query{maxDepth: -1, names: []string{"*.conf"}})), 50))
	}
	if f.isDir("/etc/avahi") {
		d.echo("--- Avahi")
		if f.isFile("/etc/avahi/avahi-daemon.conf") {
			d.lines(head(f.grepFile("/etc/avahi/avahi-daemon.conf", reNoteBlank.R(), false), 20))
		}
	}

	if f.isDir("/etc/proftpd") {
		d.echo("--- ProFTPD")
		if f.isFile("/etc/proftpd/proftpd.conf") {
			d.lines(head(f.grepFile("/etc/proftpd/proftpd.conf", reNoteBlank.R(), false), 50))
		}
		d.lines(head(f.grepFiles(f.find("/etc/proftpd/conf.d", query{maxDepth: -1, names: []string{"*.conf"}}), reNoteBlank.R(), false), 30))
	}
	if f.isDir("/etc/vsftpd") || f.isFile("/etc/vsftpd.conf") {
		d.echo("--- vsftpd")
		vsf := "/etc/vsftpd.conf"
		if f.isFile("/etc/vsftpd/vsftpd.conf") {
			vsf = "/etc/vsftpd/vsftpd.conf"
		}
		d.lines(head(f.grepFile(vsf, reNoteBlank.R(), false), 30))
	}

	if f.isDir("/etc/samba") {
		d.echo("--- Samba")
		if f.isFile("/etc/samba/smb.conf") {
			d.lines(head(f.grepFile("/etc/samba/smb.conf", reNoteSemiBlank.R(), false), 50))
		}
	}
	if f.isDir("/etc/openmediavault") {
		d.echo("--- OpenMediaVault")
		d.echo("OMV=installed")
		if f.isFile("/etc/openmediavault/config.xml") {
			size := ""
			if b, err := s.m.sys.ReadFile("/etc/openmediavault/config.xml"); err == nil {
				size = strconv.Itoa(len(b))
			}
			d.echo("config.xml=" + size + " bytes")
		}
	}
	if f.isFile("/etc/exports") {
		d.echo("--- NFS")
		d.raw(headN(f.read("/etc/exports"), 30))
		if f.isFile("/etc/nfs.conf") {
			d.lines(head(f.grepFile("/etc/nfs.conf", reNoteBlank.R(), false), 20))
		}
	}

	if f.isDir("/etc/openvpn") {
		d.echo("--- OpenVPN")
		for _, p := range f.find("/etc/openvpn", query{maxDepth: -1, names: []string{"*.conf", "*.ovpn"}}) {
			d.echo("--- " + path.Base(p))
			d.lines(head(f.grepFile(p, reNoteBlank.R(), false), 30))
		}
		if f.isFile("/etc/openvpn/mullvad.conf") {
			d.echo("Mullvad=configured")
		}
	}
	if f.isDir("/etc/wireguard") {
		d.echo("--- WireGuard")
		for _, p := range f.globIn("/etc/wireguard/", "*.conf") {
			if !f.isFile(p) {
				continue
			}
			d.echo("--- " + path.Base(p))
			// The value of every PrivateKey and PresharedKey line is
			// masked, whatever its spacing (SYS-50).
			var ls []string
			for _, l := range f.grepFile(p, reNoteBlank.R(), false) {
				ls = append(ls, reWireGuardKey.ReplaceAllString(l, "${1}[REDACTED]"))
			}
			d.lines(head(ls, 20))
		}
	}

	if f.isDir("/etc/docker") {
		d.echo("--- Docker")
		if f.isFile("/etc/docker/daemon.json") {
			d.raw(f.read("/etc/docker/daemon.json"))
		}
		if s.has("docker") {
			d.raw(s.toolHead(50, "docker", "info"))
		}
	}
	if f.isDir("/etc/containerd") {
		d.echo("--- containerd")
		if f.isFile("/etc/containerd/config.toml") {
			d.lines(head(f.grepFile("/etc/containerd/config.toml", reNoteBlank.R(), false), 30))
		}
	}
	if f.isDir("/etc/nvidia-container-runtime") {
		d.echo("--- NVIDIA Container Runtime")
		for _, p := range f.find("/etc/nvidia-container-runtime", query{maxDepth: -1, typ: 'f'}) {
			d.echo("--- " + path.Base(p))
			d.raw(headN(f.read(p), 20))
		}
	}

	if f.isDir("/etc/collectd") {
		d.echo("--- Collectd")
		if f.isFile("/etc/collectd/collectd.conf") {
			d.lines(head(f.grepFile("/etc/collectd/collectd.conf", reNoteBlank.R(), false), 30))
		}
		for _, p := range f.find("/etc/collectd/collectd.conf.d", query{maxDepth: -1, names: []string{"*.conf"}}) {
			d.echo("--- " + path.Base(p))
			d.raw(headN(f.read(p), 20))
		}
	}
	if f.isDir("/etc/monit") {
		d.echo("--- Monit")
		for _, sub := range []string{"conf.d", "conf-available"} {
			if !f.isDir("/etc/monit/" + sub) {
				continue
			}
			for _, p := range f.globIn("/etc/monit/"+sub+"/", "*") {
				if f.isFile(p) {
					d.echo("--- " + sub + "/" + path.Base(p))
					d.raw(headN(f.read(p), 20))
				}
			}
		}
	}
	if f.isDir("/etc/snmp") {
		d.echo("--- SNMP")
		if f.isFile("/etc/snmp/snmpd.conf") {
			d.lines(head(f.grepFile("/etc/snmp/snmpd.conf", reNoteBlank.R(), false), 30))
		}
	}
	if f.isDir("/etc/sysstat") {
		d.echo("--- sysstat")
		if f.isFile("/etc/default/sysstat") {
			d.raw(headN(f.read("/etc/default/sysstat"), 10))
		}
	}
	if f.isFile("/etc/smartd.conf") || f.isFile("/etc/smartmontools/smartd.conf") {
		d.echo("--- smartmontools")
		smartf := "/etc/smartd.conf"
		if f.isFile("/etc/smartmontools/smartd.conf") {
			smartf = "/etc/smartmontools/smartd.conf"
		}
		d.lines(head(f.grepFile(smartf, reNoteBlank.R(), false), 20))
	}

	if f.isDir("/etc/webmin") {
		d.echo("--- Webmin")
		if f.isFile("/etc/webmin/miniserv.conf") {
			d.lines(f.grepFile("/etc/webmin/miniserv.conf", reWebminKeys.R(), true))
		}
		d.echo("modules:")
		d.lines(head(f.ls("/etc/webmin"), 30))
		if f.isDir("/etc/webmin/virtual-server") {
			d.echo("Virtualmin=installed")
		}
	}
	if f.isDir("/etc/usermin") {
		d.echo("--- Usermin=installed")
	}

	if f.isDir("/etc/chrony") {
		d.echo("--- Chrony")
		if f.isFile("/etc/chrony/chrony.conf") {
			d.lines(head(f.grepFile("/etc/chrony/chrony.conf", reNoteBlank.R(), false), 20))
		}
	}
	if f.isFile("/etc/systemd/timesyncd.conf") {
		d.echo("--- systemd-timesyncd")
		d.lines(head(f.grepFile("/etc/systemd/timesyncd.conf", reTimesyncd.R(), false), 10))
	}

	d.echo("--- Custom systemd units")
	for _, pat := range []string{"*.service", "*.timer"} {
		for _, unit := range f.globIn("/etc/systemd/system/", pat) {
			if !f.isFile(unit) || f.isLink(unit) {
				continue
			}
			s.customUnit(d, unit)
		}
	}
	for _, p := range f.find("/etc/systemd/system", query{maxDepth: -1, names: []string{"override.conf"}}) {
		d.echo("--- Override: " + p)
		d.raw(headN(f.read(p), 20))
	}

	d.echo("--- Running services")
	out, _ := s.tool("systemctl", "list-units", "--type=service", "--state=running", "--no-pager", "--no-legend")
	d.lines(head(fieldLines(out, 1), 50))
	d.echo("--- Active timers")
	out, _ = s.tool("systemctl", "list-timers", "--no-pager", "--no-legend")
	d.lines(head(fieldLines(out, 0), 30))
	s.listening(d)
}

// cnfFiles is the loop over DIR/*.cnf of the MySQL blocks.
func (s *Session) cnfFiles(d deepOut, dir string) {
	f := s.f
	if !f.isDir(dir) {
		return
	}
	for _, p := range f.globIn(dir+"/", "*.cnf") {
		if f.isFile(p) {
			d.echo("--- " + path.Base(p))
			d.lines(f.grepFile(p, reNoteBlank.R(), false))
		}
	}
}

// customUnit is the body of the custom unit loop: the unit, the files its
// EnvironmentFile= and ExecStart= lines name, its WorkingDirectory=.
func (s *Session) customUnit(d deepOut, unit string) {
	f := s.f
	b := f.read(unit)
	d.echo("--- Custom: " + path.Base(unit))
	d.raw(headN(b, 40))
	ls := grepLines(b, f.u8)
	for _, env := range grepOAll(ls, reEnvFile.R()) {
		// ${env_file/#-/}; a relative name is the caller's directory's.
		env = strings.TrimPrefix(env, "-")
		if f.isFile(f.abs(env)) {
			d.echo("--- Config: " + env)
			d.raw(headN(f.read(f.abs(env)), 30))
		}
	}
	if exec := subst(grepOAll(ls, reExec.R())); exec != "" {
		for _, l := range grepLines([]byte(exec+"\n"), f.u8) {
			for _, cfg := range reCfgPath.FindAllString(l, -1) {
				if f.isFile(cfg) {
					d.echo("--- Config: " + cfg)
					d.raw(headN(f.read(cfg), 30))
				}
			}
		}
	}
	d.lines(grepOAll(ls, reWorkDir.R()))
}

// deepSecurity is __sysinfo_deep_security.
func (s *Session) deepSecurity(d deepOut) {
	f := s.f
	d.echo("=== SECURITY DEEP ===")

	if s.has("iptables") {
		d.echo("--- iptables")
		d.raw(headN(s.Sudo("iptables", "-L", "-n", "-v"), 50))
	}
	if f.isDir("/etc/ufw") {
		d.echo("--- UFW")
		d.raw(headN(s.Sudo("ufw", "status", "verbose"), 30))
		if f.isFile("/etc/ufw/ufw.conf") {
			d.lines(f.grepFile("/etc/ufw/ufw.conf", reNoteBlank.R(), false))
		}
	}
	if s.has("firewall-cmd") {
		d.echo("--- firewalld")
		d.raw(headN(s.Sudo("firewall-cmd", "--list-all"), 30))
	}

	if f.isDir("/etc/fail2ban") {
		d.echo("--- fail2ban")
		d.raw(headN(s.Sudo("fail2ban-client", "status"), 20))
		if f.isFile("/etc/fail2ban/jail.local") {
			d.echo("--- jail.local")
			d.lines(head(f.grep(s.sudoRead("/etc/fail2ban/jail.local"), reNoteSemiBlank.R(), false), 40))
		}
		for _, p := range s.sudoFind("/etc/fail2ban/jail.d", query{maxDepth: -1, names: []string{"*.conf"}}) {
			d.echo("--- " + path.Base(p))
			d.lines(head(f.grep(s.sudoRead(p), reNoteSemiBlank.R(), false), 30))
		}
	}
	if f.isDir("/etc/crowdsec") {
		d.echo("--- CrowdSec")
		if f.isFile("/etc/crowdsec/config.yaml") {
			d.lines(head(f.grepFile("/etc/crowdsec/config.yaml", reNoteBlank.R(), false), 30))
		}
		if f.isFile("/etc/crowdsec/acquis.yaml") {
			d.echo("--- acquis.yaml")
			d.raw(headN(f.read("/etc/crowdsec/acquis.yaml"), 30))
		}
		d.echo("--- scenarios")
		d.lines(head(f.ls("/etc/crowdsec/hub/scenarios"), 20))
	}

	d.echo("--- SSH")
	d.lines(f.grep(s.sudoRead("/etc/ssh/sshd_config"), reNoteBlank.R(), false))
	for _, p := range s.sudoFind("/etc/ssh/sshd_config.d", query{maxDepth: -1, names: []string{"*.conf"}}) {
		d.echo("--- " + path.Base(p))
		d.lines(f.grep(s.sudoRead(p), reNoteBlank.R(), false))
	}

	d.echo("--- PAM")
	for _, pam := range []string{"common-auth", "common-password", "common-session"} {
		if f.isFile("/etc/pam.d/" + pam) {
			d.echo("--- " + pam)
			d.lines(head(f.grepFile("/etc/pam.d/"+pam, reNoteBlank.R(), false), 30))
		}
	}

	if f.isDir("/etc/apparmor.d") {
		d.echo("--- AppArmor")
		if s.has("aa-status") {
			d.raw(headN(s.Sudo("aa-status"), 30))
		} else {
			d.lines(head(f.ls("/etc/apparmor.d"), 30))
		}
	}

	d.echo("--- Users")
	d.lines(passwdUsers(f.read("/etc/passwd")))

	d.echo("--- Sudoers")
	d.lines(head(f.grep(s.sudoRead("/etc/sudoers"), reNoteBlank.R(), false), 20))
	for _, p := range s.sudoFind("/etc/sudoers.d", query{maxDepth: -1, typ: 'f'}) {
		d.echo("--- " + path.Base(p))
		d.lines(f.grep(s.sudoRead(p), reNoteBlank.R(), false))
	}

	if f.isFile("/etc/login.defs") {
		d.echo("--- login.defs (key)")
		d.lines(f.grepFile("/etc/login.defs", reLoginDefs.R(), true))
	}

	d.echo("--- Login history")
	d.raw(s.toolHead(20, "last", "-20"))
	d.echo("--- Auth log (recent)")
	d.raw(headN(s.sudoTail("/var/log/auth.log", 30), 30))
}

// deepNetwork is __sysinfo_deep_network.
func (s *Session) deepNetwork(d deepOut) {
	f := s.f
	d.echo("=== NETWORK DEEP ===")
	d.echo("--- Interfaces")
	d.raw(s.toolHead(50, "ip", "addr", "show"))
	d.echo("--- Routes")
	d.raw(s.toolHead(20, "ip", "route", "show"))
	d.echo("--- Neighbors")
	d.raw(s.toolHead(20, "ip", "neigh", "show"))
	d.echo("--- resolv.conf")
	d.raw(f.read("/etc/resolv.conf"))
	if s.has("resolvectl") {
		d.echo("--- systemd-resolved")
		d.raw(s.toolHead(20, "resolvectl", "status"))
	}
	d.echo("--- hosts")
	d.raw(f.read("/etc/hosts"))
	if f.isDir("/etc/netplan") {
		d.echo("--- Netplan")
		for _, p := range f.find("/etc/netplan", query{maxDepth: -1, names: []string{"*.yaml"}}) {
			d.echo("--- " + path.Base(p))
			d.raw(headN(f.read(p), 50))
		}
	}
	if f.isFile("/etc/network/interfaces") {
		d.echo("--- interfaces (legacy)")
		d.lines(f.grepFile("/etc/network/interfaces", reNoteBlank.R(), false))
	}
	if f.isFile("/etc/dhcpcd.conf") {
		d.echo("--- dhcpcd")
		d.lines(f.grepFile("/etc/dhcpcd.conf", reNoteBlank.R(), false))
	}
	s.listening(d)
}

// deepStorage is __sysinfo_deep_storage.
func (s *Session) deepStorage(d deepOut) {
	f := s.f
	d.echo("=== STORAGE DEEP ===")
	d.echo("--- Disk usage")
	d.raw(s.toolHead(30, "df", "-hT"))
	d.echo("--- Block devices")
	d.raw(s.toolHead(30, "lsblk", "-f"))
	d.echo("--- Mounts")
	out, _ := s.tool("mount")
	d.lines(head(filter(grepLines(out, f.u8), reCgroup.R(), false), 30))
	d.echo("--- fstab")
	d.raw(f.read("/etc/fstab"))
	if f.isFile("/etc/crypttab") {
		d.echo("--- crypttab")
		d.raw(s.sudoRead("/etc/crypttab"))
	}

	if s.has("pvs") {
		for _, lv := range [][2]string{{"PVs", "pvs"}, {"VGs", "vgs"}, {"LVs", "lvs"}} {
			d.echo("--- LVM " + lv[0])
			d.raw(s.toolHead(20, lv[1]))
		}
	}
	if f.isFile("/etc/mdadm/mdadm.conf") {
		d.echo("--- mdadm")
		d.lines(f.grepFile("/etc/mdadm/mdadm.conf", reNoteBlank.R(), false))
	}
	if f.isFile("/proc/mdstat") {
		d.echo("--- mdstat")
		d.raw(headN(f.read("/proc/mdstat"), 30))
	}

	if s.has("zpool") {
		d.echo("--- ZFS pools")
		d.raw(s.toolHead(30, "zpool", "status"))
		d.echo("--- ZFS datasets")
		d.raw(s.toolHead(30, "zfs", "list"))
	}
	if f.isFile("/etc/zfs/zed.rc") {
		d.echo("--- zed.rc")
		d.lines(f.grepFile("/etc/zfs/zed.rc", reNoteBlank.R(), false))
	}
	if f.isFile("/etc/sanoid/sanoid.conf") {
		d.echo("--- Sanoid")
		d.raw(headN(f.read("/etc/sanoid/sanoid.conf"), 30))
	}

	if s.has("smartctl") {
		d.echo("--- SMART")
		for _, disk := range append(f.words("/dev/sd?"), f.words("/dev/nvme?n?")...) {
			if f.isBlock(disk) {
				d.echo("--- " + path.Base(disk))
				d.raw(headN(s.Sudo("smartctl", "-H", s.m.sys.Path(disk)), 10))
			}
		}
	}

	if f.isFile("/etc/exports") {
		d.echo("--- NFS exports")
		d.raw(headN(f.read("/etc/exports"), 30))
	}
	if f.isFile("/etc/nfs.conf") {
		d.echo("--- nfs.conf")
		d.lines(head(f.grepFile("/etc/nfs.conf", reNoteBlank.R(), false), 20))
	}
	if f.isFile("/etc/hdparm.conf") {
		d.echo("--- hdparm")
		d.lines(head(f.grepFile("/etc/hdparm.conf", reNoteBlank.R(), false), 20))
	}
}

// deepPerformance is __sysinfo_deep_performance.
func (s *Session) deepPerformance(d deepOut) {
	f := s.f
	d.echo("=== PERFORMANCE DEEP ===")
	d.echo("--- Memory")
	out, _ := s.tool("free", "-h")
	d.raw(out)
	d.echo("--- CPU")
	res, err := s.m.run.Run(s.ctx, runner.Cmd{Path: "lscpu", Options: runner.Options{Env: withEnv(s.env, "LC_ALL=C"), Stderr: io.Discard}})
	if err == nil {
		d.lines(head(filter(grepLines(res.Stdout, f.u8), reCPUInfo.R(), true), 10))
	}
	d.echo("--- Load")
	out, _ = s.tool("uptime")
	d.raw(out)
	d.echo("--- Top by memory")
	d.raw(s.toolHead(15, "ps", "aux", "--sort=-%mem"))
	d.echo("--- Top by CPU")
	d.raw(s.toolHead(10, "ps", "aux", "--sort=-%cpu"))

	if f.isDir("/etc/mysql") {
		d.echo("--- MySQL/MariaDB config")
		s.cnfFiles(d, "/etc/mysql/mariadb.conf.d")
	}
	if f.isDir("/etc/php") {
		d.echo("--- PHP FPM pools")
		for _, dir := range f.glob("/etc/php/*/fpm/pool.d/") {
			if !f.isDir(dir) {
				continue
			}
			ver := subst(grepOAll(grepLines([]byte(dir+"\n"), f.u8), rePHPVersion.R()))
			for _, p := range f.globIn(dir, "*.conf") {
				if f.isFile(p) {
					d.echo("--- PHP " + ver + " " + path.Base(p))
					d.lines(head(f.grepFile(p, reSemiBlank.R(), false), 50))
				}
			}
		}
	}

	d.echo("--- sysctl (key active params)")
	out, _ = s.tool("sysctl", "-a")
	d.lines(head(filter(grepLines(out, f.u8), reSysctlKeys.R(), true), 30))
	d.echo("--- sysctl.d configs")
	for _, p := range f.find("/etc/sysctl.d", query{maxDepth: -1, names: []string{"*.conf"}}) {
		d.echo("--- " + path.Base(p))
		d.lines(f.grepFile(p, reNoteBlank.R(), false))
	}
	if f.isFile("/etc/sysctl.conf") {
		if content := subst(f.grepFile("/etc/sysctl.conf", reNoteBlank.R(), false)); content != "" {
			d.echo("--- sysctl.conf")
			d.echo(content)
		}
	}

	d.echo("--- I/O scheduler")
	for _, p := range f.glob("/sys/block/*/queue/scheduler") {
		if f.isFile(p) {
			// /sys/block/DEV/queue/scheduler: the device, two levels up
			// (SYS-51).
			d.echo(path.Base(path.Dir(path.Dir(p))) + ": " + runner.CommandSubst(f.read(p)))
		}
	}
}

// deepMaintenance is __sysinfo_deep_maintenance.
func (s *Session) deepMaintenance(d deepOut) {
	f := s.f
	d.echo("=== MAINTENANCE DEEP ===")
	d.echo("--- crontab")
	d.lines(head(f.grepFile("/etc/crontab", reNoteBlank.R(), false), 20))
	d.echo("--- cron.d contents")
	for _, p := range f.globIn("/etc/cron.d/", "*") {
		if f.isFile(p) {
			d.echo("--- " + path.Base(p))
			d.lines(head(f.grepFile(p, reNoteBlank.R(), false), 15))
		}
	}
	for _, sub := range []string{"cron.daily", "cron.hourly", "cron.weekly", "cron.monthly"} {
		d.echo("--- " + sub)
		d.lines(f.ls("/etc/" + sub))
	}
	d.echo("--- User crontabs")
	d.lines(head(s.sudoLs("/var/spool/cron/crontabs"), 10))

	d.echo("--- Logrotate configs")
	for _, p := range f.globIn("/etc/logrotate.d/", "*") {
		if f.isFile(p) {
			d.echo("--- " + path.Base(p))
			d.raw(headN(f.read(p), 15))
		}
	}

	if f.isDir("/etc/letsencrypt") {
		d.echo("--- Let's Encrypt")
		d.echo("--- Live domains")
		d.lines(s.sudoLs("/etc/letsencrypt/live"))
		d.echo("--- Cert expiry")
		for _, cert := range s.sudoFind("/etc/letsencrypt/live", query{maxDepth: -1, names: []string{"cert.pem"}}) {
			expiry := runner.CommandSubst(s.Sudo("openssl", "x509", "-enddate", "-noout", "-in", s.m.sys.Path(cert)))
			d.echo(path.Base(path.Dir(cert)) + ": " + expiry)
		}
		d.echo("--- Renewal configs")
		for _, p := range s.sudoFind("/etc/letsencrypt/renewal", query{maxDepth: -1, names: []string{"*.conf"}}) {
			d.echo("--- " + path.Base(p))
			d.raw(headN(s.sudoRead(p), 20))
		}
		if f.isFile("/etc/letsencrypt/cli.ini") {
			d.echo("--- cli.ini")
			d.lines(f.grepFile("/etc/letsencrypt/cli.ini", reNoteBlank.R(), false))
		}
	}

	d.echo("--- Upgradable packages")
	out, _ := s.tool("apt", "list", "--upgradable")
	d.raw(headN(skipLine(out), 30))
	d.echo("--- dpkg log (recent)")
	d.raw(f.tail("/var/log/dpkg.log", 30))
	if f.isFile("/etc/apt/apt.conf.d/50unattended-upgrades") {
		d.echo("--- unattended-upgrades")
		d.lines(head(f.grepFile("/etc/apt/apt.conf.d/50unattended-upgrades", reSlashBlank.R(), false), 30))
	}

	d.echo("--- Pending config merges")
	d.lines(head(f.find("/etc", query{maxDepth: -1, names: []string{"*.dpkg-old", "*.dpkg-dist"}}), 20))

	if orgs := subst(f.lsPaths(f.glob("/etc/*.org")...)); orgs != "" {
		d.echo("--- Legacy .org backups")
		d.echo(orgs)
	}
	if f.isDir("/etc/.git") {
		d.echo("--- etckeeper log")
		out, _ := s.tool("git", "-C", s.m.sys.Path("/etc"), "log", "--oneline", "-20")
		d.raw(out)
	}
	if f.isFile("/etc/cloud/cloud.cfg") {
		d.echo("--- Cloud-init")
		d.lines(head(f.grepFile("/etc/cloud/cloud.cfg", reCloudInit.R(), true), 20))
	}

	d.echo("--- Profile scripts")
	d.lines(f.ls("/etc/profile.d"))
	if f.isFile("/etc/environment") {
		d.echo("--- environment")
		d.raw(f.read("/etc/environment"))
	}
	d.echo("--- Service defaults")
	d.lines(head(f.ls("/etc/default"), 30))
}
