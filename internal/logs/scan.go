// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package logs

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/pathguard"
)

// Entry is one log file the scan found.
type Entry struct {
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Category string    `json:"category"`
	Kind     string    `json:"kind"`    // "active", "rotated" or "journal"
	Reclaim  int64     `json:"reclaim"` // bytes clean would free (size; 0 for journal)
	// Protected marks a file in a PROTECT:content path of ~/.gash_env:
	// listed with its size, never deleted or truncated (reclaim 0).
	Protected bool `json:"protected,omitempty"`
	kindCode  kind
}

// Group is the entries of one category.
type Group struct {
	Category string  `json:"category"`
	Files    int     `json:"files"`
	Size     int64   `json:"size"`
	Reclaim  int64   `json:"reclaim"`
	Active   int     `json:"active"`
	Rotated  int     `json:"rotated"`
	Entries  []Entry `json:"entries,omitempty"`
}

// Report is the result of a scan.
type Report struct {
	Roots        []string `json:"roots"`
	Groups       []Group  `json:"groups"`
	TotalFiles   int      `json:"total_files"`
	TotalSize    int64    `json:"total_size"`
	TotalReclaim int64    `json:"total_reclaim"`
	Journal      int64    `json:"journal_bytes"`       // size held by systemd journals (vacuum, not delete)
	Unreadable   int      `json:"unreadable"`          // files a stat could not reach
	Protected    int      `json:"protected,omitempty"` // files in PROTECT:content paths, never cleaned
	entries      []Entry
}

// scanOptions tune a scan.
type scanOptions struct {
	minSize   int64         // ignore files smaller than this
	olderThan time.Duration // ignore files modified more recently than this
	maxDepth  int           // 0 = no limit
	now       time.Time     // the clock, for olderThan (tests set it)
	// hidden and sealed are the user's PROTECT:all and PROTECT:content
	// trees (resolved and as written): the walk leaves hidden ones out,
	// and lists the files of sealed ones as protected.
	hidden, sealed []string
}

// skipDirs are directories the walk never descends: huge, and their logs
// are not the ones a disk-reclaim run should touch.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	".cache/go-build": true, "System Volume Information": true, "$Recycle.Bin": true,
}

// scan walks the roots and returns the report. It never follows symlinks,
// never leaves a root, and reads only metadata (never a file's contents).
// An entry is judged on its physical path (the root resolved once, the
// rest as walked): one in a PROTECT:all tree is left out, a directory not
// descended; one in a PROTECT:content tree is listed as protected.
func scan(roots []string, o scanOptions) *Report {
	if o.now.IsZero() {
		o.now = time.Now()
	}
	r := &Report{Roots: roots}
	seen := map[string]bool{}
	for _, root := range roots {
		abs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		base := depth(abs)
		phys := abs
		if len(o.hidden)+len(o.sealed) > 0 {
			if r, err := filepath.EvalSymlinks(abs); err == nil {
				phys = r
			}
		}
		physOf := func(p string) string { return phys + strings.TrimPrefix(p, abs) }
		// The walk func never returns an error (it records unreadable
		// entries and goes on), so WalkDir's own return is always nil.
		_ = filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
			if len(o.hidden) > 0 && pathguard.UnderTrees(physOf(p), o.hidden) {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if err != nil {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				r.Unreadable++
				return nil
			}
			if d.IsDir() {
				if p != abs && skipDirs[d.Name()] {
					return fs.SkipDir
				}
				if o.maxDepth > 0 && depth(p)-base >= o.maxDepth {
					return fs.SkipDir
				}
				return nil
			}
			if d.Type()&fs.ModeSymlink != 0 || !d.Type().IsRegular() {
				return nil
			}
			if seen[p] {
				return nil
			}
			k, cat := classify(p)
			if k == notLog {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				r.Unreadable++
				return nil
			}
			if k != journal {
				if info.Size() < o.minSize {
					return nil
				}
				if o.olderThan > 0 && o.now.Sub(info.ModTime()) < o.olderThan {
					return nil
				}
			}
			seen[p] = true
			e := Entry{Path: p, Size: info.Size(), Modified: info.ModTime().UTC(),
				Category: cat, Kind: kindName(k), kindCode: k}
			switch {
			case len(o.sealed) > 0 && pathguard.UnderTrees(physOf(p), o.sealed):
				e.Protected = true
				r.Protected++
			case k != journal:
				e.Reclaim = info.Size()
			}
			r.entries = append(r.entries, e)
			return nil
		})
	}
	finish(r)
	return r
}

// finish groups the entries and totals them.
func finish(r *Report) {
	byCat := map[string]*Group{}
	for _, e := range r.entries {
		g := byCat[e.Category]
		if g == nil {
			g = &Group{Category: e.Category}
			byCat[e.Category] = g
		}
		g.Files++
		g.Size += e.Size
		g.Reclaim += e.Reclaim
		switch e.kindCode {
		case active:
			g.Active++
		case rotated:
			g.Rotated++
		}
		g.Entries = append(g.Entries, e)
		r.TotalFiles++
		r.TotalSize += e.Size
		r.TotalReclaim += e.Reclaim
		if e.kindCode == journal {
			r.Journal += e.Size
		}
	}
	for _, g := range byCat {
		sort.Slice(g.Entries, func(i, j int) bool { return g.Entries[i].Size > g.Entries[j].Size })
		r.Groups = append(r.Groups, *g)
	}
	sort.Slice(r.Groups, func(i, j int) bool {
		if r.Groups[i].Reclaim != r.Groups[j].Reclaim {
			return r.Groups[i].Reclaim > r.Groups[j].Reclaim
		}
		return r.Groups[i].Category < r.Groups[j].Category
	})
}

func kindName(k kind) string {
	switch k {
	case active:
		return "active"
	case rotated:
		return "rotated"
	case journal:
		return "journal"
	}
	return ""
}

func depth(p string) int {
	n := 0
	for _, c := range p {
		if c == os.PathSeparator {
			n++
		}
	}
	return n
}
