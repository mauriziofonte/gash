// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sysinfo

import (
	"io"

	"github.com/mauriziofonte/gash/internal/ui"
)

// out is the dual output of 2.x (__sysinfo_section, _kv, _item, _sub,
// _raw): verbose through the ui primitives, or compact (=== T ===, K=V,
// --- t). lazy is the state directory of sysinfo: a sub-header waits for
// the first line of its own block, and a block that printed nothing drops
// it (endsub), so a later line never inherits it (SYS-17); a verbose
// section that printed nothing says so. Without it every header prints at
// once.
type out struct {
	w    io.Writer
	s    ui.Style
	llm  bool
	lazy bool
	// sec, any and sub are the files sec, out and sub of the state
	// directory.
	sec, any bool
	sub      string
	// notice is a note the next section prints under its header (why
	// sudo could not be used).
	notice string
}

func (s *Session) output(w io.Writer, llm, lazy bool) *out {
	return &out{w: w, s: s.style, llm: llm, lazy: lazy}
}

func (o *out) write(s string) { _, _ = io.WriteString(o.w, s) }

// close is __sysinfo_section_close.
func (o *out) close() {
	if !o.lazy || !o.sec {
		return
	}
	if !o.any && !o.llm {
		o.s.Print(o.w, "muted", "  nothing found")
	}
	o.sec, o.any, o.sub = false, false, ""
}

// section is __sysinfo_section TITLE.
func (o *out) section(title string) {
	o.close()
	if o.lazy {
		o.sec = true
	}
	if o.llm {
		o.write("=== " + title + " ===\n")
	} else {
		o.s.Section(o.w, title)
	}
	if o.notice != "" {
		o.note(o.notice)
		o.notice = ""
	}
}

// note is a Note row: what the block it ends up in lacks and why, such as
// "ss is not installed" (3.4: 2.x left the block empty).
func (o *out) note(text string) { o.kv("Note", text) }

// flush is __sysinfo_flush, and prints a pending lazySub header in
// either mode.
func (o *out) flush() {
	if o.lazy {
		o.any = true
	}
	if o.sub == "" {
		return
	}
	t := o.sub
	o.sub = ""
	o.subPrint(t)
}

// kv is __sysinfo_kv KEY VALUE (nothing for an empty value).
func (o *out) kv(key, value string) {
	if value == "" {
		return
	}
	o.flush()
	if o.llm {
		o.write(key + "=" + value + "\n")
		return
	}
	o.s.KV(o.w, "--indent", "2", "--width", "24", key+":", value)
}

// item is __sysinfo_item TEXT.
func (o *out) item(text string) {
	o.flush()
	if o.llm {
		o.write(text + "\n")
		return
	}
	o.s.List(o.w, "--indent", "2", text)
}

// subhead is __sysinfo_sub TITLE.
func (o *out) subhead(title string) {
	if o.lazy {
		o.sub = title
		return
	}
	o.sub = ""
	o.subPrint(title)
}

// lazySub is a sub-header that prints only with the first line of its
// block, also where the other headers print at once (the compact
// collectors ai_sysinfo runs): the 3.x headings that 2.x never printed
// (F-10).
func (o *out) lazySub(title string) { o.sub = title }

// endsub ends the block of the last sub-header: a header still waiting
// for its first line is dropped, and the lines that follow belong to the
// section.
func (o *out) endsub() { o.sub = "" }

func (o *out) subPrint(title string) {
	if o.llm {
		o.write("--- " + title + "\n")
		return
	}
	o.s.Heading(o.w, "--indent", "2", title)
}

// raw is `... | __sysinfo_raw`: the lines, indented in verbose mode, the
// pending sub-header first.
func (o *out) raw(ls []string) {
	indent := "    "
	if o.llm {
		indent = ""
	}
	for i, l := range ls {
		if i == 0 {
			o.flush()
		}
		o.write(indent + l + "\n")
	}
}

// rawBytes is `cat ... | __sysinfo_raw`.
func (o *out) rawBytes(b []byte) { o.raw(lines(b)) }
