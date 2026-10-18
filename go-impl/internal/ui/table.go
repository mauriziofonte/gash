// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ui

import (
	"bufio"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// tableOptions are the parsed arguments of __gash_table. Column keys are
// the 1-based input column numbers; 2.x kept them in associative arrays
// keyed by the text given, so a key written with a leading zero (or with
// other characters) never matched a column: the parser drops those.
type tableOptions struct {
	right     string // ",LIST," as 2.x built it
	indent    int
	gap       int
	header    bool
	colColor  map[int]string
	cellColor map[int]int // column -> style column (arithmetic value)
	hidden    map[string]bool
	bar       map[int]int
	minW      map[int]int
	maxW      map[int]int
	maxLeft   map[int]int
	fit       bool
	fitCol    string // as given, or "" for the default
}

var pairRe = regexp.MustCompile(`^([0-9]+):([0-9]+)$`)

// colKey returns the column a 2.x associative-array key names: a decimal
// number without a leading zero; ok false for a key no lookup matches.
func colKey(k string) (int, bool) {
	if !isDigits(k) || (len(k) > 1 && k[0] == '0') {
		return 0, false
	}
	n, err := strconv.Atoi(k)
	return n, err == nil
}

// parseTable ports the option loop of __gash_table: a value option at the
// end of the line stops the parsing (shift 2 || break), malformed values
// are ignored, unknown words are skipped.
func parseTable(args []string) tableOptions {
	o := tableOptions{
		gap: 2, header: true,
		colColor: map[int]string{}, cellColor: map[int]int{}, hidden: map[string]bool{},
		bar: map[int]int{}, minW: map[int]int{}, maxW: map[int]int{}, maxLeft: map[int]int{},
	}
	indent := "0"
	for i := 0; i < len(args); {
		a := args[i]
		val, has := "", i+1 < len(args)
		if has {
			val = args[i+1]
		}
		switch a {
		case "--color":
			// [[ ${2-} == [0-9]*:* ]] && colcolor[${2%%:*}]=${2#*:}
			if val != "" && val[0] >= '0' && val[0] <= '9' && strings.Contains(val[1:], ":") {
				k, v, _ := strings.Cut(val, ":")
				if n, ok := colKey(k); ok {
					o.colColor[n] = v
				}
			}
		case "--cell-color":
			if m := pairRe.FindStringSubmatch(val); m != nil {
				if n, ok := colKey(m[1]); ok {
					o.cellColor[n] = Int(m[2])
				}
				o.hidden[m[2]] = true
			}
		case "--bar", "--min", "--max", "--max-left":
			if m := pairRe.FindStringSubmatch(val); m != nil {
				if n, ok := colKey(m[1]); ok {
					dst := map[string]map[int]int{"--bar": o.bar, "--min": o.minW, "--max": o.maxW, "--max-left": o.maxLeft}[a]
					dst[n] = Int(m[2])
				}
			}
		case "--fit":
			o.fit = true
			if isDigits(val) {
				o.fitCol = val
				i++
			}
			i++
			continue
		case "--gap":
			if isDigits(val) {
				o.gap = Int(val)
			}
		case "--right":
			o.right = "," + val + ","
		case "--indent":
			indent = val
			if !has {
				indent = "0"
			}
		case "--no-header":
			o.header = false
			i++
			continue
		default:
			i++
			continue
		}
		if !has {
			break
		}
		i += 2
	}
	if isDigits(indent) {
		o.indent = Int(indent)
	}
	return o
}

// splitTabs is __gash_split_tabs: every tab splits, empty fields kept.
func splitTabs(line string) []string { return strings.Split(line, "\t") }

// ReadRows reads table rows from r as `while IFS= read -r line || [[ -n
// $line ]]` does: one row per line, NUL bytes dropped, a last line without
// a newline kept.
func ReadRows(r io.Reader) []string {
	var rows []string
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if line == "" && err != nil {
			break
		}
		line = strings.TrimSuffix(line, "\n")
		rows = append(rows, strings.ReplaceAll(line, "\x00", ""))
		if err != nil {
			break
		}
	}
	return rows
}

// Table writes rows (tab-separated lines) as __gash_table [ARGS...] does,
// to w (stdout in 2.x). The arguments are the 2.x options, parsed with the
// 2.x rules:
//
//	--right N,N  --indent N  --gap N  --no-header  --color N:STYLE
//	--cell-color N:M  --bar N:W  --min N:W  --max N:W  --max-left N:W
//	--fit [N]
//
// s.Color is the color decision of the table (__gash_use_color, or
// __GASH_TABLE_COLOR=1 from __gash_table_into).
func (s Style) Table(w io.Writer, rows []string, args ...string) {
	lines := s.TableLines(rows, args...)
	if len(lines) == 0 {
		return
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	_, _ = io.WriteString(w, b.String())
}

// TableLines returns the lines Table prints (__gash_table_into).
func (s Style) TableLines(rows []string, args ...string) []string {
	o := parseTable(args)
	if len(rows) == 0 {
		return nil
	}
	rows = append([]string(nil), rows...)
	ncols := 0
	for _, r := range rows {
		if n := strings.Count(r, "\t") + 1; n > ncols {
			ncols = n
		}
	}
	var vis []int
	for c := 0; c < ncols; c++ {
		if !o.hidden[strconv.Itoa(c+1)] {
			vis = append(vis, c)
		}
	}
	if len(vis) == 0 {
		return nil
	}
	last := vis[len(vis)-1]
	fitKey := o.fitCol
	fitcol := Int(o.fitCol)
	if fitcol <= 0 {
		fitcol = last + 1
		fitKey = strconv.Itoa(fitcol)
	}
	ell := s.G.Ellipsis
	isBar := func(n int, cell string, r int) bool {
		_, ok := o.bar[n]
		return ok && barCellRe.MatchString(cell) && !(r == 0 && o.header)
	}

	// Cut cells, then measure.
	widths := map[int]int{}
	for r := range rows {
		cells := splitTabs(rows[r])
		for _, c := range vis {
			if c >= len(cells) {
				break
			}
			n := c + 1
			cell := cells[c]
			var w int
			if isBar(n, cell, r) {
				w = o.bar[n]
			} else {
				if lim, ok := o.maxW[n]; ok && lim > 0 && Len(cell) > lim {
					cell = Sub(cell, 0, lim-1) + ell
				}
				if lim, ok := o.maxLeft[n]; ok && lim > 0 && Len(cell) > lim {
					cell = ell + Tail(cell, lim-1)
				}
				cells[c] = cell
				w = Len(cell)
			}
			if w > widths[c] {
				widths[c] = w
			}
		}
		rows[r] = strings.Join(cells, "\t")
	}
	for _, c := range vis {
		if w, ok := o.minW[c+1]; ok && w > widths[c] {
			widths[c] = w
		}
	}

	// Room left for the fitted column when rows must fit the terminal.
	fitMax, fc := -1, fitcol-1
	if o.fit && !o.hidden[fitKey] && fc < ncols {
		used := o.indent
		for _, c := range vis {
			if c != fc {
				used += widths[c] + o.gap
			}
		}
		fitMax = s.Width - used
		if fitMax < 10 {
			fitMax = 10
		}
		if widths[fc] > fitMax {
			widths[fc] = fitMax
		}
	}

	color := s.Color
	pad, sep := Spaces(o.indent), Spaces(o.gap)
	muted, off := "", ""
	if color {
		muted, off = s.P.Muted, s.P.Off
	}
	var emitted []string
	for r := range rows {
		cells := splitTabs(rows[r])
		var out, plain strings.Builder
		rlast := last
		if len(cells) <= last {
			rlast = vis[0]
			for _, c := range vis {
				if c < len(cells) {
					rlast = c
				}
			}
		}
		for _, c := range vis {
			if c > rlast {
				break
			}
			n := c + 1
			cell := ""
			if c < len(cells) {
				cell = cells[c]
			}
			style := o.colColor[n]
			if m, ok := o.cellColor[n]; ok {
				style = index(cells, m-1)
			}
			esc := ""
			if color {
				esc = s.ColorOf(style)
			}
			if isBar(n, cell, r) {
				v, mx, _ := strings.Cut(cell, "/")
				full, empty := s.Bar(Int(v), Int(mx), o.bar[n])
				if color && esc == "" {
					esc = s.P.Primary
				}
				plain.WriteString(full + empty)
				if color {
					out.WriteString(esc + full + off + muted + empty + off)
				} else {
					out.WriteString(full + empty)
				}
				sp := Spaces(widths[c] - o.bar[n])
				plain.WriteString(sp)
				out.WriteString(sp)
				if c != rlast {
					plain.WriteString(sep)
					out.WriteString(sep)
				}
				continue
			}
			if c == fc && fitMax > 0 && Len(cell) > widths[c] {
				if _, ok := o.maxLeft[n]; ok {
					cell = ell + Tail(cell, widths[c]-1)
				} else {
					cell = Sub(cell, 0, widths[c]-1) + ell
				}
			}
			if strings.Contains(o.right, ","+strconv.Itoa(n)+",") {
				cell = PadLeft(cell, widths[c])
			} else if c != rlast {
				cell = PadRight(cell, widths[c])
			}
			plain.WriteString(cell)
			if color && esc != "" && !(r == 0 && o.header) {
				out.WriteString(esc + cell + s.P.Off)
			} else {
				out.WriteString(cell)
			}
			if c != rlast {
				plain.WriteString(sep)
				out.WriteString(sep)
			}
		}
		if r == 0 && o.header {
			if color {
				emitted = append(emitted, pad+s.P.Bold+s.P.Underline+plain.String()+s.P.Off)
			} else {
				emitted = append(emitted, pad+plain.String())
				total := 0
				for _, c := range vis {
					total += widths[c]
				}
				total += o.gap * (len(vis) - 1)
				emitted = append(emitted, pad+strings.Repeat(s.G.HLine, max(total, 0)))
			}
		} else {
			emitted = append(emitted, pad+out.String())
		}
	}
	return emitted
}

var barCellRe = regexp.MustCompile(`^[0-9]+/[0-9]+$`)

// index is ${arr[i]-} with Bash's negative subscripts (counted from the
// end); "" out of range.
func index(arr []string, i int) string {
	if i < 0 {
		i += len(arr)
	}
	if i < 0 || i >= len(arr) {
		return ""
	}
	return arr[i]
}
