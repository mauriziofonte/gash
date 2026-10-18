// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package render

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/state"
)

// ManifestFile is the generation file that says how to run it.
const ManifestFile = "manifest.json"

// manifestSchema is bumped when the manifest format changes.
const manifestSchema = 1

// Manifest describes the masters and sites of one generation, so that
// supervision runs a generation exactly as it was rendered and tested
// (binaries, php.ini, PHP_INI_SCAN_DIR, PID files, sockets, addresses).
// Config paths are relative to the generation directory: masters are
// started on render/current/<config>, config tests run on gen-<n>/<config>.
type Manifest struct {
	Schema int          `json:"schema"`
	Apache ApacheMaster `json:"apache"`
	FPM    []FPMMaster  `json:"fpm"`
	Sites  []Endpoint   `json:"sites"`
}

// ApacheMaster is the Apache master of a generation.
type ApacheMaster struct {
	Binary   string   `json:"binary"`
	Config   string   `json:"config"`
	PidFile  string   `json:"pid_file"`
	ErrorLog string   `json:"error_log"`
	Listen   []string `json:"listen"` // host:port, every Listen line
}

// FPMMaster is the PHP-FPM master of one version.
type FPMMaster struct {
	Version  string   `json:"version"`
	Binary   string   `json:"binary"`
	Config   string   `json:"config"`
	INI      string   `json:"ini"`
	ScanDir  string   `json:"scan_dir"` // the PHP_INI_SCAN_DIR value
	PidFile  string   `json:"pid_file"`
	ErrorLog string   `json:"error_log"`
	Sockets  []string `json:"sockets"`
}

// Env is the environment entry the master needs on top of the base one.
func (m FPMMaster) Env() []string { return []string{"PHP_INI_SCAN_DIR=" + m.ScanDir} }

// Endpoint is where an enabled site answers.
type Endpoint struct {
	Domain string     `json:"domain"`
	Kind   state.Kind `json:"kind"`
	HTTP   []string   `json:"http"`  // host:port of the HTTP vhost
	HTTPS  []string   `json:"https"` // host:port of the HTTPS vhost
	Socket string     `json:"socket,omitempty"`
}

func (in Input) manifest(sites []state.Site, versions []string, listen []string) ([]byte, error) {
	m := Manifest{
		Schema: manifestSchema,
		Apache: ApacheMaster{
			Binary:   in.Apache.Binary,
			Config:   "httpd.conf",
			PidFile:  filepath.Join(in.Paths.RuntimeDir, "httpd.pid"),
			ErrorLog: filepath.Join(in.Paths.LogsDir(), "error.log"),
			Listen:   listen,
		},
		FPM:   []FPMMaster{},
		Sites: []Endpoint{},
	}
	sockets := map[string][]string{}
	for _, s := range sites {
		e := Endpoint{Domain: s.Domain, Kind: s.Kind, HTTP: in.siteAddrs(in.Ports.HTTP, s.ExtraHTTPPorts), HTTPS: []string{}}
		if s.HTTPS {
			e.HTTPS = in.siteAddrs(in.Ports.HTTPS, s.ExtraHTTPSPorts)
		}
		if s.HasPool() {
			e.Socket = SocketPath(in.Paths.RuntimeDir, s.PHP, s.Domain)
			sockets[s.PHP] = append(sockets[s.PHP], e.Socket)
		}
		m.Sites = append(m.Sites, e)
	}
	for _, v := range versions {
		facts := in.PHP[v]
		m.FPM = append(m.FPM, FPMMaster{
			Version:  v,
			Binary:   facts.FPM,
			Config:   fpmConfRel(v),
			INI:      facts.INI,
			ScanDir:  ScanDirEnv(in.Paths, facts, v),
			PidFile:  filepath.Join(in.Paths.RuntimeDir, "php-fpm-"+v+".pid"),
			ErrorLog: filepath.Join(in.Paths.LogsDir(), "php-fpm-"+v+".log"),
			Sockets:  sockets[v],
		})
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// ReadManifest reads the manifest of the generation directory dir.
func ReadManifest(dir string) (*Manifest, error) {
	path := filepath.Join(dir, ManifestFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, hteerr.Newf(hteerr.InvalidState, "run gash hte service reload to render again", "%s does not exist", path)
	}
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, hteerr.Newf(hteerr.InvalidState, "run gash hte service reload to render again", "%s: %v", path, err)
	}
	if m.Schema != manifestSchema {
		return nil, hteerr.Newf(hteerr.InvalidState, "run gash hte service reload to render again",
			"%s has schema %d, not %d", path, m.Schema, manifestSchema)
	}
	return &m, nil
}

// Master returns the FPM master of version v.
func (m *Manifest) Master(v string) (FPMMaster, bool) {
	i := slices.IndexFunc(m.FPM, func(f FPMMaster) bool { return f.Version == v })
	if i < 0 {
		return FPMMaster{}, false
	}
	return m.FPM[i], true
}

// Versions returns the PHP versions of the generation's masters.
func (m *Manifest) Versions() []string {
	out := make([]string, 0, len(m.FPM))
	for _, f := range m.FPM {
		out = append(out, f.Version)
	}
	return out
}
