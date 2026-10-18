// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sysinfo

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The 2.x outputs over etcTree (as root, a plain pipe), but for the
// listing order, which is byte order here (SYS-41) and directory order
// in 2.x.
var llmWant = map[string]string{
	"identity": `=== IDENTITY ===
OS=Debian GNU/Linux 12 (bookworm)
OS ID=debian
Version=12
Debian Version=12.5
Kernel=6.1.0
Arch=x86_64
Hostname=web1.example.com
Machine-ID=0123456789abcdef
Uptime=up 3 days, 2 hours
Virtualization=kvm
Cloud=Hetzner
`,
	"storage": `=== STORAGE ===
--- fstab
UUID=abc / ext4 defaults 0 1
--- crypttab
cryptroot UUID=x none luks

LVM=present
--- mdadm
ARRAY /dev/md0 metadata=1.2
ZFS=present
--- zed.rc
ZED_EMAIL_ADDR="root"
--- smartd
DEVICESCAN -a
--- NFS exports
/srv *(rw)
NFS conf=present
`,
	"services": `=== SERVICES ===
--- Detected (by /etc/ dirs)
/etc/apache2/
/etc/nginx/
/etc/php/
/etc/mysql/
/etc/postgresql/
/etc/postfix/
/etc/dovecot/
/etc/fail2ban/
/etc/crowdsec/
/etc/ufw/
/etc/apparmor.d/
/etc/bind/
/etc/unbound/
/etc/docker/
/etc/containerd/
/etc/openmediavault/
/etc/samba/
/etc/openvpn/
/etc/webmin/
/etc/collectd/
/etc/zfs/
/etc/letsencrypt/
/etc/chrony/
/etc/cloud/
/etc/netplan/
boot.d hooks: 10-a,20-b
--- saslauthd
START=yes
Legacy .org backups found
--- Running services
cron.service
ssh.service
--- Timers
logrotate.service
--- Listening ports
LISTEN 0 128 0.0.0.0:22 0.0.0.0:*
--- Custom systemd units
/etc/systemd/system/custom.service
/etc/systemd/system/sshd.service
/etc/systemd/system/custom.service.d/override.conf
--- Unit: custom.service
[Unit]
Description=Custom
--- multi-user.target.wants
cron.service
`,
	"auth": `=== AUTH ===
--- Users
alice:1000:/home/alice:/bin/bash
--- Sudo
Defaults env_reset
root ALL=(ALL:ALL) ALL
--- /etc/sudoers.d/alice:
alice ALL=(ALL) NOPASSWD: ALL
--- SSH
Port 2222
PasswordAuthentication no
--- /etc/ssh/sshd_config.d/50-local.conf:
X11Forwarding no
`,
	"network": `=== NETWORK ===
--- resolv.conf
nameserver 1.1.1.1

search example.com
--- hosts
127.0.0.1 localhost
::1 localhost
--- netplan
network:
  version: 2
`,
	"security": `=== SECURITY ===
--- fail2ban
[sshd]
enabled = true
--- /etc/fail2ban/jail.d/defaults.conf:
[DEFAULT]
bantime = 1h
--- crowdsec
CrowdSec=installed
source: file
ssh-bf.yaml
--- ufw
ENABLED=yes
AppArmor=present
`,
	"webstack": `=== WEBSTACK ===
--- Apache
ports.conf:
Listen 80
sites-enabled:
000-default.conf
  ServerName example.com
  DocumentRoot /var/www/html
mods-enabled:
rewrite
--- Nginx
sites-enabled:
linked
site
  server_name example.org;
  root /srv;
--- PHP
Versions:
/etc/php/8.2
/etc/php/8.3
FPM pools:
/etc/php/8.2/fpm/pool.d/www.conf
[www]
user = www-data
listen = /run/php.sock
pm = dynamic
pm.max_children = 5
Modules:
curl,opcache
--- MySQL/MariaDB
mariadb.conf.d:
/etc/mysql/mariadb.conf.d/50-server.cnf
bind-address = 127.0.0.1
max_connections = 100
--- PostgreSQL
/etc/postgresql/16/main/postgresql.conf
`,
	// a.conf has no final newline: cat runs it into b.conf.
	"mail": `=== MAIL ===
--- Postfix
myhostname = mail.example.com
--- Dovecot
!include conf.d/*.conf
mail_location = maildir:~/Maildir
--- BIND
zone "example.com" {
--- Unbound
server:
  verbosity: 1  port: 53
`,
	// The Docker heading lands on containerd (SYS-17).
	"infra": `=== INFRA ===
--- Samba
[global]
  workgroup = WORKGROUP
--- OpenMediaVault
OMV=installed
config.xml=10 bytes
--- OpenVPN
/etc/openvpn/client.conf
/etc/openvpn/sub/site.ovpn
--- Docker
containerd=present
--- Collectd
cpu.conf
--- Webmin
miniserv.conf
virtual-server
Virtualmin: installed
`,
	// The private key names land under cron.monthly: (SYS-17).
	"system": `=== SYSTEM ===
--- Cron
SHELL=/bin/sh
cron.d:
certbot
cron.daily:
cron.hourly:
cron.weekly:
cron.monthly:
--- Let's Encrypt
example.com
example.com
max-log-backups = 0
Virtualmin SSL=1 domains
/etc/ssl/private/server.key
/etc/ssl/private/server.pem
--- Kernel tuning
--- /etc/sysctl.d/99-local.conf:
vm.swappiness = 10
--- GRUB
GRUB_TIMEOUT=5
--- modprobe
--- /etc/modprobe.d/blacklist.conf:
blacklist pcspkr
--- Logrotate
apache2
--- Service defaults
grub
saslauthd
--- Profile
locale.sh
PATH="/usr/bin"
update-motd.d:
--- Cloud
distro: debian
system_info:
--- Chrony
pool 2.debian.pool.ntp.org iburst
etckeeper=git tracked
`,
}

func TestSectionsLLM(t *testing.T) {
	fx := newFixture(t, etcTree)
	for name, want := range llmWant {
		require.Equal(t, result{want, "", 0}, fx.call(t, nil, name, "--llm"), name)
	}
	require.Empty(t, fx.sudo(t))
}

// The identity probes outside etcTree: the container marker of PID 1's
// environment (read as root when elevated), OpenVZ, and WSL with its
// config in the compact form only.
func TestIdentityProbes(t *testing.T) {
	base := "=== IDENTITY ===\nKernel=6.1.0\nArch=x86_64\nHostname=web1.example.com\nUptime=up 3 days, 2 hours\nVirtualization=kvm\n"
	fx := newFixture(t, tree{"proc/1/environ": "PATH=/bin\x00HOME=/\x00container=lxc\x00", "proc/vz/": ""})
	require.Equal(t, result{base + "Container=container=lxc\n", "", 0}, fx.call(t, nil, "identity", "--llm"))
	fx.uid = 1000
	fx.env["SUDO_CACHED"] = "0"
	require.Equal(t, result{base + "Container=container=lxc\n", "", 0}, fx.call(t, nil, "identity", "--llm"))
	require.Equal(t, "sudo -n true\nsudo cat R/proc/1/environ\n", fx.sudo(t))

	fx = newFixture(t, tree{"proc/1/environ": "PATH=/bin\x00", "proc/vz/": "", "etc/wsl.conf": "[boot]\nsystemd=true", "etc/ubwsl-installed": ""})
	require.Equal(t, result{base + "Container=OpenVZ\nWSL=yes\nWSL Marker=ubwsl-installed\nWSL_CONF:\n[boot]\nsystemd=true", "", 0},
		fx.call(t, nil, "identity", "--llm"))
	require.NotContains(t, fx.call(t, nil, "identity").out, "WSL_CONF")
}
