// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package discover

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/google/renameio/v2"
)

// cacheSchema is bumped when the entry format changes; another schema is
// read as an empty cache.
const cacheSchema = 1

// Cache keeps the first line of `php-fpm -v` per binary, valid while the
// binary keeps its size and mtime (design/hte.md section 4). It is a
// disposable file in the state directory, php-versions.json: a missing,
// unreadable or corrupt cache is an empty one and is rewritten, never an
// error. A nil *Cache caches nothing.
type Cache struct {
	Schema  int                   `json:"schema"`
	Entries map[string]CacheEntry `json:"entries"` // by system path
	dirty   bool
}

// CacheEntry is one probed binary.
type CacheEntry struct {
	Size    int64  `json:"size"`
	MtimeNS int64  `json:"mtime_ns"`
	Output  string `json:"output"`
}

// LoadCache reads the cache at path.
func LoadCache(path string) *Cache {
	c := &Cache{Schema: cacheSchema, Entries: map[string]CacheEntry{}}
	data, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	var on Cache
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if dec.Decode(&on) != nil || on.Schema != cacheSchema || on.Entries == nil {
		c.dirty = true
		return c
	}
	on.dirty = false
	return &on
}

// Save writes the cache atomically when it changed since LoadCache.
func (c *Cache) Save(path string) error {
	if c == nil || !c.dirty {
		return nil
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := renameio.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return err
	}
	c.dirty = false
	return nil
}

func (c *Cache) lookup(bin string, fi fs.FileInfo) (string, bool) {
	if c == nil {
		return "", false
	}
	e, ok := c.Entries[bin]
	if !ok || e.Size != fi.Size() || e.MtimeNS != fi.ModTime().UnixNano() {
		return "", false
	}
	return e.Output, true
}

func (c *Cache) store(bin string, fi fs.FileInfo, out string) {
	if c == nil {
		return
	}
	c.Entries[bin] = CacheEntry{Size: fi.Size(), MtimeNS: fi.ModTime().UnixNano(), Output: out}
	c.dirty = true
}
