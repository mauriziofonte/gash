// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package state

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/google/renameio/v2"

	"github.com/mauriziofonte/gash/internal/hostshelper"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
)

// File modes of the userland tree: private to the user.
const (
	dirMode  = 0o700
	fileMode = 0o600
)

// siteKeys must all be present in a site file (php_ini is optional).
var siteKeys = []string{"domain", "aliases", "docroot", "php", "profile", "https", "force_https",
	"force_https_bypass_loopback", "http2", "extra_http_ports", "extra_https_ports", "enabled", "kind", "raw_conf"}

// settingsKeys must all be present in hte.toml.
var settingsKeys = []string{"ports", "ports.http", "ports.https", "listen", "default_php", "modules", "autostart"}

// strictDecode decodes TOML into v and refuses unknown or missing keys:
// nothing absent from a file is replaced by a default.
func strictDecode(data []byte, file string, v any, required []string) error {
	md, err := toml.NewDecoder(bytes.NewReader(data)).Decode(v)
	if err != nil {
		return hteerr.Newf(hteerr.InvalidState, "", "%s: %v", file, err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		return hteerr.Newf(hteerr.InvalidState, "", "%s: unknown key %s", file, und[0])
	}
	for _, k := range required {
		if !md.IsDefined(strings.Split(k, ".")...) {
			return hteerr.Newf(hteerr.InvalidState, "", "%s: key %s is missing", file, k)
		}
	}
	return nil
}

func encode(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := toml.NewEncoder(&b)
	enc.Indent = ""
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// DecodeSite parses and validates a site file. file names it in errors.
// Every problem is invalid_state: a stored file is fixed by a human.
func DecodeSite(data []byte, file string) (Site, error) {
	var s Site
	if err := strictDecode(data, file, &s, siteKeys); err != nil {
		return Site{}, err
	}
	if err := s.Validate(); err != nil {
		return Site{}, asInvalidState(file, err)
	}
	return s, nil
}

func asInvalidState(file string, err error) error {
	if e, ok := hteerr.As(err); ok {
		if e.Code == hteerr.InvalidState {
			return hteerr.Newf(hteerr.InvalidState, e.Hint, "%s: %s", file, e.Details)
		}
		return hteerr.Newf(hteerr.InvalidState, e.Hint, "%s: %s: %s", file, e.Code, e.Details)
	}
	return hteerr.Newf(hteerr.InvalidState, "", "%s: %v", file, err)
}

// EncodeSite validates s and renders its TOML.
func EncodeSite(s Site) ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if s.Aliases == nil {
		s.Aliases = []string{}
	}
	if s.ExtraHTTPPorts == nil {
		s.ExtraHTTPPorts = []int{}
	}
	if s.ExtraHTTPSPorts == nil {
		s.ExtraHTTPSPorts = []int{}
	}
	return encode(s)
}

// DecodeSettings parses and validates hte.toml.
func DecodeSettings(data []byte, file string) (Settings, error) {
	var s Settings
	if err := strictDecode(data, file, &s, settingsKeys); err != nil {
		return Settings{}, err
	}
	if err := s.Validate(); err != nil {
		return Settings{}, asInvalidState(file, err)
	}
	return s, nil
}

// EncodeSettings validates s and renders its TOML.
func EncodeSettings(s Settings) ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if s.Modules == nil {
		s.Modules = []string{}
	}
	return encode(s)
}

// Store reads and writes the state files of one user. Reads take no lock;
// callers hold the Lock around a mutation.
type Store struct {
	Paths Paths
}

// writeFile writes data atomically (temporary file, fsync, rename).
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return err
	}
	return renameio.WriteFile(path, data, fileMode)
}

// ErrNotInstalled is wrapped by LoadSettings when hte.toml does not exist.
var ErrNotInstalled = errors.New("hte.toml does not exist")

// LoadSettings reads hte.toml. A missing file is hte_not_installed.
func (st Store) LoadSettings() (Settings, error) {
	path := st.Paths.Settings()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Settings{}, fmt.Errorf("%w: %w", ErrNotInstalled,
			hteerr.New(hteerr.HTENotInstalled, path+" does not exist", "run gash hte install"))
	}
	if err != nil {
		return Settings{}, err
	}
	return DecodeSettings(data, path)
}

// SaveSettings writes hte.toml atomically.
func (st Store) SaveSettings(s Settings) error {
	data, err := EncodeSettings(s)
	if err != nil {
		return err
	}
	return writeFile(st.Paths.Settings(), data)
}

// LoadSite reads the site named by a --domain argument. An unknown site is
// site_not_found; a file whose domain key differs from its name is
// invalid_state (LEDGER HTE-05).
func (st Store) LoadSite(arg string) (Site, error) {
	domain, err := NormalizeDomain(arg)
	if err != nil {
		return Site{}, err
	}
	path := st.Paths.SiteFile(domain)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Site{}, hteerr.Newf(hteerr.SiteNotFound, "gash hte site list", "no site %s", domain)
	}
	if err != nil {
		return Site{}, err
	}
	s, err := DecodeSite(data, path)
	if err != nil {
		return Site{}, err
	}
	if s.Domain != domain {
		return Site{}, hteerr.Newf(hteerr.InvalidState, "", "%s: domain is %q, the file name says %q", path, s.Domain, domain)
	}
	return s, nil
}

// LoadSites reads every site file, sorted by domain. Files that do not
// load are returned as problems (doctor reports them) and left out.
func (st Store) LoadSites() (sites []Site, problems []error, err error) {
	ents, err := os.ReadDir(st.Paths.SitesDir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	for _, e := range ents {
		name, ok := strings.CutSuffix(e.Name(), ".toml")
		if !ok || e.IsDir() {
			continue
		}
		path := filepath.Join(st.Paths.SitesDir(), e.Name())
		if !hostshelper.ValidHostname(name) {
			problems = append(problems, hteerr.Newf(hteerr.InvalidState, "", "%s: the file name is not a site name", path))
			continue
		}
		s, err := st.LoadSite(name)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		sites = append(sites, s)
	}
	slices.SortFunc(sites, func(a, b Site) int { return strings.Compare(a.Domain, b.Domain) })
	return sites, problems, nil
}

// SaveSite validates s and writes sites/<domain>.toml atomically.
func (st Store) SaveSite(s Site) error {
	data, err := EncodeSite(s)
	if err != nil {
		return err
	}
	return writeFile(st.Paths.SiteFile(s.Domain), data)
}

// RemoveSite deletes the state file of a site; its overlay is kept (the
// renderer never writes overlays, and remove never deletes user files).
func (st Store) RemoveSite(domain string) error {
	err := os.Remove(st.Paths.SiteFile(domain))
	if errors.Is(err, fs.ErrNotExist) {
		return hteerr.Newf(hteerr.SiteNotFound, "gash hte site list", "no site %s", domain)
	}
	return err
}
