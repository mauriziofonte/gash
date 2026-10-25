// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"io"
	"os"
	"regexp"
)

// scanChunk is the read size of the file scan; maxPartial bounds a line
// that has no newline yet (a longer one is matched on what is held).
const (
	scanChunk  = 1 << 20
	maxPartial = 4 << 20
)

// lineScan is the state of llm_wait --file-match between two looks: the
// file's identity, the offset of the bytes already scanned and the start
// of a last line that has no newline yet. A file is read once; later
// looks read only what was appended. A file replaced (rotated) or
// truncated is scanned again from its start.
type lineScan struct {
	path string
	re   *regexp.Regexp
	// lit is the literal every match starts with (the regexp's literal
	// prefix): the scan looks for it with bytes.Index and runs the regexp
	// only on the lines that hold it; whole is true when the regexp is
	// that literal alone.
	lit   []byte
	whole bool

	fi      os.FileInfo
	off     int64
	partial []byte
}

func newLineScan(path string, re *regexp.Regexp) *lineScan {
	p, complete := re.LiteralPrefix()
	return &lineScan{path: path, re: re, lit: []byte(p), whole: complete && p != ""}
}

// poll reads what is new and returns the first matching line. changed
// reports that the file appeared or was replaced (the caller watches the
// new one). stop, asked after every chunk, ends a long scan early (the
// offset is kept: the next poll goes on from there).
func (s *lineScan) poll(stop func() bool) (line string, found, changed bool) {
	fi, err := os.Stat(s.path)
	if err != nil {
		s.fi, s.off, s.partial = nil, 0, nil
		return "", false, false
	}
	if s.fi == nil || !os.SameFile(s.fi, fi) || fi.Size() < s.off {
		changed = s.fi == nil || !os.SameFile(s.fi, fi)
		s.off, s.partial = 0, nil
	}
	s.fi = fi
	if fi.Size() == s.off {
		return "", false, changed
	}
	f, err := os.Open(s.path)
	if err != nil {
		return "", false, changed
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Seek(s.off, io.SeekStart); err != nil {
		return "", false, changed
	}
	buf := make([]byte, scanChunk)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			s.off += int64(n)
			data := buf[:n]
			if len(s.partial) > 0 {
				data = append(s.partial, data...)
				s.partial = nil
			}
			end := bytes.LastIndexByte(data, '\n')
			if end >= 0 {
				if l, ok := s.match(data[:end+1]); ok {
					return l, true, changed
				}
			}
			rest := data[end+1:]
			if len(rest) > maxPartial {
				if l, ok := s.match(rest); ok {
					return l, true, changed
				}
				rest = nil
			}
			s.partial = append([]byte(nil), rest...)
		}
		if err != nil {
			break
		}
		if stop != nil && stop() {
			return "", false, changed
		}
	}
	// A last line without its newline yet may already hold the match.
	if len(s.partial) > 0 {
		if l, ok := s.match(s.partial); ok {
			return l, true, changed
		}
	}
	return "", false, changed
}

// match finds the first line of block (whole lines, the last one with or
// without its newline) that the regexp matches.
func (s *lineScan) match(block []byte) (string, bool) {
	if len(s.lit) == 0 {
		for len(block) > 0 {
			i := bytes.IndexByte(block, '\n')
			l := block
			if i >= 0 {
				l, block = block[:i], block[i+1:]
			} else {
				block = nil
			}
			l = bytes.TrimSuffix(l, []byte{'\r'})
			if s.re.Match(l) {
				return string(l), true
			}
		}
		return "", false
	}
	for len(block) > 0 {
		i := bytes.Index(block, s.lit)
		if i < 0 {
			return "", false
		}
		start := bytes.LastIndexByte(block[:i], '\n') + 1
		end := bytes.IndexByte(block[i:], '\n')
		var l []byte
		if end < 0 {
			l, block = block[start:], nil
		} else {
			l, block = block[start:i+end], block[i+end+1:]
		}
		l = bytes.TrimSuffix(l, []byte{'\r'})
		if s.whole || s.re.Match(l) {
			return string(l), true
		}
	}
	return "", false
}

// tailFile returns the last n lines of path, read backwards from its end
// (nil when it cannot be read).
func tailFile(path string, n int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return nil
	}
	size := fi.Size()
	const block = 64 << 10
	var data []byte
	pos := size
	for pos > 0 && bytes.Count(bytes.TrimSuffix(data, []byte{'\n'}), []byte{'\n'}) < n && len(data) < 64<<20 {
		step := int64(block)
		if pos < step {
			step = pos
		}
		pos -= step
		chunk := make([]byte, step)
		if _, err := f.ReadAt(chunk, pos); err != nil && err != io.EOF {
			return nil
		}
		data = append(chunk, data...)
	}
	ls := textLines(data)
	if pos > 0 && len(ls) > 0 {
		ls = ls[1:] // the first one may be cut
	}
	if len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	return ls
}
