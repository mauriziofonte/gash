// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/hte/child"
	"github.com/mauriziofonte/gash/internal/hte/discover"
	"github.com/mauriziofonte/gash/internal/hte/hosts"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/receipt"
	"github.com/mauriziofonte/gash/internal/hte/rootops"
	"github.com/mauriziofonte/gash/internal/hte/service"
	"github.com/mauriziofonte/gash/internal/hte/state"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
)

// Check statuses.
const (
	CheckOK    = "ok"
	CheckWarn  = "warn"
	CheckFail  = "fail"
	CheckFixed = "fixed" // doctor repaired it (the hosts block)
	CheckSkip  = "skip"  // not applicable here
)

// Check is one doctor finding: what, how it is, and the exact remedy of
// a failure doctor cannot fix.
type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Details string `json:"details"`
	Remedy  string `json:"remedy,omitempty"`
}

// DoctorResult is doctor's report. OK is false when any check failed: the
// command then exits non-zero.
type DoctorResult struct {
	OK     bool    `json:"ok"`
	Checks []Check `json:"checks"`
}

type report struct{ checks []Check }

func (r *report) add(name, status, details, remedy string) {
	r.checks = append(r.checks, Check{Name: name, Status: status, Details: details, Remedy: remedy})
}

func (r *report) err(name string, err error) {
	remedy := ""
	if he, ok := hteerr.As(err); ok {
		remedy = he.Hint
	}
	r.add(name, CheckFail, err.Error(), remedy)
}

// Doctor is doctor: it checks the machine, the install, the stack and
// every site, repairs a drifted hosts block, and names the remedy of
// everything else.
func (e *Engine) Doctor(ctx context.Context) (*DoctorResult, error) {
	if err := e.CheckIdentity("doctor"); err != nil {
		return nil, err
	}
	v, err := e.resolve()
	if err != nil {
		return nil, err
	}
	r := &report{}
	r.add("platform", CheckOK, v.os.Platform(), "")
	if e.EUID == 0 {
		r.add("user", CheckWarn, "running as root: the userland checks below are those of root's own (empty) stack",
			"run gash hte doctor as your own user")
	}

	// The machine.
	set, setErr := e.installed(v)
	if setErr != nil {
		set = state.DefaultSettings(state.StandardPorts, service.AutostartNone)
	}
	e.doctorMachine(ctx, v, set, r)

	// The install.
	if setErr != nil {
		if errors.Is(setErr, state.ErrNotInstalled) {
			r.add("install", CheckFail, "gash hte is not installed for this user", "run gash hte install")
		} else {
			r.err("install", setErr)
		}
		e.doctorPorts(v, set, nil, r)
		return r.result(), nil
	}
	r.add("install", CheckOK, fmt.Sprintf("ports %d/%d, autostart %s", set.Ports.HTTP, set.Ports.HTTPS, set.Autostart), "")
	e.doctorReceipt(v, r)
	e.doctorRoot(ctx, v, set, r)

	sites, problems, err := v.store.LoadSites()
	if err != nil {
		r.err("sites", err)
	}
	for _, p := range problems {
		r.err("sites", p)
	}
	if err == nil && len(problems) == 0 {
		r.add("sites", CheckOK, fmt.Sprintf("%d sites (%d enabled)", len(sites), len(enabledDomains(sites))), "")
	}

	// The stack.
	st, stErr := e.supervisor(v).Status(ctx, true)
	e.doctorPorts(v, set, &st, r)
	e.doctorStack(ctx, v, st, stErr, r)
	e.doctorAutostart(ctx, v, set, r)

	// Names and certificates.
	e.doctorHosts(ctx, v, sites, r)
	e.doctorCerts(ctx, v, sites, r)
	for _, s := range sites {
		for _, w := range htaccessWarnings(s) {
			r.add("site "+s.Domain, CheckWarn, w, "")
		}
	}
	return r.result(), nil
}

func (r *report) result() *DoctorResult {
	ok := !slices.ContainsFunc(r.checks, func(c Check) bool { return c.Status == CheckFail })
	return &DoctorResult{OK: ok, Checks: r.checks}
}

// doctorMachine checks Apache and its modules, PHP-FPM and openssl.
func (e *Engine) doctorMachine(ctx context.Context, v *env, set state.Settings, r *report) *machine {
	a, err := discover.FindApache(e.Sysroot, v.os)
	if err != nil {
		r.err("apache", err)
		return nil
	}
	r.add("apache", CheckOK, a.Binary, "")
	builtin, err := discover.BuiltinModules(a, e.probe(ctx))
	if err != nil {
		r.err("apache modules", err)
		return nil
	}
	m := &machine{apache: a, modules: discover.ResolveModules(e.Sysroot, a, builtin, set.Modules)}
	if err := m.modules.MissingError(a, v.os.Family); err != nil {
		r.err("apache modules", err)
	} else {
		r.add("apache modules", CheckOK, fmt.Sprintf("%d to load, %d built in", len(m.modules.Load), len(m.modules.Builtin)), "")
	}
	for _, w := range m.modules.Warnings {
		r.add("apache modules", CheckWarn, w, "")
	}
	m.php = e.discoverPHP(ctx, v)
	if len(m.php.Versions) == 0 {
		r.add("php", CheckWarn, "no PHP-FPM version is installed", "install php-fpm with the system package manager")
	} else {
		var vs []string
		for _, p := range m.php.Versions {
			vs = append(vs, p.Version+" ("+p.FPM+")")
		}
		r.add("php", CheckOK, strings.Join(vs, ", "), "")
	}
	for _, p := range m.php.Problems {
		r.add("php", CheckWarn, p, "")
	}
	if e.find("openssl") == "" {
		r.add("openssl", CheckFail, "openssl is not installed", "install openssl with the system package manager")
	} else {
		r.add("openssl", CheckOK, e.find("openssl"), "")
	}
	return m
}

// doctorReceipt compares the user copy of the receipt with the root copy.
func (e *Engine) doctorReceipt(v *env, r *report) {
	rootPath := receipt.RootPath(e.User)
	rootData, rootErr := e.Sysroot.ReadFile(rootPath)
	userData, userErr := os.ReadFile(v.paths.Receipt())
	switch {
	case errors.Is(rootErr, fs.ErrNotExist) && errors.Is(userErr, fs.ErrNotExist):
		r.add("receipt", CheckWarn, "no receipt: install recorded no root change", "run gash hte install")
		return
	case rootErr != nil && !errors.Is(rootErr, fs.ErrNotExist):
		r.add("receipt", CheckFail, "the root copy cannot be read: "+rootErr.Error(), "")
		return
	}
	if err := receipt.Compare(v.paths.Receipt(), userData, rootPath, rootData); err != nil {
		r.err("receipt", err)
		return
	}
	rc, err := receipt.Parse(rootData, rootPath)
	if err != nil {
		r.err("receipt", err)
		return
	}
	pending := 0
	for _, c := range rc.Changes {
		if c.Pending {
			pending++
		}
	}
	if pending > 0 {
		r.add("receipt", CheckFail, fmt.Sprintf("%d changes were interrupted (pending)", pending),
			"run the interrupted command again; gash hte uninstall undoes them as far as they went")
		return
	}
	r.add("receipt", CheckOK, fmt.Sprintf("%d changes, both copies equal", len(rc.Changes)), "")
}

// doctorRoot checks what install put outside the userland: the helper,
// the sudoers drop-in, the port setting, linger.
func (e *Engine) doctorRoot(ctx context.Context, v *env, set state.Settings, r *report) {
	if !e.Sysroot.IsRegular(rootops.HelperPath) {
		r.add("hosts helper", CheckFail, rootops.HelperPath+" is missing", "run gash hte install")
	} else if sum, err := sha256File(e.Helper); err == nil {
		inst, ierr := e.Sysroot.ReadFile(rootops.HelperPath)
		switch {
		case ierr != nil:
			r.add("hosts helper", CheckWarn, "cannot read "+rootops.HelperPath+": "+ierr.Error(), "")
		case sumBytes(inst) != sum:
			r.add("hosts helper", CheckWarn, rootops.HelperPath+" differs from the helper of this gash ("+e.Helper+")",
				"run gash hte install to update it")
		default:
			r.add("hosts helper", CheckOK, rootops.HelperPath, "")
		}
	} else {
		r.add("hosts helper", CheckOK, rootops.HelperPath, "")
	}
	if !e.Sysroot.Exists(rootops.SudoersPath) {
		r.add("sudoers", CheckFail, rootops.SudoersPath+" is missing", "run gash hte install")
	} else if sudo := e.find("sudo"); sudo != "" {
		res, err := child.Run(ctx, e.Runner, rootopsListCmd(sudo))
		if err != nil || !res.Success() {
			r.add("sudoers", CheckFail, "sudo does not allow the hosts helper without a password", "run gash hte install")
		} else {
			r.add("sudoers", CheckOK, rootops.SudoersPath, "")
		}
	}
	if v.os.GOOS == "linux" && set.Ports.HTTP < 1024 {
		data, err := e.Sysroot.ReadFile("/proc/sys/net/ipv4/ip_unprivileged_port_start")
		n, perr := strconv.Atoi(strings.TrimSpace(string(data)))
		switch {
		case err != nil || perr != nil:
			r.add("ports setting", CheckWarn, "cannot read "+rootops.PortSysctl, "")
		case n > set.Ports.HTTP:
			r.add("ports setting", CheckFail, fmt.Sprintf("%s is %d: ports %d and %d need root", rootops.PortSysctl, n, set.Ports.HTTP, set.Ports.HTTPS),
				"run gash hte install (it sets the value in "+rootops.SysctlFile+"), or install --ports high")
		default:
			r.add("ports setting", CheckOK, fmt.Sprintf("%s = %d", rootops.PortSysctl, n), "")
		}
	}
}

func sumBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// doctorPorts names who listens on each port of the stack.
func (e *Engine) doctorPorts(v *env, set state.Settings, st *service.Status, r *report) {
	if v.os.GOOS != "linux" {
		r.add("ports", CheckSkip, "the port holders are read from /proc on Linux only", "")
		return
	}
	ours := map[int]bool{}
	if st != nil && st.Apache.Running {
		for _, a := range st.Apache.Addrs {
			if ap, err := netip.ParseAddrPort(a.Addr); err == nil {
				ours[int(ap.Port())] = true
			}
		}
	}
	ls := listeners(e.Sysroot)
	for _, p := range []int{set.Ports.HTTP, set.Ports.HTTPS} {
		holders := ls[p]
		switch {
		case len(holders) == 0:
			r.add(fmt.Sprintf("port %d", p), CheckOK, "free", "")
		case ours[p]:
			r.add(fmt.Sprintf("port %d", p), CheckOK, "the gash stack", "")
		default:
			var who []string
			for _, h := range holders {
				who = append(who, e.holder(h))
			}
			r.add(fmt.Sprintf("port %d", p), CheckFail, "held by "+strings.Join(who, ", "),
				"stop that service; for the system Apache run gash hte migrate (it stops it once its vhosts moved)")
		}
	}
}

// listener is one listening TCP socket of /proc/net/tcp{,6}.
type listener struct {
	addr  string
	uid   int
	inode string
}

// listeners reads the listening sockets by port.
func listeners(sr platform.Sysroot) map[int][]listener {
	out := map[int][]listener{}
	for _, f := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, err := sr.ReadFile(f)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Scan() // the header
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) < 10 || fields[3] != "0A" {
				continue
			}
			host, port, ok := strings.Cut(fields[1], ":")
			if !ok {
				continue
			}
			n, err := strconv.ParseUint(port, 16, 16)
			uid, uerr := strconv.Atoi(fields[7])
			if err != nil || uerr != nil {
				continue
			}
			out[int(n)] = append(out[int(n)], listener{addr: procAddr(host) + ":" + strconv.Itoa(int(n)), uid: uid, inode: fields[9]})
		}
	}
	return out
}

// procAddr decodes the address of /proc/net/tcp: 32-bit words in host
// (little-endian) order.
func procAddr(h string) string {
	b, err := hex.DecodeString(h)
	if err != nil || (len(b) != 4 && len(b) != 16) {
		return h
	}
	for i := 0; i+4 <= len(b); i += 4 {
		b[i], b[i+1], b[i+2], b[i+3] = b[i+3], b[i+2], b[i+1], b[i]
	}
	a, _ := netip.AddrFromSlice(b)
	if len(b) == 16 {
		return "[" + a.Unmap().String() + "]"
	}
	return a.String()
}

// holder describes the process of a listening socket: its command and
// PID when the socket is one of the user's own processes, else its uid.
func (e *Engine) holder(l listener) string {
	entries, _ := e.Sysroot.ReadDir("/proc")
	want := "socket:[" + l.inode + "]"
	for _, en := range entries {
		pid := en.Name()
		if _, err := strconv.Atoi(pid); err != nil {
			continue
		}
		fds, err := e.Sysroot.ReadDir("/proc/" + pid + "/fd")
		if err != nil {
			continue
		}
		for _, fd := range fds {
			if t, err := e.Sysroot.Readlink("/proc/" + pid + "/fd/" + fd.Name()); err == nil && t == want {
				comm, _ := e.Sysroot.ReadFile("/proc/" + pid + "/comm")
				return fmt.Sprintf("%s (pid %s) on %s", strings.TrimSpace(string(comm)), pid, l.addr)
			}
		}
	}
	who := "uid " + strconv.Itoa(l.uid)
	if l.uid == 0 {
		who = "a root process (the system Apache?)"
	}
	return who + " on " + l.addr
}

// doctorStack checks the masters, the config test of the current
// generation and every site's answer.
func (e *Engine) doctorStack(ctx context.Context, v *env, st service.Status, stErr error, r *report) {
	if stErr != nil {
		r.err("stack", stErr)
		return
	}
	if st.Generation == 0 {
		r.add("stack", CheckWarn, "nothing is rendered yet", "run gash hte service start")
		return
	}
	sup := e.supervisor(v)
	_, man, _, err := sup.Current()
	if err == nil {
		err = sup.Test(ctx, sup.Layout.GenDir(st.Generation), man)
	}
	if err != nil {
		r.err("config test", err)
	} else {
		r.add("config test", CheckOK, fmt.Sprintf("generation %d", st.Generation), "")
	}
	masters := append([]service.MasterStatus{st.Apache}, st.FPM...)
	var down []string
	for _, ms := range masters {
		if !ms.Running {
			down = append(down, ms.Name)
		}
	}
	switch {
	case len(down) == len(masters):
		r.add("stack", CheckWarn, "stopped", "run gash hte service start")
		return
	case len(down) > 0:
		r.add("stack", CheckFail, "not running: "+strings.Join(down, ", "), "run gash hte service restart, then gash hte service logs")
	default:
		r.add("stack", CheckOK, fmt.Sprintf("apache pid %d, %d FPM masters", st.Apache.PID, len(st.FPM)), "")
	}
	for _, ms := range masters {
		for _, a := range ms.Addrs {
			if ms.Running && !a.Accepts {
				r.add(ms.Name, CheckFail, a.Addr+" does not accept connections", "run gash hte service restart")
			}
		}
	}
	for _, s := range st.Sites {
		if err := answered(s); err != nil {
			r.add("site "+s.Domain, CheckFail, err.Error(), "run gash hte service logs --site "+s.Domain)
		} else {
			r.add("site "+s.Domain, CheckOK, "answers", "")
		}
	}
}

// doctorAutostart checks the recorded autostart mode.
func (e *Engine) doctorAutostart(ctx context.Context, v *env, set state.Settings, r *report) {
	switch set.Autostart {
	case service.AutostartSystemd:
		if _, err := os.Stat(service.UnitPath(v.dirs.ConfigHome)); err != nil {
			r.add("autostart", CheckFail, service.UnitPath(v.dirs.ConfigHome)+" is missing", "run gash hte install")
			return
		}
		if err := e.systemctlUser(ctx, v, "is-enabled", "--quiet", service.UnitName); err != nil {
			r.add("autostart", CheckFail, service.UnitName+" is not enabled", "systemctl --user enable "+service.UnitName)
			return
		}
		if !e.Sysroot.Exists(filepath.Join("/var/lib/systemd/linger", e.User)) {
			r.add("autostart", CheckWarn, "linger is off: the stack starts at login, not at boot", "run gash hte install")
			return
		}
		r.add("autostart", CheckOK, "systemd user unit, linger on", "")
	case service.AutostartShell:
		r.add("autostart", CheckOK, "the first interactive shell starts the stack (gash init bash)", "")
	default:
		r.add("autostart", CheckOK, set.Autostart, "")
	}
}

// doctorHosts checks the hosts block and repairs it through the helper;
// on WSL it also lists the Windows lines.
func (e *Engine) doctorHosts(ctx context.Context, v *env, sites []state.Site, r *report) {
	desired, err := desiredHosts(v, sites)
	if err != nil {
		r.err("hosts", err)
		return
	}
	d := hosts.Check(e.Sysroot, desired)
	switch {
	case d.InSync:
		r.add("hosts", CheckOK, "the gash block of /etc/hosts is in sync", "")
	case !e.Sysroot.IsRegular(hosts.HelperPath) || e.EUID == 0:
		r.add("hosts", CheckFail, driftText(d), "run gash hte hosts apply")
	default:
		_, err := e.locked(v, "doctor", "", func() (any, error) { return nil, e.applyHosts(ctx, v, sites) })
		if err != nil {
			r.add("hosts", CheckFail, driftText(d)+"; the repair failed: "+err.Error(), "run gash hte hosts apply")
		} else {
			r.add("hosts", CheckFixed, driftText(d)+"; the block was written again", "")
		}
	}
	if v.os.WSL {
		var names []string
		for _, en := range desired {
			names = append(names, en.Names...)
		}
		missing := names
		if e.Sysroot.IsRegular(hosts.PowerShell) {
			if m, err := hosts.WindowsUnresolved(ctx, e.Runner, hosts.PowerShell, names); err == nil {
				missing = m
			}
		}
		if len(missing) > 0 {
			r.add("windows hosts", CheckWarn, "Windows does not resolve: "+strings.Join(missing, ", "),
				"add these lines to C:\\Windows\\System32\\drivers\\etc\\hosts: "+strings.Join(hosts.WindowsLines(missing), " | "))
		} else {
			r.add("windows hosts", CheckOK, "Windows resolves every name", "")
		}
	}
}

func driftText(d hosts.Drift) string {
	var parts []string
	if d.Problem != "" {
		parts = append(parts, d.Problem)
	}
	if !d.Found {
		parts = append(parts, "the gash block is missing")
	}
	if len(d.Missing) > 0 {
		parts = append(parts, "missing "+strings.Join(d.Missing, ", "))
	}
	if len(d.Unexpected) > 0 {
		parts = append(parts, "unexpected "+strings.Join(d.Unexpected, ", "))
	}
	if len(parts) == 0 {
		parts = append(parts, "the block differs")
	}
	return strings.Join(parts, "; ")
}

// doctorCerts checks the pair of every HTTPS site.
func (e *Engine) doctorCerts(ctx context.Context, v *env, sites []state.Site, r *report) {
	cm := e.certs(v)
	for _, s := range sites {
		if !s.HTTPS {
			continue
		}
		name := "certificate " + s.Domain
		h := cm.Check(ctx, s)
		for _, p := range h.Problems {
			r.add(name, CheckFail, p, "run gash hte cert renew --domain "+s.Domain)
		}
		for _, w := range h.Warnings {
			r.add(name, CheckWarn, w, "run gash hte cert renew --domain "+s.Domain)
		}
		if len(h.Problems) == 0 && len(h.Warnings) == 0 && h.Info != nil {
			r.add(name, CheckOK, "valid until "+h.Info.NotAfter.UTC().Format("2006-01-02"), "")
		}
	}
}

// rootopsListCmd asks sudo, without a password, whether the drop-in
// allows the helper's check command.
func rootopsListCmd(sudo string) runner.Cmd {
	c := child.Cmd(sudo, nil, "-n", "-l", rootops.HelperPath, "check")
	c.Timeout = 10 * time.Second
	return c
}
