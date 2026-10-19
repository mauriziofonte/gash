// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sysinfo

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// deepTree is a server for the drill-downs: /etc with one or two entries
// per probe, logs, a crontab spool, /opt files a unit names and the
// block queue schedulers.
var deepTree = tree{
	"etc/apache2/ports.conf":                     "# p\nListen 80\n\n",
	"etc/apache2/mods-enabled/rewrite.load":      "x",
	"etc/apache2/mods-enabled/ssl.load":          "x",
	"etc/apache2/mods-enabled/ssl.conf":          "x",
	"etc/apache2/sites-enabled/000-default.conf": "<VirtualHost *:80>\n  # comment\n  ServerName example.com\n\n</VirtualHost>\n",
	"etc/apache2/sites-enabled/notes.txt":        "x\n",
	"etc/nginx/sites-enabled/site":               "server {\n    # c\n  listen 80;\n}",
	"etc/nginx/sites-enabled/link":               "->site",
	"etc/nginx/sites-enabled/sub/":               "",
	"etc/lighttpd/lighttpd.conf":                 "server.port = 80\n#x\n",
	"etc/lighttpd/conf-enabled/10-fastcgi.conf":  "",
	"etc/php/8.2/fpm/pool.d/www.conf":            "[www]\n; comment\nuser = www-data\n\npm = dynamic\n",
	"etc/php/8.2/cli/php.ini":                    "memory_limit = 128M\n;x\ndisplay_errors = Off\nfoo=1\n",
	"etc/php/8.2/mods-available/opcache.ini":     "",
	"etc/php/8.2/mods-available/curl.ini":        "",
	"etc/php/8.3/":                               "",
	"etc/mysql/mariadb.conf.d/50-server.cnf":     "[mysqld]\n# c\nbind-address = 127.0.0.1\n",
	"etc/mysql/conf.d/my.cnf":                    "[client]\n",
	"etc/postgresql/16/main/postgresql.conf":     "port = 5432\n#x\n",
	"etc/postgresql/16/main/pg_hba.conf":         "local all all peer\n",
	"etc/redis/redis.conf":                       "bind 127.0.0.1\n# x\n",
	"etc/postfix/main.cf":                        "myhostname = mail\n",
	"etc/postfix/master.cf":                      "smtp inet n - y - - smtpd\n",
	"etc/dovecot/dovecot.conf":                   "protocols = imap\n",
	"etc/dovecot/conf.d/10-mail.conf":            "mail_location = maildir:~/Maildir\n",
	"etc/dovecot/conf.d/a/b.conf":                "x = 1\n",
	"etc/dovecot/conf.d/a.conf":                  "# only\n",
	"etc/dovecot/conf.d/B.conf":                  "y = 2\n",
	"etc/exim4/update-exim4.conf.conf":           "dc_eximconfig_configtype='internet'\n",
	"etc/spamassassin/local.cf":                  "required_score 5.0\n",
	"etc/spamassassin/razor/":                    "",
	"etc/default/opendkim":                       "SOCKET=local:/run/opendkim.sock\n",
	"etc/opendkim/TrustedHosts":                  "127.0.0.1\nlocalhost",
	"etc/opendkim/KeyTable":                      "mail._domainkey.example.com example.com:mail:/etc/opendkim/keys/mail.private\n",
	"etc/bind/named.conf.local":                  "zone \"example.com\" {\n  type master;\n};\n",
	"etc/bind/named.conf.options":                "// c\noptions {\n\n  directory \"/var/cache/bind\";\n};\n",
	"etc/unbound/unbound.conf":                   "server:\n  verbosity: 1\n",
	"etc/unbound/unbound.conf.d/a.conf":          "  port: 53",
	"etc/avahi/avahi-daemon.conf":                "[server]\nuse-ipv4=yes\n",
	"etc/proftpd/proftpd.conf":                   "ServerName \"ftp\"\n",
	"etc/proftpd/conf.d/tls.conf":                "TLSEngine on\n#x\n",
	"etc/vsftpd.conf":                            "listen=YES\n#x\n",
	"etc/samba/smb.conf":                         "[global]\n; c\n  workgroup = WORKGROUP\n",
	"etc/openmediavault/config.xml":              "<config/>\n",
	"etc/exports":                                "# e\n/srv *(rw)\n",
	"etc/nfs.conf":                               "[nfsd]\n#x\n",
	"etc/openvpn/client.conf":                    "client\n# c\nremote vpn.example.com 1194\n",
	"etc/openvpn/sub/site.ovpn":                  "dev tun\n",
	"etc/openvpn/mullvad.conf":                   "x\n",
	"etc/wireguard/wg0.conf":                     "[Interface]\nPrivateKey = abc123\nPrivateKey=nospace\nAddress = 10.0.0.1/24\n  PrivateKey = indented\n",
	"etc/docker/daemon.json":                     `{"log-driver":"json-file"}`,
	"etc/containerd/config.toml":                 "version = 2\n# c\n",
	"etc/nvidia-container-runtime/config.toml":   "disable-require = false\n",
	"etc/collectd/collectd.conf":                 "LoadPlugin cpu\n",
	"etc/collectd/collectd.conf.d/cpu.conf":      "<Plugin cpu>",
	"etc/monit/conf.d/nginx":                     "check process nginx\n",
	"etc/monit/conf-available/apache":            "check process apache\n",
	"etc/snmp/snmpd.conf":                        "rocommunity public\n",
	"etc/sysstat/":                               "",
	"etc/default/sysstat":                        "ENABLED=\"true\"\n",
	"etc/smartd.conf":                            "DEVICESCAN\n",
	"etc/webmin/miniserv.conf":                   "port=10000\nssl=1\nroot=/usr/share/webmin\n",
	"etc/webmin/virtual-server/":                 "",
	"etc/usermin/":                               "",
	"etc/chrony/chrony.conf":                     "pool 2.debian.pool.ntp.org iburst\n",
	"etc/systemd/timesyncd.conf":                 "[Time]\nNTP=ntp.example.com\n#x\n",
	"etc/systemd/system/custom.service": "[Service]\nEnvironmentFile=-/etc/default/custom\nEnvironmentFile=/etc/missing\nEnvironmentFile=\n" +
		"ExecStart=/opt/app/bin/app --config /opt/app/app.yaml --other /etc/app.conf\nWorkingDirectory=/opt/app\nWorkingDirectory=\n",
	"etc/systemd/system/backup.timer":                   "[Timer]\nOnCalendar=daily",
	"etc/systemd/system/sshd.service":                   "->/lib/systemd/system/ssh.service",
	"etc/systemd/system/custom.service.d/override.conf": "[Service]\nRestart=always\n",
	"etc/default/custom":                                "PORT=8080\n",
	"etc/app.conf":                                      "x=1\n",
	"opt/app/app.yaml":                                  "listen: 8080\n",
	"etc/pam.d/common-auth":                             "auth required pam_unix.so\n#x\n",
	"etc/passwd":                                        "root:x:0:0:root:/root:/bin/bash\nalice:x:1000:1000::/home/alice:/bin/bash\n",
	"etc/sudoers":                                       "Defaults env_reset\n#x\n",
	"etc/sudoers.d/alice":                               "alice ALL=(ALL) NOPASSWD: ALL\n",
	"etc/ssh/sshd_config":                               "Port 2222\n#x\n",
	"etc/ssh/sshd_config.d/50-local.conf":               "X11Forwarding no\n",
	"etc/ufw/ufw.conf":                                  "ENABLED=yes\n",
	"etc/fail2ban/jail.local":                           "[sshd]\n; c\nenabled = true\n",
	"etc/fail2ban/jail.d/defaults.conf":                 "[DEFAULT]\nbantime = 1h\n",
	"etc/crowdsec/config.yaml":                          "common:\n  daemonize: true\n",
	"etc/crowdsec/acquis.yaml":                          "source: file\n",
	"etc/crowdsec/hub/scenarios/ssh-bf.yaml":            "",
	"etc/apparmor.d/usr.sbin.nginx":                     "",
	"etc/login.defs":                                    "PASS_MAX_DAYS 99999\nUMASK 022\nOTHER 1\n",
	"etc/resolv.conf":                                   "nameserver 1.1.1.1\n",
	"etc/hosts":                                         "127.0.0.1 localhost\n",
	"etc/netplan/01.yaml":                               "network:\n  version: 2\n",
	"etc/network/interfaces":                            "auto lo\n#x\n",
	"etc/dhcpcd.conf":                                   "hostname\n",
	"etc/fstab":                                         "UUID=abc / ext4 defaults 0 1\n",
	"etc/crypttab":                                      "cryptroot UUID=x none luks\n",
	"etc/mdadm/mdadm.conf":                              "ARRAY /dev/md0\n#x\n",
	"etc/zfs/zed.rc":                                    "ZED_EMAIL_ADDR=\"root\"\n",
	"etc/sanoid/sanoid.conf":                            "[tank]\n",
	"etc/hdparm.conf":                                   "/dev/sda {\n}\n",
	"etc/sysctl.d/99-local.conf":                        "vm.swappiness = 10\n",
	"etc/sysctl.d/B.conf":                               "x = 1\n",
	"etc/sysctl.conf":                                   "#c\nnet.ipv4.ip_forward=1\n",
	"etc/crontab":                                       "SHELL=/bin/sh\n#x\n",
	"etc/cron.d/certbot":                                "0 */12 * * * root certbot -q renew\n",
	"etc/cron.daily/logrotate":                          "",
	"etc/cron.weekly/":                                  "",
	"etc/logrotate.d/apache2":                           "/var/log/apache2/*.log {\n  daily\n}\n",
	"etc/letsencrypt/live/example.com/cert.pem":         "CERT\n",
	"etc/letsencrypt/renewal/example.com.conf":          "version = 2.1\n",
	"etc/letsencrypt/cli.ini":                           "max-log-backups = 0\n",
	"etc/apt/apt.conf.d/50unattended-upgrades":          "// c\nUnattended-Upgrade::Origins-Pattern {\n};\n",
	"etc/foo.conf.dpkg-old":                             "",
	"etc/apt/x.dpkg-dist":                               "",
	"etc/hosts.org":                                     "",
	"etc/.git/":                                         "",
	"etc/cloud/cloud.cfg":                               "distro: debian\nmanage_etc_hosts: true\nfoo: 1\n",
	"etc/profile.d/locale.sh":                           "",
	"etc/environment":                                   "PATH=\"/usr/bin\"\n",
	"etc/default/grub":                                  "",
	"var/log/dpkg.log":                                  "1 install a\n2 install b\n3 status c",
	"var/log/auth.log":                                  numbered(35, "sshd[%d]: Accepted publickey for alice"),
	"var/spool/cron/crontabs/alice":                     "",
	"sys/block/sda/queue/scheduler":                     "[mq-deadline] none\n",
	"sys/block/vda/queue/scheduler":                     "none\n",
}

// numbered is N lines of format, each with its number.
func numbered(n int, format string) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, format+"\n", i)
	}
	return b.String()
}

// deepStubs are the tools of the drill-downs, with fixed answers (POSIX
// sh builtins only).
var deepStubs = map[string]string{
	"ip": `case "$1 $2" in
"addr show") printf '1: lo: <LOOPBACK>\n    inet 127.0.0.1/8 scope host lo\n2: eth0: <UP>\n    inet 10.0.0.5/24\n' ;;
"route show") echo "default via 10.0.0.1 dev eth0" ;;
"neigh show") echo "10.0.0.1 dev eth0 lladdr aa:bb:cc:dd:ee:ff REACHABLE" ;;
esac`,
	"resolvectl": `printf 'Global\n  Protocols: +LLMNR'`,
	"df":         `[ "$1" = -hT ] && printf 'Filesystem Type Size Used Avail Use%% Mounted on\n/dev/vda1 ext4 100G 50G 50G 50%% /\n'`,
	"lsblk":      `printf 'NAME FSTYPE LABEL UUID\nvda\n` + "`" + `-vda1 ext4 root abc\n'`,
	"mount":      `printf 'proc on /proc type proc (rw)\ncgroup2 on /sys/fs/cgroup type cgroup2 (rw)\n/dev/vda1 on / type ext4 (rw)\n'`,
	"pvs":        `echo "  PV /dev/vda2 VG vg0"`,
	"vgs":        `echo "  VG vg0"`,
	"lvs":        `echo "  LV root vg0"`,
	"zpool":      `printf '  pool: tank\n state: ONLINE\n'`,
	"zfs":        `printf 'NAME USED\ntank 1G\n'`,
	"smartctl":   `echo "SMART overall-health: PASSED"`,
	"free":       `printf '               total        used\nMem:            15Gi        4Gi\n'`,
	"lscpu":      `printf 'Architecture: x86_64\nCPU(s): 4\nModel name: Xeon\nThread(s) per core: 2\nFlags: fpu\nCPU MHz: 2200.000\nSocket LC=%s\n' "$LC_ALL"`,
	"ps": `i=0; echo "USER PID %CPU %MEM COMMAND"
while [ $i -lt 20 ]; do i=$((i+1)); echo "root $i 0.$i 1.$i proc$i $2"; done`,
	"sysctl":          `printf 'fs.file-max = 9223372036854775807\nkernel.x = 1\nvm.swappiness = 60\nnet.core.somaxconn = 4096\n'`,
	"docker":          `[ "$1" = info ] && printf 'Client:\n Version: 24\nServer:\n Containers: 3\n'`,
	"last":            `printf 'alice pts/0 10.0.0.2 Mon Oct 19 10:00 still logged in\n\nwtmp begins Mon Oct  1 00:00:00 2026\n'`,
	"apt":             `printf 'Listing...\nbash/stable 5.2 amd64 [upgradable from: 5.1]\n'`,
	"git":             `echo "abc123 daily autocommit"`,
	"iptables":        `echo "Chain INPUT (policy ACCEPT 0 packets, 0 bytes)"`,
	"ufw":             `echo "Status: active"`,
	"firewall-cmd":    `echo "public (active)"`,
	"fail2ban-client": `printf 'Status\n|- Number of jail: 1\n'`,
	"aa-status":       `echo "apparmor module is loaded."`,
	"openssl":         `echo "notAfter=Jan  1 00:00:00 2027 GMT"`,
}

// deepFixture is a fixture over deepTree with the drill-down tools.
func deepFixture(t *testing.T, trees ...tree) *fixture {
	t.Helper()
	fx := newFixture(t, append([]tree{deepTree}, trees...)...)
	for name, body := range deepStubs {
		fx.stub(t, name, body)
	}
	return fx
}

// deep runs the drill-down NAME as ai_sysinfo would (a plain pipe).
func (fx *fixture) deep(t *testing.T, name string) string {
	t.Helper()
	e := invoke.EnvMap(fx.env)
	var w, errb bytes.Buffer
	c := &invoke.Call{Name: "ai_sysinfo", Env: e, Stdin: strings.NewReader(""), Stdout: &w, Stderr: &errb, Term: term.Fixed(e, false, false, false)}
	s := fx.m.Begin(c, true, false)
	defer s.Release()
	require.True(t, s.Deep(&w, name))
	require.Empty(t, errb.String())
	return strings.ReplaceAll(w.String(), fx.root, "R")
}

// TestDumpDeepFixture writes deepTree and the tools to
// $SYSINFO_DEEP_FIXTURE for a 2.x run (a development aid).
func TestDumpDeepFixture(t *testing.T) {
	dir := os.Getenv("SYSINFO_DEEP_FIXTURE")
	if dir == "" {
		t.Skip("SYSINFO_DEEP_FIXTURE not set")
	}
	deepTree.write(t, dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "bin"), 0o755))
	for _, m := range []map[string]string{stubs, deepStubs} {
		for name, body := range m {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "bin", name), []byte("#!/bin/sh\n"+body+"\n"), 0o755))
		}
	}
}

// TestDeepPrint prints the drill-downs over deepTree (a development aid).
func TestDeepPrint(t *testing.T) {
	dir := os.Getenv("SYSINFO_DEEP_OUT")
	if dir == "" {
		t.Skip("SYSINFO_DEEP_OUT not set")
	}
	fx := deepFixture(t)
	for _, n := range DeepSections {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "deep."+n+".go"), []byte(fx.deep(t, n)), 0o644))
	}
}

// want is a pinned drill-down of testdata.
func want(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name+".txt"))
	require.NoError(t, err)
	return string(b)
}

// The 2.x bytes of the drill-downs over deepTree (testdata/deep.*.txt),
// but for the find order, which is byte order here (SYS-41: 2.x listed
// openvpn's sub/site.ovpn and sysctl.d's B.conf in directory order).
// Root reads natively.
func TestDeep(t *testing.T) {
	fx := deepFixture(t)
	for _, n := range DeepSections {
		require.Equal(t, want(t, "deep."+n), fx.deep(t, n), n)
	}
	require.Empty(t, fx.sudo(t))
}

// With cached credentials the privileged reads go through sudo, with the
// same output.
func TestDeepElevated(t *testing.T) {
	fx := deepFixture(t)
	require.NoError(t, os.Symlink(lookTool(t, "tail"), filepath.Join(fx.bin, "tail")))
	fx.uid = 1000
	fx.env["SUDO_CACHED"] = "0"
	for _, n := range DeepSections {
		require.Equal(t, want(t, "deep."+n), fx.deep(t, n), n)
	}
	require.Equal(t, strings.Join([]string{
		// Begin, once per drill-down.
		"sudo -n true",
		"sudo -n true",
		"sudo iptables -L -n -v",
		"sudo ufw status verbose",
		"sudo firewall-cmd --list-all",
		"sudo fail2ban-client status",
		"sudo cat R/etc/fail2ban/jail.local",
		"sudo find R/etc/fail2ban/jail.d -name *.conf",
		"sudo cat R/etc/fail2ban/jail.d/defaults.conf",
		"sudo cat R/etc/ssh/sshd_config",
		"sudo find R/etc/ssh/sshd_config.d -name *.conf",
		"sudo cat R/etc/ssh/sshd_config.d/50-local.conf",
		"sudo aa-status",
		"sudo cat R/etc/sudoers",
		"sudo find R/etc/sudoers.d -type f",
		"sudo cat R/etc/sudoers.d/alice",
		"sudo tail -30 R/var/log/auth.log",
		"sudo -n true",
		"sudo -n true",
		"sudo cat R/etc/crypttab",
		"sudo -n true",
		"sudo -n true",
		"sudo ls R/var/spool/cron/crontabs/",
		"sudo ls R/etc/letsencrypt/live/",
		"sudo find R/etc/letsencrypt/live -name cert.pem",
		"sudo openssl x509 -enddate -noout -in R/etc/letsencrypt/live/example.com/cert.pem",
		"sudo find R/etc/letsencrypt/renewal -name *.conf",
		"sudo cat R/etc/letsencrypt/renewal/example.com.conf",
	}, "\n")+"\n", fx.sudo(t))
}

// lookTool is the path of a system tool for a fixture's PATH.
func lookTool(t *testing.T, name string) string {
	t.Helper()
	for _, dir := range []string{"/usr/bin", "/bin"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return filepath.Join(dir, name)
		}
	}
	t.Skip(name + " not installed")
	return ""
}

// Nothing to find and no tools: the headers only, as 2.x printed them
// (testdata/bare.*.txt), which sysinfo_spec checks at lines 607 to 689.
func TestDeepBare(t *testing.T) {
	fx := newFixture(t)
	for name := range stubs {
		if name != "sudo" {
			require.NoError(t, os.Remove(filepath.Join(fx.bin, name)))
		}
	}
	for _, n := range DeepSections {
		require.Equal(t, want(t, "bare."+n), fx.deep(t, n), n)
	}
	for n, heads := range map[string][]string{
		"services": {"=== SERVICES DEEP ==="}, "security": {"=== SECURITY DEEP ==="},
		"network":     {"=== NETWORK DEEP ===", "--- Interfaces", "--- Routes"},
		"storage":     {"=== STORAGE DEEP ===", "--- Disk usage", "--- fstab"},
		"performance": {"=== PERFORMANCE DEEP ===", "--- Memory", "--- CPU", "--- Load"},
		"maintenance": {"=== MAINTENANCE DEEP ===", "--- crontab", "--- Upgradable packages"},
	} {
		for _, h := range heads {
			require.Contains(t, fx.deep(t, n), h+"\n", n)
		}
	}
}

// The probes deepTree leaves out: /proc/mdstat (head -30), a missing
// unit config, a relative EnvironmentFile, a unit read through a symlink.
func TestDeepProbes(t *testing.T) {
	fx := deepFixture(t, tree{
		"proc/mdstat":                      numbered(35, "md%d : active raid1"),
		"etc/systemd/system/rel.service":   "EnvironmentFile=-rel.env\nExecStart=/etc/missing.conf\n",
		"etc/systemd/system/z.service":     "->/etc/real.service",
		"etc/real.service":                 "x\n",
		"etc/wireguard/wg1.conf":           "PrivateKey = a\n",
		"etc/wireguard/notes":              "PrivateKey = b\n",
		"etc/nginx/sites-enabled/.hidden":  "server {}\n",
		"etc/apache2/mods-enabled/x.load/": "",
	})
	storage := fx.deep(t, "storage")
	require.Contains(t, storage, "--- mdstat\n"+numbered(30, "md%d : active raid1")+"--- ZFS pools\n")
	services := fx.deep(t, "services")
	require.Contains(t, services, "--- Custom: rel.service\nEnvironmentFile=-rel.env\nExecStart=/etc/missing.conf\n--- Custom: backup.timer\n")
	require.NotContains(t, services, "z.service")
	require.Contains(t, services, "--- wg1.conf\nPrivateKey = [REDACTED]\n--- Docker\n")
	require.NotContains(t, services, ".hidden")
	require.Contains(t, services, "--- mods-enabled\nrewrite\nssl\nx\n")

	require.False(t, (&Session{}).Deep(&bytes.Buffer{}, "bogus"))
}

// Sudo is __sysinfo_sudo for ai_sysinfo's own probes.
func TestSudoTool(t *testing.T) {
	fx := deepFixture(t)
	e := invoke.EnvMap(fx.env)
	c := &invoke.Call{Name: "ai_sysinfo", Env: e, Stdin: strings.NewReader(""), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Term: term.Fixed(e, false, false, false)}
	s := fx.m.Begin(c, true, false)
	require.Equal(t, "Status: active\n", string(s.Sudo("ufw", "status")))
	require.Nil(t, s.Sudo("no-such-tool"))
	s.Release()
	require.Empty(t, fx.sudo(t))

	fx.uid = 1000
	fx.env["SUDO_CACHED"] = "0"
	c.Env = invoke.EnvMap(fx.env)
	s = fx.m.Begin(c, true, false)
	defer s.Release()
	require.Equal(t, "Status: active\n", string(s.Sudo("ufw", "status")))
	require.Equal(t, "sudo -n true\nsudo ufw status\n", fx.sudo(t))
}

func TestByteTools(t *testing.T) {
	for _, c := range []struct {
		in       string
		n        int
		head, tl string
	}{
		{"a\nb\nc\n", 2, "a\nb\n", "b\nc\n"},
		{"a\nb\nc", 2, "a\nb\n", "b\nc"},
		{"a\nb", 5, "a\nb", "a\nb"},
		{"", 3, "", ""},
		{"a\n", 0, "", ""},
		{"\n\n", 1, "\n", "\n"},
	} {
		require.Equal(t, c.head, string(headN([]byte(c.in), c.n)), c)
		require.Equal(t, c.tl, string(tailN([]byte(c.in), c.n)), c)
	}
	require.Equal(t, "x\n", string(skipLine([]byte("h\nx\n"))))
	require.Empty(t, skipLine([]byte("h")))
	require.Equal(t, []string{"a", "-b"}, grepOAll([]string{"E=a", "E=", "E=-b", "x"}, regexp.MustCompile(`^E=(.*)`)))
	require.Equal(t, []string{"A=1", "LC_ALL=C"}, withEnv([]string{"LC_ALL=x", "A=1", "LC_ALLX=2"}[:2], "LC_ALL=C"))

	// tail reads from the end, whatever the size.
	fx := newFixture(t)
	big := numbered(100000, "line %d")
	require.NoError(t, os.WriteFile(filepath.Join(fx.root, "big"), []byte(big), 0o644))
	require.Equal(t, string(tailN([]byte(big), 30)), string(files{sys: fx.m.sys}.tail("/big", 30)))
	require.Equal(t, big, string(files{sys: fx.m.sys}.tail("/big", 200000)))
	require.Nil(t, files{sys: fx.m.sys}.tail("/missing", 3))
}
