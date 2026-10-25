// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

// The remote multi-file read (plan R5): several files of one host in ONE
// ssh invocation, each checked on the host on its resolved path (secret
// file, protected path, regular file, readable, the size cap) before a
// byte of it is sent. llm_read --host builds on it.

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/contract"
)

// RemoteFile is one file of a ReadFiles answer: Data when Err is nil.
type RemoteFile struct {
	// Path is the path as asked, Abs the path the host resolved it to.
	Path, Abs string
	Data      []byte
	Err       *contract.Envelope
}

// ReadScript is the remote side: for each path a header line
// "<nonce> file I STATUS ABS" on stdout, the content when STATUS is ok,
// then "\n<nonce> end I"; the "rc" trailer on stderr.
func ReadScript(nonce string, paths []string, maxBytes int64) string {
	var s strings.Builder
	s.WriteString("N=" + Quote(nonce) + "\nLC_ALL=C; export LC_ALL\nMAX=" + strconv.FormatInt(maxBytes, 10) + "\n")
	s.WriteString(ShResolve() + ShForbidden() + ShSecret())
	s.WriteString(`gash_read() {
  r_a=$(gash_resolve "$2")
  if gash_secret "$r_a"; then r_s=secret_file_blocked
  elif gash_forbidden "$r_a"; then r_s=forbidden_path
  elif [ ! -f "$r_a" ]; then r_s=file_not_found
  elif [ ! -r "$r_a" ]; then r_s=path_unreadable
  elif [ "$(wc -c <"$r_a" | tr -d ' ')" -gt "$MAX" ]; then r_s=file_too_large
  else r_s=ok
  fi
  printf '%s file %s %s %s\n' "$N" "$1" "$r_s" "$r_a"
  [ "$r_s" = ok ] && cat -- "$r_a"
  printf '\n%s end %s\n' "$N" "$1"
}
`)
	for i, p := range paths {
		s.WriteString("gash_read " + strconv.Itoa(i+1) + " " + Quote(p) + "\n")
	}
	s.WriteString(Trailer(nonce, "rc", "0") + "\nexit 0\n")
	return s.String()
}

// ParseRead splits the stdout of ReadScript into the files of paths, in
// order; a file the output does not hold completely is a
// remote_protocol_error of its own.
func ParseRead(nonce string, paths []string, maxBytes int64, out []byte) []RemoteFile {
	files := make([]RemoteFile, len(paths))
	for i, p := range paths {
		f := RemoteFile{Path: p}
		n := strconv.Itoa(i + 1)
		head := []byte(nonce + " file " + n + " ")
		end := []byte("\n" + nonce + " end " + n + "\n")
		h := bytes.Index(out, head)
		nl := -1
		if h >= 0 {
			nl = bytes.IndexByte(out[h:], '\n')
		}
		if h < 0 || nl < 0 {
			f.Err = readErr("remote_protocol_error", p+": the host's answer is incomplete", "Retry once; the remote login shell must run POSIX sh commands")
			files[i] = f
			continue
		}
		status, abs, _ := strings.Cut(string(out[h+len(head):h+nl]), " ")
		body := out[h+nl+1:]
		e := bytes.Index(body, end)
		if e < 0 {
			f.Err = readErr("remote_protocol_error", p+": the host's answer is incomplete", "Retry once; the remote login shell must run POSIX sh commands")
			files[i] = f
			continue
		}
		f.Abs = abs
		switch status {
		case "ok":
			f.Data = append([]byte(nil), body[:e]...)
		case "secret_file_blocked":
			f.Err = readErr(status, abs, "This file contains secrets and cannot be read")
		case "forbidden_path":
			f.Err = readErr(status, abs, "This path is protected and cannot be accessed")
		case "file_not_found":
			f.Err = readErr(status, abs, "Check the file path and try again")
		case "path_unreadable":
			f.Err = readErr(status, abs, "The file is not readable by this user")
		case "file_too_large":
			f.Err = readErr(status, abs, "Over the "+strconv.FormatInt(maxBytes, 10)+" byte limit of a remote read")
		default:
			f.Err = readErr("remote_protocol_error", p+": unknown status "+status, "")
		}
		out = body[e+len(end):]
		files[i] = f
	}
	return files
}

func readErr(code, details, hint string) *contract.Envelope {
	e := contract.New(code, details, hint)
	return &e
}

// ReadFiles reads paths from h in one invocation. The paths pass CheckRead
// first (nothing connects otherwise); the envelope is the call's own
// failure (a refused path, the host unreachable), per-file refusals are in
// each RemoteFile.
func (t Transport) ReadFiles(ctx context.Context, h Host, paths []string, maxBytes int64, timeout time.Duration) ([]RemoteFile, *contract.Envelope) {
	for _, p := range paths {
		if e := CheckRead(p); e != nil {
			return nil, e
		}
	}
	nonce := NewNonce()
	r := t.Exec(ctx, h, nonce, ReadScript(nonce, paths, maxBytes), nil, timeout)
	if e := r.Failure(h); e != nil {
		return nil, e
	}
	return ParseRead(nonce, paths, maxBytes, r.Stdout), nil
}
