// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package service

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/hte/render"
	"github.com/mauriziofonte/gash/internal/hte/state"
)

// Addr is one address and whether it accepts connections.
type Addr struct {
	Addr    string `json:"addr"`
	Accepts bool   `json:"accepts"`
}

// MasterStatus is one master.
type MasterStatus struct {
	Name    string     `json:"name"`
	Running bool       `json:"running"`
	PID     int        `json:"pid,omitempty"`
	Started *time.Time `json:"started,omitempty"`
	Uptime  int64      `json:"uptime_seconds,omitempty"`
	Addrs   []Addr     `json:"addrs"` // Listen addresses, or FPM sockets
}

// Probe is one HTTP request to a site.
type Probe struct {
	URL    string `json:"url"`
	Status int    `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

// SiteStatus is whether a site answers.
type SiteStatus struct {
	Domain string  `json:"domain"`
	HTTP   []Probe `json:"http"`
	HTTPS  []Probe `json:"https"`
}

// Status is service status.
type Status struct {
	Generation int            `json:"generation"`
	Running    bool           `json:"running"` // every master runs
	Apache     MasterStatus   `json:"apache"`
	FPM        []MasterStatus `json:"fpm"`
	Sites      []SiteStatus   `json:"sites"`
}

func (s *Supervisor) masterStatus(ctx context.Context, m master, now time.Time) MasterStatus {
	st := MasterStatus{Name: m.name, Addrs: []Addr{}}
	if pid, ok := s.pid(m); ok {
		st.Running, st.PID = true, pid
		if t, err := s.Procs.Started(pid); err == nil {
			st.Started = &t
			st.Uptime = int64(now.Sub(t).Seconds())
		}
	}
	for _, a := range m.addrs {
		st.Addrs = append(st.Addrs, Addr{Addr: a, Accepts: s.dial(ctx, m.network, a) == nil})
	}
	return st
}

// Status reports the masters of the current generation and, with probe,
// whether each site answers on HTTP and HTTPS.
func (s *Supervisor) Status(ctx context.Context, probe bool) (Status, error) {
	n, m, ok, err := s.Current()
	if err != nil {
		return Status{}, err
	}
	st := Status{FPM: []MasterStatus{}, Sites: []SiteStatus{}}
	if !ok {
		st.Apache = MasterStatus{Name: "apache", Addrs: []Addr{}}
		return st, nil
	}
	now := time.Now()
	st.Generation = n
	st.Apache = s.masterStatus(ctx, s.apache(m.Apache), now)
	st.Running = st.Apache.Running
	for _, f := range m.FPM {
		ms := s.masterStatus(ctx, s.fpm(f), now)
		st.Running = st.Running && ms.Running
		st.FPM = append(st.FPM, ms)
	}
	if probe {
		for _, e := range m.Sites {
			st.Sites = append(st.Sites, ProbeSite(ctx, e))
		}
	}
	return st, nil
}

// ProbeSite requests / of a site on each of its addresses, with its Host
// header and, for HTTPS, its name as SNI. Redirects are not followed: an
// answer is any HTTP status. The certificate is not verified (it is the
// site's self-signed one; doctor checks it separately).
func ProbeSite(ctx context.Context, e render.Endpoint) SiteStatus {
	st := SiteStatus{Domain: e.Domain, HTTP: []Probe{}, HTTPS: []Probe{}}
	for _, a := range e.HTTP {
		st.HTTP = append(st.HTTP, probe(ctx, "http", e.Domain, a))
	}
	for _, a := range e.HTTPS {
		st.HTTPS = append(st.HTTPS, probe(ctx, "https", e.Domain, a))
	}
	return st
}

func probe(ctx context.Context, scheme, host, addr string) Probe {
	p := Probe{URL: scheme + "://" + addr + "/"}
	tr := &http.Transport{
		Proxy:             nil,
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", addr)
		},
		TLSClientConfig: &tls.Config{ServerName: host, InsecureSkipVerify: true}, // #nosec: our own loopback site
	}
	c := &http.Client{Transport: tr, Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scheme+"://"+host+"/", nil)
	if err != nil {
		p.Error = err.Error()
		return p
	}
	resp, err := c.Do(req)
	if err != nil {
		p.Error = err.Error()
		return p
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	resp.Body.Close()
	p.Status = resp.StatusCode
	return p
}

// LogFile is one log and its last lines.
type LogFile struct {
	Path  string   `json:"path"`
	Lines []string `json:"lines"`
}

// Logs returns the last n lines of the global Apache log and of every FPM
// log, or, for a site, of its access, error and PHP error logs. There is
// no follow mode, so a caller never hangs.
func Logs(p state.Paths, m *render.Manifest, site string, n int) ([]LogFile, error) {
	var paths []string
	if site == "" {
		paths = append(paths, filepath.Join(p.LogsDir(), "error.log"))
		if m != nil {
			for _, f := range m.FPM {
				paths = append(paths, f.ErrorLog)
			}
		}
	} else {
		dir := p.SiteLogDir(site)
		paths = append(paths, filepath.Join(dir, "access.log"), filepath.Join(dir, "error.log"), filepath.Join(dir, "php-error.log"))
	}
	out := []LogFile{}
	for _, path := range paths {
		lines, err := Tail(path, n)
		if err != nil {
			return nil, err
		}
		out = append(out, LogFile{Path: path, Lines: lines})
	}
	return out, nil
}

// Tail returns the last n lines of path; a missing file has none.
func Tail(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	const window = 1 << 20
	if fi, err := f.Stat(); err == nil && fi.Size() > window {
		if _, err := f.Seek(fi.Size()-window, io.SeekStart); err != nil {
			return nil, err
		}
	}
	ring := make([]string, 0, n)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		if n <= 0 {
			continue
		}
		if len(ring) == n {
			ring = ring[1:]
		}
		ring = append(ring, strings.TrimRight(sc.Text(), "\r"))
	}
	return ring, sc.Err()
}
