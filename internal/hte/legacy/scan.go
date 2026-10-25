// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package legacy

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/platform"
)

// Problem is a path that could not be read; the scan goes on without it
// (LEDGER HTE-28: hte-cli stopped silently at the first one).
type Problem struct {
	Path string `json:"path"`
	Err  string `json:"error"`
}

// HostsLine is a line of the hosts file marked "# hte-cli".
type HostsLine struct {
	Line  int      `json:"line"`
	Text  string   `json:"text"`
	Addr  string   `json:"address"`
	Names []string `json:"names"`
}

// Snippet is a Debian conf-enabled file.
type Snippet struct {
	Path string `json:"path"`
}

// Inventory is everything Scan read.
type Inventory struct {
	Layout     Layout
	Vhosts     []Vhost         // sorted by path
	Pools      map[string]Pool // every parsed pool under PHPRoot, by path
	PoolErrors []Problem       // pool files that do not parse
	CertFiles  []string        // files of CertDir, sorted
	Hosts      []HostsLine
	// RunUser is APACHE_RUN_USER of envvars, "" when unset or absent.
	RunUser    string
	Snippets   []Snippet // enabled conf-enabled snippets, and loaded files of a vhost directory with no <VirtualHost>
	Indexes    []int     // NNN- prefixes of the files of the first vhost directory
	Unreadable []Problem
}

// Scan reads the legacy configuration of a layout through sr.
func Scan(sr platform.Sysroot, l Layout) *Inventory {
	inv := &Inventory{Layout: l, Pools: map[string]Pool{}}
	inv.scanVhosts(sr)
	inv.scanPools(sr)
	inv.scanCerts(sr)
	inv.scanHosts(sr)
	inv.scanEnvvars(sr)
	inv.scanSnippets(sr)
	// Unreadable entries come from several passes: report them by path.
	slices.SortStableFunc(inv.Unreadable, func(a, b Problem) int { return strings.Compare(a.Path, b.Path) })
	return inv
}

func (inv *Inventory) unreadable(p string, err error) {
	inv.Unreadable = append(inv.Unreadable, Problem{Path: p, Err: errText(err)})
}

func errText(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}

// walkConf lists the *.conf files under dir, recursively, reporting
// unreadable entries and going on.
func (inv *Inventory) walkConf(sr platform.Sysroot, dir string) []string {
	var out []string
	if !sr.Exists(dir) {
		return nil
	}
	err := sr.Walk(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			inv.unreadable(p, err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(p, ".conf") {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		inv.unreadable(dir, err)
	}
	return out
}

func (inv *Inventory) scanVhosts(sr platform.Sysroot) {
	l := inv.Layout
	seen := map[string]bool{}
	add := func(p string, enabled bool) {
		if seen[p] {
			return
		}
		seen[p] = true
		data, err := sr.ReadFile(p)
		if err != nil {
			inv.unreadable(p, err)
			return
		}
		v := Classify(p, data)
		if !v.Marked() && v.Conf != nil && len(v.Conf.Vhosts) == 0 {
			// Server configuration with no <VirtualHost>: Fedora's
			// conf.d holds welcome.conf, autoindex.conf, userdir.conf and
			// the php package's php.conf next to the vhosts, all loaded.
			// They serve no site, so they are snippets, as Debian's
			// conf-enabled files are: never a vhost that blocks install.
			if enabled {
				inv.Snippets = append(inv.Snippets, Snippet{Path: p})
			}
			return
		}
		v.Enabled = enabled
		inv.Vhosts = append(inv.Vhosts, v)
	}
	var files []string
	for _, dir := range l.VhostDirs {
		files = append(files, inv.walkConf(sr, dir)...)
	}
	if len(l.VhostDirs) > 0 {
		if ents, err := sr.ReadDir(l.VhostDirs[0]); err == nil {
			for _, e := range ents {
				if n := fileIndex(e.Name()); n >= 0 {
					inv.Indexes = append(inv.Indexes, n)
				}
			}
			slices.Sort(inv.Indexes)
		}
	}
	enabled := inv.enabledSet(sr, files)
	for _, f := range files {
		add(f, enabled[f])
	}
	// Entries of sites-enabled that are not links into a vhost directory
	// are served too: they are vhosts of their own.
	for _, extra := range inv.enabledExtras(sr) {
		add(extra, true)
	}
	slices.SortFunc(inv.Vhosts, func(a, b Vhost) int { return strings.Compare(a.Path, b.Path) })
}

// enabledSet decides which vhost files the system Apache serves.
func (inv *Inventory) enabledSet(sr platform.Sysroot, files []string) map[string]bool {
	l := inv.Layout
	out := map[string]bool{}
	switch {
	case l.AllEnabled:
		for _, f := range files {
			out[f] = true
		}
	case l.EnabledDir != "":
		for _, f := range files {
			if sr.Exists(path.Join(l.EnabledDir, path.Base(f))) {
				out[f] = true
			}
		}
	case l.MainConf != "":
		data, err := sr.ReadFile(l.MainConf)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				inv.unreadable(l.MainConf, err)
			}
			return out
		}
		for _, pattern := range includePatterns(data, l.ServerRoot) {
			for _, f := range files {
				if ok, _ := path.Match(pattern, f); ok {
					out[f] = true
				}
			}
		}
	}
	return out
}

var includeRE = lazyre.MustCompile(`(?im)^[ \t]*Include(?:Optional)?[ \t]+("[^"]+"|\S+)[ \t]*$`)

// includePatterns returns the Include targets of a main configuration, as
// absolute patterns.
func includePatterns(data []byte, serverRoot string) []string {
	var out []string
	for _, m := range includeRE.FindAllSubmatch(data, -1) {
		p := strings.Trim(string(m[1]), `"`)
		if !path.IsAbs(p) {
			p = path.Join(serverRoot, p)
		}
		out = append(out, p)
	}
	return out
}

func (inv *Inventory) enabledExtras(sr platform.Sysroot) []string {
	l := inv.Layout
	if l.EnabledDir == "" || !sr.Exists(l.EnabledDir) {
		return nil
	}
	ents, err := sr.ReadDir(l.EnabledDir)
	if err != nil {
		inv.unreadable(l.EnabledDir, err)
		return nil
	}
	var out []string
	for _, e := range ents {
		p := path.Join(l.EnabledDir, e.Name())
		if !strings.HasSuffix(e.Name(), ".conf") {
			continue
		}
		target, err := sr.Resolve(p)
		if err != nil {
			inv.unreadable(p, err)
			continue
		}
		if !sr.Exists(p) {
			inv.unreadable(p, errors.New("dangling link to "+target))
			continue
		}
		inside := false
		for _, dir := range l.VhostDirs {
			if rd, err := sr.Resolve(dir); err == nil && strings.HasPrefix(target, rd+"/") {
				inside = true
			}
		}
		if !inside {
			out = append(out, p)
		}
	}
	return out
}

// poolPathRE matches the pool files hte-cli searched: */fpm/pool.d/*.conf.
var poolPathRE = lazyre.MustCompile(`/fpm/pool\.d/[^/]+\.conf$`)

func (inv *Inventory) scanPools(sr platform.Sysroot) {
	if inv.Layout.PHPRoot == "" {
		return
	}
	for _, p := range inv.walkConf(sr, inv.Layout.PHPRoot) {
		if !poolPathRE.MatchString(p) {
			continue
		}
		data, err := sr.ReadFile(p)
		if err != nil {
			inv.unreadable(p, err)
			continue
		}
		pool, err := ParsePool(p, data)
		if err != nil {
			inv.PoolErrors = append(inv.PoolErrors, Problem{Path: p, Err: err.Error()})
			continue
		}
		inv.Pools[p] = pool
	}
}

func (inv *Inventory) scanCerts(sr platform.Sysroot) {
	dir := inv.Layout.CertDir
	if dir == "" || !sr.Exists(dir) {
		return
	}
	ents, err := sr.ReadDir(dir)
	if err != nil {
		inv.unreadable(dir, err)
		return
	}
	for _, e := range ents {
		if !e.IsDir() {
			inv.CertFiles = append(inv.CertFiles, path.Join(dir, e.Name()))
		}
	}
	slices.Sort(inv.CertFiles)
}

// scanHosts reads the lines hte-cli appended: "127.0.0.1    <d> www.<d> # hte-cli".
func (inv *Inventory) scanHosts(sr platform.Sysroot) {
	data, err := sr.ReadFile(inv.Layout.Hosts)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			inv.unreadable(inv.Layout.Hosts, err)
		}
		return
	}
	for i, line := range strings.Split(string(data), "\n") {
		body, comment, ok := strings.Cut(line, "#")
		if !ok || strings.TrimSpace(comment) != "hte-cli" {
			continue
		}
		f := strings.Fields(body)
		if len(f) < 2 {
			continue
		}
		inv.Hosts = append(inv.Hosts, HostsLine{Line: i + 1, Text: line, Addr: f[0], Names: f[1:]})
	}
}

var runUserRE = lazyre.MustCompile(`(?m)^[ \t]*export[ \t]+APACHE_RUN_USER=("?)([^"\s]*)("?)[ \t]*$`)

func (inv *Inventory) scanEnvvars(sr platform.Sysroot) {
	if inv.Layout.Envvars == "" {
		return
	}
	data, err := sr.ReadFile(inv.Layout.Envvars)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			inv.unreadable(inv.Layout.Envvars, err)
		}
		return
	}
	if m := runUserRE.FindAllSubmatch(data, -1); m != nil {
		inv.RunUser = string(m[len(m)-1][2])
	}
}

func (inv *Inventory) scanSnippets(sr platform.Sysroot) {
	dir := inv.Layout.ConfEnabledDir
	if dir == "" || !sr.Exists(dir) {
		return
	}
	ents, err := sr.ReadDir(dir)
	if err != nil {
		inv.unreadable(dir, err)
		return
	}
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".conf") {
			inv.Snippets = append(inv.Snippets, Snippet{Path: path.Join(dir, e.Name())})
		}
	}
}

// IndexGaps lists the NNN- numbers missing between 1 and the highest one
// (information only: gash never numbers files), as ranges "001-019".
func (inv *Inventory) IndexGaps() []string {
	if len(inv.Indexes) == 0 {
		return nil
	}
	have := map[int]bool{}
	for _, n := range inv.Indexes {
		have[n] = true
	}
	var out []string
	high := inv.Indexes[len(inv.Indexes)-1]
	for n := 1; n <= high; n++ {
		if have[n] {
			continue
		}
		start := n
		for n+1 <= high && !have[n+1] {
			n++
		}
		if start == n {
			out = append(out, fmt.Sprintf("%03d", start))
		} else {
			out = append(out, fmt.Sprintf("%03d-%03d", start, n))
		}
	}
	return out
}

// PoolByPath returns the parsed pool at p.
func (inv *Inventory) PoolByPath(p string) (Pool, bool) {
	pool, ok := inv.Pools[p]
	return pool, ok
}
