// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bufio"
	"bytes"
	"io"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/pathguard"
)

// patchDefaultLines is the per-file line budget of llm_git_diff --patch
// and llm_git_show --patch when no --max-lines, --head or --tail is given.
const patchDefaultLines = 200

// patchArgs are the diff options of every --patch output: fixed prefixes
// (diff.noprefix and diff.mnemonicPrefix cannot change the headers this
// filter reads) and no external diff or textconv.
var patchArgs = []string{"--patch", "--no-color", "--no-ext-diff", "--no-textconv", "--src-prefix=a/", "--dst-prefix=b/"}

// patchFilter is the io.Writer git's patch output goes through: each
// file's section keeps its header lines and gets the line budget of o for
// its body (the S2 marker where lines are cut), --width cuts every line,
// and the body of a secret file (pathguard.SecretName on any of its
// names) is replaced by one marker line. Lines before the first file (the
// commit header of llm_git_show) pass through, cut at --width.
type patchFilter struct {
	o       *llmfmt.Options
	out     *bufio.Writer
	pending []byte
	header  []string
	inFile  bool
	inHead  bool
	secret  bool
	lw      *llmfmt.LineWriter
	omitted int
	// blanks are the blank lines held back in the commit header: a run
	// becomes one line, and none is left at its end.
	blanks int
}

func newPatchFilter(w io.Writer, o *llmfmt.Options) *patchFilter {
	budget := *o
	if !budget.Capped() {
		budget.Head = patchDefaultLines
	}
	return &patchFilter{o: &budget, out: bufio.NewWriterSize(w, 32<<10)}
}

// Write takes git's output in any chunking.
func (p *patchFilter) Write(b []byte) (int, error) {
	n := len(b)
	for len(b) > 0 {
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			p.pending = append(p.pending, b...)
			break
		}
		line := b[:i]
		if len(p.pending) > 0 {
			line = append(p.pending, line...)
			p.pending = nil
		}
		p.line(string(line))
		b = b[i+1:]
	}
	return n, nil
}

// Close ends the last section, flushes, and returns how many lines the
// budget cut (secret bodies included).
func (p *patchFilter) Close() int {
	if len(p.pending) > 0 {
		p.line(string(p.pending))
		p.pending = nil
	}
	p.endFile()
	_ = p.out.Flush()
	return p.omitted
}

func (p *patchFilter) emit(s string) {
	_, _ = p.out.WriteString(llmfmt.Cut(s, p.o.Width))
	_ = p.out.WriteByte('\n')
}

// fileHeader reports the header lines of a file section, the ones that
// come before its first hunk.
func fileHeader(s string) bool {
	for _, pre := range []string{"index ", "--- ", "+++ ", "new file mode ", "deleted file mode ", "old mode ",
		"new mode ", "similarity index ", "dissimilarity index ", "rename from ", "rename to ", "copy from ",
		"copy to ", "mode "} {
		if strings.HasPrefix(s, pre) {
			return true
		}
	}
	return false
}

func (p *patchFilter) line(s string) {
	if strings.HasPrefix(s, "diff --git ") || strings.HasPrefix(s, "diff --cc ") || strings.HasPrefix(s, "diff --combined ") {
		p.endFile()
		if p.blanks > 0 {
			p.emit("")
			p.blanks = 0
		}
		p.inFile, p.inHead, p.header = true, true, []string{s}
		return
	}
	if !p.inFile {
		if strings.TrimSpace(s) == "" {
			p.blanks++
			return
		}
		if p.blanks > 0 {
			p.emit("")
			p.blanks = 0
		}
		p.emit(s)
		return
	}
	if p.inHead {
		if fileHeader(s) {
			p.header = append(p.header, s)
			return
		}
		p.startBody()
	}
	if p.secret {
		p.omitted++
		return
	}
	p.lw.Line(s)
}

// startBody decides on the header gathered so far: a secret file shows its
// first header line and a marker, any other its whole header.
func (p *patchFilter) startBody() {
	p.inHead = false
	p.secret = false
	for _, name := range patchNames(p.header) {
		if pathguard.SecretName(name) {
			p.secret = true
			break
		}
	}
	if p.secret {
		p.emit(p.header[0])
		p.emit("[... secret file: patch omitted]")
		return
	}
	for _, h := range p.header {
		p.emit(h)
	}
	_ = p.out.Flush()
	p.lw = p.o.Lines(p.out, "lines")
}

func (p *patchFilter) endFile() {
	if !p.inFile {
		return
	}
	if p.inHead {
		// A section without a body (a mode change, an empty file).
		p.startBody()
	}
	if p.lw != nil {
		p.omitted += p.lw.Close()
		p.lw = nil
	}
	p.inFile, p.secret = false, false
}

// patchNames are the paths a file section names: both sides of its "diff
// --git a/X b/Y" line (when they can be split: equal names, or quoted
// ones) and the names of its ---, +++, rename and copy lines.
func patchNames(header []string) []string {
	var names []string
	add := func(s string) {
		s = unquoteGit(s)
		if s != "/dev/null" && s != "" {
			names = append(names, s)
		}
	}
	strip := func(s string) string {
		s = unquoteGit(s)
		if len(s) > 2 && (strings.HasPrefix(s, "a/") || strings.HasPrefix(s, "b/")) {
			return s[2:]
		}
		return s
	}
	for _, h := range header {
		switch {
		case strings.HasPrefix(h, "diff --git "):
			rest := strings.TrimPrefix(h, "diff --git ")
			if strings.HasPrefix(rest, `"`) {
				if a, b, ok := splitQuotedPair(rest); ok {
					add(strip(a))
					add(strip(b))
				}
				continue
			}
			// "a/P b/P": the two halves are equal for anything but a
			// rename, whose names come on their own lines.
			if n := len(rest); n%2 == 1 {
				half := (n - 1) / 2
				if rest[half] == ' ' && rest[2:half] == rest[half+3:] {
					add(rest[2:half])
				}
			}
		case strings.HasPrefix(h, "--- "), strings.HasPrefix(h, "+++ "):
			add(strip(strings.TrimRight(h[4:], "\t")))
		case strings.HasPrefix(h, "rename from "), strings.HasPrefix(h, "copy from "):
			add(h[strings.Index(h, "from ")+5:])
		case strings.HasPrefix(h, "rename to "), strings.HasPrefix(h, "copy to "):
			add(h[strings.Index(h, "to ")+3:])
		}
	}
	return names
}

// splitQuotedPair splits `"a/x" "b/y"` (either side may be unquoted).
func splitQuotedPair(s string) (string, string, bool) {
	first, rest, ok := cutQuoted(s)
	if !ok || !strings.HasPrefix(rest, " ") {
		return "", "", false
	}
	return first, rest[1:], true
}

// cutQuoted returns the leading C-quoted string of s and what follows it.
func cutQuoted(s string) (string, string, bool) {
	if !strings.HasPrefix(s, `"`) {
		i := strings.IndexByte(s, ' ')
		if i < 0 {
			return s, "", true
		}
		return s[:i], s[i:], true
	}
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return s[:i+1], s[i+1:], true
		}
	}
	return "", "", false
}

// unquoteGit undoes git's C quoting of a path ("a\tb", octal bytes).
func unquoteGit(s string) string {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return s
	}
	if u, err := strconv.Unquote(s); err == nil {
		return u
	}
	return s
}
