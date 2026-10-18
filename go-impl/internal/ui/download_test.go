// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ui

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/term"
)

func TestHumanSizeMatches2x(t *testing.T) {
	in := []string{"0", "1", "1023", "1024", "1536", "1048575", "1048576", "150000000", "1099511627776", "x", "", "-5", "1152921504606846976"}
	var script strings.Builder
	for _, v := range in {
		script.WriteString("__gash_human_size " + shq(v) + "; printf '\\0'\n")
	}
	want := strings.Split(run2x(t, nil, script.String()), "\x00")
	for i, v := range in {
		require.Equal(t, want[i], HumanSize(v), v)
	}
}

func TestDownload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/f":
			w.Header().Set("Content-Length", "5")
			_, _ = w.Write([]byte("hello"))
		case "/redirect":
			http.Redirect(w, r, "/f", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	s := Style{G: term.ASCIIGlyphs}
	tt := term.Fixed(nil, false, false, false)
	d := Downloader{Client: srv.Client()}

	var errb bytes.Buffer
	rc := d.Run(t.Context(), s, tt, &errb, srv.URL+"/redirect", filepath.Join(dir, "f"))
	require.Equal(t, 0, rc)
	b, _ := os.ReadFile(filepath.Join(dir, "f"))
	require.Equal(t, "hello", string(b))
	require.Equal(t, "Get: "+srv.URL+"/redirect [5 B]\nFetched 5 B in 1s (5 B/s)\n", errb.String())

	errb.Reset()
	rc = d.Run(t.Context(), s, tt, &errb, srv.URL+"/missing", filepath.Join(dir, "g"))
	require.Equal(t, 22, rc)
	require.NoFileExists(t, filepath.Join(dir, "g"))
	require.Equal(t, "Get: "+srv.URL+"/missing\nError: Download failed ("+srv.URL+"/missing): curl: (22) The requested URL returned error: 404\n", errb.String())

	errb.Reset()
	rc = d.Run(t.Context(), s, tt, &errb, "", "x")
	require.Equal(t, 1, rc)
	require.Equal(t, "Error: Usage: __gash_download URL DEST\n", errb.String())
}

func TestDownloadProgress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "4")
		_, _ = w.Write([]byte("data"))
	}))
	defer srv.Close()
	var errb bytes.Buffer
	rc := Downloader{Client: srv.Client()}.Run(t.Context(), Style{G: term.ASCIIGlyphs}, term.Fixed(nil, false, false, true),
		&errb, srv.URL, filepath.Join(t.TempDir(), "o"))
	require.Equal(t, 0, rc)
	// A terminal stderr without colors: the progress line, then spaces.
	require.Contains(t, errb.String(), "\r  0% .............................. ")
	require.Contains(t, errb.String(), "\r"+strings.Repeat(" ", 72)+"\r")
	require.True(t, strings.HasSuffix(errb.String(), "Fetched 4 B in 1s (4 B/s)\n"))
}

// LEDGER CORE-10: Ctrl-C during a download removes the partial file by
// path (a quote in the path is only a name), prints Aborted. and returns
// 130.
func TestLedger_CORE10(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write([]byte("part"))
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer srv.Close()
	dir := t.TempDir()
	dest := filepath.Join(dir, "o'$(touch MARK)'")
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		<-started
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	var errb bytes.Buffer
	rc := Downloader{Client: srv.Client()}.Run(ctx, Style{G: term.ASCIIGlyphs}, term.Fixed(nil, false, false, false), &errb, srv.URL, dest)
	require.Equal(t, 130, rc)
	require.NoFileExists(t, dest)
	require.NoFileExists(t, filepath.Join(dir, "MARK"))
	require.True(t, strings.HasSuffix(errb.String(), "\nAborted.\n"), errb.String())
}

// A proxy that refuses the tunnel and a server that closes without an
// answer get curl's exit codes and messages: 56 and 52.
func TestDownloadConnectionFailures(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodConnect, r.Method)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxy.Close()
	pu, _ := url.Parse(proxy.URL)
	tt := term.Fixed(nil, false, false, false)
	s := Style{G: term.ASCIIGlyphs}
	dir := t.TempDir()
	var errb bytes.Buffer
	d := Downloader{Client: &http.Client{Transport: proxyTransport(http.ProxyURL(pu))}}
	rc := d.Run(t.Context(), s, tt, &errb, "https://down.test/f", filepath.Join(dir, "f"))
	require.Equal(t, 56, rc)
	require.Equal(t, "Get: https://down.test/f\nError: Download failed (https://down.test/f): curl: (56) CONNECT tunnel failed, response 502\n", errb.String())
	require.NoFileExists(t, filepath.Join(dir, "f"))

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = c.Read(make([]byte, 4096))
			_ = c.Close()
		}
	}()
	errb.Reset()
	target := "http://" + ln.Addr().String() + "/f"
	rc = Downloader{Client: &http.Client{Transport: proxyTransport(nil)}}.Run(t.Context(), s, tt, &errb, target, filepath.Join(dir, "g"))
	require.Equal(t, 52, rc)
	require.Equal(t, "Get: "+target+"\nError: Download failed ("+target+"): curl: (52) Empty reply from server\n", errb.String())
}
