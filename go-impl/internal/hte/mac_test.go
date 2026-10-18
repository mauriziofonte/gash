// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/mauriziofonte/gash/internal/hte/receipt"
	"github.com/mauriziofonte/gash/internal/hte/rootops"
	"github.com/mauriziofonte/gash/internal/hte/service"
	"github.com/mauriziofonte/gash/internal/hte/state"
)

// TestMacOSInstall: on macOS, install --ports standard binds 80 and 443
// directly when the kernel lets the user; when it refuses, the stack
// listens on the high ports behind the pf redirect, which root sets up
// (anchor file, LaunchDaemon, anchor loaded), and a forced HTTPS redirect
// targets 443. The LaunchAgent carries the XDG directories and is enabled
// in the user's GUI domain. Uninstall unloads and removes both, empties
// the anchor, and leaves /etc, /usr/local, /var/lib and /Library as they
// were.
func TestMacOSInstall(t *testing.T) {
	ctx := context.Background()
	for _, refused := range []bool{true, false} {
		t.Run(fmt.Sprintf("bind refused %v", refused), func(t *testing.T) {
			m := newMacRig(t)
			var binds []string
			m.e.BindLow = func(addr string) error {
				binds = append(binds, addr)
				if refused {
					return &os.SyscallError{Syscall: "bind", Err: syscall.EACCES}
				}
				return nil
			}
			before := m.snapshot()
			res, err := m.e.Install(ctx, InstallOptions{Yes: true})
			if err != nil {
				t.Fatalf("install: %v", err)
			}
			if !slices.Equal(binds, []string{"127.0.0.1:80", "127.0.0.1:443"}) && !(refused && slices.Equal(binds, []string{"127.0.0.1:80"})) {
				t.Errorf("bind checks %v", binds)
			}
			root, user := m.lines()
			pfPlanned := slices.ContainsFunc(res.Plan, func(l string) bool { return strings.Contains(l, "pf anchor") })
			wantPorts, wantRedirect := state.StandardPorts, ""
			if refused {
				wantPorts, wantRedirect = state.HighPorts, RedirectPF
			}
			if res.Settings.Ports != wantPorts || res.Redirect != wantRedirect || pfPlanned != refused || m.pfLoaded != refused || !res.Started {
				t.Fatalf("ports %+v redirect %q pf planned %v loaded %v started %v; plan %q", res.Settings.Ports, res.Redirect, pfPlanned, m.pfLoaded, res.Started, res.Plan)
			}
			if refused != slices.Contains(root, "pfctl -E -a com.apple/gash-hte -f /etc/pf.anchors/gash-hte") ||
				refused != (m.read(rootops.PFAnchorFile) == string(rootops.PFRules(8080, 8443))) ||
				refused != (m.read(rootops.PFDaemonPath) == string(rootops.PFDaemon())) {
				t.Errorf("pf files or load: root %v", root)
			}
			if slices.ContainsFunc(root, func(l string) bool { return strings.HasPrefix(l, "sysctl") }) {
				t.Errorf("sysctl on macOS: %v", root)
			}

			// The LaunchAgent.
			v, err := m.e.resolve()
			must(t, err)
			agent := service.LaunchAgentPath(m.home)
			want, err := service.LaunchAgent(m.e.Self, service.AgentEnv(v.dirs.ConfigHome, v.dirs.StateHome, v.dirs.DataHome, v.dirs.RuntimeDir))
			must(t, err)
			if got, err := os.ReadFile(agent); err != nil || string(got) != string(want) || res.Settings.Autostart != service.AutostartLaunchd {
				t.Errorf("agent %v, autostart %q:\n%s", err, res.Settings.Autostart, got)
			}
			target := fmt.Sprintf("gui/%d/%s", m.e.UID, service.LaunchAgentLabel)
			if !slices.Contains(user, "launchctl enable "+target) {
				t.Errorf("user commands %v", user)
			}
			r, _, err := receipt.Load(filepath.Join(m.root, receipt.RootPath(m.e.User)))
			must(t, err)
			if c, ok := r.Find(receipt.Autostart, ""); !ok || c.AutoMode != "launchd" || c.Path != agent {
				t.Errorf("autostart change %+v", c)
			}

			// A forced HTTPS site redirects to the port clients reach.
			_, err = m.e.SiteCreate(ctx, state.Flags{Domain: "mac.test", Docroot: ptr(m.docroot(t)), ForceHTTPS: ptr(true)})
			must(t, err)
			vhost := m.read(filepath.Join(strings.TrimPrefix(v.paths.RenderDir(), m.root), "current/sites/mac.test.conf"))
			if vhost == "" {
				data, _ := os.ReadFile(filepath.Join(v.paths.RenderDir(), "current/sites/mac.test.conf"))
				vhost = string(data)
			}
			if !strings.Contains(vhost, "https://%{HTTP_HOST}%{REQUEST_URI}") || strings.Contains(vhost, ":8443%{REQUEST_URI}") {
				t.Errorf("redirect of the vhost:\n%s", vhost)
			}

			if _, err := m.e.Uninstall(ctx, UninstallOptions{Yes: true}); err != nil {
				t.Fatal(err)
			}
			root, user = m.lines()
			if !slices.Contains(user, "launchctl bootout "+target) {
				t.Errorf("agent not unloaded: %v", user)
			}
			if _, err := os.Lstat(agent); err == nil {
				t.Error("the agent stayed")
			}
			if refused && (m.pfLoaded || !slices.Contains(root, "launchctl bootout system/"+rootops.PFDaemonLabel)) {
				t.Errorf("pf after uninstall: loaded %v, root %v", m.pfLoaded, root)
			}
			after := m.snapshot()
			for p, v := range before {
				if after[p] != v {
					t.Errorf("%s: %q, before %q", p, after[p], v)
				}
			}
			for p := range after {
				if _, ok := before[p]; !ok && !strings.HasPrefix(p, "/etc/hosts") {
					t.Errorf("%s left behind", p)
				}
			}
		})
	}
}

// With --ports high on macOS there is no bind check and no redirect: the
// redirect target carries the HTTPS port.
func TestMacOSHighPorts(t *testing.T) {
	ctx := context.Background()
	m := newMacRig(t)
	m.e.BindLow = func(addr string) error { t.Errorf("bind check of %s", addr); return nil }
	res, err := m.e.Install(ctx, InstallOptions{Yes: true, Ports: "high", Autostart: "none"})
	if err != nil || res.Redirect != "" || res.Settings.Ports != state.HighPorts {
		t.Fatalf("%+v %v", res, err)
	}
	if root, _ := m.lines(); slices.ContainsFunc(root, func(l string) bool { return strings.HasPrefix(l, "pfctl") }) {
		t.Errorf("pf on high ports: %v", root)
	}
	_, err = m.e.SiteCreate(ctx, state.Flags{Domain: "mac.test", Docroot: ptr(m.docroot(t)), ForceHTTPS: ptr(true)})
	must(t, err)
	v, err := m.e.resolve()
	must(t, err)
	data, _ := os.ReadFile(filepath.Join(v.paths.RenderDir(), "current/sites/mac.test.conf"))
	if !strings.Contains(string(data), "https://%{SERVER_NAME}:8443%{REQUEST_URI}") {
		t.Errorf("redirect of the vhost:\n%s", data)
	}
}
