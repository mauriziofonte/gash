// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ui

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
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
