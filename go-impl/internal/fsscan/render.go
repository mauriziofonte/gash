// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import (
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/ui"
)

// sizeRow is one entry of a size list: the apparent or disk size, an
// optional mtime string, and the path.
type sizeRow struct {
	bytes int64
	mtime string
	path  string
}

// rowKey is the tab-joined row text `bytes\tmtime\tpath` that 2.x sorted,
// so ties fall in C byte order (FS-28).
func (r sizeRow) rowKey() string {
	return strconv.FormatInt(r.bytes, 10) + "\t" + r.mtime + "\t" + r.path
}

// sortSizeRows orders by size descending, ties by the whole 2.x row text in
// C byte order descending, as `sort -nr -k1,1 -t'\t'` did (FS-28).
func sortSizeRows(rows []sizeRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].bytes != rows[j].bytes {
			return rows[i].bytes > rows[j].bytes
		}
		return rows[i].rowKey() > rows[j].rowKey()
	})
}

// escSizeList escapes a string for the files_largest/dirs_largest/
// dirs_find_large JSON envelopes, which escaped only \ " \n \r \t and left
// every other control byte, DEL and high byte raw (FS-10, reproduced).
func escSizeList(s string) string {
	return escSet(s, true)
}

// escEmpty escapes for the dirs_list_empty data array, which escaped \ " \t
// \r only (FS-10).
func escEmpty(s string) string {
	return escSet(s, false)
}

func escSet(s string, withNL bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			if withNL {
				b.WriteString(`\n`)
			} else {
				b.WriteByte(c)
			}
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// escFull is the full contract escaper (__gash_json_escape): \" \\ \n \r \t
// \b \f and \u00xx for the other C0, used by tree_stats and the
// dirs_list_empty path field.
func escFull(s string) string {
	q := contract.Quote(s, contract.EscapeBash)
	return q[1 : len(q)-1]
}

// renderSizeJSON writes the one-line size-list envelope
// (__gash_fs_render_size_list_json).
func renderSizeJSON(w io.Writer, rows []sizeRow, scanTime string, errCount int, kind string) {
	var b strings.Builder
	b.WriteString(`{"data":[`)
	var total int64
	for i, r := range rows {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"size":`)
		b.WriteString(strconv.FormatInt(r.bytes, 10))
		b.WriteString(`,"size_human":"`)
		b.WriteString(humanIEC5(r.bytes)) // per-row: __gash_fs awk hbytes, PiB cap
		b.WriteByte('"')
		if r.mtime != "" {
			b.WriteString(`,"mtime":"`)
			b.WriteString(escSizeList(r.mtime))
			b.WriteByte('"')
		}
		b.WriteString(`,"path":"`)
		b.WriteString(escSizeList(r.path))
		b.WriteString(`"}`)
		total += r.bytes
	}
	b.WriteString(`],"count":`)
	b.WriteString(strconv.Itoa(len(rows)))
	b.WriteString(`,"total_size":`)
	b.WriteString(strconv.FormatInt(total, 10))
	b.WriteString(`,"total_size_human":"`)
	b.WriteString(ui.HumanBytes(total))
	b.WriteString(`","kind":"`)
	b.WriteString(kind)
	b.WriteString(`","scan_time":"`)
	b.WriteString(scanTime)
	b.WriteString(`","errors":`)
	b.WriteString(strconv.Itoa(errCount))
	b.WriteString("}\n")
	_, _ = io.WriteString(w, b.String())
}

// renderSizeNull writes each path followed by a NUL, nothing else.
func renderSizeNull(w io.Writer, rows []sizeRow) {
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(r.path)
		b.WriteByte(0)
	}
	_, _ = io.WriteString(w, b.String())
}

// renderSizeText writes the human size list: the rich layout on a color
// terminal (__gash_fs_rich_ok, which ignores --no-color, FS-42), else the
// plain "SIZE  PATH[  MTIME]" table.
func renderSizeText(w io.Writer, s ui.Style, rows []sizeRow, human bool, kind, scanTime, root, title string) {
	if s.Color {
		renderSizeRich(w, s, rows, human, kind, scanTime, root, title)
		return
	}
	if len(rows) == 0 {
		return
	}
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		sz := humanOrBytes(r.bytes, human)
		p := r.path
		if r.mtime != "" {
			p = p + "  " + r.mtime
		}
		lines = append(lines, sz+"\t"+p)
	}
	s.Table(w, lines, "--no-header", "--min", "1:12", "--color", "2:accent")
}

func humanOrBytes(n int64, human bool) string {
	if human {
		return humanIEC5(n)
	}
	return strconv.FormatInt(n, 10) + " B"
}

// humanIEC5 is the IEC formatter the size lists use (__GASH_FS_AWK_HBYTES and
// the inline awk of __gash_fs_render_size_list_json): B..PiB only, capped at
// PiB (i < 5), one truncated decimal. It differs from ui.HumanBytes
// (__gash_human_size, B..EiB) for values of 1 EiB or more, which 2.x showed
// as "N.N PiB" in a size row while total_size_human used EiB.
func humanIEC5(size int64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
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

// renderSizeRich is __gash_fs_render_size_list_rich: rank, size, a share
// bar, optional mtime and the path relative to the root, with a footer.
func renderSizeRich(w io.Writer, s ui.Style, rows []sizeRow, human bool, kind, scanTime, root, title string) {
	muted := func(msg string) {
		if s.Color {
			_, _ = io.WriteString(w, s.P.Muted+msg+s.P.Off+"\n")
		} else {
			_, _ = io.WriteString(w, msg+"\n")
		}
	}
	if len(rows) == 0 {
		_, _ = io.WriteString(w, "\n")
		muted("  No " + kind + " matched.")
		_, _ = io.WriteString(w, "\n")
		return
	}
	if title == "" {
		title = "Largest " + kind + " in " + root
	}
	var total, max int64
	hasMt := false
	for i, r := range rows {
		if i == 0 || r.bytes > max {
			max = r.bytes
		}
		total += r.bytes
		if r.mtime != "" {
			hasMt = true
		}
	}
	noun := kind
	if len(rows) == 1 {
		if kind == "files" {
			noun = "file"
		} else {
			noun = "directory"
		}
	}
	dot := s.G.Dot
	footer := strconv.Itoa(len(rows)) + " " + noun
	if kind == "files" {
		footer += " " + dot + " " + humanOrBytes(total, human) + " total"
	} else {
		footer += " " + dot + " largest " + humanOrBytes(rows[0].bytes, human)
	}
	if scanTime != "" {
		footer += " " + dot + " " + scanTime
	}

	// 2.x emits a header row as the first table line (the awk END block,
	// files.sh:1823-1825), then calls the table with its default header
	// on; the table styles row 0 as the header and skips its share bar.
	header := "#\tSIZE\tSHARE"
	if hasMt {
		header += "\tMODIFIED"
	}
	header += "\tPATH"
	tableRows := make([]string, 0, len(rows)+1)
	tableRows = append(tableRows, header)
	for i, r := range rows {
		// The 2.x rich awk rejoins a tab-split path with spaces, so an
		// embedded tab shows as a space in the rich layout only.
		rel := relTo(strings.ReplaceAll(r.path, "\t", " "), root)
		cells := []string{
			strconv.Itoa(i + 1),
			humanOrBytes(r.bytes, human),
			strconv.FormatInt(r.bytes, 10) + "/" + strconv.FormatInt(max, 10),
		}
		if hasMt {
			cells = append(cells, r.mtime)
		}
		cells = append(cells, rel)
		tableRows = append(tableRows, strings.Join(cells, "\t"))
	}
	pcol := "4"
	if hasMt {
		pcol = "5"
	}
	_, _ = io.WriteString(w, "\n")
	s.Heading(w, "--indent", "2", title)
	_, _ = io.WriteString(w, "\n")
	s.Table(w, tableRows,
		"--indent", "2", "--right", "1,2", "--bar", "3:16",
		"--color", "1:muted", "--color", "4:muted", "--color", pcol+":accent",
		"--max-left", pcol+":"+strconv.Itoa(s.Width), "--fit", pcol)
	muted("  " + footer)
	_, _ = io.WriteString(w, "\n")
}

// relTo is the awk path shortening of the rich renderer: ROOT/x becomes x,
// ROOT itself becomes ".".
func relTo(p, root string) string {
	if root != "/" && strings.HasPrefix(p, root+"/") {
		return p[len(root)+1:]
	}
	if p == root {
		return "."
	}
	return p
}

// nowClock is the clock used for scan timing; a test seam.
var nowClock = time.Now

// formatElapsed is __gash_fs_format_elapsed.
func formatElapsed(ms int64) string {
	switch {
	case ms < 1000:
		return strconv.FormatInt(ms, 10) + "ms"
	case ms < 60000:
		return strconv.FormatInt(ms/1000, 10) + "." + pad2((ms%1000)/10) + "s"
	default:
		return strconv.FormatInt(ms/60000, 10) + "m" + strconv.FormatInt((ms%60000)/1000, 10) + "s"
	}
}

func pad2(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}
