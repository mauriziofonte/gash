// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ui

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/mauriziofonte/gash/internal/term"
)

// HumanSize is __gash_human_size BYTES: IEC units with one decimal digit,
// truncated (0 B, 1.5 KiB, 142.3 MiB); text that is not a run of digits
// is 0 B.
func HumanSize(b string) string {
	if !isDigits(b) {
		b = "0"
	}
	n, err := strconv.ParseUint(b, 10, 63)
	if err != nil {
		n = uint64(Int(b))
	}
	return HumanBytes(int64(n))
}

// HumanBytes is HumanSize of a number.
func HumanBytes(size int64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}
	if size < 0 {
		size = 0
	}
	rem, i := int64(0), 0
	for size >= 1024 && i < len(units)-1 {
		rem = (size * 10 / 1024) % 10
		size /= 1024
		i++
	}
	if i == 0 {
		return strconv.FormatInt(size, 10) + " " + units[0]
	}
	return strconv.FormatInt(size, 10) + "." + strconv.FormatInt(rem, 10) + " " + units[i]
}

// Download test seams.
var (
	downloadNow  = time.Now
	downloadTick = 250 * time.Millisecond
)

// Downloader runs __gash_download URL DEST over net/http (2.x ran curl
// -fsSL; architecture.md 7 replaces curl): the "Get:" note with the size
// a HEAD request reports, a progress line on a terminal stderr, then the
// "Fetched" note, or "Download failed (URL): ERROR" with curl's exit code
// and message for the failure, the partial file removed. A cancelled
// context (Ctrl-C) removes the partial file, prints "Aborted." and
// returns 130 (LEDGER CORE-10).
type Downloader struct {
	// Client is the HTTP client; nil means one with curl -L's redirect
	// limit and the proxies and CA of the environment.
	Client *http.Client
}

func (d Downloader) client() *http.Client {
	if d.Client != nil {
		return d.Client
	}
	return &http.Client{
		// The proxy variables of the process, which is the caller's
		// exported environment.
		Transport: proxyTransport(http.ProxyFromEnvironment),
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) > 50 {
				return errTooManyRedirects
			}
			return nil
		},
	}
}

// proxyTransport is a transport through the proxies of proxy that reports
// a refused tunnel as curl does (tunnelRefused).
func proxyTransport(proxy func(*http.Request) (*url.URL, error)) *http.Transport {
	return &http.Transport{
		Proxy: proxy,
		OnProxyConnectResponse: func(_ context.Context, _ *url.URL, _ *http.Request, resp *http.Response) error {
			if resp.StatusCode != http.StatusOK {
				return tunnelRefused{resp.StatusCode}
			}
			return nil
		},
	}
}

// tunnelRefused is a proxy that answered the CONNECT of an HTTPS request
// with another status than 200 (an unreachable host, behind a proxy).
type tunnelRefused struct{ status int }

func (e tunnelRefused) Error() string {
	return "CONNECT tunnel failed, response " + strconv.Itoa(e.status)
}

var errTooManyRedirects = errors.New("too many redirects")

// RemoteSize is __gash_remote_size URL: the Content-Length a HEAD request
// reports after redirects, whatever its status, 0 when the server gives no
// length or the request fails. 2.x ran `curl -fsIL URL`, whose -I prints
// the response headers even on a 4xx/5xx (-f suppresses only the body), so
// the awk over them returns the length of an error response too; a caller
// that proceeds to download then fails at the download itself, as 2.x does
// (not a "no download" shortcut). Only a response with no Content-Length
// (an unknown or chunked length, resp.ContentLength < 0) is 0.
func (d Downloader) RemoteSize(ctx context.Context, rawURL string) int64 {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return 0
	}
	resp, err := d.client().Do(req)
	if err != nil {
		return 0
	}
	_ = resp.Body.Close()
	if resp.ContentLength < 0 {
		return 0
	}
	return resp.ContentLength
}

// Run downloads rawURL to dest, writing every message to stderr with the
// style s on the terminal t, and returns the 2.x status.
func (d Downloader) Run(ctx context.Context, s Style, t *term.Term, stderr io.Writer, rawURL, dest string) int {
	p := s.Printer(stderr)
	if rawURL == "" || dest == "" {
		p.Error("Usage: __gash_download URL DEST")
		return 1
	}
	total := d.RemoteSize(ctx, rawURL)
	if total > 0 {
		p.Note("Get: " + rawURL + " [" + HumanBytes(total) + "]")
	} else {
		p.Note("Get: " + rawURL)
	}

	start := downloadNow()
	var got atomic.Int64
	done := make(chan downloadResult, 1)
	go func() { done <- d.fetch(ctx, rawURL, dest, &got) }()

	var res downloadResult
	if t.StderrTTY() && !t.Headless() {
		el := ""
		if t.StderrFancy() {
			el = term.EraseEOL
		}
		var prev, speed int64
		tick := 0
		ticker := time.NewTicker(downloadTick)
		finished := false
		for !finished {
			b := got.Load()
			tick++
			if tick%4 == 0 {
				speed, prev = b-prev, b
			}
			_, _ = io.WriteString(stderr, progressLine(s, b, total, speed, el))
			select {
			case res = <-done:
				finished = true
			case <-ticker.C:
			}
		}
		ticker.Stop()
		if el != "" {
			_, _ = io.WriteString(stderr, "\r"+el)
		} else {
			_, _ = io.WriteString(stderr, "\r"+Spaces(72)+"\r")
		}
	} else {
		res = <-done
	}

	if res.code != 0 {
		_ = os.Remove(dest)
		if res.cancelled {
			_, _ = io.WriteString(stderr, "\nAborted.\n")
			return 130
		}
		p.Error("Download failed (" + rawURL + "): " + res.msg)
		return res.code
	}
	elapsed := int64(downloadNow().Sub(start) / time.Second)
	if elapsed <= 0 {
		elapsed = 1
	}
	size := int64(0)
	if fi, err := os.Stat(dest); err == nil {
		size = fi.Size()
	}
	p.Note("Fetched " + HumanBytes(size) + " in " + strconv.FormatInt(elapsed, 10) + "s (" + HumanBytes(size/elapsed) + "/s)")
	return 0
}

// progressLine is one redraw of the 2.x progress loop.
func progressLine(s Style, b, total, speed int64, el string) string {
	if total > 0 {
		pct := b * 100 / total
		if pct > 100 {
			pct = 100
		}
		filled := pct * 30 / 100
		var bar strings.Builder
		for i := int64(0); i < 30; i++ {
			if i < filled {
				bar.WriteString(s.G.BarFull)
			} else {
				bar.WriteString(s.G.BarEmpty)
			}
		}
		eta := "--"
		if speed > 0 {
			eta = strconv.FormatInt((total-b)/speed, 10) + "s"
		}
		return fmt.Sprintf("\r%3d%% %s %10s / %-10s %10s/s %6s%s", pct, bar.String(),
			HumanBytes(b), HumanBytes(total), HumanBytes(speed), eta, el)
	}
	return fmt.Sprintf("\r%10s %10s/s%s", HumanBytes(b), HumanBytes(speed), el)
}

type downloadResult struct {
	code      int
	msg       string
	cancelled bool
}

// countWriter counts the bytes written through it.
type countWriter struct {
	w io.Writer
	n *atomic.Int64
}

func (c countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n.Add(int64(n))
	return n, err
}

// fetch is curl -fsSL -o DEST URL: curl's exit code and the error line it
// printed ("curl: (N) MESSAGE").
func (d Downloader) fetch(ctx context.Context, rawURL, dest string, got *atomic.Int64) downloadResult {
	fail := func(code int, msg string) downloadResult {
		if ctx.Err() != nil {
			return downloadResult{code: 130, cancelled: true}
		}
		return downloadResult{code: code, msg: "curl: (" + strconv.Itoa(code) + ") " + msg}
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return fail(3, "URL rejected: Malformed input to a URL function")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fail(1, `Protocol "`+u.Scheme+`" not supported`)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fail(3, "URL rejected: Malformed input to a URL function")
	}
	start := time.Now()
	resp, err := d.client().Do(req)
	if err != nil {
		return fail(curlError(err, u, time.Since(start)))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return fail(22, "The requested URL returned error: "+strconv.Itoa(resp.StatusCode))
	}
	f, err := os.Create(dest)
	if err != nil {
		return fail(23, "Failure writing output to destination, passed 0 returned 4294967295")
	}
	_, cerr := io.Copy(countWriter{f, got}, resp.Body)
	if err := f.Close(); err != nil && cerr == nil {
		return fail(23, "Failure writing output to destination")
	}
	if cerr != nil {
		return fail(56, "Failure when receiving data from the peer")
	}
	return downloadResult{}
}

// curlError maps a transport error to curl's exit code and message.
func curlError(err error, u *url.URL, took time.Duration) (int, string) {
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[u.Scheme]
	}
	var dnsErr *net.DNSError
	var opErr *net.OpError
	var certErr *x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	var refused tunnelRefused
	switch {
	case errors.Is(err, errTooManyRedirects):
		return 47, "Maximum (50) redirects followed"
	case errors.As(err, &refused):
		return 56, refused.Error()
	case errors.Is(err, io.EOF):
		// The server closed the connection without an answer.
		return 52, "Empty reply from server"
	case errors.As(err, &dnsErr):
		return 6, "Could not resolve host: " + host
	case errors.As(err, &certErr):
		return 60, "SSL certificate problem: unable to get local issuer certificate"
	case errors.As(err, &hostErr):
		return 60, "SSL: no alternative certificate subject name matches target host name '" + host + "'"
	case errors.Is(err, context.DeadlineExceeded):
		return 28, "Operation timed out"
	case errors.As(err, &opErr) && opErr.Op == "dial":
		return 7, "Failed to connect to " + host + " port " + port + " after " + strconv.FormatInt(took.Milliseconds(), 10) + " ms: Couldn't connect to server"
	}
	return 56, "Failure when receiving data from the peer"
}
