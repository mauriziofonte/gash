// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sysinfo

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
)

// tree is a fixture: relative path to content; a value starting with
// "->" is a symlink, a key ending in "/" a directory.
type tree map[string]string

// etcTree is a server's /etc with one or two entries per probe; the
// expected outputs of the tests are those of 2.x over this tree.
var etcTree = tree{
	"etc/os-release":         "PRETTY_NAME=\"Debian GNU/Linux 12 (bookworm)\"\nNAME=\"Debian GNU/Linux\"\nVERSION_ID=\"12\"\nID=debian\n",
	"etc/debian_version":     "12.5\n",
	"etc/machine-id":         "0123456789abcdef\n",
	"etc/hetzner-build":      "",
	"etc/fstab":              "# comment\n\nUUID=abc / ext4 defaults 0 1\n",
	"etc/crypttab":           "# c\ncryptroot UUID=x none luks\n\n",
	"etc/lvm/":               "",
	"etc/mdadm/mdadm.conf":   "# x\nARRAY /dev/md0 metadata=1.2\nMAILADDR root\n",
	"etc/zfs/zed.rc":         "# zed\nZED_EMAIL_ADDR=\"root\"\n",
	"etc/smartd.conf":        "# s\nDEVICESCAN -a\n",
	"etc/exports":            "# e\n/srv *(rw)\n",
	"etc/nfs.conf":           "[nfsd]\n",
	"etc/apache2/ports.conf": "# p\nListen 80\n\n",
	"etc/apache2/sites-available/000-default.conf":            "<VirtualHost *:80>\n  ServerName example.com\n  DocumentRoot /var/www/html\n</VirtualHost>\n",
	"etc/apache2/sites-enabled/000-default.conf":              "->../sites-available/000-default.conf",
	"etc/apache2/mods-enabled/rewrite.load":                   "LoadModule",
	"etc/apache2/mods-enabled/ssl.conf":                       "x",
	"etc/nginx/sites-enabled/site":                            "server {\n  server_name example.org;\n  root /srv;\n}\n",
	"etc/nginx/sites-enabled/linked":                          "->/etc/nginx/sites-enabled/site",
	"etc/php/8.2/fpm/pool.d/www.conf":                         "[www]\nuser = www-data\nlisten = /run/php.sock\npm = dynamic\npm.max_children = 5\n",
	"etc/php/8.2/mods-available/opcache.ini":                  "",
	"etc/php/8.2/mods-available/curl.ini":                     "",
	"etc/php/8.3/":                                            "",
	"etc/mysql/mariadb.conf.d/50-server.cnf":                  "[mysqld]\nbind-address = 127.0.0.1\n# x\nmax_connections = 100\nuser = mysql\n",
	"etc/postgresql/16/main/postgresql.conf":                  "x",
	"etc/postfix/main.cf":                                     "# c\nmyhostname = mail.example.com\nalias_maps = hash:/etc/aliases\n",
	"etc/dovecot/dovecot.conf":                                "# d\n!include conf.d/*.conf\n",
	"etc/dovecot/conf.d/10-mail.conf":                         "mail_location = maildir:~/Maildir\n#x\n",
	"etc/bind/named.conf.local":                               "zone \"example.com\" {\n  type master;\n};\n",
	"etc/unbound/a.conf":                                      "server:\n  verbosity: 1",
	"etc/unbound/b.conf":                                      "  port: 53\n",
	"etc/samba/smb.conf":                                      "[global]\n; c\n  workgroup = WORKGROUP\n",
	"etc/openmediavault/config.xml":                           "<config/>\n",
	"etc/openvpn/client.conf":                                 "client\n",
	"etc/openvpn/sub/site.ovpn":                               "x",
	"etc/docker/":                                             "",
	"etc/containerd/":                                         "",
	"etc/collectd/collectd.conf.d/cpu.conf":                   "",
	"etc/webmin/miniserv.conf":                                "port=10000\n",
	"etc/webmin/virtual-server/":                              "",
	"etc/passwd":                                              "root:x:0:0:root:/root:/bin/bash\nalice:x:1000:1000:Alice:/home/alice:/bin/bash\nnobody:x:65534:65534:nobody:/nonexistent:/usr/sbin/nologin\nweird:x:abc:1:w:/w:/bin/sh\n",
	"etc/sudoers":                                             "# s\nDefaults env_reset\n\nroot ALL=(ALL:ALL) ALL\n",
	"etc/sudoers.d/alice":                                     "alice ALL=(ALL) NOPASSWD: ALL\n",
	"etc/ssh/sshd_config":                                     "# x\nPort 2222\n  PasswordAuthentication yes\nPasswordAuthentication no\nUsePAM yes\n",
	"etc/ssh/sshd_config.d/50-local.conf":                     "# l\nX11Forwarding no\n",
	"etc/resolv.conf":                                         "# r\nnameserver 1.1.1.1\n\nsearch example.com\n",
	"etc/hosts":                                               "127.0.0.1 localhost\n# x\n\n::1 localhost\n",
	"etc/netplan/01-net.yaml":                                 "network:\n  version: 2\n",
	"etc/fail2ban/jail.local":                                 "[sshd]\n; c\nenabled = true\n",
	"etc/fail2ban/jail.d/defaults.conf":                       "[DEFAULT]\nbantime = 1h\n",
	"etc/crowdsec/config.yaml":                                "x: 1\n",
	"etc/crowdsec/acquis.yaml":                                "source: file\nfilenames:\n",
	"etc/crowdsec/hub/scenarios/ssh-bf.yaml":                  "",
	"etc/ufw/ufw.conf":                                        "# u\nENABLED=yes\n",
	"etc/apparmor.d/":                                         "",
	"etc/systemd/system/custom.service":                       "[Unit]\nDescription=Custom\n",
	"etc/systemd/system/sshd.service":                         "->/lib/systemd/system/sshd.service",
	"etc/systemd/system/custom.service.d/override.conf":       "[Service]\nNice=5\n",
	"etc/systemd/system/multi-user.target.wants/cron.service": "->/lib/systemd/system/cron.service",
	"etc/boot.d/10-a":                                         "",
	"etc/boot.d/20-b":                                         "",
	"etc/default/saslauthd":                                   "# x\nSTART=yes\n",
	"etc/default/grub":                                        "# g\nGRUB_TIMEOUT=5\n",
	"etc/foo.org":                                             "x",
	"etc/crontab":                                             "# c\nSHELL=/bin/sh\n",
	"etc/cron.d/certbot":                                      "",
	"etc/cron.monthly/.placeholder":                           "",
	"etc/letsencrypt/live/example.com/cert.pem":               "x",
	"etc/letsencrypt/renewal/example.com.conf":                "x",
	"etc/letsencrypt/cli.ini":                                 "# c\nmax-log-backups = 0\n",
	"etc/ssl/virtualmin/a":                                    "",
	"etc/ssl/private/server.pem":                              "x",
	"etc/ssl/private/server.key":                              "x",
	"etc/sysctl.d/99-local.conf":                              "# c\nvm.swappiness = 10\n",
	"etc/modprobe.d/blacklist.conf":                           "blacklist pcspkr\n",
	"etc/logrotate.d/apache2":                                 "",
	"etc/profile.d/locale.sh":                                 "",
	"etc/environment":                                         "PATH=\"/usr/bin\"\n",
	"etc/cloud/cloud.cfg":                                     "distro: debian\nsystem_info:\n",
	"etc/chrony/chrony.conf":                                  "# c\npool 2.debian.pool.ntp.org iburst\n",
	"etc/.git/":                                               "",
}

func (tr tree) write(t *testing.T, root string) {
	t.Helper()
	for rel, body := range tr {
		p := filepath.Join(root, rel)
		if strings.HasSuffix(rel, "/") {
			require.NoError(t, os.MkdirAll(p, 0o755))
			continue
		}
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		if target, ok := strings.CutPrefix(body, "->"); ok {
			require.NoError(t, os.Symlink(target, p))
			continue
		}
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
}

// Tool stubs on PATH, with fixed answers; sudo logs its words to
// $SUDO_LOG, answers -n true with $SUDO_CACHED (1 by default), -v with
// $SUDO_V, and runs anything else.
var stubs = map[string]string{
	"hostname":            `case "$1" in -f) echo web1.example.com ;; -I) echo "10.0.0.5 172.17.0.1 " ;; *) echo web1 ;; esac`,
	"uname":               `case "$1" in -sr) echo "Linux 6.1.0" ;; -r) echo 6.1.0 ;; -m) echo x86_64 ;; -n) echo web1 ;; *) echo Linux ;; esac`,
	"uptime":              `echo "up 3 days, 2 hours"`,
	"df":                  `printf 'Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/vda1 104857600 52428800 52428800 50%% /\n'`,
	"getconf":             `echo 4`,
	"systemd-detect-virt": `echo "${MOCK_VIRT:-kvm}"; exit "${MOCK_VIRT_RC:-0}"`,
	"systemctl": `case "$1" in
list-units) printf '  cron.service loaded active running Cron\n  ssh.service  loaded active running SSH\n' ;;
list-timers) printf 'Mon 2026-10-19 00:00:00 UTC 3h left n/a n/a logrotate.timer logrotate.service\n' ;;
esac`,
	"ss": `printf 'State Recv-Q Send-Q Local Peer\nLISTEN 0 128 0.0.0.0:22 0.0.0.0:*\n'`,
	"sudo": `printf 'sudo %s\n' "$*" >>"$SUDO_LOG"
case "$*" in "-n true") exit "${SUDO_CACHED:-1}" ;; -v) exit "${SUDO_V:-1}" ;; esac
exec "$@"`,
}

type fixture struct {
	root, bin string
	env       map[string]string
	m         *Module
	uid       int
}

func newFixture(t *testing.T, trees ...tree) *fixture {
	t.Helper()
	root := t.TempDir()
	for _, tr := range trees {
		tr.write(t, root)
	}
	require.NoError(t, os.MkdirAll(filepath.Join(root, "etc"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "proc"), 0o755))
	bin := t.TempDir()
	for name, body := range stubs {
		require.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755))
	}
	for _, tool := range []string{"cat", "find", "ls"} {
		for _, dir := range []string{"/usr/bin", "/bin"} {
			if _, err := os.Stat(filepath.Join(dir, tool)); err == nil {
				require.NoError(t, os.Symlink(filepath.Join(dir, tool), filepath.Join(bin, tool)))
				break
			}
		}
	}
	sr, err := platform.NewSysroot(root)
	require.NoError(t, err)
	fx := &fixture{root: root, bin: bin, uid: 0, env: map[string]string{
		"PATH": bin, "LANG": "C.UTF-8", "HOME": root, "SUDO_LOG": filepath.Join(bin, "sudo.log"),
	}}
	fx.m = &Module{run: runner.Exec{}, sys: sr, uid: func() int { return fx.uid }}
	return fx
}

type result struct {
	out, err string
	rc       int
}

// call runs sysinfo ARGS on tt (a plain pipe when nil).
func (fx *fixture) call(t *testing.T, tt *term.Term, args ...string) result {
	t.Helper()
	e := invoke.EnvMap(fx.env)
	if tt == nil {
		tt = term.Fixed(e, false, false, false)
	}
	var out, errb bytes.Buffer
	c := &invoke.Call{Name: "sysinfo", Args: args, Env: e, Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb, Term: tt}
	tbl := invoke.NewTable()
	fx.m.Register(tbl)
	rc := tbl.Run(c)
	return result{strings.ReplaceAll(out.String(), fx.root, "R"), strings.ReplaceAll(errb.String(), fx.root, "R"), rc}
}

// sudo is the sudo log, root paths as R.
func (fx *fixture) sudo(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(fx.env["SUDO_LOG"])
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return strings.ReplaceAll(string(b), fx.root, "R")
}

// TestDumpFixture writes etcTree to $SYSINFO_FIXTURE for a 2.x run (a
// development aid; skipped otherwise).
func TestDumpFixture(t *testing.T) {
	dir := os.Getenv("SYSINFO_FIXTURE")
	if dir == "" {
		t.Skip("SYSINFO_FIXTURE not set")
	}
	etcTree.write(t, dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "bin"), 0o755))
	for name, body := range stubs {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "bin", name), []byte("#!/bin/sh\n"+body+"\n"), 0o755))
	}
}

// stub replaces (or adds) the tool NAME on PATH with a shell body.
func (fx *fixture) stub(t *testing.T, name, body string) {
	t.Helper()
	p := filepath.Join(fx.bin, name)
	_ = os.Remove(p)
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755))
}
