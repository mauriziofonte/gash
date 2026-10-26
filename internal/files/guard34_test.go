// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package files

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tarWithJunk is a tar of ok.txt, then a 512-byte block that is no header,
// then ../evil.txt: GNU tar skips the junk and extracts ../evil.txt, which
// the guard used to leave unchecked.
func tarWithJunk(t *testing.T, junkFirst bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	junk := bytes.Repeat([]byte("x"), 512)
	if junkFirst {
		buf.Write(junk)
	}
	tw := tar.NewWriter(&buf)
	add := func(name string) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: 3, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte("hi\n")); err != nil {
			t.Fatal(err)
		}
	}
	add("ok.txt")
	if err := tw.Flush(); err != nil {
		t.Fatal(err)
	}
	if !junkFirst {
		buf.Write(junk)
	}
	tw2 := tar.NewWriter(&buf)
	if err := tw2.WriteHeader(&tar.Header{Name: "../evil.txt", Mode: 0o644, Size: 3, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	_, _ = tw2.Write([]byte("hi\n"))
	_ = tw2.Close()
	return buf.Bytes()
}

// A tar header the guard cannot read refuses the archive: nothing is
// extracted, and the message says which member it follows (3.4).
func TestGuardRefusesUnreadableHeader(t *testing.T) {
	need(t, "tar")
	for _, tc := range []struct {
		junkFirst bool
		want      string
	}{
		{false, "the member header after 'ok.txt' is unreadable"},
		{true, "its first member header is unreadable"},
	} {
		d := t.TempDir()
		write(t, filepath.Join(d, "a.tar"), tarWithJunk(t, tc.junkFirst))
		_, errb, rc := runIn(t, d, nil, nil, "archive_extract", "a.tar", "out")
		if rc != 1 || !strings.Contains(errb, "Archive 'a.tar' cannot be checked: "+tc.want+"; nothing extracted.") {
			t.Errorf("junkFirst=%v: rc=%d err=%q", tc.junkFirst, rc, errb)
		}
		if _, err := os.Stat(filepath.Join(d, "out")); err == nil {
			t.Errorf("junkFirst=%v: the output directory was created", tc.junkFirst)
		}
		if _, err := os.Stat(filepath.Join(d, "evil.txt")); err == nil {
			t.Errorf("junkFirst=%v: evil.txt escaped", tc.junkFirst)
		}
	}
}
