// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/mauriziofonte/gash/internal/pathguard"
)

// errLimit ends a search once head -n has its lines.
var errLimit = errors.New("line limit reached")

// lineLimiter is `| head -n N`: it passes the first N lines through.
type lineLimiter struct {
	w    io.Writer
	left int
}

func (l *lineLimiter) line(b []byte) error {
	if l.left <= 0 {
		return errLimit
	}
	l.left--
	if _, err := l.w.Write(b); err != nil {
		return err
	}
	if l.left == 0 {
		return errLimit
	}
	return nil
}

// rgSearch is one rg run of the llm functions: -e PATTERN -- PATH with the
// given globs and ignore policy, its output cut by head -n.
type rgSearch struct {
	re   *rgRegex
	over *overrides
	// userOver is over without the secret excludes: the verdict for a
	// file whose name is a secret exception (.env.example, *.pub), which
	// a walk searches as the explicit-file check allows (LEDGER LLM-15).
	userOver  *overrides
	rules     ignoreRules
	context   int
	filesOnly bool // -l
	cwd       string
	// forbidden leaves out an entry at or below a protected tree
	// (LEDGER LLM-09).
	forbidden func(path string) bool

	out         *lineLimiter
	withName    bool
	printedSome bool // a context group was printed (for the -- separator)

	// 3.4 (plan F5), all zero for the 3.0 output. relative prints paths
	// below the root (--relative); excludes are the --exclude globs,
	// matched below the root; onlyMatching prints each match instead of
	// its line; hit, when set, receives every match instead of the
	// printer (rows, counts, --distinct).
	root         string
	relative     bool
	excludes     []string
	onlyMatching bool
	hit          func(h grepHit) error
}

// grepHit is one match a search reports to its hit function: the file,
// the 1-based line and its text (the match alone with onlyMatching; the
// binary notice for a binary FILE PATH), or the file alone with filesOnly.
// text aliases the file's buffer: a hit function copies what it keeps.
type grepHit struct {
	path string
	line int
	text []byte
}

// show is a path as printed: whole, or below the root with --relative
// (a FILE PATH by its name).
func (s *rgSearch) show(path string) string {
	if !s.relative {
		return path
	}
	if rel := relBelow(s.root, path); rel != "" {
		return rel
	}
	return basename(path)
}

// run searches root as rg does with one PATH argument: a directory is
// walked in path order (deterministic, LLM-03), a file is searched
// whatever its name, a missing path prints nothing.
func (s *rgSearch) run(root string, w io.Writer, limit int) {
	s.root = root
	s.out = &lineLimiter{w: w, left: limit}
	if limit <= 0 {
		return
	}
	st, err := os.Stat(root)
	if err != nil {
		return
	}
	if !st.IsDir() {
		_ = s.searchFile(root, true)
		return
	}
	s.withName = true
	abs := root
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(s.cwd, abs)
	}
	// The walk runs ahead of the search in its own goroutine and starts
	// reading each file it hands over, at most readAheadFiles ahead (3.4):
	// a cold cache serves parallel reads far sooner than one after the
	// other. The files reach the search in walk order, so the output is
	// the same; the walk owns the ignore rules, the search the printer.
	files := make(chan walkFile, readAheadFiles)
	done, walked := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(walked)
		defer close(files)
		var chain *ignoreDir
		for _, a := range ancestors(abs) {
			chain = s.rules.newIgnoreDir(chain, a)
		}
		s.walk(root, abs, s.rules.newIgnoreDir(chain, abs), files, done)
	}()
	// A search that stops early waits for the walk to see done: nothing
	// of this run outlives it (the reads under way only fill their own
	// fileRead).
	defer func() { close(done); <-walked }()
	for f := range files {
		data, err := f.read.wait()
		if err != nil {
			continue
		}
		if s.searchData(f.path, data, false) != nil {
			return
		}
	}
}

// walkFile is one file the walk hands to the search, its read under way.
type walkFile struct {
	path string
	read *fileRead
}

// ancestors lists the directories above abs, from / down to its parent.
func ancestors(abs string) []string {
	var out []string
	for p := filepath.Dir(abs); ; p = filepath.Dir(p) {
		out = append([]string{p}, out...)
		if p == "/" || p == "." || p == filepath.Dir(p) {
			break
		}
	}
	if abs == "/" {
		return nil
	}
	return out
}

// walk sends the files below dir to out, in path order, each with its
// read started; it returns false once done is closed (the search stopped).
func (s *rgSearch) walk(dir, abs string, ig *ignoreDir, out chan<- walkFile, done <-chan struct{}) bool {
	select {
	case <-done:
		return false
	default:
	}
	f, err := os.Open(dir)
	if err != nil {
		return true
	}
	ents, _ := f.ReadDir(-1)
	_ = f.Close()
	sort.Slice(ents, func(i, j int) bool { return ents[i].Name() < ents[j].Name() })
	for _, e := range ents {
		name := e.Name()
		path := findJoin(dir, name)
		apath := findJoin(abs, name)
		isDir := e.IsDir()
		if s.forbidden != nil && s.forbidden(apath) {
			continue
		}
		if s.excludes != nil && walkExcluded(s.excludes, relBelow(s.root, path), name) {
			continue
		}
		if s.skip(ig, apath, name, isDir) {
			continue
		}
		switch {
		case isDir:
			if !s.walk(path, apath, s.rules.newIgnoreDir(ig, apath), out, done) {
				return false
			}
		case e.Type().IsRegular():
			select {
			case out <- walkFile{path: path, read: readAhead(path)}:
			case <-done:
				return false
			}
		}
	}
	return true
}

// readAheadFiles is how many files the walk reads ahead of the one the
// search is on (3.4, plan F5: the cold grep).
const readAheadFiles = 16

// fileRead is one file read in the background.
type fileRead struct {
	done chan struct{}
	data []byte
	err  error
}

func readAhead(path string) *fileRead {
	r := &fileRead{done: make(chan struct{})}
	go func() {
		r.data, r.err = os.ReadFile(path)
		close(r.done)
	}()
	return r
}

func (r *fileRead) wait() ([]byte, error) {
	<-r.done
	return r.data, r.err
}

// skip is the ignore crate's verdict on one walked entry: the overrides
// decide first, then the ignore files, then the hidden rule.
func (s *rgSearch) skip(ig *ignoreDir, path, name string, isDir bool) bool {
	over := s.over
	// A secret exception (.env.example, *.pub) is judged without the
	// secret excludes and, being named to be searched, even when hidden;
	// the user's globs and the ignore files still apply (LEDGER LLM-15).
	exception := !isDir && s.userOver != nil && pathguard.SecretException(name)
	if exception {
		over = s.userOver
	}
	switch over.matched(path, isDir) {
	case matchIgnore:
		return true
	case matchWhitelist:
		return false
	}
	switch s.rules.matchedIgnore(ig, path, isDir) {
	case matchIgnore:
		return true
	case matchWhitelist:
		return false
	}
	return s.rules.hidden && !exception && strings.HasPrefix(name, ".")
}

// decodeText is rg's BOM sniffing: a UTF-8 BOM is dropped, UTF-16 with a
// BOM is transcoded to UTF-8.
func decodeText(data []byte) []byte {
	switch {
	case bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}):
		return data[3:]
	case bytes.HasPrefix(data, []byte{0xff, 0xfe}), bytes.HasPrefix(data, []byte{0xfe, 0xff}):
		le := data[0] == 0xff
		body := data[2:]
		u := make([]uint16, 0, len(body)/2)
		for i := 0; i+1 < len(body); i += 2 {
			if le {
				u = append(u, uint16(body[i])|uint16(body[i+1])<<8)
			} else {
				u = append(u, uint16(body[i])<<8|uint16(body[i+1]))
			}
		}
		var out []byte
		for _, r := range utf16.Decode(u) {
			out = utf8.AppendRune(out, r)
		}
		if len(body)%2 == 1 {
			out = utf8.AppendRune(out, utf8.RuneError)
		}
		return out
	}
	return data
}

// searchFile searches one file. explicit is a file given as PATH: rg
// searches it even when binary and reports a match there with one line.
func (s *rgSearch) searchFile(path string, explicit bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return s.searchData(path, data, explicit)
}

// searchData searches the content of one file.
func (s *rgSearch) searchData(path string, data []byte, explicit bool) error {
	data = decodeText(data)
	nul := bytes.IndexByte(data, 0)
	if nul >= 0 && !explicit {
		return nil // binary files are skipped in a walk
	}
	if !s.re.MayMatch(data) {
		return nil
	}
	lines := splitLines(data)
	var hits []int
	for i, l := range lines {
		if s.re.Match(l) {
			hits = append(hits, i)
			if s.filesOnly || nul >= 0 {
				break
			}
		}
	}
	if len(hits) == 0 {
		return nil
	}
	if s.hit != nil {
		return s.report(path, lines, hits, nul)
	}
	if s.filesOnly {
		return s.out.line([]byte(s.show(path) + "\n"))
	}
	if nul >= 0 {
		// The printer stops at the first event after binary data: a match
		// prints the notice, a context line before it prints nothing.
		if s.context > 0 && hits[0] > 0 {
			return nil
		}
		return s.out.line([]byte(`binary file matches (found "\0" byte around offset ` + strconv.Itoa(nul) + ")\n"))
	}
	isHit := make(map[int]bool, len(hits))
	for _, h := range hits {
		isHit[h] = true
	}
	last := -1 // last line printed in this file
	for _, h := range hits {
		if h <= last {
			continue // printed as context of an earlier match
		}
		from := max(h-s.context, 0)
		if s.context > 0 && s.printedSome && (last < 0 || from > last+1) {
			if err := s.out.line([]byte("--\n")); err != nil {
				return err
			}
		}
		from = max(from, last+1)
		to := min(h+s.context, len(lines)-1)
		for i := from; i <= to; i++ {
			sep := byte('-')
			if isHit[i] {
				sep = ':'
				to = min(max(to, i+s.context), len(lines)-1)
			}
			if err := s.printLine(path, i, sep, lines[i]); err != nil {
				return err
			}
			last = i
		}
		s.printedSome = true
	}
	return nil
}

func (s *rgSearch) printLine(path string, i int, sep byte, line []byte) error {
	if s.onlyMatching && sep == ':' {
		for _, m := range s.matches(line) {
			if err := s.printOne(path, i, sep, m); err != nil {
				return err
			}
		}
		return nil
	}
	return s.printOne(path, i, sep, line)
}

func (s *rgSearch) printOne(path string, i int, sep byte, line []byte) error {
	var b []byte
	if s.withName {
		b = append(b, s.show(path)...)
		b = append(b, sep)
	}
	b = strconv.AppendInt(b, int64(i+1), 10)
	b = append(b, sep)
	b = append(b, line...)
	b = append(b, '\n')
	return s.out.line(b)
}

// splitLines splits on \n; a final line without one is a line too.
func splitLines(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}
	lines := bytes.Split(data, []byte("\n"))
	if len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// report hands the matches of one file to the hit function (3.4).
func (s *rgSearch) report(path string, lines [][]byte, hits []int, nul int) error {
	if s.filesOnly {
		return s.hit(grepHit{path: path})
	}
	if nul >= 0 {
		return s.hit(grepHit{path: path, line: hits[0] + 1,
			text: []byte(`binary file matches (found "\0" byte around offset ` + strconv.Itoa(nul) + ")")})
	}
	for _, h := range hits {
		if !s.onlyMatching {
			if err := s.hit(grepHit{path: path, line: h + 1, text: lines[h]}); err != nil {
				return err
			}
			continue
		}
		for _, m := range s.matches(lines[h]) {
			if err := s.hit(grepHit{path: path, line: h + 1, text: m}); err != nil {
				return err
			}
		}
	}
	return nil
}

// matches are the non-empty matches of a matching line, left to right
// (rg -o). RE2 finds them; on the rare line where only the rg-rules NFA
// matched (text that is not UTF-8, or \b next to non-ASCII text) RE2's
// matches are all there is.
func (s *rgSearch) matches(line []byte) [][]byte {
	var out [][]byte
	for _, loc := range s.re.re.FindAllIndex(line, -1) {
		if loc[1] > loc[0] {
			out = append(out, line[loc[0]:loc[1]])
		}
	}
	return out
}
