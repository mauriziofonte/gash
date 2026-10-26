// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sysinfo

import (
	"net"
	"os"
	"runtime"
	"strconv"
)

// natives are what the overview and identity read in-process when the
// tool that 2.x ran is missing or prints nothing (hostname, uname,
// getconf and nproc, df, hostname -I): the card keeps its fields on a
// system without them (macOS has no hostname -I, BusyBox no getconf).
type natives struct {
	hostname func() (string, error)
	uname    func() (sysname, release, machine string, ok bool)
	numCPU   func() int
	statfs   func(path string) (FS, bool)
	// addr is the first address hostname -I would print: a global
	// unicast address, IPv4 first.
	addr func() string
}

func realNatives() *natives {
	return &natives{hostname: os.Hostname, uname: Uname, numCPU: runtime.NumCPU, statfs: Statfs, addr: firstAddr}
}

// firstAddr is the first global unicast address of the interfaces, IPv4
// before IPv6, as hostname -I lists them (no loopback, no link-local).
func firstAddr() string {
	as, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	v6 := ""
	for _, a := range as {
		n, ok := a.(*net.IPNet)
		if !ok || !n.IP.IsGlobalUnicast() {
			continue
		}
		if n.IP.To4() != nil {
			return n.IP.String()
		}
		if v6 == "" {
			v6 = n.IP.String()
		}
	}
	return v6
}

// host is the host name (os.Hostname), "" without natives.
func (n *natives) host() string {
	if n == nil {
		return ""
	}
	h, err := n.hostname()
	if err != nil {
		return ""
	}
	return h
}

// kernel is uname -s -r and uname -m, "" without natives.
func (n *natives) kernel() (sysRelease, release, machine string) {
	if n == nil {
		return "", "", ""
	}
	s, r, m, ok := n.uname()
	if !ok {
		return "", "", ""
	}
	return s + " " + r, r, m
}

// cpus is the count of online CPUs, "" without natives.
func (n *natives) cpus() string {
	if n == nil {
		return ""
	}
	return strconv.Itoa(n.numCPU())
}

// disk is df -Pk PATH as the overview reads it: the used KiB and the KiB
// users can fill (used + available, as df's Capacity), 0 without natives.
func (n *natives) disk(path string) (total, used int64) {
	if n == nil {
		return 0, 0
	}
	fs, ok := n.statfs(path)
	if !ok {
		return 0, 0
	}
	used = int64((fs.Total - fs.Free) / 1024) //nolint:gosec // sizes in KiB fit in int64
	return used + int64(fs.Avail/1024), used  //nolint:gosec // sizes in KiB fit in int64
}

// ip is the first address, "" without natives.
func (n *natives) ip() string {
	if n == nil {
		return ""
	}
	return n.addr()
}
