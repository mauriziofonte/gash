// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package receipt is the install receipt of design/hte.md section 8: every
// system change install and migrate --apply made, with what was there
// before, so that uninstall restores it. Root keeps its own copy
// (/var/lib/gash-hte/receipt-<user>.json, written only by the root step);
// the user copy mirrors it, and a difference is receipt_mismatch, so a
// process running as the user cannot steer what root restores.
package receipt

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/renameio/v2"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
)

// Schema is the receipt format version.
const Schema = 1

// Root-owned locations (design/hte.md 8).
const (
	RootDir    = "/var/lib/gash-hte"
	BackupDir  = RootDir + "/backup"
	ArchiveDir = RootDir + "/archive"
)

// RootPath is the root copy of a user's receipt.
func RootPath(user string) string { return path.Join(RootDir, "receipt-"+user+".json") }

// Kind is the kind of one change.
type Kind string

// The change kinds. The root step makes and undoes all of them except
// autostart, which the user side makes and undoes (root only records it).
const (
	FileCreated  Kind = "file_created"  // path, sha256, mode
	FileModified Kind = "file_modified" // path, sha256, backup (the prior content), prior_sha256, mode
	DirCreated   Kind = "dir_created"   // path
	Sysctl       Kind = "sysctl"        // key, prior, applied, file
	Linger       Kind = "linger"        // prior, applied
	Unit         Kind = "unit"          // name, prior, applied
	HostsBlock   Kind = "hosts_block"   // prior: the block before, null when there was none
	LegacyVhost  Kind = "legacy_vhost"  // path, prior, archive, link
	LegacyPool   Kind = "legacy_pool"   // path, archive
	Autostart    Kind = "autostart"     // mode, path (a user file)
)

var kinds = []Kind{FileCreated, FileModified, DirCreated, Sysctl, Linger, Unit, HostsBlock, LegacyVhost, LegacyPool, Autostart}

// UnitState is the state of a system unit.
type UnitState struct {
	Enabled bool `json:"enabled"`
	Active  bool `json:"active"`
}

// VhostState is whether a legacy vhost was enabled.
type VhostState struct {
	Enabled bool `json:"enabled"`
}

// FilePrior is the sysctl drop-in: Prior is the backup of its content
// before install, nil when the file did not exist.
type FilePrior struct {
	Path  string  `json:"path"`
	Prior *string `json:"prior"`
}

// Link is a Debian sites-enabled link and its target, recreated on
// restore exactly as it was.
type Link struct {
	Path   string `json:"path"`
	Target string `json:"target"`
}

// Change is one system change. Prior and Applied hold the kind's states:
// a string for sysctl, a bool for linger, a UnitState for unit, a
// VhostState for legacy_vhost, the block text or null for hosts_block.
type Change struct {
	ID          int             `json:"id"`
	Kind        Kind            `json:"kind"`
	Path        string          `json:"path,omitempty"`
	SHA256      string          `json:"sha256,omitempty"`
	Mode        string          `json:"mode,omitempty"` // octal, of a written file
	Backup      string          `json:"backup,omitempty"`
	PriorSHA256 string          `json:"prior_sha256,omitempty"`
	Key         string          `json:"key,omitempty"`
	Name        string          `json:"name,omitempty"`
	Prior       json.RawMessage `json:"prior,omitzero"`
	Applied     json.RawMessage `json:"applied,omitzero"`
	File        *FilePrior      `json:"file,omitempty"`
	Archive     string          `json:"archive,omitempty"`
	Link        *Link           `json:"link,omitempty"`
	AutoMode    string          `json:"autostart,omitempty"`
	// Pending marks a change recorded before it is made: an interrupted
	// root step leaves it, and restoring it tolerates a change that never
	// happened.
	Pending bool `json:"pending,omitzero"`
}

// Receipt is the whole receipt.
type Receipt struct {
	Schema   int       `json:"schema"`
	User     string    `json:"user"`
	UID      int       `json:"uid"`
	Platform string    `json:"platform"`
	Created  time.Time `json:"created"`
	Changes  []Change  `json:"changes"`
}

// New is an empty receipt.
func New(user string, uid int, platform string, now time.Time) *Receipt {
	return &Receipt{Schema: Schema, User: user, UID: uid, Platform: platform, Created: now.UTC().Truncate(time.Second), Changes: []Change{}}
}

// Raw encodes v for Prior and Applied.
func Raw(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err) // only the state types above are encoded
	}
	return data
}

// Decode reads a Prior or Applied value into v.
func Decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return errors.New("receipt: the state is missing")
	}
	return json.Unmarshal(raw, v)
}

// Add appends c with the next id and returns it.
func (r *Receipt) Add(c Change) Change {
	c.ID = 1
	if n := len(r.Changes); n > 0 {
		c.ID = r.Changes[n-1].ID + 1
	}
	r.Changes = append(r.Changes, c)
	return c
}

// Remove drops the change with id (after it was restored).
func (r *Receipt) Remove(id int) {
	r.Changes = slices.DeleteFunc(r.Changes, func(c Change) bool { return c.ID == id })
}

// Last returns the change with the highest id.
func (r *Receipt) Last() (Change, bool) {
	if len(r.Changes) == 0 {
		return Change{}, false
	}
	return r.Changes[len(r.Changes)-1], true
}

// Find returns the last change of kind for path (or name, for units).
func (r *Receipt) Find(kind Kind, key string) (Change, bool) {
	for i := len(r.Changes) - 1; i >= 0; i-- {
		c := r.Changes[i]
		if c.Kind == kind && (c.Path == key || c.Name == key || key == "") {
			return c, true
		}
	}
	return Change{}, false
}

var (
	sha256RE = regexp.MustCompile(`^[0-9a-f]{64}$`)
	modeRE   = regexp.MustCompile(`^0[0-7]{3}$`)
	// UserRE is the user names the root step accepts: they go into the
	// sudoers drop-in and file names.
	UserRE = regexp.MustCompile(`^[a-z_][a-z0-9_.-]{0,31}$`)
)

func absClean(p string) bool {
	return strings.HasPrefix(p, "/") && path.Clean(p) == p && !strings.ContainsAny(p, "\x00\n")
}

// Validate checks the receipt's structure: every field a kind needs, every
// path absolute and clean, backups and archives inside RootDir.
func (r *Receipt) Validate() error {
	bad := func(format string, args ...any) error {
		return hteerr.Newf(hteerr.InvalidState, "", "receipt: "+format, args...)
	}
	if r.Schema != Schema {
		return bad("schema %d, not %d", r.Schema, Schema)
	}
	if !UserRE.MatchString(r.User) || r.UID <= 0 {
		return bad("user %q uid %d", r.User, r.UID)
	}
	last := 0
	for _, c := range r.Changes {
		if c.ID <= last {
			return bad("change ids are not ascending at %d", c.ID)
		}
		last = c.ID
		if !slices.Contains(kinds, c.Kind) {
			return bad("change %d has the unknown kind %q", c.ID, c.Kind)
		}
		if c.Path != "" && !absClean(c.Path) {
			return bad("change %d: path %q", c.ID, c.Path)
		}
		inRoot := func(p string) bool { return absClean(p) && strings.HasPrefix(p, RootDir+"/") }
		switch c.Kind {
		case FileCreated, FileModified:
			if c.Path == "" || !sha256RE.MatchString(c.SHA256) || !modeRE.MatchString(c.Mode) {
				return bad("change %d (%s) lacks its path, sha256 or mode", c.ID, c.Kind)
			}
			if c.Kind == FileModified && (!inRoot(c.Backup) || !sha256RE.MatchString(c.PriorSHA256)) {
				return bad("change %d: backup %q", c.ID, c.Backup)
			}
		case DirCreated:
			if c.Path == "" {
				return bad("change %d lacks its path", c.ID)
			}
		case Sysctl:
			var prior, applied string
			if c.Key == "" || Decode(c.Prior, &prior) != nil || Decode(c.Applied, &applied) != nil {
				return bad("change %d (sysctl) lacks its key or values", c.ID)
			}
			if c.File != nil && (!absClean(c.File.Path) || (c.File.Prior != nil && !inRoot(*c.File.Prior))) {
				return bad("change %d: sysctl file %+v", c.ID, *c.File)
			}
		case Linger:
			var prior, applied bool
			if Decode(c.Prior, &prior) != nil || Decode(c.Applied, &applied) != nil {
				return bad("change %d (linger) lacks its values", c.ID)
			}
		case Unit:
			var prior, applied UnitState
			if c.Name == "" || Decode(c.Prior, &prior) != nil || Decode(c.Applied, &applied) != nil {
				return bad("change %d (unit) lacks its name or states", c.ID)
			}
		case HostsBlock:
			var prior *string
			if Decode(c.Prior, &prior) != nil {
				return bad("change %d (hosts_block) lacks its prior block", c.ID)
			}
		case LegacyVhost, LegacyPool:
			if c.Path == "" || !inRoot(c.Archive) {
				return bad("change %d (%s) lacks its path or archive", c.ID, c.Kind)
			}
			if c.Link != nil && (!absClean(c.Link.Path) || c.Link.Target == "") {
				return bad("change %d: link %+v", c.ID, *c.Link)
			}
		case Autostart:
			if c.AutoMode == "" {
				return bad("change %d (autostart) lacks its mode", c.ID)
			}
		}
	}
	return nil
}

// Marshal is the canonical encoding of both copies: compact JSON and a
// newline (indenting would re-indent the embedded states).
func Marshal(r *Receipt) ([]byte, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Parse decodes and validates a receipt, strictly.
func Parse(data []byte, name string) (*Receipt, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var r Receipt
	if err := dec.Decode(&r); err != nil {
		return nil, hteerr.Newf(hteerr.InvalidState, "", "%s: %v", name, err)
	}
	if dec.More() {
		return nil, hteerr.Newf(hteerr.InvalidState, "", "%s: data after the receipt", name)
	}
	if r.Changes == nil {
		r.Changes = []Change{}
	}
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return &r, nil
}

// Load reads a receipt file; ok is false when it does not exist.
func Load(file string) (r *Receipt, ok bool, err error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	r, err = Parse(data, file)
	return r, err == nil, err
}

// Save writes r atomically with mode.
func Save(file string, r *Receipt, mode os.FileMode) error {
	data, err := Marshal(r)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	return renameio.WriteFile(file, data, mode)
}

// Compare checks the user copy against the root copy: they must be byte
// for byte the canonical encoding of the same receipt.
func Compare(userFile string, userData []byte, rootFile string, rootData []byte) error {
	if bytes.Equal(userData, rootData) {
		return nil
	}
	return hteerr.Newf(hteerr.ReceiptMismatch,
		"a human checks both files; uninstall acts on the root copy and needs them equal",
		"%s differs from the root copy %s", userFile, rootFile)
}
