// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/pathguard"
)

// treeCap is the awk cap= of the JSON listing, textCap the head -n of the
// text listing.
const (
	treeCap = 500
	textCap = 200
)

// tree is llm_tree (fs.sh __llm_tree_impl).
func (m *Module) tree(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	textMode, withStats := false, false
	maxDepth := ""
	var p pathTaker
	p.path = "."
	args := c.Args
	for len(args) > 0 {
		a := args[0]
		switch {
		case a == "--text":
			textMode = true
			args = args[1:]
		case a == "--depth" || a == "--max-depth":
			if len(args) < 2 {
				return fail(c, "missing_value", a, a+" needs a value")
			}
			maxDepth = args[1]
			args = args[2:]
		case a == "--stats":
			withStats = true
			args = args[1:]
		case strings.HasPrefix(a, "-"):
			return fail(c, "unknown_option", a, "Use --text, --depth N, or --stats")
		default:
			if !p.take(c, a) {
				return 1
			}
			args = args[1:]
		}
	}
	if maxDepth == "" {
		maxDepth = "1"
		if textMode {
			maxDepth = "3"
		}
	}
	if !validUint(c, "depth", maxDepth) {
		return 1
	}
	depth := atoi(maxDepth)
	if depth < 1 {
		return fail(c, "invalid_depth", maxDepth, "Provide a positive integer")
	}
	safe, ok := m.validatePath(c, p.path)
	if !ok {
		return 1
	}
	if st, err := os.Stat(safe); err != nil || !st.IsDir() {
		return fail(c, "not_a_directory", safe, "Provide a valid directory path")
	}
	forbidden, ok := m.walkRoot(c, safe, "names")
	if !ok {
		return 1
	}
	if textMode {
		_, _ = c.Stdout.Write(treeText(safe, depth, forbidden))
		return 0
	}
	_, _ = c.Stdout.Write(treeJSON(safe, depth, withStats, forbidden))
	return 0
}

// treeText is the --text listing: find's output (start point included,
// pruned names and everything but files and directories left out) split
// into lines, sorted, cut at 200 lines and indented by the slashes below
// the start point.
func treeText(safe string, maxDepth int, forbidden func(string) bool) []byte {
	var raw strings.Builder
	w := findWalker{maxDepth: maxDepth, withStart: true, forbidden: forbidden}
	w.visit = func(e findEntry) bool {
		name := startName(e.path)
		if e.depth > 0 {
			name = e.path[strings.LastIndexByte(e.path, '/')+1:]
		}
		if prunedNames[name] {
			return false
		}
		if t := e.typeLetter(); t == 'f' || t == 'd' {
			raw.WriteString(e.path)
			raw.WriteByte('\n')
		}
		return true
	}
	w.walk(safe)
	lines := strings.Split(raw.String(), "\n")
	lines = lines[:len(lines)-1]
	sort.Strings(lines)
	if len(lines) > textCap {
		lines = lines[:textCap]
	}
	var out bytes.Buffer
	for _, item := range lines {
		rel := stripPrefixGlob(item, safe)
		rel = strings.TrimPrefix(rel, "/")
		indent := strings.Repeat("  ", strings.Count(rel, "/"))
		name := basename(item)
		if st, err := os.Stat(item); err == nil && st.IsDir() {
			name += "/"
		}
		echo(&out, indent+name)
	}
	return out.Bytes()
}

// stripPrefixGlob is ${item#$pat} with an unquoted pattern: the shortest
// prefix of item that matches the shell pattern pat is removed.
func stripPrefixGlob(item, pat string) string {
	for i := 0; i <= len(item); i++ {
		if pathguard.MatchGlobSegment(pat, item[:i]) {
			return item[i:]
		}
	}
	return item
}

// basename is $(basename "$p"): trailing slashes removed, the last
// component kept, "/" for a path of slashes.
func basename(p string) string {
	t := strings.TrimRight(p, "/")
	if t == "" {
		if p == "" {
			return ""
		}
		return "/"
	}
	return t[strings.LastIndexByte(t, '/')+1:]
}

// echo writes s as Bash's builtin echo prints its one argument: a word made
// only of -n, -e and -E letters is taken as options.
func echo(out *bytes.Buffer, s string) {
	if len(s) > 1 && s[0] == '-' && strings.Trim(s[1:], "neE") == "" {
		if !strings.Contains(s, "n") {
			out.WriteByte('\n')
		}
		return
	}
	out.WriteString(s)
	out.WriteByte('\n')
}

// treeNode is one listed entry of the JSON tree.
type treeNode struct {
	rel      string
	typ      string
	size     int64
	children []*treeNode
}

// treeJSON is the JSON tree: the find stream below the start point (pruned
// names, other file types and paths with a tab or newline left out) folded
// as the 2.x awk program does.
func treeJSON(safe string, maxDepth int, withStats bool, forbidden func(string) bool) []byte {
	total := map[string]int64{}
	nchild := map[string]int{}
	nodes := map[string]*treeNode{}
	root := &treeNode{}
	nodes[""] = root
	listed, truncated := 0, false

	w := findWalker{maxDepth: maxDepth, forbidden: forbidden}
	if withStats {
		w.maxDepth = -1
	}
	w.visit = func(e findEntry) bool {
		name := e.path[strings.LastIndexByte(e.path, '/')+1:]
		if prunedNames[name] {
			return false
		}
		t := e.typeLetter()
		if t == '?' {
			return true
		}
		// ! -path '*\n*' ! -path '*\t*': tested on the whole path, the
		// start point included (LLM-08, LLM-29).
		if strings.ContainsAny(e.path, "\n\t") {
			return true
		}
		parts := strings.Split(e.rel, "/")
		n := len(parts)
		par := ""
		if n > 1 {
			par = strings.Join(parts[:n-1], "/")
		}
		if t != 'd' {
			total[""] += e.size
			anc := ""
			for i := 1; i < n && i <= maxDepth; i++ {
				if i == 1 {
					anc = parts[0]
				} else {
					anc += "/" + parts[i-1]
				}
				total[anc] += e.size
			}
		}
		nchild[par]++
		if n > maxDepth {
			return true
		}
		if listed >= treeCap {
			truncated = true
			return true
		}
		listed++
		node := &treeNode{rel: e.rel, size: e.size}
		switch t {
		case 'd':
			node.typ = "directory"
		case 'l':
			node.typ = "link"
		default:
			node.typ = "file"
		}
		nodes[e.rel] = node
		if pn := nodes[par]; pn != nil {
			pn.children = append(pn.children, node)
		}
		return true
	}
	w.walk(safe)

	var b []byte
	b = append(b, `{"path":`...)
	b = contract.AppendString(b, safe, contract.EscapeBash)
	b = append(b, `,"type":"directory"`...)
	if withStats {
		b = appendStats(b, total[""], nchild[""])
	}
	if truncated {
		b = append(b, `,"truncated":true`...)
	}
	b = append(b, `,"children":[`...)
	for i, ch := range root.children {
		if i > 0 {
			b = append(b, ',')
		}
		b = emitTreeNode(b, ch, maxDepth, withStats, total, nchild)
	}
	return append(b, "]}\n"...)
}

func appendStats(b []byte, size int64, count int) []byte {
	b = append(b, `,"size":`...)
	b = strconv.AppendInt(b, size, 10)
	b = append(b, `,"children_count":`...)
	return strconv.AppendInt(b, int64(count), 10)
}

func emitTreeNode(b []byte, n *treeNode, depthRem int, withStats bool, total map[string]int64, nchild map[string]int) []byte {
	b = append(b, `{"name":"`...)
	b = appendAwkEscaped(b, n.rel[strings.LastIndexByte(n.rel, '/')+1:])
	b = append(b, `","type":"`...)
	b = append(b, n.typ...)
	b = append(b, '"')
	if withStats {
		if n.typ == "directory" {
			b = appendStats(b, total[n.rel], nchild[n.rel])
		} else {
			b = append(b, `,"size":`...)
			b = strconv.AppendInt(b, n.size, 10)
		}
	}
	if n.typ == "directory" && depthRem > 1 && len(n.children) > 0 {
		b = append(b, `,"children":[`...)
		for i, ch := range n.children {
			if i > 0 {
				b = append(b, ',')
			}
			b = emitTreeNode(b, ch, depthRem-1, withStats, total, nchild)
		}
		b = append(b, ']')
	}
	return append(b, '}')
}

// appendAwkEscaped is the esc() of the 2.x awk programs: backslash, double
// quote, newline, carriage return and tab escaped, every other byte raw
// (LLM-06, LLM-23).
func appendAwkEscaped(b []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\':
			b = append(b, '\\', '\\')
		case '"':
			b = append(b, '\\', '"')
		case '\n':
			b = append(b, '\\', 'n')
		case '\r':
			b = append(b, '\\', 'r')
		case '\t':
			b = append(b, '\\', 't')
		default:
			b = append(b, c)
		}
	}
	return b
}
