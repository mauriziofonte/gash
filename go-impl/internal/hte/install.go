// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/renameio/v2"

	"github.com/mauriziofonte/gash/internal/hte/discover"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/legacy"
	"github.com/mauriziofonte/gash/internal/hte/receipt"
	"github.com/mauriziofonte/gash/internal/hte/rootops"
	"github.com/mauriziofonte/gash/internal/hte/service"
	"github.com/mauriziofonte/gash/internal/hte/state"
	"github.com/mauriziofonte/gash/internal/platform"
)

// InstallOptions are the flags of install.
type InstallOptions struct {
	Yes          bool
	Ports        string // "standard" (default) or "high"
	Autostart    string // "auto" (default), "systemd", "launchd", "shell", "none"
	RestoreStage string // --restore-stage PATH
	NoRestore    bool   // --no-restore
}

// InstallResult is what install did.
type InstallResult struct {
	Plan     []string         `json:"plan"` // the root steps, as shown before sudo
	Changes  []receipt.Change `json:"changes"`
	Settings state.Settings   `json:"settings"`
	Restored string           `json:"restored_stage,omitempty"`
	Started  bool             `json:"started"`
	// Blocking are the enabled system vhosts that keep the system Apache
	// running (install never stops it while any is left; migrate does).
	Blocking []string `json:"blocking"`
	Warnings []string `json:"warnings"`
}

func sha256File(p string) (string, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// systemUnit is the system Apache unit of a platform family.
func systemUnit(f platform.Family) string {
	if f == platform.FamilyDebian {
		return "apache2"
	}
	return "httpd"
}

// hasSystemd reports a system booted with systemd.
func (e *Engine) hasSystemd() bool { return e.Sysroot.IsDir("/run/systemd/system") }

func (e *Engine) resolveAutostart(v *env, want string) (string, error) {
	switch want {
	case "", "auto":
		switch {
		case v.os.GOOS == "darwin":
			return service.AutostartLaunchd, nil
		case e.hasSystemd() && v.dirs.RuntimeDir != "":
			return service.AutostartSystemd, nil
		}
		return service.AutostartNone, nil
	case service.AutostartSystemd:
		if !e.hasSystemd() {
			return "", hteerr.New(hteerr.PlatformUnsupported, "this system does not run systemd", "use --autostart shell or none")
		}
		return want, nil
	case service.AutostartLaunchd:
		if v.os.GOOS != "darwin" {
			return "", hteerr.New(hteerr.PlatformUnsupported, "launchd is macOS only", "use --autostart auto")
		}
		return want, nil
	case service.AutostartShell, service.AutostartNone:
		return want, nil
	}
	return "", hteerr.InvalidValue(hteerr.InvalidAutostart, "autostart", want, "auto", "systemd", "launchd", "shell", "none")
}

// stages lists the uninstall stage directories.
func stages(v *env) []string {
	found, _ := filepath.Glob(v.paths.Stage("*"))
	slices.Sort(found)
	return found
}

// saveUserReceipt stores the root copy as the user copy, byte for byte;
// "" (no root copy any more) removes it.
func saveUserReceipt(v *env, text string) error {
	if text == "" {
		if err := os.Remove(v.paths.Receipt()); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(v.paths.Receipt()), 0o700); err != nil {
		return err
	}
	return renameio.WriteFile(v.paths.Receipt(), []byte(text), 0o600)
}

func (e *Engine) rootSession(ctx context.Context, v *env, plan []rootops.Op) (*rootops.Client, error) {
	sudo := e.find("sudo")
	if sudo == "" {
		return nil, hteerr.New(hteerr.PrivilegedStepRequired, "sudo is not installed", "install sudo, then run the command again")
	}
	c, err := rootops.Open(ctx, e.Starter, rootops.SudoCmd(sudo, e.Self, e.Interactive), plan)
	if err != nil {
		return nil, err
	}
	c.OnReceipt = func(text string) error { return saveUserReceipt(v, text) }
	return c, nil
}

// legacyLayout is the system Apache layout of the platform.
func legacyLayout(v *env, a discover.Apache) (legacy.Layout, error) {
	return legacy.LayoutFor(v.os.Family, a.Prefix)
}

// enabledSystemVhosts are the enabled vhosts of the system Apache that no
// gash site serves: install does not stop the system Apache while any is
// left (design/hte.md 9).
func (e *Engine) enabledSystemVhosts(v *env, a discover.Apache, sites []state.Site) []string {
	l, err := legacyLayout(v, a)
	if err != nil {
		return nil
	}
	inv := legacy.Scan(e.Sysroot, l)
	var out []string
	for _, vh := range inv.Vhosts {
		if !vh.Enabled {
			continue
		}
		d := strings.ToLower(vh.Domain)
		if d != "" && slices.ContainsFunc(sites, func(s state.Site) bool { return s.Domain == d }) {
			continue
		}
		out = append(out, vh.Path)
	}
	return out
}

// Install is install: prerequisites, the plan, one sudo for the root
// steps, the receipt, the userland tree, the stack, autostart. It is
// idempotent: a second run finds every root step done.
func (e *Engine) Install(ctx context.Context, opt InstallOptions) (*InstallResult, error) {
	if err := e.CheckIdentity("install"); err != nil {
		return nil, err
	}
	v, err := e.resolve()
	if err != nil {
		return nil, err
	}
	res := &InstallResult{Plan: []string{}, Changes: []receipt.Change{}, Blocking: []string{}, Warnings: []string{}}
	if _, err := e.locked(v, "install", "", func() (any, error) { return res, e.install(ctx, v, opt, res) }); err != nil {
		return nil, err
	}
	// The stack starts once the lock is released: the systemd unit runs
	// gash hte __unit start, which takes the lock itself.
	if len(res.Blocking) > 0 {
		return res, nil
	}
	sr, err := e.ServiceStart(ctx)
	if err != nil {
		return res, err
	}
	res.Started = sr.Running
	return res, nil
}

func (e *Engine) install(ctx context.Context, v *env, opt InstallOptions, res *InstallResult) error {
	ports := state.StandardPorts
	switch opt.Ports {
	case "", "standard":
	case "high":
		ports = state.HighPorts
	default:
		return hteerr.InvalidValue(hteerr.InvalidPorts, "ports", opt.Ports, "standard", "high")
	}
	if v.os.GOOS == "darwin" && ports == state.StandardPorts {
		return hteerr.New(hteerr.PlatformUnsupported, "ports 80 and 443 on macOS (the pf redirect) are not implemented yet",
			"use gash hte install --ports high")
	}
	autostart, err := e.resolveAutostart(v, opt.Autostart)
	if err != nil {
		return err
	}
	// Prerequisites.
	m, err := e.discover(ctx, v, nil)
	if err != nil {
		return err
	}
	if len(m.php.Versions) == 0 {
		res.Warnings = append(res.Warnings, "no PHP-FPM version is installed: site create needs one (install php-fpm, then run gash hte doctor)")
	}
	if e.find("openssl") == "" {
		return hteerr.New(hteerr.OpenSSLNotFound, "openssl is not installed", discover.OpenSSLInstallHint(v.os.Family))
	}
	if !filepath.IsAbs(e.Self) || !filepath.IsAbs(e.Helper) {
		return fmt.Errorf("hte: the gash binary and the hosts helper need absolute paths")
	}
	sum, err := sha256File(e.Helper)
	if err != nil {
		return hteerr.Newf(hteerr.PlatformUnsupported, "reinstall gash: the release ships gash-hosts-helper next to gash",
			"the hosts helper %s cannot be read: %v", e.Helper, err)
	}
	// An uninstall stage: restore it, or say no.
	if found := stages(v); len(found) > 0 && opt.RestoreStage == "" && !opt.NoRestore {
		return hteerr.Newf(hteerr.MissingValue, "pass --restore-stage "+found[len(found)-1]+" to get the sites back, or --no-restore",
			"an uninstall stage exists: %s", strings.Join(found, ", "))
	}
	if opt.RestoreStage != "" && !slices.Contains(stages(v), filepath.Clean(opt.RestoreStage)) {
		return hteerr.Newf(hteerr.InvalidState, "the stages are "+v.paths.Stage("<UTC timestamp>"), "%s is not an uninstall stage", opt.RestoreStage)
	}

	// The plan.
	plan := []rootops.Op{
		{Op: rootops.OpInstallHelper, Src: e.Helper, SHA256: sum},
		{Op: rootops.OpWriteSudoers},
	}
	if ports == state.StandardPorts && v.os.GOOS == "linux" {
		plan = append(plan, rootops.Op{Op: rootops.OpSysctlPorts, Value: ports.HTTP})
	}
	if autostart == service.AutostartSystemd {
		plan = append(plan, rootops.Op{Op: rootops.OpLinger})
	}
	plan = append(plan, rootops.Op{Op: rootops.OpHostsSnapshot})
	sites, _, _ := v.store.LoadSites()
	if ports == state.StandardPorts {
		res.Blocking = append(res.Blocking, e.enabledSystemVhosts(v, m.apache, sites)...)
		if len(res.Blocking) == 0 && e.systemApacheInstalled(v) {
			plan = append(plan, rootops.Op{Op: rootops.OpUnitStop, Name: systemUnit(v.os.Family)})
		}
	}
	unitFile := ""
	if autostart == service.AutostartSystemd {
		unitFile = service.UnitPath(v.dirs.ConfigHome)
	}
	plan = append(plan, rootops.Op{Op: rootops.OpRecordAutostart, Mode: autostart, Path: unitFile})
	for i := range plan {
		plan[i].ID = i + 1
		res.Plan = append(res.Plan, plan[i].String())
	}
	if slices.ContainsFunc(plan, func(o rootops.Op) bool { return o.Op == rootops.OpUnitStop }) && !opt.Yes {
		return hteerr.Newf(hteerr.ConfirmationRequired, "add --yes",
			"install stops and disables the system Apache (it serves no enabled vhost); the plan: %s", strings.Join(res.Plan, "; "))
	}

	// The userland tree, restored from a stage first when asked.
	if opt.RestoreStage != "" {
		if err := restoreStage(v, opt.RestoreStage); err != nil {
			return err
		}
		res.Restored = opt.RestoreStage
	}
	set, err := v.store.LoadSettings()
	if err != nil {
		if !errors.Is(err, state.ErrNotInstalled) {
			return err
		}
		set = state.DefaultSettings(ports, autostart)
	}
	set.Ports, set.Autostart = ports, autostart
	if err := set.Validate(); err != nil {
		return err
	}

	// One sudo for every root step.
	c, err := e.rootSession(ctx, v, plan)
	if err != nil {
		return err
	}
	defer c.Close()
	last := plan[len(plan)-1]
	for _, o := range plan[:len(plan)-1] {
		r, err := c.Run(o.ID)
		res.Changes = append(res.Changes, r.Changes...)
		res.Warnings = append(res.Warnings, r.Warnings...)
		if err != nil {
			return err
		}
	}
	if err := v.store.SaveSettings(set); err != nil {
		return err
	}
	res.Settings = set
	if err := e.setupAutostart(ctx, v, autostart); err != nil {
		return err
	}
	r, err := c.Run(last.ID)
	res.Changes = append(res.Changes, r.Changes...)
	if err != nil {
		return err
	}
	c.Close()

	// The stack.
	if len(res.Blocking) > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("the system Apache still serves %d enabled vhosts, so it keeps ports %d and %d: "+
			"run gash hte migrate to move them (or exclude them), then the stack starts", len(res.Blocking), ports.HTTP, ports.HTTPS))
	}
	return nil
}

// systemApacheInstalled reports a system Apache unit or init script.
func (e *Engine) systemApacheInstalled(v *env) bool {
	name := systemUnit(v.os.Family)
	for _, p := range []string{"/etc/init.d/" + name, "/lib/systemd/system/" + name + ".service", "/usr/lib/systemd/system/" + name + ".service"} {
		if e.Sysroot.Exists(p) {
			return true
		}
	}
	return false
}

// setupAutostart writes and enables the user unit (systemd); the other
// modes need no file.
func (e *Engine) setupAutostart(ctx context.Context, v *env, mode string) error {
	if mode != service.AutostartSystemd {
		return nil
	}
	unit, err := service.SystemdUnit(e.Self)
	if err != nil {
		return err
	}
	p := service.UnitPath(v.dirs.ConfigHome)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := renameio.WriteFile(p, unit, 0o644); err != nil {
		return err
	}
	if err := e.systemctlUser(ctx, v, "daemon-reload"); err != nil {
		return err
	}
	return e.systemctlUser(ctx, v, "enable", service.UnitName)
}

// UninstallOptions are the flags of uninstall.
type UninstallOptions struct {
	Yes bool
}

// UninstallResult is what uninstall did.
type UninstallResult struct {
	Stage    string           `json:"stage"`
	Restored []receipt.Change `json:"restored"`
	Warnings []string         `json:"warnings"`
}

// Uninstall is uninstall: stop the stack, remove autostart, restore the
// receipt's pre-install state from the root copy (refused when the user
// copy differs: receipt_mismatch), then move the userland tree into
// hte-stage-<ts>/, which a later install offers to restore.
func (e *Engine) Uninstall(ctx context.Context, opt UninstallOptions) (*UninstallResult, error) {
	if err := e.CheckIdentity("uninstall"); err != nil {
		return nil, err
	}
	if !opt.Yes {
		return nil, hteerr.New(hteerr.ConfirmationRequired, "uninstall removes the stack and restores the system", "add --yes")
	}
	v, err := e.resolve()
	if err != nil {
		return nil, err
	}
	res := &UninstallResult{Restored: []receipt.Change{}, Warnings: []string{}}
	lockDir := v.paths.StateDir
	if _, err := os.Stat(lockDir); err != nil {
		return nil, hteerr.New(hteerr.HTENotInstalled, lockDir+" does not exist", "nothing to uninstall")
	}
	out, err := e.locked(v, "uninstall", "", func() (any, error) { return res, e.uninstall(ctx, v, res) })
	if err != nil {
		return nil, err
	}
	// The tree moves after the lock is released (the lock file is in it).
	return out.(*UninstallResult), e.stage(v, res)
}

func (e *Engine) uninstall(ctx context.Context, v *env, res *UninstallResult) error {
	rootPath := receipt.RootPath(e.User)
	rootData, rootErr := e.Sysroot.ReadFile(rootPath)
	userData, userErr := os.ReadFile(v.paths.Receipt())
	switch {
	case rootErr == nil || userErr == nil:
		if err := receipt.Compare(v.paths.Receipt(), userData, rootPath, rootData); err != nil {
			return err
		}
	}
	var r *receipt.Receipt
	if rootErr == nil {
		var err error
		if r, err = receipt.Parse(rootData, rootPath); err != nil {
			return err
		}
	}
	// The stack and autostart.
	set, _ := v.store.LoadSettings()
	if e.systemdUnit(v, set) {
		if err := e.systemctlUser(ctx, v, "disable", "--now", service.UnitName); err != nil {
			res.Warnings = append(res.Warnings, err.Error())
		}
		os.Remove(service.UnitPath(v.dirs.ConfigHome))
		e.systemctlUser(ctx, v, "daemon-reload")
	}
	if err := e.supervisor(v).Stop(ctx); err != nil {
		return err
	}
	if r == nil || len(r.Changes) == 0 {
		return nil
	}
	// Every change, last first, then the receipt itself. A system Apache
	// that migrate left running (HTE-70) is reloaded once its vhosts are
	// back, so that it serves them again.
	var plan []rootops.Op
	for i := len(r.Changes) - 1; i >= 0; i-- {
		plan = append(plan, rootops.Op{ID: len(plan) + 1, Op: rootops.OpRestore, Change: r.Changes[i].ID})
	}
	if unit := reloadAfterRestore(r, v.os.Family); unit != "" {
		plan = append(plan, rootops.Op{ID: len(plan) + 1, Op: rootops.OpUnitReload, Name: unit})
	}
	plan = append(plan, rootops.Op{ID: len(plan) + 1, Op: rootops.OpFinish})
	c, err := e.rootSession(ctx, v, plan)
	if err != nil {
		return err
	}
	defer c.Close()
	for i, o := range plan {
		resp, err := c.Run(o.ID)
		res.Warnings = append(res.Warnings, resp.Warnings...)
		if err != nil && o.Op == rootops.OpUnitReload {
			res.Warnings = append(res.Warnings, "the system Apache was not reloaded: "+err.Error())
			continue
		}
		if err != nil {
			return err
		}
		if i < len(r.Changes) {
			res.Restored = append(res.Restored, r.Changes[len(r.Changes)-1-i])
		}
	}
	return nil
}

// reloadAfterRestore is the system Apache unit to reload after the
// restores: when the receipt archived vhosts but never stopped the unit.
func reloadAfterRestore(r *receipt.Receipt, f platform.Family) string {
	unit := systemUnit(f)
	vhosts := false
	for _, c := range r.Changes {
		switch {
		case c.Kind == receipt.Unit && rootops.ApacheUnit(c.Name):
			return ""
		case c.Kind == receipt.LegacyVhost:
			vhosts = true
		}
	}
	if !vhosts {
		return ""
	}
	return unit
}

// stage moves the userland tree (config, state, data) into a new stage
// directory and removes the runtime directory.
func (e *Engine) stage(v *env, res *UninstallResult) error {
	ts := e.now().UTC().Format("20060102T150405Z")
	dir := v.paths.Stage(ts)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for name, src := range map[string]string{"config": v.paths.ConfigDir, "state": v.paths.StateDir, "data": v.paths.DataDir} {
		if _, err := os.Lstat(src); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err := os.Rename(src, filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	os.RemoveAll(v.paths.RuntimeDir)
	res.Stage = dir
	return nil
}

// restoreStage moves a stage's tree back into place; a target that exists
// and is not empty is refused.
func restoreStage(v *env, dir string) error {
	for name, dst := range map[string]string{"config": v.paths.ConfigDir, "state": v.paths.StateDir, "data": v.paths.DataDir} {
		src := filepath.Join(dir, name)
		if _, err := os.Lstat(src); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if entries, err := os.ReadDir(dst); err == nil && len(entries) > 0 && !(name == "state" && onlyLock(entries)) {
			return hteerr.Newf(hteerr.InvalidState, "move it away first", "%s exists and is not empty", dst)
		}
		os.RemoveAll(dst)
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return err
		}
		if err := os.Rename(src, dst); err != nil {
			return err
		}
	}
	// The staged receipt copy belongs to the uninstalled stack: install
	// writes a new one from the root copy.
	os.Remove(v.paths.Receipt())
	return os.Remove(dir)
}

// onlyLock reports a state directory holding only what this install
// already wrote (the lock and the audit log).
func onlyLock(entries []os.DirEntry) bool {
	for _, en := range entries {
		if !slices.Contains([]string{"lock", "audit.jsonl", "php-versions.json"}, en.Name()) {
			return false
		}
	}
	return true
}
