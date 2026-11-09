// Gash 4.1. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/renameio/v2"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/service"
	"github.com/mauriziofonte/gash/internal/hte/state"
	"github.com/mauriziofonte/gash/internal/platform"
)

// Gash 4.0 and older kept the userland tree under the hte names
// (~/.config/gash/hte, hte.toml, gash-hte.service). The first gash
// localweb command that runs as the user moves it to the localweb names
// (moveLayout); root, doctor and the hidden commands the system runs
// (Engine.KeepLayout) read it where it is until then.

// LayoutMove is what moving the tree of Gash 4.0 and older did.
type LayoutMove struct {
	// Moved is one "FROM -> TO" line per directory that moved.
	Moved []string `json:"moved"`
	// Restarted reports a stack that ran from the old tree and runs again
	// from the new one.
	Restarted bool     `json:"restarted"`
	Warnings  []string `json:"warnings,omitempty"`
}

// legacyTree reports a tree of Gash 4.0 and older not moved yet: hte.toml
// exists and localweb.toml does not.
func legacyTree(cur, old state.Paths) bool {
	_, errOld := os.Stat(old.Settings())
	_, errNew := os.Lstat(cur.Settings())
	return errOld == nil && errors.Is(errNew, fs.ErrNotExist)
}

func (e *Engine) newEnv(dirs platform.Dirs, p state.Paths) *env {
	return &env{dirs: dirs, paths: p, os: platform.DetectOS(e.Sysroot, e.GOOS), store: state.Store{Paths: p}}
}

func (e *Engine) lockWait() time.Duration {
	if e.LockWait == 0 {
		return 30 * time.Second
	}
	return e.LockWait
}

// moveLayout moves the tree of old (the legacy layout) to v under both
// state locks: the current one first (a second command that waits for it
// finds the tree moved), then the legacy one (a command still working on
// the old tree finishes first). The stack is stopped, the directories
// move, the rendering of the old paths is dropped, raw sites name the new
// paths, autostart gets its new unit; a stack that ran starts again.
func (e *Engine) moveLayout(ctx context.Context, v, old *env) error {
	l, err := state.AcquireLock(v.paths.Lock(), e.lockWait(), nil)
	if err != nil {
		return err
	}
	defer func() { _ = l.Unlock() }()
	if !legacyTree(v.paths, old.paths) {
		return nil // another command moved it while this one waited
	}
	ol, err := state.AcquireLock(old.paths.Lock(), e.lockWait(), nil)
	if err != nil {
		return err
	}
	defer func() { _ = ol.Unlock() }()
	mv := &LayoutMove{Moved: []string{}}
	err = e.moveTree(ctx, v, old, mv)
	if legacyTree(v.paths, old.paths) {
		e.audit(old, "layout move", "", mv, err) // the move was undone
	} else {
		e.audit(v, "layout move", "", mv, err)
	}
	if e.OnLayoutMove != nil && len(mv.Moved) > 0 {
		e.OnLayoutMove(*mv)
	}
	return err
}

// moved is one rename moveTree made, undone in reverse order on failure.
type moved struct{ from, to string }

// undoMoves puts every rename back, last first; it returns what failed.
func undoMoves(done []moved) error {
	var errs []error
	for i := len(done) - 1; i >= 0; i-- {
		if err := os.MkdirAll(filepath.Dir(done[i].from), 0o700); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := os.Rename(done[i].to, done[i].from); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// moveTree moves the old tree into the new one. hte.toml, which makes the
// old tree a tree to move (legacyTree), moves last: until then the next
// command finds the old tree still to move and finishes an interrupted
// move (every step finds its own work done); after it, the move is
// complete.
func (e *Engine) moveTree(ctx context.Context, v, old *env, mv *LayoutMove) error {
	set, err := old.store.LoadSettings()
	if err != nil {
		return err
	}
	// A site file that does not load moves as it is: doctor names it.
	sites, _, err := old.store.LoadSites()
	if err != nil {
		return err
	}

	// Stop what runs from the old tree: its unit, then any master by its
	// PID file.
	sup := e.supervisor(old)
	running, err := sup.Running()
	if err != nil {
		return err
	}
	if running {
		if err := e.stopStack(ctx, old, set); err != nil {
			return err
		}
	}
	// On any failure before hte.toml moved, every rename is undone and the
	// stack runs again from the old tree.
	var done []moved
	undo := func(cause error) error {
		if err := undoMoves(done); err != nil {
			cause = fmt.Errorf("%w; putting the files back failed: %v", cause, err)
		}
		if running {
			if err := e.startStack(ctx, old, set); err != nil {
				return fmt.Errorf("%w; starting the stack again from %s failed: %v", cause, old.paths.ConfigDir, err)
			}
		}
		return cause
	}
	keep := map[string]bool{"lock": true, state.LegacySettingsName: true}
	for _, d := range []moved{{old.paths.ConfigDir, v.paths.ConfigDir}, {old.paths.StateDir, v.paths.StateDir}, {old.paths.DataDir, v.paths.DataDir}} {
		if err := moveDir(d.from, d.to, keep, &done); err != nil {
			return undo(err)
		}
	}
	// Raw sites name the new directories (a site rewritten by an earlier,
	// interrupted move has nothing left to replace).
	movedSites, _, err := v.store.LoadSites()
	if err == nil {
		var raw []rawFile
		if raw, err = rawRewrites(old.paths, v.paths, movedSites); err == nil {
			err = writeRaw(raw)
		}
	}
	if err != nil {
		return undo(err)
	}
	if err := os.MkdirAll(v.paths.ConfigDir, 0o700); err != nil {
		return undo(err)
	}
	if err := os.Rename(old.paths.Settings(), v.paths.Settings()); err != nil {
		return undo(err)
	}
	_ = os.Remove(old.paths.ConfigDir) // empty now; anything else stays for a human
	for _, d := range []moved{{old.paths.ConfigDir, v.paths.ConfigDir}, {old.paths.StateDir, v.paths.StateDir}, {old.paths.DataDir, v.paths.DataDir}} {
		mv.Moved = append(mv.Moved, d.from+" -> "+d.to)
	}

	// The tree moved. The rendering names the old paths and the runtime
	// directory holds the stopped masters' files: both go, the next start
	// renders again.
	_ = os.RemoveAll(v.paths.RenderDir())
	_ = os.RemoveAll(old.paths.RuntimeDir)
	if left, err := os.ReadDir(old.paths.ConfigDir); err == nil && len(left) > 0 {
		mv.Warnings = append(mv.Warnings, old.paths.ConfigDir+" still holds files gash does not know: move them yourself")
	}
	mv.Warnings = append(mv.Warnings, overlayWarnings(old.paths, v.paths, sites)...)
	mv.Warnings = append(mv.Warnings, e.moveAutostart(ctx, v, set)...)

	if !running {
		return nil
	}
	if sites, err = e.sites(v); err == nil {
		if _, err = e.applyStack(ctx, v, set, sites); err == nil {
			err = e.startStack(ctx, v, set)
		}
	}
	if err != nil {
		if he, ok := hteerr.As(err); ok {
			return hteerr.Newf(he.Code, he.Hint, "the files moved to %s; starting the stack again failed: %s", v.paths.ConfigDir, he.Details)
		}
		return fmt.Errorf("the files moved to %s; starting the stack again failed: %w", v.paths.ConfigDir, err)
	}
	mv.Restarted = true
	return nil
}

// moveDir moves directory from to to: one rename when to does not exist
// and from holds none of keep, else entry by entry into it. The entries
// named in keep stay in from (the lock this command holds, the settings
// file that moves last); from goes once nothing but the lock is left in
// it. An entry both hold is invalid_state.
func moveDir(from, to string, keep map[string]bool, done *[]moved) error {
	entries, err := os.ReadDir(from)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	kept := slices.ContainsFunc(entries, func(en os.DirEntry) bool { return keep[en.Name()] })
	if _, err := os.Lstat(to); errors.Is(err, fs.ErrNotExist) && !kept {
		if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
			return err
		}
		if err := os.Rename(from, to); err != nil {
			return err
		}
		*done = append(*done, moved{from, to})
		return nil
	}
	if err := os.MkdirAll(to, 0o700); err != nil {
		return err
	}
	for _, en := range entries {
		if keep[en.Name()] {
			continue
		}
		src, dst := filepath.Join(from, en.Name()), filepath.Join(to, en.Name())
		if _, err := os.Lstat(dst); err == nil {
			if merged, err := mergeState(en.Name(), src, dst); merged || err != nil {
				if err != nil {
					return err
				}
				continue
			}
			return hteerr.Newf(hteerr.InvalidState, "move one of them away, then run the command again", "%s and %s both exist", src, dst)
		}
		if err := os.Rename(src, dst); err != nil {
			return err
		}
		*done = append(*done, moved{src, dst})
	}
	if left, err := os.ReadDir(from); err == nil && !slices.ContainsFunc(left, func(en os.DirEntry) bool { return en.Name() != "lock" }) {
		_ = os.Remove(filepath.Join(from, "lock"))
		_ = os.Remove(from)
	}
	return nil
}

// mergeState joins a file of the state directory that both trees hold
// (an earlier move that did not finish wrote the old one): the audit log
// of the old tree is appended to the new one, the PHP discovery cache and
// a unit lease of the old tree are dropped. merged is false for any other
// file.
func mergeState(name, src, dst string) (merged bool, err error) {
	switch name {
	case "audit.jsonl":
		data, err := os.ReadFile(src)
		if err != nil {
			return true, err
		}
		f, err := os.OpenFile(dst, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return true, err
		}
		if _, err := f.Write(data); err != nil {
			f.Close()
			return true, err
		}
		if err := f.Close(); err != nil {
			return true, err
		}
		return true, os.Remove(src)
	case "php-versions.json", "unit.lease":
		return true, os.Remove(src)
	}
	return false, nil
}

// replacer maps the directories of the old tree to those of the new one,
// as path prefixes.
func replacer(old, cur state.Paths) *strings.Replacer {
	var pairs []string
	for _, d := range [][2]string{{old.ConfigDir, cur.ConfigDir}, {old.StateDir, cur.StateDir}, {old.DataDir, cur.DataDir}, {old.RuntimeDir, cur.RuntimeDir}} {
		pairs = append(pairs, d[0]+"/", d[1]+"/")
	}
	return strings.NewReplacer(pairs...)
}

// rawFile is a site file of the new tree and its new content.
type rawFile struct {
	path string
	data []byte
}

// rawRewrites are the site files of raw sites (imported vhosts kept
// verbatim) whose configuration names the old tree (certificates, PHP-FPM
// sockets, logs), rewritten for the new one.
func rawRewrites(old, cur state.Paths, sites []state.Site) ([]rawFile, error) {
	r := replacer(old, cur)
	var out []rawFile
	for _, s := range sites {
		if s.Kind != state.KindRaw {
			continue
		}
		conf := r.Replace(s.RawConf)
		if conf == s.RawConf {
			continue
		}
		s.RawConf = conf
		data, err := state.EncodeSite(s)
		if err != nil {
			return nil, err
		}
		out = append(out, rawFile{path: cur.SiteFile(s.Domain), data: data})
	}
	return out, nil
}

// writeRaw writes the rewritten site files; a failure puts back those
// already written.
func writeRaw(files []rawFile) error {
	var prior []rawFile
	for _, f := range files {
		cur, err := os.ReadFile(f.path)
		if err == nil {
			err = renameio.WriteFile(f.path, f.data, 0o600)
		}
		if err != nil {
			for _, p := range prior {
				_ = renameio.WriteFile(p.path, p.data, 0o600)
			}
			return err
		}
		prior = append(prior, rawFile{path: f.path, data: cur})
	}
	return nil
}

// overlayWarnings names the user's own files (the Apache overlays, the
// global PHP overlays) that still name a directory of the old tree: gash
// never writes them.
func overlayWarnings(old, cur state.Paths, sites []state.Site) []string {
	files := []string{cur.ApacheOverlay()}
	for _, s := range sites {
		files = append(files, cur.SiteOverlay(s.Domain))
	}
	if inis, err := filepath.Glob(filepath.Join(cur.ConfigDir, "php", "*", "*.ini")); err == nil {
		files = append(files, inis...)
	}
	var out []string
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, d := range []string{old.ConfigDir, old.StateDir, old.DataDir, old.RuntimeDir} {
			if strings.Contains(string(data), d+"/") {
				out = append(out, fmt.Sprintf("%s names %s, which is now %s: change it there", f, d, replacer(old, cur).Replace(d+"/")))
				break
			}
		}
	}
	return out
}

// moveAutostart replaces the user unit or the LaunchAgent of Gash 4.0
// with the current one. A failure is a warning: the old unit still works
// (it runs gash hte __unit, which works through 4.x), and service commands
// use whichever unit is in place.
func (e *Engine) moveAutostart(ctx context.Context, v *env, set state.Settings) []string {
	var warnings []string
	warn := func(err error) { warnings = append(warnings, "autostart: "+err.Error()) }
	switch set.Autostart {
	case service.AutostartSystemd:
		legacy := service.LegacyUnitPath(v.dirs.ConfigHome)
		if _, err := os.Lstat(legacy); err != nil {
			return nil
		}
		unit, err := service.SystemdUnit(e.Self)
		if err == nil {
			err = renameio.WriteFile(service.UnitPath(v.dirs.ConfigHome), unit, 0o644)
		}
		if err != nil {
			warn(err)
			return warnings
		}
		// A unit left active (its masters ended) runs its stop now, not at
		// the next shutdown.
		if err := e.unitJob(ctx, v, "stop", service.LegacyUnitName); err != nil {
			warn(err)
		}
		if err := e.systemctlUser(ctx, v, "disable", service.LegacyUnitName); err != nil {
			warn(err)
		}
		os.Remove(legacy)
		if err := e.systemctlUser(ctx, v, "daemon-reload"); err != nil {
			warn(err)
		}
		if err := e.systemctlUser(ctx, v, "enable", service.UnitName); err != nil {
			warn(err)
		}
	case service.AutostartLaunchd:
		legacy := service.LegacyLaunchAgentPath(v.dirs.Home)
		if _, err := os.Lstat(legacy); err != nil {
			return nil
		}
		agent, err := service.LaunchAgent(e.Self, service.AgentEnv(v.dirs.ConfigHome, v.dirs.StateHome, v.dirs.DataHome, v.dirs.RuntimeDir))
		if err == nil {
			err = renameio.WriteFile(service.LaunchAgentPath(v.dirs.Home), agent, 0o644)
		}
		if err != nil {
			warn(err)
			return warnings
		}
		e.removeAgent(ctx, v, service.LegacyLaunchAgentLabel)
		if _, err := e.launchctl(ctx, v, "enable", e.agentTarget(service.LaunchAgentLabel)); err != nil {
			warn(err)
		}
	}
	return warnings
}
