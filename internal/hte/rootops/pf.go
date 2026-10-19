// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package rootops

import (
	"context"
	"fmt"
	"os"
	"path"
	"regexp"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/receipt"
)

// The macOS port redirect (design/hte.md 6). When the kernel does not let
// the user bind 127.0.0.1:80, install asks root for a pf anchor that
// redirects ports 80 and 443 of the loopback to the stack's ports, and
// for a LaunchDaemon that loads it at boot. /etc/pf.conf is never edited:
// the anchor is a child of com.apple, which the stock pf.conf evaluates
// (rdr-anchor "com.apple/*").
const (
	PFAnchor      = "com.apple/gash-hte"
	PFAnchorFile  = "/etc/pf.anchors/gash-hte"
	pfAnchorTmp   = "/etc/pf.anchors/.gash-hte.tmp"
	PFDaemonLabel = "io.github.mauriziofonte.gash.hte.pf"
	PFDaemonPath  = "/Library/LaunchDaemons/" + PFDaemonLabel + ".plist"
	pfDaemonTmp   = "/Library/LaunchDaemons/.gash-hte-pf.tmp"
	PFConf        = "/etc/pf.conf"
	// pfctlPath is where macOS keeps pfctl: the LaunchDaemon names it.
	pfctlPath = "/sbin/pfctl"
)

var rdrAnchorRE = regexp.MustCompile(`(?m)^[ \t]*rdr-anchor[ \t]+"com\.apple/\*"[ \t]*$`)

// PFRules is the anchor file: ports 80 and 443 of 127.0.0.1 and ::1 to
// the stack's HTTP and HTTPS ports.
func PFRules(http, https int) []byte {
	b := fmt.Appendf(nil, "# Managed by gash hte install; gash hte uninstall removes it.\n"+
		"# The rules of the pf anchor %s: the standard web ports of the\n"+
		"# loopback reach the userland Apache.\n", PFAnchor)
	for _, fam := range []struct{ af, addr string }{{"inet", "127.0.0.1"}, {"inet6", "::1"}} {
		for _, p := range [][2]int{{80, http}, {443, https}} {
			b = fmt.Appendf(b, "rdr pass on lo0 %s proto tcp from any to %s port %d -> %s port %d\n", fam.af, fam.addr, p[0], fam.addr, p[1])
		}
	}
	return b
}

// pfLoad is the one command that loads the anchor: the LaunchDaemon's
// program and what install runs now. -E takes a reference on pf being
// enabled (it is off by default), without turning it off for others.
var pfLoad = []string{"-E", "-a", PFAnchor, "-f", PFAnchorFile}

// PFDaemon is the LaunchDaemon that loads the anchor at boot.
func PFDaemon() []byte {
	b := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<!-- Managed by gash hte install; gash hte uninstall removes it. -->
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>` + PFDaemonLabel + `</string>
  <key>ProgramArguments</key>
  <array>
`)
	for _, a := range append([]string{pfctlPath}, pfLoad...) {
		b = append(b, "    <string>"+a+"</string>\n"...)
	}
	return append(b, `  </array>
  <key>RunAtLoad</key>
  <true/>
</dict>
</plist>
`...)
}

func (s *server) pfRedirect(ctx context.Context, o Op) ([]string, error) {
	if s.env.OS.GOOS != "darwin" {
		return nil, hteerr.New(hteerr.PlatformUnsupported, "the pf redirect is macOS only", "")
	}
	to := receipt.Redirect{HTTP: o.Ports[0], HTTPS: o.Ports[1]}
	// A second install finds its own redirect: nothing to record.
	if s.r != nil {
		if c, ok := s.r.Find(receipt.PFRedirect, ""); ok && !c.Pending {
			var prev receipt.Redirect
			if receipt.Decode(c.Applied, &prev) == nil && prev == to {
				return nil, nil
			}
		}
	}
	conf, err := os.ReadFile(s.host(PFConf))
	if err != nil || !rdrAnchorRE.Match(conf) {
		return nil, hteerr.New(hteerr.PlatformUnsupported, PFConf+` does not evaluate the com.apple anchors (rdr-anchor "com.apple/*")`,
			"restore the stock "+PFConf+", or use gash hte install --ports high (gash never edits "+PFConf+")")
	}
	if err := s.ensureDirs(path.Dir(PFAnchorFile)); err != nil {
		return nil, err
	}
	err = s.writeFile(PFAnchorFile, PFRules(to.HTTP, to.HTTPS), 0o644, pfAnchorTmp, func(tmp string) error {
		return s.mustCmd(ctx, "pfctl", "-n", "-a", PFAnchor, "-f", tmp)
	})
	if err != nil {
		return nil, err
	}
	if err := s.ensureDirs(path.Dir(PFDaemonPath)); err != nil {
		return nil, err
	}
	err = s.writeFile(PFDaemonPath, PFDaemon(), 0o644, pfDaemonTmp, func(tmp string) error {
		return s.mustCmd(ctx, "plutil", "-lint", tmp)
	})
	if err != nil {
		return nil, err
	}
	c, err := s.record(receipt.Change{Kind: receipt.PFRedirect, Name: PFAnchor, Path: PFAnchorFile, Applied: receipt.Raw(to)})
	if err != nil {
		return nil, err
	}
	if err := s.mustCmd(ctx, "pfctl", pfLoad...); err != nil {
		return nil, err
	}
	return nil, s.commit(c)
}

// pfUndo empties the anchor and unloads the LaunchDaemon; its files are
// removed by the changes recorded before this one. pf itself stays
// enabled until the next restart (the reference of the boot-time load
// has no token to release), with an empty anchor.
func (s *server) pfUndo(ctx context.Context) ([]string, error) {
	// Loaded at boot, or not at all since install: either is fine.
	s.cmd(ctx, "launchctl", "bootout", "system/"+PFDaemonLabel)
	return nil, s.mustCmd(ctx, "pfctl", "-a", PFAnchor, "-F", "nat")
}
