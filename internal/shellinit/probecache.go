// Gash 3.1. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package shellinit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// The probe cache. The probes prefetch runs (dircolors, `grep --color`,
// `ls --color`, `rm --version`) cost a fork and an exec each, most of
// `gash init bash`. Their answers (each program's exit and output, not
// what the layer makes of them) are kept, as 2.x kept its __GASH_PROBE_*
// answers, in ${XDG_CACHE_HOME:-~/.cache}/gash/init-probes, and reused
// while the key (the OS, TERM, COLORTERM, the locale, the effective PATH
// and the dircolors database) is the same, no PATH directory and no
// dircolors database changed after the file was written, and the file is
// less than a day old. A probe the file does not answer runs, and the file
// is written again.

// probeCacheVersion starts the key: a new layout of the file changes it.
const probeCacheVersion = "gash-init-probes 1"

// probeCacheMaxAge is how long the answers are reused, in seconds.
const probeCacheMaxAge = 86400

type probeCacheFile struct {
	Key     string                 `json:"key"`
	Created int64                  `json:"created"`
	Probes  map[string]cachedProbe `json:"probes"`
}

type cachedProbe struct {
	OK  bool   `json:"ok"`
	Out string `json:"out"`
}

// probeCachePath is the file, "" without an absolute cache directory.
func (st *state) probeCachePath() string {
	dir := st.c.Env.Get("XDG_CACHE_HOME")
	if !filepath.IsAbs(dir) {
		if !filepath.IsAbs(st.home) {
			return ""
		}
		dir = filepath.Join(st.home, ".cache")
	}
	return filepath.Join(dir, "gash", "init-probes")
}

// probeCacheKey is the key of this shell's answers and the paths that must
// not have changed after the file was written.
func (st *state) probeCacheKey() (string, []string) {
	var deps []string
	key := []string{probeCacheVersion, st.e.goos}
	for _, name := range []string{"TERM", "COLORTERM", "LANG", "LC_ALL", "LC_MESSAGES", "LC_CTYPE"} {
		key = append(key, name+"="+st.c.Env.Get(name))
	}
	key = append(key, "PATH="+st.effPath)
	for _, dir := range strings.Split(st.effPath, ":") {
		if filepath.IsAbs(dir) {
			deps = append(deps, dir)
		}
	}
	args := st.dircolorsArgs()
	key = append(key, "dircolors="+strings.Join(args, " "))
	if len(args) > 1 {
		deps = append(deps, args[1])
	}
	return strings.Join(key, "\x00"), deps
}

// readProbeCache returns the cached answers valid for this shell, by
// probeKey; nil when there are none.
func (st *state) readProbeCache() map[string]probeResult {
	path := st.probeCachePath()
	if path == "" {
		return nil
	}
	fi, err := st.e.stat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0o022 != 0 {
		return nil
	}
	if sys, ok := fi.Sys().(*syscall.Stat_t); ok && int(sys.Uid) != os.Geteuid() {
		return nil // someone else's answers are not this shell's to evaluate
	}
	key, deps := st.probeCacheKey()
	for _, d := range deps {
		dfi, err := st.e.stat(d)
		if err == nil && !fi.ModTime().After(dfi.ModTime()) {
			return nil
		}
	}
	b, err := st.e.readFile(path)
	if err != nil {
		return nil
	}
	var f probeCacheFile
	if json.Unmarshal(b, &f) != nil || f.Key != key {
		return nil
	}
	if now := st.e.now(); now < f.Created || now-f.Created >= probeCacheMaxAge {
		return nil
	}
	out := make(map[string]probeResult, len(f.Probes))
	for k, p := range f.Probes {
		out[k] = probeResult{ok: p.OK, out: p.Out}
	}
	return out
}

// writeProbeCache writes the answers of the probes prefetch started: aside,
// then renamed over the old file. Errors leave the cache as it was.
func (st *state) writeProbeCache() {
	path := st.probeCachePath()
	if path == "" {
		return
	}
	key, _ := st.probeCacheKey()
	f := probeCacheFile{Key: key, Created: st.e.now(), Probes: map[string]cachedProbe{}}
	for k, ch := range st.probes {
		r := <-ch
		ch <- r
		f.Probes[k] = cachedProbe{OK: r.ok, Out: r.out}
	}
	b, err := json.Marshal(f)
	if err != nil || st.e.mkdirAll(filepath.Dir(path), 0o700) != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".init-probes-*")
	if err != nil {
		return
	}
	_, werr := tmp.Write(b)
	if cerr := tmp.Close(); werr != nil || cerr != nil || os.Rename(tmp.Name(), path) != nil {
		_ = os.Remove(tmp.Name())
	}
}
