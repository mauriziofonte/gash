// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package rootops

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"

	"github.com/mauriziofonte/gash/internal/hostshelper"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/receipt"
)

// restore undoes the last change of the root receipt. Changes are undone
// in reverse order, so each one finds the system as it left it. A pending
// change (an interrupted step) is undone as far as it happened.
func (s *server) restore(ctx context.Context, id int) ([]string, error) {
	if s.r == nil {
		return nil, hteerr.New(hteerr.InvalidState, "there is no root receipt", "")
	}
	last, ok := s.r.Last()
	if !ok || last.ID != id {
		return nil, hteerr.Newf(hteerr.InvalidState, "", "change %d is not the last change of the receipt", id)
	}
	warnings, err := s.undo(ctx, last)
	if err != nil {
		return warnings, err
	}
	s.r.Remove(id)
	return warnings, s.save()
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func (s *server) undo(ctx context.Context, c receipt.Change) ([]string, error) {
	switch c.Kind {
	case receipt.FileCreated:
		data, err := os.ReadFile(s.host(c.Path))
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if sum(data) != c.SHA256 {
			return []string{c.Path + " changed after install: left in place"}, nil
		}
		return nil, os.Remove(s.host(c.Path))
	case receipt.FileModified:
		if !exists(s.host(c.Backup)) {
			if c.Pending {
				return nil, nil
			}
			return nil, hteerr.Newf(hteerr.InvalidState, "", "the backup %s of %s is missing", c.Backup, c.Path)
		}
		if data, err := os.ReadFile(s.host(c.Path)); err == nil && sum(data) != c.SHA256 && !c.Pending {
			return []string{c.Path + " changed after install: restored anyway, its content was " + sum(data)}, moveFile(s.host(c.Backup), s.host(c.Path))
		}
		return nil, moveFile(s.host(c.Backup), s.host(c.Path))
	case receipt.DirCreated:
		if err := os.Remove(s.host(c.Path)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return []string{c.Path + " is not empty: left in place"}, nil
		}
		return nil, nil
	case receipt.Sysctl:
		var prior, applied string
		receipt.Decode(c.Prior, &prior)
		receipt.Decode(c.Applied, &applied)
		if c.File != nil {
			if c.File.Prior == nil {
				if err := os.Remove(s.host(c.File.Path)); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return nil, err
				}
			} else if exists(s.host(*c.File.Prior)) {
				if err := moveFile(s.host(*c.File.Prior), s.host(c.File.Path)); err != nil {
					return nil, err
				}
			}
		}
		if prior != applied {
			return nil, s.mustCmd(ctx, "sysctl", "-w", c.Key+"="+prior)
		}
		return nil, nil
	case receipt.Linger:
		var prior bool
		receipt.Decode(c.Prior, &prior)
		if prior || !s.env.Systemd {
			return nil, nil
		}
		return nil, s.mustCmd(ctx, "loginctl", "disable-linger", s.user)
	case receipt.Unit:
		var prior receipt.UnitState
		receipt.Decode(c.Prior, &prior)
		return nil, s.unitRestore(ctx, c.Name, prior)
	case receipt.HostsBlock:
		var prior *string
		receipt.Decode(c.Prior, &prior)
		return nil, hostshelper.Rewrite(s.host(hostshelper.HostsFile), func(h []byte) ([]byte, error) {
			if prior == nil {
				return hostshelper.Strip(h)
			}
			return hostshelper.Splice(h, []byte(*prior))
		})
	case receipt.LegacyVhost, receipt.LegacyPool:
		if exists(s.host(c.Archive)) {
			if exists(s.host(c.Path)) {
				return nil, hteerr.Newf(hteerr.InvalidState, "move one of them away", "%s exists again: %s stays in the archive", c.Path, c.Archive)
			}
			if err := moveFile(s.host(c.Archive), s.host(c.Path)); err != nil {
				return nil, err
			}
		} else if !c.Pending {
			return nil, hteerr.Newf(hteerr.InvalidState, "", "the archived %s is missing", c.Archive)
		}
		if c.Link != nil && !exists(s.host(c.Link.Path)) {
			if err := os.Symlink(c.Link.Target, s.host(c.Link.Path)); err != nil {
				return nil, err
			}
		}
		if c.Kind == receipt.LegacyPool {
			return nil, s.reloadPoolUnit(ctx, c.Path)
		}
		return nil, nil
	case receipt.Autostart:
		return nil, nil // the user side removed its own file
	}
	return nil, fmt.Errorf("rootops: cannot undo %s", c.Kind)
}

func (s *server) unitRestore(ctx context.Context, name string, prior receipt.UnitState) error {
	if s.env.Systemd {
		if prior.Enabled {
			if err := s.mustCmd(ctx, "systemctl", "enable", name+".service"); err != nil {
				return err
			}
		}
		if prior.Active {
			return s.mustCmd(ctx, "systemctl", "start", name+".service")
		}
		return nil
	}
	if prior.Enabled {
		if err := s.mustCmd(ctx, "update-rc.d", name, "enable"); err != nil {
			return err
		}
	}
	if prior.Active {
		return s.mustCmd(ctx, "service", name, "start")
	}
	return nil
}

// reloadPoolUnit reloads the system FPM of a restored pool when it runs,
// so the pool is served again.
func (s *server) reloadPoolUnit(ctx context.Context, pool string) error {
	// <PHPRoot>/<X.Y>/fpm/pool.d/<domain>.conf
	version := path.Base(path.Dir(path.Dir(path.Dir(pool))))
	name := "php" + version + "-fpm"
	if !unitRE.MatchString(name) {
		return nil
	}
	_, err := s.unitReload(ctx, name)
	return err
}
