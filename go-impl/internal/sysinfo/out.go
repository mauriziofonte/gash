// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sysinfo

import (
	"io"

	"github.com/mauriziofonte/gash/internal/ui"
)

// out is the dual output of 2.x (__sysinfo_section, _kv, _item, _sub,
// _raw): verbose through the ui primitives, or compact (=== T ===, K=V,
// --- t). lazy is the state directory of sysinfo: a sub-header waits for
// the first line under it, whatever block that line comes from (SYS-17),
// and a verbose section that printed nothing says so. Without it every
// header prints at once.
type out struct {
	w    io.Writer
	s    ui.Style
	llm  bool
	lazy bool
	// sec, any and sub are the files sec, out and sub of the state
	// directory.
	sec, any bool
	sub      string
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
		return
	}
	o.s.Section(o.w, title)
}

// flush is __sysinfo_flush.
func (o *out) flush() {
	if !o.lazy {
		return
	}
	o.any = true
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
	o.subPrint(title)
}

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
