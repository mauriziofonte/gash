// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package render

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/google/renameio/v2"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/state"
)

// Layout is the render directory of design/hte.md section 2: complete
// generations gen-<n>/, the symlink current (what Apache and FPM are
// started and reloaded from) and the symlink previous (the rollback
// target). Both links are relative ("gen-<n>"). Callers hold the state
// lock around every method that writes.
//
// A generation is rendered for its final directory (Input.GenDir =
// GenDir(n)), so its includes name gen-<n> and stay valid while current
// moves on: a graceful reload reads current/httpd.conf and everything it
// includes from the same generation.
type Layout struct {
	Dir string // $XDG_STATE_HOME/gash/hte/render
}

const (
	currentLink  = "current"
	previousLink = "previous"
	genPrefix    = "gen-"
	tmpSuffix    = ".tmp"
	layoutDir    = 0o700
	layoutFile   = 0o600
)

var genNameRE = regexp.MustCompile(`^gen-([1-9][0-9]{0,8})$`)

// GenDir is the absolute directory of generation n.
func (l Layout) GenDir(n int) string { return filepath.Join(l.Dir, genName(n)) }

func genName(n int) string { return genPrefix + strconv.Itoa(n) }

func parseGen(name string) (int, bool) {
	m := genNameRE.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// Generations lists the complete generations, ascending.
func (l Layout) Generations() ([]int, error) {
	ents, err := os.ReadDir(l.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []int
	for _, e := range ents {
		if n, ok := parseGen(e.Name()); ok && e.IsDir() && e.Type()&fs.ModeSymlink == 0 {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out, nil
}

func (l Layout) link(name string) (int, bool, error) {
	target, err := os.Readlink(filepath.Join(l.Dir, name))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, hteerr.Newf(hteerr.InvalidState, "", "%s: %v", filepath.Join(l.Dir, name), err)
	}
	n, ok := parseGen(target)
	if !ok {
		return 0, false, hteerr.Newf(hteerr.InvalidState, "",
			"%s points to %q, not to a generation", filepath.Join(l.Dir, name), target)
	}
	if fi, err := os.Lstat(l.GenDir(n)); err != nil || !fi.IsDir() {
		return 0, false, hteerr.Newf(hteerr.InvalidState, "",
			"%s points to %s, which is not a generation directory", filepath.Join(l.Dir, name), target)
	}
	return n, true, nil
}

// Current is the generation in service; ok is false before the first swap.
func (l Layout) Current() (n int, ok bool, err error) { return l.link(currentLink) }

// Previous is the rollback target; ok is false when there is none.
func (l Layout) Previous() (n int, ok bool, err error) { return l.link(previousLink) }

// Next is the number of the next generation: one more than any generation
// on disk, complete or not, so a number is never reused.
func (l Layout) Next() (int, error) {
	ents, err := os.ReadDir(l.Dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return 0, err
	}
	high := 0
	for _, e := range ents {
		if n, ok := parseGen(strings.TrimSuffix(e.Name(), tmpSuffix)); ok && n > high {
			high = n
		}
	}
	return high + 1, nil
}

// Write stores g as generation n: every file goes into gen-<n>.tmp and is
// synced, then the directory is renamed to gen-<n>, so a generation on disk
// is always complete. An existing gen-<n> is refused.
func (l Layout) Write(n int, g *Generation) (string, error) {
	if n < 1 {
		return "", fmt.Errorf("render: generation %d", n)
	}
	final := l.GenDir(n)
	if _, err := os.Lstat(final); err == nil {
		return "", hteerr.Newf(hteerr.InvalidState, "", "%s already exists", final)
	}
	if err := os.MkdirAll(l.Dir, layoutDir); err != nil {
		return "", err
	}
	tmp := final + tmpSuffix
	if err := os.RemoveAll(tmp); err != nil {
		return "", err
	}
	if err := os.Mkdir(tmp, layoutDir); err != nil {
		return "", err
	}
	for _, f := range g.Files {
		if !filepath.IsLocal(f.Path) || filepath.Clean(f.Path) != f.Path {
			os.RemoveAll(tmp)
			return "", fmt.Errorf("render: generation file %q is not a clean relative path", f.Path)
		}
		if err := writeSynced(filepath.Join(tmp, f.Path), f.Data); err != nil {
			os.RemoveAll(tmp)
			return "", err
		}
	}
	if err := os.Rename(tmp, final); err != nil {
		os.RemoveAll(tmp)
		return "", err
	}
	return final, syncDir(l.Dir)
}

func writeSynced(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), layoutDir); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, layoutFile)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// Swap makes generation n current: previous takes the generation that was
// current, then current is replaced atomically (renameio.Symlink). Swapping
// to the current generation changes nothing.
func (l Layout) Swap(n int) error {
	if fi, err := os.Lstat(l.GenDir(n)); err != nil || !fi.IsDir() {
		return hteerr.Newf(hteerr.InvalidState, "", "%s is not a generation directory", l.GenDir(n))
	}
	cur, ok, err := l.Current()
	if err != nil {
		return err
	}
	if ok && cur == n {
		return nil
	}
	if ok {
		if err := renameio.Symlink(genName(cur), filepath.Join(l.Dir, previousLink)); err != nil {
			return err
		}
	}
	if err := renameio.Symlink(genName(n), filepath.Join(l.Dir, currentLink)); err != nil {
		return err
	}
	return syncDir(l.Dir)
}

// Rollback makes the previous generation current again and removes the
// previous link; it returns the restored generation. The failed one stays
// on disk until Prune. Without a previous generation it fails.
func (l Layout) Rollback() (int, error) {
	prev, ok, err := l.Previous()
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, hteerr.New(hteerr.InvalidState, "there is no previous generation to roll back to", "")
	}
	if err := renameio.Symlink(genName(prev), filepath.Join(l.Dir, currentLink)); err != nil {
		return 0, err
	}
	if err := os.Remove(filepath.Join(l.Dir, previousLink)); err != nil {
		return 0, err
	}
	return prev, syncDir(l.Dir)
}

// Prune removes every generation other than current and previous, and any
// incomplete gen-<n>.tmp directory; it returns the removed names.
func (l Layout) Prune() ([]string, error) {
	keep := map[int]bool{}
	for _, f := range []func() (int, bool, error){l.Current, l.Previous} {
		n, ok, err := f()
		if err != nil {
			return nil, err
		}
		if ok {
			keep[n] = true
		}
	}
	ents, err := os.ReadDir(l.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, e := range ents {
		name := e.Name()
		n, ok := parseGen(strings.TrimSuffix(name, tmpSuffix))
		if !ok || !e.IsDir() || (keep[n] && !strings.HasSuffix(name, tmpSuffix)) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(l.Dir, name)); err != nil {
			return removed, err
		}
		removed = append(removed, name)
	}
	return removed, nil
}

// ReadOverlays reads the Apache overlays a generation of sites includes
// (Input.Overlays): the global one and those of the enabled managed
// sites, each by its path, the missing ones left out. The files are only
// read: a generation holds copies, and the user's files are never
// written (LEDGER HTE-71). A path that is not a regular file (a
// directory, a FIFO) is invalid_state.
func ReadOverlays(p state.Paths, sites []state.Site) (map[string][]byte, error) {
	paths := []string{p.ApacheOverlay()}
	for _, s := range sites {
		if s.Enabled && s.Kind == state.KindManaged {
			paths = append(paths, p.SiteOverlay(s.Domain))
		}
	}
	out := map[string][]byte{}
	for _, path := range paths {
		fi, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, hteerr.Newf(hteerr.InvalidState, "", "the overlay %s: %v", path, err)
		}
		if !fi.Mode().IsRegular() {
			return nil, hteerr.Newf(hteerr.InvalidState, "make it a regular file, or remove it",
				"the overlay %s is not a regular file", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, hteerr.Newf(hteerr.InvalidState, "", "the overlay %s: %v", path, err)
		}
		out[path] = data
	}
	return out, nil
}
