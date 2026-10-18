// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package rootops

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mauriziofonte/gash/internal/hostshelper"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/legacy"
	"github.com/mauriziofonte/gash/internal/hte/proc"
	"github.com/mauriziofonte/gash/internal/hte/receipt"
	"github.com/mauriziofonte/gash/internal/platform"
)

// Env is the root side's view of the system.
type Env struct {
	Sysroot platform.Sysroot // the real root; a fixture tree in tests
	Runner  proc.Runner
	Getenv  func(string) string // SUDO_USER, SUDO_UID, SUDO_GID
	Euid    int
	Now     func() time.Time
	OS      platform.OS
	Layout  legacy.Layout // where the legacy vhosts and pools are
	Systemd bool          // /run/systemd/system exists
	// Groups returns the supplementary groups of a user, for copy_key's
	// child; Home its home directory.
	Groups func(user string) ([]uint32, error)
	Home   func(user string) (string, error)
}

type server struct {
	env  Env
	user string
	uid  int
	gid  int
	plan map[int]Op
	done map[int]bool
	r    *receipt.Receipt
	// made are the receipt changes this session recorded.
	made  map[int]bool
	stamp string
}

// Serve runs one root session: the hello, then the requested operations,
// until the user side closes its end. Every answer is one JSON line.
func Serve(ctx context.Context, in io.Reader, out io.Writer, env Env) error {
	enc := json.NewEncoder(out)
	dec := json.NewDecoder(bufio.NewReader(in))
	dec.DisallowUnknownFields()
	s, err := open(env)
	if err == nil {
		err = s.hello(dec)
	}
	if err != nil {
		enc.Encode(Response{Error: wire(err)})
		return err
	}
	if err := enc.Encode(Response{OK: true, Receipt: s.receiptText()}); err != nil {
		return err
	}
	for {
		var req Request
		if err := dec.Decode(&req); errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			enc.Encode(Response{Error: wire(hteerr.Newf(hteerr.InvalidState, "", "bad request: %v", err))})
			return err
		}
		resp := s.run(ctx, req)
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
}

func open(env Env) (*server, error) {
	if env.Euid != 0 {
		return nil, hteerr.New(hteerr.PrivilegedStepRequired, "the root step must run as root", "it is started by gash through sudo")
	}
	user := env.Getenv("SUDO_USER")
	uid, uerr := strconv.Atoi(env.Getenv("SUDO_UID"))
	gid, gerr := strconv.Atoi(env.Getenv("SUDO_GID"))
	if uerr != nil || gerr != nil || uid <= 0 || !receipt.UserRE.MatchString(user) {
		return nil, hteerr.New(hteerr.PrivilegedStepRequired, "the root step needs the invoking user from sudo (SUDO_USER, SUDO_UID), not root",
			"run gash hte as your own user")
	}
	s := &server{env: env, user: user, uid: uid, gid: gid, plan: map[int]Op{}, done: map[int]bool{}, made: map[int]bool{},
		stamp: env.Now().UTC().Format("20060102T150405Z")}
	r, ok, err := receipt.Load(s.host(receipt.RootPath(user)))
	if err != nil {
		return nil, err
	}
	if ok {
		if r.User != user || r.UID != uid {
			return nil, hteerr.Newf(hteerr.ReceiptMismatch, "", "%s belongs to %s (uid %d)", receipt.RootPath(user), r.User, r.UID)
		}
		s.r = r
	}
	return s, nil
}

func (s *server) hello(dec *json.Decoder) error {
	var req Request
	if err := dec.Decode(&req); err != nil || req.Hello == nil {
		return hteerr.New(hteerr.InvalidState, "the root session did not start with the plan", "")
	}
	for _, o := range req.Hello.Plan {
		if err := o.Check(); err != nil {
			return err
		}
		if _, dup := s.plan[o.ID]; dup || o.ID <= 0 {
			return hteerr.Newf(hteerr.InvalidState, "", "root operation id %d is not unique and positive", o.ID)
		}
		s.plan[o.ID] = o
	}
	return nil
}

// TestLedger_HTE17 lives in rootops_test: an operation is run only when
// the plan holds it, once.
func (s *server) run(ctx context.Context, req Request) Response {
	o, ok := s.plan[req.Run]
	resp := Response{ID: req.Run}
	switch {
	case !ok:
		resp.Error = wire(hteerr.Newf(hteerr.InvalidState, "", "operation %d is not in the plan", req.Run))
	case s.done[req.Run]:
		resp.Error = wire(hteerr.Newf(hteerr.InvalidState, "", "operation %d already ran", req.Run))
	default:
		s.done[req.Run] = true
		before := len(s.changes())
		warnings, err := s.exec(ctx, o)
		if err != nil {
			resp.Error = wire(err)
		} else {
			resp.OK = true
		}
		resp.Warnings = warnings
		if after := s.changes(); len(after) > before {
			resp.Changes = after[before:]
		}
	}
	resp.Receipt = s.receiptText()
	return resp
}

func (s *server) changes() []receipt.Change {
	if s.r == nil {
		return nil
	}
	return s.r.Changes
}

func (s *server) receiptText() string {
	data, err := os.ReadFile(s.host(receipt.RootPath(s.user)))
	if err != nil {
		return ""
	}
	return string(data)
}

func (s *server) exec(ctx context.Context, o Op) ([]string, error) {
	switch o.Op {
	case OpInstallHelper:
		return s.installHelper(ctx, o)
	case OpWriteSudoers:
		return s.writeSudoers(ctx)
	case OpSysctlPorts:
		return s.sysctlPorts(ctx, o.Value)
	case OpLinger:
		return s.linger(ctx)
	case OpUnitStop:
		return s.unitStop(ctx, o.Name)
	case OpUndoUnitStop:
		return s.undoUnitStop(ctx, o.Name)
	case OpUnitReload:
		return s.unitReload(ctx, o.Name)
	case OpHostsSnapshot:
		return s.hostsSnapshot()
	case OpArchiveVhost:
		return s.archiveVhost(o.Path)
	case OpArchivePool:
		return s.archivePool(o.Path)
	case OpCopyKey:
		return s.copyKey(ctx, o)
	case OpRecordAutostart:
		return s.recordAutostart(o)
	case OpRestore:
		return s.restore(ctx, o.Change)
	case OpFinish:
		return s.finish()
	}
	return nil, fmt.Errorf("rootops: %s", o.Op)
}

// host maps a system path through the sysroot.
func (s *server) host(sys string) string { return s.env.Sysroot.Path(sys) }

func (s *server) find(name string) (string, error) {
	p := proc.Find(name, s.env.Sysroot.Exists)
	if p == "" {
		return "", hteerr.Newf(hteerr.PrivilegedStepRequired, "install "+name, "%s is not installed", name)
	}
	return p, nil
}

func (s *server) cmd(ctx context.Context, name string, args ...string) (proc.Result, error) {
	bin, err := s.find(name)
	if err != nil {
		return proc.Result{}, err
	}
	return s.env.Runner.Run(ctx, proc.Cmd{Path: bin, Args: args, Setsid: true, Timeout: 2 * time.Minute})
}

func (s *server) mustCmd(ctx context.Context, name string, args ...string) error {
	res, err := s.cmd(ctx, name, args...)
	if err != nil {
		return err
	}
	if !res.OK() {
		return hteerr.Newf(hteerr.PrivilegedStepRequired, "", "%s %s: %s", name, strings.Join(args, " "), res.Output())
	}
	return nil
}

// record adds c to the root receipt, pending, and saves it: the receipt
// names a change before the change is made.
func (s *server) record(c receipt.Change) (receipt.Change, error) {
	if s.r == nil {
		s.r = receipt.New(s.user, s.uid, s.env.OS.Platform(), s.env.Now())
	}
	c.Pending = true
	c = s.r.Add(c)
	s.made[c.ID] = true
	return c, s.save()
}

// commit marks the change made.
func (s *server) commit(c receipt.Change) error {
	for i := range s.r.Changes {
		if s.r.Changes[i].ID == c.ID {
			s.r.Changes[i].Pending = false
		}
	}
	return s.save()
}

func (s *server) save() error {
	if err := os.MkdirAll(s.host(receipt.RootDir), 0o755); err != nil {
		return err
	}
	return receipt.Save(s.host(receipt.RootPath(s.user)), s.r, 0o644)
}

func sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// backupPath is where the prior content of path is kept.
func (s *server) backupPath(sys string) string {
	return path.Join(receipt.BackupDir, s.stamp, strings.TrimPrefix(sys, "/"))
}

func (s *server) archivePath(sys string) string {
	return path.Join(receipt.ArchiveDir, s.stamp, strings.TrimPrefix(sys, "/"))
}

// ensureDirs creates dir and its missing ancestors (0755), each recorded,
// outermost first, so that restore removes them innermost first.
func (s *server) ensureDirs(dir string) error {
	var missing []string
	for d := dir; d != "/"; d = path.Dir(d) {
		if _, err := os.Lstat(s.host(d)); err == nil {
			break
		}
		missing = append(missing, d)
	}
	slices.Reverse(missing)
	for _, d := range missing {
		c, err := s.record(receipt.Change{Kind: receipt.DirCreated, Path: d})
		if err != nil {
			return err
		}
		if err := os.Mkdir(s.host(d), 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
		if err := s.commit(c); err != nil {
			return err
		}
	}
	return nil
}

// writeAtomic writes data to a temporary file next to sys, then renames it.
func writeAtomic(host string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(host), ".gash-hte-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), host)
}

// copyFile copies src to dst (both host paths) with mode, through a
// temporary file.
func copyFile(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	return writeAtomic(dst, data, mode)
}

// moveFile renames src to dst, copying across file systems.
func moveFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	err := os.Rename(src, dst)
	if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if err := copyFile(src, dst, fi.Mode().Perm()); err != nil {
		return err
	}
	return os.Remove(src)
}

// writeFile installs data at sys with mode. validate, when set, checks
// the temporary file before it replaces anything. An identical file is
// no change; an existing different one is backed up first.
func (s *server) writeFile(sys string, data []byte, mode os.FileMode, tmpSys string, validate func(tmpHost string) error) error {
	host := s.host(sys)
	cur, err := os.ReadFile(host)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if exists {
		if fi, err := os.Lstat(host); err == nil && fi.Mode().IsRegular() && fi.Mode().Perm() == mode && bytes.Equal(cur, data) {
			return nil
		}
	}
	tmp := s.host(tmpSys)
	os.Remove(tmp)
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	defer os.Remove(tmp)
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	if validate != nil {
		if err := validate(tmp); err != nil {
			return err
		}
	}
	c := receipt.Change{Kind: receipt.FileCreated, Path: sys, SHA256: sum(data), Mode: fmt.Sprintf("%04o", mode)}
	if exists {
		fi, err := os.Lstat(host)
		if err != nil {
			return err
		}
		c.Kind, c.Backup, c.PriorSHA256 = receipt.FileModified, s.backupPath(sys), sum(cur)
		if err := os.MkdirAll(s.host(path.Dir(c.Backup)), 0o700); err != nil {
			return err
		}
		if err := copyFile(host, s.host(c.Backup), fi.Mode().Perm()); err != nil {
			return err
		}
	}
	c, err = s.record(c)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, host); err != nil {
		return err
	}
	return s.commit(c)
}

func (s *server) installHelper(ctx context.Context, o Op) ([]string, error) {
	data, err := os.ReadFile(s.host(o.Src))
	if err != nil {
		return nil, hteerr.Newf(hteerr.PrivilegedStepRequired, "", "reading the hosts helper: %v", err)
	}
	if sum(data) != o.SHA256 {
		return nil, hteerr.Newf(hteerr.InvalidState, "run gash hte install again", "%s changed after the plan was made", o.Src)
	}
	if err := s.ensureDirs(path.Dir(HelperPath)); err != nil {
		return nil, err
	}
	return nil, s.writeFile(HelperPath, data, 0o755, path.Dir(HelperPath)+"/.gash-hosts-helper.tmp", nil)
}

var includedirRE = regexp.MustCompile(`(?m)^[ \t]*[#@]includedir[ \t]+/etc/sudoers\.d[ \t]*$`)

// SudoersContent is the drop-in for user: the helper, with its exact
// arguments, and nothing else.
func SudoersContent(user string) []byte {
	return fmt.Appendf(nil, "# Managed by gash hte install; gash hte uninstall removes it.\n"+
		"%[1]s ALL=(root) NOPASSWD: %[2]s apply\n%[1]s ALL=(root) NOPASSWD: %[2]s check\n", user, HelperPath)
}

func (s *server) writeSudoers(ctx context.Context) ([]string, error) {
	main, err := os.ReadFile(s.host("/etc/sudoers"))
	if err != nil || !includedirRE.Match(main) {
		return nil, hteerr.New(hteerr.PrivilegedStepRequired, "/etc/sudoers does not include /etc/sudoers.d",
			"add the line @includedir /etc/sudoers.d to /etc/sudoers with visudo, then run gash hte install again (gash never edits /etc/sudoers)")
	}
	if err := s.ensureDirs(SudoersDir); err != nil {
		return nil, err
	}
	return nil, s.writeFile(SudoersPath, SudoersContent(s.user), 0o440, sudoersTmp, func(tmp string) error {
		res, err := s.cmd(ctx, "visudo", "-c", "-f", tmp)
		if err != nil {
			return err
		}
		if !res.OK() {
			return hteerr.Newf(hteerr.PrivilegedStepRequired, "", "visudo refused the drop-in: %s", res.Output())
		}
		return nil
	})
}

func (s *server) sysctlPorts(ctx context.Context, value int) ([]string, error) {
	raw, err := os.ReadFile(s.host(sysctlProc))
	if err != nil {
		return nil, hteerr.Newf(hteerr.PlatformUnsupported, "use gash hte install --ports high",
			"%s cannot be read: %v (Linux 4.11 or later has it)", sysctlProc, err)
	}
	cur := strings.TrimSpace(string(raw))
	n, err := strconv.Atoi(cur)
	if err != nil {
		return nil, fmt.Errorf("%s: %q", sysctlProc, cur)
	}
	if n <= value {
		c, err := s.record(receipt.Change{Kind: receipt.Sysctl, Key: PortSysctl, Prior: receipt.Raw(cur), Applied: receipt.Raw(cur)})
		if err != nil {
			return nil, err
		}
		return []string{fmt.Sprintf("%s is already %d: nothing changed", PortSysctl, n)}, s.commit(c)
	}
	applied := strconv.Itoa(value)
	fp := &receipt.FilePrior{Path: SysctlFile}
	content := fmt.Appendf(nil, "# Managed by gash hte install; gash hte uninstall removes it.\n%s = %s\n", PortSysctl, applied)
	if prior, err := os.ReadFile(s.host(SysctlFile)); err == nil {
		b := s.backupPath(SysctlFile)
		if err := os.MkdirAll(s.host(path.Dir(b)), 0o700); err != nil {
			return nil, err
		}
		if err := writeAtomic(s.host(b), prior, 0o644); err != nil {
			return nil, err
		}
		fp.Prior = &b
	}
	if err := s.ensureDirs(path.Dir(SysctlFile)); err != nil {
		return nil, err
	}
	c, err := s.record(receipt.Change{Kind: receipt.Sysctl, Key: PortSysctl, Prior: receipt.Raw(cur), Applied: receipt.Raw(applied), File: fp})
	if err != nil {
		return nil, err
	}
	if err := writeAtomic(s.host(SysctlFile), content, 0o644); err != nil {
		return nil, err
	}
	if err := s.mustCmd(ctx, "sysctl", "-w", PortSysctl+"="+applied); err != nil {
		return nil, err
	}
	return nil, s.commit(c)
}

func (s *server) linger(ctx context.Context) ([]string, error) {
	if !s.env.Systemd {
		return nil, hteerr.New(hteerr.PlatformUnsupported, "linger needs systemd", "use --autostart shell or none")
	}
	if _, err := os.Lstat(s.host(path.Join(lingerDir, s.user))); err == nil {
		c, err := s.record(receipt.Change{Kind: receipt.Linger, Prior: receipt.Raw(true), Applied: receipt.Raw(true)})
		if err != nil {
			return nil, err
		}
		return []string{"linger was already enabled"}, s.commit(c)
	}
	c, err := s.record(receipt.Change{Kind: receipt.Linger, Prior: receipt.Raw(false), Applied: receipt.Raw(true)})
	if err != nil {
		return nil, err
	}
	if err := s.mustCmd(ctx, "loginctl", "enable-linger", s.user); err != nil {
		return nil, err
	}
	return nil, s.commit(c)
}

// unitState reads whether a unit is enabled and active: systemctl, or
// the SysV scripts where there is no systemd (containers).
func (s *server) unitState(ctx context.Context, name string) (receipt.UnitState, error) {
	var st receipt.UnitState
	if s.env.Systemd {
		res, err := s.cmd(ctx, "systemctl", "is-enabled", name+".service")
		if err != nil {
			return st, err
		}
		st.Enabled = res.OK() && strings.TrimSpace(string(res.Stdout)) == "enabled"
		res, err = s.cmd(ctx, "systemctl", "is-active", name+".service")
		if err != nil {
			return st, err
		}
		st.Active = res.OK()
		return st, nil
	}
	links, _ := filepath.Glob(s.host("/etc") + "/rc[2-5].d/S[0-9][0-9]" + name)
	st.Enabled = len(links) > 0
	res, err := s.cmd(ctx, "service", name, "status")
	if err != nil {
		return st, err
	}
	st.Active = res.OK()
	return st, nil
}

func (s *server) unitStop(ctx context.Context, name string) ([]string, error) {
	st, err := s.unitState(ctx, name)
	if err != nil {
		return nil, err
	}
	if !st.Enabled && !st.Active {
		return []string{name + " is already stopped and disabled"}, nil
	}
	c, err := s.record(receipt.Change{Kind: receipt.Unit, Name: name, Prior: receipt.Raw(st), Applied: receipt.Raw(receipt.UnitState{})})
	if err != nil {
		return nil, err
	}
	if s.env.Systemd {
		err = s.mustCmd(ctx, "systemctl", "disable", "--now", name+".service")
	} else {
		if st.Active {
			err = s.mustCmd(ctx, "service", name, "stop")
		}
		if err == nil && st.Enabled {
			err = s.mustCmd(ctx, "update-rc.d", name, "disable")
		}
	}
	if err != nil {
		return nil, err
	}
	if now, err := s.unitState(ctx, name); err != nil {
		return nil, err
	} else if now.Active {
		return nil, hteerr.Newf(hteerr.PrivilegedStepRequired, "", "%s is still active after the stop", name)
	}
	return nil, s.commit(c)
}

func (s *server) unitReload(ctx context.Context, name string) ([]string, error) {
	st, err := s.unitState(ctx, name)
	if err != nil || !st.Active {
		return nil, err
	}
	if s.env.Systemd {
		return nil, s.mustCmd(ctx, "systemctl", "reload-or-restart", name+".service")
	}
	return nil, s.mustCmd(ctx, "service", name, "reload")
}

func (s *server) hostsSnapshot() ([]string, error) {
	if s.r != nil {
		if _, ok := s.r.Find(receipt.HostsBlock, ""); ok {
			return nil, nil
		}
	}
	data, err := os.ReadFile(s.host(hostshelper.HostsFile))
	if err != nil {
		return nil, err
	}
	block, found, err := hostshelper.Extract(data)
	if err != nil {
		return nil, hteerr.Newf(hteerr.InvalidState, "fix the gash markers in /etc/hosts by hand", "%v", err)
	}
	var prior *string
	if found {
		b := string(block)
		prior = &b
	}
	c, err := s.record(receipt.Change{Kind: receipt.HostsBlock, Prior: receipt.Raw(prior)})
	if err != nil {
		return nil, err
	}
	return nil, s.commit(c)
}

func (s *server) inventory() *legacy.Inventory { return legacy.Scan(s.env.Sysroot, s.env.Layout) }

func (s *server) archiveVhost(p string) ([]string, error) {
	inv := s.inventory()
	i := slices.IndexFunc(inv.Vhosts, func(v legacy.Vhost) bool { return v.Path == p })
	if i < 0 {
		return nil, hteerr.Newf(hteerr.InvalidState, "run gash hte migrate again", "%s is not a vhost of the system Apache", p)
	}
	v := inv.Vhosts[i]
	c := receipt.Change{Kind: receipt.LegacyVhost, Path: p, Prior: receipt.Raw(receipt.VhostState{Enabled: v.Enabled}), Archive: s.archivePath(p)}
	if dir := s.env.Layout.EnabledDir; dir != "" && path.Dir(p) != dir {
		link := path.Join(dir, path.Base(p))
		if fi, err := os.Lstat(s.host(link)); err == nil && fi.Mode()&fs.ModeSymlink != 0 {
			target, err := os.Readlink(s.host(link))
			if err != nil {
				return nil, err
			}
			if resolved, err := s.env.Sysroot.Resolve(link); err == nil && resolved == p {
				c.Link = &receipt.Link{Path: link, Target: target}
			}
		}
	}
	c, err := s.record(c)
	if err != nil {
		return nil, err
	}
	if c.Link != nil {
		if err := os.Remove(s.host(c.Link.Path)); err != nil {
			return nil, err
		}
	}
	if err := moveFile(s.host(p), s.host(c.Archive)); err != nil {
		return nil, err
	}
	return nil, s.commit(c)
}

func (s *server) archivePool(p string) ([]string, error) {
	inv := s.inventory()
	if _, ok := inv.Pools[p]; !ok {
		return nil, hteerr.Newf(hteerr.InvalidState, "run gash hte migrate again", "%s is not a pool of the system PHP-FPM", p)
	}
	c, err := s.record(receipt.Change{Kind: receipt.LegacyPool, Path: p, Archive: s.archivePath(p)})
	if err != nil {
		return nil, err
	}
	if err := moveFile(s.host(p), s.host(c.Archive)); err != nil {
		return nil, err
	}
	return nil, s.commit(c)
}

// copyKey hands a legacy key to openssl running as the user. The key must
// be the SSLCertificateKeyFile of a vhost of the system Apache, a regular
// file; root opens it (never following a symlink) and passes the open
// file as openssl's standard input. Root reads no byte of it.
func (s *server) copyKey(ctx context.Context, o Op) ([]string, error) {
	inv := s.inventory()
	if !slices.ContainsFunc(inv.Vhosts, func(v legacy.Vhost) bool { return v.KeyFile == o.Src }) {
		return nil, hteerr.Newf(hteerr.InvalidState, "run gash hte migrate again", "%s is not the key of a vhost of the system Apache", o.Src)
	}
	f, err := os.OpenFile(s.host(o.Src), os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, hteerr.Newf(hteerr.PrivilegedStepRequired, "", "opening %s: %v", o.Src, err)
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() {
		return nil, hteerr.Newf(hteerr.InvalidState, "", "%s is not a regular file", o.Src)
	}
	bin, err := s.find("openssl")
	if err != nil {
		return nil, err
	}
	groups, err := s.env.Groups(s.user)
	if err != nil {
		return nil, err
	}
	home, err := s.env.Home(s.user)
	if err != nil {
		return nil, err
	}
	res, err := s.env.Runner.Run(ctx, proc.Cmd{Path: bin, Args: []string{"pkey", "-out", o.Dst}, StdinFile: f, Setsid: true,
		Env: proc.BaseEnv(home), Credential: &proc.Credential{UID: uint32(s.uid), GID: uint32(s.gid), Groups: groups}})
	if err != nil {
		return nil, err
	}
	if !res.OK() {
		return nil, hteerr.Newf(hteerr.PrivilegedStepRequired, "migrate with --reissue-certs makes a new pair instead",
			"openssl could not copy %s: %s", o.Src, res.Output())
	}
	return nil, nil
}

func (s *server) recordAutostart(o Op) ([]string, error) {
	if s.r != nil {
		if c, ok := s.r.Find(receipt.Autostart, ""); ok && c.AutoMode == o.Mode && c.Path == o.Path {
			return nil, nil
		}
	}
	c, err := s.record(receipt.Change{Kind: receipt.Autostart, AutoMode: o.Mode, Path: o.Path})
	if err != nil {
		return nil, err
	}
	return nil, s.commit(c)
}

func (s *server) finish() ([]string, error) {
	if s.r != nil && len(s.r.Changes) > 0 {
		return nil, hteerr.Newf(hteerr.InvalidState, "", "%d changes are not undone yet", len(s.r.Changes))
	}
	if err := os.Remove(s.host(receipt.RootPath(s.user))); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	s.r = nil
	for _, d := range []string{receipt.BackupDir, receipt.ArchiveDir, receipt.RootDir} {
		removeEmptyDirs(s.host(d))
	}
	return nil, nil
}

// removeEmptyDirs removes dir and every directory under it that holds no
// file.
func removeEmptyDirs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			removeEmptyDirs(filepath.Join(dir, e.Name()))
		}
	}
	os.Remove(dir) // fails while anything is left
}
