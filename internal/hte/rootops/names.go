// Gash 4.1. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package rootops

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/mauriziofonte/gash/internal/hte/child"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/receipt"
	"github.com/mauriziofonte/gash/internal/hte/service"
)

// migrateNames is OpMigrateNames. It is driven by the receipt: it moves
// only what the receipt of this user names. The receipt is saved after
// every step, where it still lives (receipt.Validate accepts paths under
// both root directories), so that an interrupted move leaves a receipt
// that names every file where it is: running the operation again finishes
// the move, and uninstall undoes everything from it as well. The receipt
// itself moves last.
//
// The markers of the hosts block are not touched here: the hosts helper
// of Gash 4.0 may still be the installed one (migrate --apply does not
// replace it), and it only knows the legacy markers. The user side gives
// the block the current markers once the current helper is in place.
func (s *server) migrateNames(ctx context.Context) ([]string, error) {
	legacy := s.host(receipt.LegacyRootPath(s.user))
	if s.dir == receipt.RootDir {
		if s.r != nil && exists(legacy) {
			return nil, s.dropLegacyCopy(legacy)
		}
		return nil, nil
	}
	if s.r == nil {
		return nil, nil
	}
	// The new root directory exists before anything moves into it, readable
	// by the user as save makes it (the user side compares the copies).
	if err := os.MkdirAll(s.host(receipt.RootDir), 0o755); err != nil {
		return nil, err
	}
	if err := s.moveRootFiles(); err != nil {
		return nil, err
	}
	var warnings []string
	w, err := s.moveSudoers(ctx)
	warnings = append(warnings, w...)
	if err != nil {
		return warnings, err
	}
	if err := s.moveSysctl(); err != nil {
		return warnings, err
	}
	w, err = s.movePF(ctx)
	warnings = append(warnings, w...)
	if err != nil {
		return warnings, err
	}
	for i := range s.r.Changes {
		c := &s.r.Changes[i]
		if c.Kind == receipt.Autostart && c.Path != "" {
			c.Path = autostartPath(c.Path)
		}
	}
	// The receipt moves last: written at its new place, then the legacy
	// copy goes, with the legacy directory once nothing is left in it.
	s.dir = receipt.RootDir
	if err := s.save(); err != nil {
		return warnings, err
	}
	if err := os.Remove(legacy); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return warnings, err
	}
	removeEmptyDirs(s.host(receipt.LegacyRootDir))
	return warnings, nil
}

// dropLegacyCopy removes a legacy receipt left next to the current one by
// a move interrupted after the current one was written: it is that copy
// when it was created at the same moment and its changes are the first
// changes of the current receipt. Any other legacy receipt (an install of
// Gash 4.0 made after the move) still undoes something: a human decides.
func (s *server) dropLegacyCopy(legacy string) error {
	old, ok, err := receipt.Load(legacy)
	copyOf := err == nil && ok && old.User == s.r.User && old.Created.Equal(s.r.Created) && len(old.Changes) <= len(s.r.Changes)
	for i := 0; copyOf && i < len(old.Changes); i++ {
		copyOf = old.Changes[i].ID == s.r.Changes[i].ID && old.Changes[i].Kind == s.r.Changes[i].Kind
	}
	if !copyOf {
		return hteerr.Newf(hteerr.InvalidState, "keep the receipt whose changes are on this system and move the other one away, then run the command again",
			"%s and %s both exist and are not copies of the same install", receipt.LegacyRootPath(s.user), receipt.RootPath(s.user))
	}
	if err := os.Remove(legacy); err != nil {
		return err
	}
	removeEmptyDirs(s.host(receipt.LegacyRootDir))
	return nil
}

// newRootPath maps a path under LegacyRootDir to the same path under
// RootDir; ok is false for any other path.
func newRootPath(p string) (string, bool) {
	rest, ok := strings.CutPrefix(p, receipt.LegacyRootDir+"/")
	if !ok {
		return p, false
	}
	return path.Join(receipt.RootDir, rest), true
}

// moveRootFiles moves the backups and archives the receipt names from
// LegacyRootDir to RootDir, saving the receipt after each one.
func (s *server) moveRootFiles() error {
	for i := range s.r.Changes {
		c := &s.r.Changes[i]
		refs := []*string{&c.Backup, &c.Archive}
		if c.File != nil && c.File.Prior != nil {
			refs = append(refs, c.File.Prior)
		}
		for _, ref := range refs {
			to, ok := newRootPath(*ref)
			if !ok {
				continue
			}
			if err := s.moveOwned(*ref, to); err != nil {
				return err
			}
			*ref = to
			if err := s.save(); err != nil {
				return err
			}
		}
	}
	return nil
}

// moveOwned moves a file gash owns from one system path to another; a
// file already at its destination (an earlier, interrupted run) is fine,
// and so is one that is at neither place (a pending change that never
// made it).
func (s *server) moveOwned(from, to string) error {
	src, dst := s.host(from), s.host(to)
	switch {
	case !exists(src):
		return nil
	case exists(dst):
		return hteerr.Newf(hteerr.InvalidState, "move one of them away, then run gash localweb install again",
			"%s and %s both exist", from, to)
	}
	return moveFile(src, dst)
}

// replaceFile installs data at sys through tmpSys, validated: the write of
// writeFile without a receipt change (the change that names the file is
// rewritten by the caller).
func (s *server) replaceFile(sys string, data []byte, mode os.FileMode, tmpSys string, validate func(tmpHost string) error) error {
	if err := s.secureParents(sys); err != nil {
		return err
	}
	tmp := s.host(tmpSys)
	os.Remove(tmp)
	if err := writeExcl(tmp, data, mode); err != nil {
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
	return os.Rename(tmp, s.host(sys))
}

// moveFileChange moves a file the receipt names under its legacy name
// (from) to its current name (to), with the current content data:
//
//   - from exists: data is written at to (validated), the changes that
//     named from name to with the checksum of data, the receipt is saved,
//     and from goes when it still holds what this install wrote. A from
//     that changed since (a later install by another user, an edit by
//     hand) is left in place, with a warning.
//   - from is gone (an earlier, interrupted run, or removed by hand):
//     nothing is written; the changes name to with the checksum of data,
//     so uninstall removes to only when it holds what gash writes.
//
// A receipt that does not name from leaves both names alone.
func (s *server) moveFileChange(from, to string, data []byte, mode os.FileMode, tmpSys string, validate func(string) error) ([]string, error) {
	recorded := ""
	for _, c := range s.r.Changes {
		if (c.Kind == receipt.FileCreated || c.Kind == receipt.FileModified) && c.Path == from {
			recorded = c.SHA256
		}
	}
	if recorded == "" {
		return nil, nil
	}
	cur, err := os.ReadFile(s.host(from))
	present := err == nil
	if present {
		if err := s.replaceFile(to, data, mode, tmpSys, validate); err != nil {
			return nil, err
		}
	}
	for i := range s.r.Changes {
		c := &s.r.Changes[i]
		if (c.Kind == receipt.FileCreated || c.Kind == receipt.FileModified) && c.Path == from {
			c.Path, c.SHA256 = to, sum(data)
		}
	}
	if err := s.save(); err != nil {
		return nil, err
	}
	if !present {
		return nil, nil
	}
	if sum(cur) != recorded {
		return []string{from + " changed since install (another user's install, or an edit by hand): left in place"}, nil
	}
	if err := os.Remove(s.host(from)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return nil, nil
}

// moveSudoers moves the sudoers drop-in, checked with visudo as
// write_sudoers checks it.
func (s *server) moveSudoers(ctx context.Context) ([]string, error) {
	return s.moveFileChange(LegacySudoersPath, SudoersPath, SudoersContent(s.user), 0o440, sudoersTmp, func(tmp string) error {
		res, err := s.cmd(ctx, "visudo", "-c", "-f", tmp)
		if err != nil {
			return err
		}
		if !res.Success() {
			return hteerr.Newf(hteerr.PrivilegedStepRequired, "", "visudo refused the drop-in: %s", child.Output(res))
		}
		return nil
	})
}

// moveSysctl moves the sysctl drop-in of the port change: the value stays,
// so the kernel setting is not touched.
func (s *server) moveSysctl() error {
	for i := range s.r.Changes {
		c := &s.r.Changes[i]
		if c.Kind != receipt.Sysctl || c.File == nil || c.File.Path != LegacySysctlFile {
			continue
		}
		var applied string
		if err := receipt.Decode(c.Applied, &applied); err != nil {
			return hteerr.Newf(hteerr.InvalidState, "", "receipt change %d: %v", c.ID, err)
		}
		present := exists(s.host(LegacySysctlFile))
		if present {
			if err := writeAtomic(s.host(SysctlFile), sysctlContent(applied), 0o644); err != nil {
				return err
			}
		}
		c.File.Path = SysctlFile
		if err := s.save(); err != nil {
			return err
		}
		if present {
			if err := os.Remove(s.host(LegacySysctlFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

// movePF moves the macOS pf redirect: the anchor file and the LaunchDaemon
// under their current names, the rules loaded into the current anchor,
// then the legacy anchor emptied and the legacy LaunchDaemon unloaded (a
// failure there is a warning: the receipt already names the current
// anchor, and the legacy one holds nothing after the next boot).
func (s *server) movePF(ctx context.Context) ([]string, error) {
	i := slices.IndexFunc(s.r.Changes, func(c receipt.Change) bool { return c.Kind == receipt.PFRedirect && c.Name == LegacyPFAnchor })
	if i < 0 {
		return nil, nil
	}
	var to receipt.Redirect
	if err := receipt.Decode(s.r.Changes[i].Applied, &to); err != nil {
		return nil, hteerr.Newf(hteerr.InvalidState, "", "receipt change %d: %v", s.r.Changes[i].ID, err)
	}
	warnings, err := s.moveFileChange(LegacyPFAnchorFile, PFAnchorFile, PFRules(to.HTTP, to.HTTPS), 0o644, pfAnchorTmp, func(tmp string) error {
		return s.mustCmd(ctx, "pfctl", "-n", "-a", PFAnchor, "-f", tmp)
	})
	if err != nil {
		return warnings, err
	}
	w, err := s.moveFileChange(LegacyPFDaemonPath, PFDaemonPath, PFDaemon(), 0o644, pfDaemonTmp, func(tmp string) error {
		return s.mustCmd(ctx, "plutil", "-lint", tmp)
	})
	warnings = append(warnings, w...)
	if err != nil {
		return warnings, err
	}
	pending := s.r.Changes[i].Pending
	if !pending {
		if err := s.mustCmd(ctx, "pfctl", pfLoad...); err != nil {
			return warnings, err
		}
	}
	s.r.Changes[i].Name, s.r.Changes[i].Path = PFAnchor, PFAnchorFile
	if err := s.save(); err != nil {
		return warnings, err
	}
	if !pending {
		// Loaded at boot, or not at all since install: either is fine.
		_, _ = s.cmd(ctx, "launchctl", "bootout", "system/"+LegacyPFDaemonLabel)
		if err := s.mustCmd(ctx, "pfctl", "-a", LegacyPFAnchor, "-F", "all"); err != nil {
			warnings = append(warnings, "the pf anchor "+LegacyPFAnchor+" was not emptied ("+err.Error()+"): it empties at the next boot")
		}
	}
	return warnings, nil
}

// autostartPath is the autostart file of a receipt change under its
// current name: the user unit and the LaunchAgent of Gash 4.0 and older
// were named after hte.
func autostartPath(p string) string {
	switch path.Base(p) {
	case service.LegacyUnitName:
		return path.Join(path.Dir(p), service.UnitName)
	case service.LegacyLaunchAgentLabel + ".plist":
		return path.Join(path.Dir(p), service.LaunchAgentLabel+".plist")
	}
	return p
}
