// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// peerEnv is the environment of the local process at the other end of c:
// its socket in /proc/net/tcp, the process holding that socket, its
// /proc/PID/environ. Empty when any step fails.
func peerEnv(c net.Conn) map[string]string {
	remote, ok1 := c.RemoteAddr().(*net.TCPAddr)
	local, ok2 := c.LocalAddr().(*net.TCPAddr)
	if !ok1 || !ok2 {
		return map[string]string{}
	}
	inode := socketInode(hexAddr(remote), hexAddr(local))
	if inode == "" {
		return map[string]string{}
	}
	pid := socketOwner(inode)
	if pid == "" {
		return map[string]string{}
	}
	b, err := os.ReadFile("/proc/" + pid + "/environ")
	if err != nil {
		return map[string]string{}
	}
	env := map[string]string{}
	for _, kv := range bytes.Split(b, []byte{0}) {
		if k, v, ok := strings.Cut(string(kv), "="); ok {
			if _, dup := env[k]; !dup {
				env[k] = v
			}
		}
	}
	return env
}

// hexAddr is an IPv4 address as /proc/net/tcp writes it.
func hexAddr(a *net.TCPAddr) string {
	ip := a.IP.To4()
	if ip == nil {
		return ""
	}
	return fmt.Sprintf("%02X%02X%02X%02X:%04X", ip[3], ip[2], ip[1], ip[0], a.Port)
}

// socketInode finds the inode of the socket whose local address is
// client and remote address is server.
func socketInode(client, server string) string {
	f, err := os.Open("/proc/net/tcp")
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) > 9 && fs[1] == client && fs[2] == server {
			return fs[9]
		}
	}
	return ""
}

// socketOwner is the pid of a process with the socket open, other than
// this one.
func socketOwner(inode string) string {
	want := "socket:[" + inode + "]"
	self := fmt.Sprint(os.Getpid())
	fds, _ := filepath.Glob("/proc/[0-9]*/fd/*")
	for _, fd := range fds {
		if t, err := os.Readlink(fd); err == nil && t == want {
			pid := strings.Split(fd, "/")[2]
			if pid != self {
				return pid
			}
		}
	}
	return ""
}

// loopbackUp sets lo up, as `ip link set lo up` does.
func loopbackUp() error {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(fd) }()
	ifr, err := unix.NewIfreq("lo")
	if err != nil {
		return err
	}
	if err := unix.IoctlIfreq(fd, unix.SIOCGIFFLAGS, ifr); err != nil {
		return err
	}
	ifr.SetUint16(ifr.Uint16() | unix.IFF_UP)
	return unix.IoctlIfreq(fd, unix.SIOCSIFFLAGS, ifr)
}
