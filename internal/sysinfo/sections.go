// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sysinfo

import (
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/runner"
)

// The grep expressions of the collectors.
var (
	reNoteBlank     = regexp.MustCompile(`^(#|$)`)
	reNoteSemiBlank = regexp.MustCompile(`^(#|;|$)`)
	reHash          = regexp.MustCompile(`^#`)
	reSlashBlank    = regexp.MustCompile(`^(//|$)`)

	// grep -oP '^ID=\K\w+' and friends (SYS-18: \w is ASCII, as PCRE
	// without UCP).
	reOSID      = regexp.MustCompile(`^ID=([A-Za-z0-9_]+)`)
	reOSPretty  = regexp.MustCompile(`^PRETTY_NAME="([^"]+)`)
	reOSVersion = regexp.MustCompile(`^VERSION_ID="([^"]+)`)
	reContainer = regexp.MustCompile(`(?i)container|vz|lxc|docker`)
)

// grepO is `grep -oP RE FILE` for an expression whose match is group 1.
func (f files) grepO(data []byte, re *regexp.Regexp) string {
	var out []string
	for _, l := range grepLines(data, f.u8) {
		if m := re.FindStringSubmatch(l); m != nil {
			out = append(out, m[1])
		}
	}
	return subst(out)
}

// identity is __sysinfo_collect_identity.
func (s *Session) identity(o *out) {
	f := s.f
	o.section("IDENTITY")
	pretty, id, version := "", "", ""
	if f.isFile("/etc/os-release") {
		b := f.read("/etc/os-release")
		id, pretty, version = f.grepO(b, reOSID), f.grepO(b, reOSPretty), f.grepO(b, reOSVersion)
	}
	o.kv("OS", pretty)
	o.kv("OS ID", id)
	o.kv("Version", version)
	o.kv("Debian Version", runner.CommandSubst(f.read("/etc/debian_version")))
	kernel, _ := s.toolShown("uname", "-r")
	o.kv("Kernel", runner.CommandSubst(kernel))
	arch, _ := s.toolShown("uname", "-m")
	o.kv("Arch", runner.CommandSubst(arch))
	o.kv("Hostname", s.either([]string{"hostname", "-f"}, []string{"hostname"}, true))
	o.kv("Machine-ID", runner.CommandSubst(f.read("/etc/machine-id")))
	up, _ := s.tool("uptime", "-p")
	o.kv("Uptime", runner.CommandSubst(up))

	if s.has("systemd-detect-virt") {
		// `|| echo none` after the tool's own `none` (SYS-16).
		out, rc := s.tool("systemd-detect-virt")
		if rc != 0 {
			out = append(append([]byte(nil), out...), "none\n"...)
		}
		o.kv("Virtualization", runner.CommandSubst(out))
	}

	// cat /proc/1/environ | tr '\0' '\n' | grep -iE ... | head -1
	env := strings.ReplaceAll(string(s.sudoRead("/proc/1/environ")), "\x00", "\n")
	container := ""
	for _, l := range grepLines([]byte(env), f.u8) {
		if reContainer.MatchString(l) {
			container = l
			break
		}
	}
	if container != "" {
		o.kv("Container", container)
	} else if f.isDir("/proc/vz") {
		o.kv("Container", "OpenVZ")
	}

	if f.isFile("/etc/wsl.conf") {
		o.kv("WSL", "yes")
		if f.isFile("/etc/ubwsl-installed") {
			o.kv("WSL Marker", "ubwsl-installed")
		}
		if o.llm {
			o.write("WSL_CONF:\n")
			o.write(string(f.read("/etc/wsl.conf")))
		}
	}
	if f.isFile("/etc/hetzner-build") {
		o.kv("Cloud", "Hetzner")
	}
}

// storage is __sysinfo_collect_storage.
func (s *Session) storage(o *out) {
	f := s.f
	o.section("STORAGE")
	o.subhead("fstab")
	o.raw(f.grep(f.read("/etc/fstab"), reNoteBlank, false))
	if f.isFile("/etc/crypttab") {
		o.subhead("crypttab")
		o.raw(f.grep(s.sudoRead("/etc/crypttab"), reHash, false))
	}
	if f.isDir("/etc/lvm") {
		o.kv("LVM", "present")
	}
	if f.isFile("/etc/mdadm/mdadm.conf") {
		o.subhead("mdadm")
		o.raw(f.grep(f.read("/etc/mdadm/mdadm.conf"), regexp.MustCompile(`^ARRAY`), true))
	}
	if f.isDir("/etc/zfs") {
		o.kv("ZFS", "present")
		if f.isFile("/etc/hostid") {
			o.kv("ZFS Host-ID", "present")
		}
		if f.isFile("/etc/zfs/zed.rc") {
			o.subhead("zed.rc")
			o.raw(f.grep(f.read("/etc/zfs/zed.rc"), reNoteBlank, false))
		}
	}
	if f.isFile("/etc/sanoid/sanoid.conf") {
		o.subhead("sanoid")
		o.rawBytes(f.read("/etc/sanoid/sanoid.conf"))
	}
	if f.isFile("/etc/smartmontools/smartd.conf") || f.isFile("/etc/smartd.conf") {
		o.subhead("smartd")
		smartf := "/etc/smartd.conf"
		if f.isFile("/etc/smartmontools/smartd.conf") {
			smartf = "/etc/smartmontools/smartd.conf"
		}
		o.raw(f.grep(f.read(smartf), reNoteBlank, false))
	}
	if f.isFile("/etc/hdparm.conf") {
		o.kv("hdparm", "present")
	}
	if f.isFile("/etc/exports") {
		o.subhead("NFS exports")
		o.raw(f.grep(f.read("/etc/exports"), reNoteBlank, false))
	}
	if f.isFile("/etc/nfs.conf") {
		o.kv("NFS conf", "present")
	}
}

// serviceDirs are the /etc directories that name a service, in the 2.x
// order.
var serviceDirs = []string{
	"apache2", "nginx", "lighttpd",
	"php",
	"mysql", "mariadb", "postgresql", "postgresql-common", "redis",
	"postfix", "dovecot", "exim4", "spamassassin", "postgrey",
	"fail2ban", "crowdsec", "ufw", "firewalld", "iptables", "apparmor", "apparmor.d",
	"bind", "unbound", "avahi",
	"docker", "containerd", "nvidia-container-runtime", "nvidia-container-toolkit",
	"openmediavault", "samba", "openvpn", "wireguard",
	"webmin", "usermin",
	"monit", "collectd", "snmp", "sysstat",
	"zfs", "sanoid", "smartmontools",
	"proftpd", "vsftpd",
	"letsencrypt", "certbot",
	"chrony", "ntp",
	"salt", "runit", "jailkit", "clamav", "logcheck", "awstats",
	"qemu", "libvirt",
	"cloud", "netplan", "NetworkManager", "wpa_supplicant",
	"landscape", "ubuntu-advantage", "update-manager", "PackageKit", "apticron",
}

// services is __sysinfo_collect_services.
func (s *Session) services(o *out) {
	f := s.f
	o.section("SERVICES")
	o.subhead("Detected (by /etc/ dirs)")
	for _, d := range serviceDirs {
		if f.isDir("/etc/" + d) {
			o.item("/etc/" + d + "/")
		}
	}
	for _, mk := range []struct{ path, text string }{
		{"/etc/cufile.json", "NVIDIA cuFile GDS: present"},
		{"/etc/dkim-domains.txt", "OpenDKIM: present"},
		{"/etc/sasldb2", "SASL: present"},
		{"/etc/gprofng.rc", "GPROFNG: present"},
	} {
		if f.isFile(mk.path) {
			o.item(mk.text)
		}
	}
	if f.isDir("/etc/boot.d") {
		o.item("boot.d hooks: " + strings.Join(f.ls("/etc/boot.d"), ","))
	}
	if f.isDir("/etc/shutdown.d") {
		o.item("shutdown.d hooks: " + strings.Join(f.ls("/etc/shutdown.d"), ","))
	}
	if f.isFile("/etc/default/saslauthd") {
		o.subhead("saslauthd")
		o.raw(f.grep(f.read("/etc/default/saslauthd"), reNoteBlank, false))
	}
	if len(f.lsPaths(f.glob("/etc/*.org")...)) > 0 {
		o.item("Legacy .org backups found")
	}

	if s.has("systemctl") {
		o.subhead("Running services")
		out, _ := s.tool("systemctl", "list-units", "--type=service", "--state=running", "--no-pager", "--no-legend")
		o.raw(fieldLines(out, 1))
		o.subhead("Timers")
		out, _ = s.tool("systemctl", "list-timers", "--no-pager", "--no-legend")
		o.raw(fieldLines(out, 0))
	}
	o.subhead("Listening ports")
	ss, _ := s.tool("ss", "-tlnp")
	o.raw(tail2(lines(ss)))

	if f.isDir("/etc/systemd/system") {
		o.subhead("Custom systemd units")
		o.raw(f.find("/etc/systemd/system", query{maxDepth: 1, names: []string{"*.service", "*.timer", "*.mount", "*.socket"}}))
		o.raw(f.find("/etc/systemd/system", query{maxDepth: -1, names: []string{"override.conf"}}))
		for _, pat := range []string{"/etc/systemd/system/*.service", "/etc/systemd/system/*.timer"} {
			for _, unit := range f.glob(pat) {
				if !f.isFile(unit) || f.isLink(unit) {
					continue
				}
				o.subhead("Unit: " + path.Base(unit))
				o.raw(head(lines(s.sudoRead(unit)), 40))
			}
		}
		o.subhead("multi-user.target.wants")
		o.raw(f.ls("/etc/systemd/system/multi-user.target.wants"))
		o.subhead("timers.target.wants")
		o.raw(f.ls("/etc/systemd/system/timers.target.wants"))
	}
}

// fieldLines is `awk '{print $N}'` over a tool's output.
func fieldLines(out []byte, n int) []string {
	var ls []string
	for _, l := range lines(out) {
		ls = append(ls, awkField(l, n))
	}
	return ls
}

// tail2 is `tail -n +2`.
func tail2(ls []string) []string {
	if len(ls) < 2 {
		return nil
	}
	return ls[1:]
}

// passwdUsers is `awk -F: '$3>=1000 && $3<65534{print $1":"$3":"$6":"$7}'`.
func passwdUsers(b []byte) []string {
	var out []string
	for _, l := range lines(b) {
		fl := append(strings.Split(l, ":"), "", "", "", "", "", "", "")
		if awkCmp(fl[2], 1000) >= 0 && awkCmp(fl[2], 65534) < 0 {
			out = append(out, fl[0]+":"+fl[2]+":"+fl[5]+":"+fl[6])
		}
	}
	return out
}

var reSSHD = regexp.MustCompile(`^(Port|PermitRootLogin|PasswordAuthentication|PubkeyAuthentication|AuthorizedKeysFile|AllowUsers|AllowGroups|MaxAuthTries|ClientAliveInterval)`)

// auth is __sysinfo_collect_auth.
func (s *Session) auth(o *out) {
	f := s.f
	o.section("AUTH")
	o.subhead("Users")
	o.raw(passwdUsers(f.read("/etc/passwd")))
	o.subhead("Sudo")
	o.raw(head(f.grep(s.sudoRead("/etc/sudoers"), reNoteBlank, false), 20))
	for _, p := range s.sudoFind("/etc/sudoers.d", query{maxDepth: -1, typ: 'f'}) {
		o.item("--- " + p + ":")
		o.raw(f.grep(s.sudoRead(p), reNoteBlank, false))
	}
	o.subhead("SSH")
	o.raw(f.grep(s.sudoRead("/etc/ssh/sshd_config"), reSSHD, true))
	for _, p := range s.sudoFind("/etc/ssh/sshd_config.d", query{maxDepth: -1, names: []string{"*.conf"}}) {
		o.item("--- " + p + ":")
		o.raw(f.grep(s.sudoRead(p), reNoteBlank, false))
	}
}

// network is __sysinfo_collect_network.
func (s *Session) network(o *out) {
	f := s.f
	o.section("NETWORK")
	o.subhead("resolv.conf")
	o.raw(f.grep(f.read("/etc/resolv.conf"), reHash, false))
	o.subhead("hosts")
	o.raw(f.grep(f.read("/etc/hosts"), reNoteBlank, false))
	if f.isFile("/etc/network/interfaces") {
		o.subhead("interfaces")
		o.raw(f.grep(f.read("/etc/network/interfaces"), reNoteBlank, false))
	}
	if f.isDir("/etc/netplan") {
		o.subhead("netplan")
		o.rawBytes(f.cat(f.find("/etc/netplan", query{maxDepth: -1, names: []string{"*.yaml"}})))
	}
	if f.isFile("/etc/dhcpcd.conf") {
		o.subhead("dhcpcd")
		o.raw(f.grep(f.read("/etc/dhcpcd.conf"), reNoteBlank, false))
	}
}

// security is __sysinfo_collect_security.
func (s *Session) security(o *out) {
	f := s.f
	o.section("SECURITY")
	if f.isDir("/etc/fail2ban") {
		o.subhead("fail2ban")
		if f.isFile("/etc/fail2ban/jail.local") {
			o.raw(f.grep(s.sudoRead("/etc/fail2ban/jail.local"), reNoteSemiBlank, false))
		}
		for _, p := range s.sudoFind("/etc/fail2ban/jail.d", query{maxDepth: -1, names: []string{"*.conf"}}) {
			o.item("--- " + p + ":")
			o.raw(f.grep(s.sudoRead(p), reNoteSemiBlank, false))
		}
	}
	if f.isDir("/etc/crowdsec") {
		o.subhead("crowdsec")
		if f.isFile("/etc/crowdsec/config.yaml") {
			o.kv("CrowdSec", "installed")
		}
		if f.isFile("/etc/crowdsec/acquis.yaml") {
			o.raw(f.grep(f.read("/etc/crowdsec/acquis.yaml"), regexp.MustCompile(`source:`), true))
		}
		o.raw(head(f.ls("/etc/crowdsec/hub/scenarios"), 20))
	}
	if f.isDir("/etc/ufw") {
		o.subhead("ufw")
		if f.isFile("/etc/ufw/ufw.conf") {
			o.raw(f.grep(f.read("/etc/ufw/ufw.conf"), reNoteBlank, false))
		}
	}
	if f.isDir("/etc/firewalld") {
		o.kv("firewalld", "present")
	}
	if f.isDir("/etc/iptables") {
		o.kv("iptables", "present")
	}
	if f.isDir("/etc/apparmor.d") {
		o.kv("AppArmor", "present")
	}
}

// grepFiles is `find ... -exec grep [-v] -E RE {} \;`.
func (f files) grepFiles(paths []string, re *regexp.Regexp, keep bool) []string {
	var out []string
	for _, p := range paths {
		out = append(out, f.grep(f.read(p), re, keep)...)
	}
	return out
}

var (
	reApacheSite = regexp.MustCompile(`ServerName|ServerAlias|DocumentRoot|ProxyPass |ProxyPassReverse`)
	reNginxSite  = regexp.MustCompile(`server_name|root |proxy_pass`)
	reFPMPool    = regexp.MustCompile(`^\[|^(user|listen|pm |pm\.max_children|php_admin_value\[memory_limit\])`)
	reCnfNoise   = regexp.MustCompile(`^(#|$|\[)`)
	reCnfKeys    = regexp.MustCompile(`(bind|port|max_connections|innodb_buffer_pool_size|innodb_log_file_size|innodb_flush|default_storage|character.set|log_bin|disable_log_bin|galera)`)
)

// webstack is __sysinfo_collect_webstack.
func (s *Session) webstack(o *out) {
	f := s.f
	o.section("WEBSTACK")
	if f.isDir("/etc/apache2") {
		o.subhead("Apache")
		if f.isFile("/etc/apache2/ports.conf") {
			o.item("ports.conf:")
			o.raw(f.grep(f.read("/etc/apache2/ports.conf"), reNoteBlank, false))
		}
		if f.isDir("/etc/apache2/sites-enabled") {
			o.item("sites-enabled:")
			o.raw(f.ls("/etc/apache2/sites-enabled"))
			o.raw(f.grepFiles(f.find("/etc/apache2/sites-enabled", query{maxDepth: -1, names: []string{"*.conf"}}), reApacheSite, true))
		}
		if f.isDir("/etc/apache2/mods-enabled") {
			o.item("mods-enabled:")
			o.raw(f.basenames("/etc/apache2/mods-enabled", ".load", false))
		}
	}
	if f.isDir("/etc/nginx") {
		o.subhead("Nginx")
		if f.isDir("/etc/nginx/sites-enabled") {
			o.item("sites-enabled:")
			o.raw(f.ls("/etc/nginx/sites-enabled"))
			o.raw(f.grepFiles(f.find("/etc/nginx/sites-enabled", query{maxDepth: -1, typ: 'f'}), reNginxSite, true))
		}
	}
	if f.isDir("/etc/php") {
		o.subhead("PHP")
		o.item("Versions:")
		o.raw(sorted(f.find("/etc/php", query{minDepth: 1, maxDepth: 1, typ: 'd'})))
		o.item("FPM pools:")
		pools := f.findAll(f.words("/etc/php/*/fpm/pool.d"), query{maxDepth: -1, names: []string{"*.conf"}})
		o.raw(sorted(pools))
		o.raw(f.grepFiles(pools, reFPMPool, true))
		o.item("Modules:")
		var mods []string
		for _, p := range f.find("/etc/php", query{maxDepth: -1, path: "*/mods-available/*.ini"}) {
			mods = append(mods, basenameSuffix(p, ".ini"))
		}
		o.raw(pasteS(sortUniq(mods)))
	}
	if f.isDir("/etc/mysql") {
		o.subhead("MySQL/MariaDB")
		if f.isDir("/etc/mysql/mariadb.conf.d") {
			o.item("mariadb.conf.d:")
			cnfs := f.find("/etc/mysql/mariadb.conf.d", query{maxDepth: -1, names: []string{"*.cnf"}})
			o.raw(sorted(cnfs))
			o.raw(filter(f.grepFiles(cnfs, reCnfNoise, false), reCnfKeys, true))
		}
		if f.isDir("/etc/mysql/conf.d") {
			o.item("conf.d:")
			o.raw(f.grepFiles(f.find("/etc/mysql/conf.d", query{maxDepth: -1, names: []string{"*.cnf"}}), reNoteBlank, false))
		}
	}
	if f.isDir("/etc/postgresql") {
		o.subhead("PostgreSQL")
		o.raw(f.lsPaths(f.glob("/etc/postgresql/*/main/*.conf")...))
	}
}

var (
	rePostfixKeys = regexp.MustCompile(`(myhostname|mydomain|mydestination|mynetworks|inet_interfaces|relayhost|smtpd_tls|smtpd_sasl|smtp_tls|smtpd_milters|virtual_alias|content_filter|home_mailbox|mailbox_command)`)
	reDovecotKeys = regexp.MustCompile(`^(protocols|ssl_cert|ssl_key|mail_location|auth_mechanisms)`)
)

// mail is __sysinfo_collect_mail.
func (s *Session) mail(o *out) {
	f := s.f
	o.section("MAIL")
	if f.isDir("/etc/postfix") {
		o.subhead("Postfix")
		if f.isFile("/etc/postfix/main.cf") {
			o.raw(filter(f.grep(f.read("/etc/postfix/main.cf"), reNoteBlank, false), rePostfixKeys, true))
		}
	}
	if f.isDir("/etc/dovecot") {
		o.subhead("Dovecot")
		if f.isFile("/etc/dovecot/dovecot.conf") {
			o.raw(head(f.grep(f.read("/etc/dovecot/dovecot.conf"), reNoteBlank, false), 30))
		}
		o.raw(f.grepFiles(f.find("/etc/dovecot/conf.d", query{maxDepth: -1, names: []string{"*.conf"}}), reDovecotKeys, true))
	}
	if f.isDir("/etc/exim4") {
		o.subhead("Exim4")
		if f.isFile("/etc/exim4/update-exim4.conf.conf") {
			o.raw(f.grep(f.read("/etc/exim4/update-exim4.conf.conf"), reNoteBlank, false))
		}
	}
	if f.isDir("/etc/spamassassin") {
		o.subhead("SpamAssassin")
		if f.isFile("/etc/spamassassin/local.cf") {
			o.raw(f.grep(f.read("/etc/spamassassin/local.cf"), reNoteBlank, false))
		}
		if f.isDir("/etc/spamassassin/razor") {
			o.item("Razor: configured")
		}
		if f.isDir("/etc/spamassassin/pyzor") {
			o.item("Pyzor: configured")
		}
	}
	if f.isFile("/etc/default/opendkim") {
		o.subhead("OpenDKIM")
		o.raw(f.grep(f.read("/etc/default/opendkim"), reNoteBlank, false))
	}
	if f.isDir("/etc/bind") {
		o.subhead("BIND")
		if f.isFile("/etc/bind/named.conf.local") {
			o.raw(f.grep(f.read("/etc/bind/named.conf.local"), regexp.MustCompile(`^zone`), true))
		}
		if f.isFile("/etc/bind/named.conf.options") {
			o.raw(head(f.grep(f.read("/etc/bind/named.conf.options"), reSlashBlank, false), 15))
		}
	}
	if f.isDir("/etc/unbound") {
		o.subhead("Unbound")
		o.rawBytes(f.cat(f.find("/etc/unbound", query{maxDepth: -1, names: []string{"*.conf"}})))
	}
}

// infra is __sysinfo_collect_infra.
func (s *Session) infra(o *out) {
	f := s.f
	o.section("INFRA")
	if f.isDir("/etc/proftpd") {
		o.subhead("ProFTPD")
		if f.isFile("/etc/proftpd/proftpd.conf") {
			o.raw(head(f.grep(f.read("/etc/proftpd/proftpd.conf"), reNoteBlank, false), 20))
		}
		o.raw(head(f.grepFiles(f.find("/etc/proftpd/conf.d", query{maxDepth: -1, names: []string{"*.conf"}}), reNoteBlank, false), 30))
	}
	if f.isDir("/etc/vsftpd") || f.isFile("/etc/vsftpd.conf") {
		o.subhead("vsftpd")
		vsf := "/etc/vsftpd.conf"
		if f.isFile("/etc/vsftpd/vsftpd.conf") {
			vsf = "/etc/vsftpd/vsftpd.conf"
		}
		o.raw(head(f.grep(f.read(vsf), reNoteBlank, false), 15))
	}
	if f.isDir("/etc/samba") {
		o.subhead("Samba")
		if f.isFile("/etc/samba/smb.conf") {
			o.raw(f.grep(f.read("/etc/samba/smb.conf"), reNoteSemiBlank, false))
		}
	}
	if f.isDir("/etc/openmediavault") {
		o.subhead("OpenMediaVault")
		o.kv("OMV", "installed")
		if f.isFile("/etc/openmediavault/config.xml") {
			size := ""
			if b, err := s.m.sys.ReadFile("/etc/openmediavault/config.xml"); err == nil {
				size = strconv.Itoa(len(b))
			}
			o.kv("config.xml", size+" bytes")
		}
	}
	if f.isDir("/etc/openvpn") {
		o.subhead("OpenVPN")
		o.raw(head(f.find("/etc/openvpn", query{maxDepth: -1, names: []string{"*.conf", "*.ovpn"}}), 20))
		if f.isFile("/etc/openvpn/mullvad.conf") {
			o.item("Mullvad VPN: configured")
		}
	}
	if f.isDir("/etc/wireguard") {
		o.subhead("WireGuard")
		o.raw(f.lsPaths(f.glob("/etc/wireguard/*.conf")...))
	}
	if f.isDir("/etc/docker") {
		// The heading waits for the next line, whatever it is (SYS-17).
		o.subhead("Docker")
		if f.isFile("/etc/docker/daemon.json") {
			o.rawBytes(f.read("/etc/docker/daemon.json"))
		}
	}
	if f.isDir("/etc/containerd") {
		o.kv("containerd", "present")
	}
	if f.isDir("/etc/nvidia-container-runtime") {
		o.kv("NVIDIA Container Runtime", "present")
	}
	if f.isDir("/etc/collectd") {
		o.subhead("Collectd")
		o.raw(f.basenames("/etc/collectd/collectd.conf.d", ".conf", true))
	}
	if f.isDir("/etc/monit") {
		o.subhead("Monit")
		o.raw(f.ls("/etc/monit/conf.d"))
		o.raw(f.ls("/etc/monit/conf-available"))
	}
	if f.isDir("/etc/snmp") {
		o.kv("SNMP", "present")
	}
	if f.isDir("/etc/sysstat") {
		o.kv("sysstat", "present")
	}
	if f.isDir("/etc/webmin") {
		o.subhead("Webmin")
		o.raw(head(f.ls("/etc/webmin"), 20))
		if f.isDir("/etc/webmin/virtual-server") {
			o.item("Virtualmin: installed")
		}
	}
	if f.isDir("/etc/usermin") {
		o.kv("Usermin", "installed")
	}
}

// system is __sysinfo_collect_system.
func (s *Session) system(o *out) {
	f := s.f
	o.section("SYSTEM")
	o.subhead("Cron")
	o.raw(f.grep(f.read("/etc/crontab"), reNoteBlank, false))
	for _, d := range []string{"cron.d", "cron.daily", "cron.hourly", "cron.weekly", "cron.monthly"} {
		o.item(d + ":")
		o.raw(f.ls("/etc/" + d))
	}

	if f.isDir("/etc/letsencrypt") {
		o.subhead("Let's Encrypt")
		o.raw(s.sudoLs("/etc/letsencrypt/live"))
		var names []string
		for _, p := range s.sudoFind("/etc/letsencrypt/renewal", query{maxDepth: -1, names: []string{"*.conf"}}) {
			names = append(names, basenameSuffix(p, ".conf"))
		}
		o.raw(names)
		if f.isFile("/etc/letsencrypt/cli.ini") {
			o.raw(f.grep(f.read("/etc/letsencrypt/cli.ini"), reNoteBlank, false))
		}
	}
	if f.isDir("/etc/ssl/virtualmin") {
		o.kv("Virtualmin SSL", strconv.Itoa(len(f.ls("/etc/ssl/virtualmin")))+" domains")
	}
	// The globs are the user's: unreadable for a normal user, so the
	// literal words reach sudo ls (SYS-19).
	var words []string
	for _, pat := range []string{"/etc/ssl/private/*.pem", "/etc/ssl/private/*.key", "/etc/ssl/private/*.crt"} {
		words = append(words, f.words(pat)...)
	}
	o.raw(head(s.sudoLsPaths(words...), 10))

	o.subhead("Kernel tuning")
	if f.isDir("/etc/sysctl.d") {
		for _, p := range f.find("/etc/sysctl.d", query{maxDepth: -1, names: []string{"*.conf"}}) {
			o.item("--- " + p + ":")
			o.raw(f.grep(f.read(p), reNoteBlank, false))
		}
	}
	o.raw(f.grep(f.read("/etc/sysctl.conf"), reNoteBlank, false))
	o.subhead("GRUB")
	if f.isFile("/etc/default/grub") {
		o.raw(f.grep(f.read("/etc/default/grub"), reNoteBlank, false))
	}
	o.subhead("modprobe")
	for _, p := range f.find("/etc/modprobe.d", query{maxDepth: -1, names: []string{"*.conf"}}) {
		o.item("--- " + p + ":")
		o.rawBytes(f.read(p))
	}
	o.subhead("Logrotate")
	o.raw(f.ls("/etc/logrotate.d"))
	o.subhead("Service defaults")
	o.raw(f.ls("/etc/default"))
	o.subhead("Profile")
	o.raw(f.ls("/etc/profile.d"))
	o.rawBytes(f.read("/etc/environment"))
	o.item("update-motd.d:")
	o.raw(f.ls("/etc/update-motd.d"))
	if f.isDir("/etc/cloud") {
		o.subhead("Cloud")
		if f.isFile("/etc/cloud/cloud.cfg") {
			o.raw(f.grep(f.read("/etc/cloud/cloud.cfg"), regexp.MustCompile(`^(distro|system_info)`), true))
		}
	}
	if f.isDir("/etc/chrony") {
		o.subhead("Chrony")
		if f.isFile("/etc/chrony/chrony.conf") {
			o.raw(head(f.grep(f.read("/etc/chrony/chrony.conf"), reNoteBlank, false), 10))
		}
	}
	if f.isDir("/etc/.git") {
		o.kv("etckeeper", "git tracked")
	}
	if f.isFile("/etc/.etckeeper") {
		o.kv("etckeeper metadata", "present")
	}
}
