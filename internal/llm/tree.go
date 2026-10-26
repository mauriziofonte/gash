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
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/registry"
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
	// 3.4 (plan F2, F3, S1-S4): the shared output options, --count and
	// --top; without them the output is the 3.0 output.
	o := llmfmt.New(llmfmt.FormatFlag | llmfmt.BudgetFlags | llmfmt.PathFlags)
	var x treeOpts
	args := c.Args
	for len(args) > 0 {
		n, ok := o.Take(c, args)
		if ok && n == 0 {
			n, ok = x.take(c, args)
		}
		if !ok {
			return 1
		}
		if n > 0 {
			args = args[n:]
			continue
		}
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
			return fail(c, "unknown_option", a, registry.OptionsHint(c.Name))
		default:
			if !p.take(c, a) {
				return 1
			}
			args = args[1:]
		}
	}
	depthSet := maxDepth != ""
	if maxDepth == "" {
		maxDepth = "1"
		if textMode {
			maxDepth = "3"
		}
	}
	// One invalid_depth message for 0 and for a non-integer: the depth is
	// 1 or more (LLM-28; 2.x had two detail formats).
	if !uintRE.MatchString(maxDepth) || atoi(maxDepth) < 1 {
		return fail(c, "invalid_depth", "Value '"+maxDepth+"' is not an integer from 1 to 999999999",
			"Provide a depth of 1 or more")
	}
	depth := atoi(maxDepth)
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
	if x.any() || extActive(o) {
		return m.treeExt(c, treeQuery{safe: safe, depth: depth, depthSet: depthSet, textMode: textMode, withStats: withStats,
			forbidden: forbidden, o: o, x: x})
	}
	if textMode {
		_, _ = c.Stdout.Write(treeText(safe, depth, forbidden, m.linkHidden(c), prunedNames, textCap))
		return 0
	}
	_, _ = c.Stdout.Write(treeJSON(safe, safe, depth, withStats, forbidden, prunedNames))
	return 0
}

// treeText is the --text listing: the start point and the entries below
// it (pruned names and other file types left out), in byte order of their
// path components, cut at 200 lines and indented by the slashes below the
// start point. 3.0 (LLM-19): the noise names are pruned below the start
// point only, so a root named vendor lists its contents; the prefix is
// cut literally (a root like a[1] indents right); symlinks are listed as
// "name -> target", as the JSON tree lists them; names with a tab or a
// newline are left out, as in the JSON tree; the root / prints as /.
//
// 3.4: prune is the set of pruned names (nil with --all) and limit the
// line cap (textCap, or no cap when the output budget decides). 3.4.1:
// a link whose target lies in a PROTECT:all path (hidden, nil without
// one) prints "-> [protected]" instead of the target.
func treeText(safe string, maxDepth int, forbidden, hidden func(string) bool, prune map[string]bool, limit int) []byte {
	type textEntry struct {
		path, rel string
		parts     []string
		typ       byte
	}
	var ents []textEntry
	w := findWalker{maxDepth: maxDepth, withStart: true, forbidden: forbidden}
	w.visit = func(e findEntry) bool {
		if e.depth > 0 {
			name := e.path[strings.LastIndexByte(e.path, '/')+1:]
			if prune[name] {
				return false
			}
			if strings.ContainsAny(e.rel, "\n\t") {
				return true
			}
		}
		t := e.typeLetter()
		if t != 'f' && t != 'd' && t != 'l' {
			return true
		}
		var parts []string
		if e.rel != "" {
			parts = strings.Split(e.rel, "/")
		}
		ents = append(ents, textEntry{path: e.path, rel: e.rel, parts: parts, typ: t})
		return true
	}
	w.walk(safe)
	sort.SliceStable(ents, func(i, j int) bool {
		a, b := ents[i].parts, ents[j].parts
		for k := 0; k < len(a) && k < len(b); k++ {
			if a[k] != b[k] {
				return a[k] < b[k]
			}
		}
		return len(a) < len(b)
	})
	if len(ents) > limit {
		ents = ents[:limit]
	}
	var out bytes.Buffer
	for _, e := range ents {
		indent := strings.Repeat("  ", strings.Count(e.rel, "/"))
		name := basename(e.path)
		switch e.typ {
		case 'd':
			if name != "/" {
				name += "/"
			}
		case 'l':
			if hidden != nil && hidden(e.path) {
				name += " -> " + protectedTarget
			} else if target, err := os.Readlink(e.path); err == nil {
				name += " -> " + target
			}
		}
		echo(&out, indent+name)
	}
	return out.Bytes()
}

// protectedTarget stands for the target of a link into a PROTECT:all
// path in a listing (llm_tree --text, here and on a host).
const protectedTarget = "[protected]"

// linkHidden is the test of a link for treeText: whether it leads into
// one of the user's PROTECT:all paths; nil when there are none.
func (m *Module) linkHidden(c *invoke.Call) func(string) bool {
	g := m.guard(c)
	if len(g.Hidden) == 0 {
		return nil
	}
	return g.Hides
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
// as the 2.x awk program does. 3.4: shown is the root's "path" (safe, or
// "." with --relative) and prune the set of pruned names (nil with --all).
func treeJSON(safe, shown string, maxDepth int, withStats bool, forbidden func(string) bool, prune map[string]bool) []byte {
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
		if prune[name] {
			return false
		}
		t := e.typeLetter()
		if t == '?' {
			return true
		}
		// Names with a tab or a newline are left out (LLM-29), tested on
		// the path below the start point so a root whose own name holds
		// one still lists its children (LLM-08).
		if strings.ContainsAny(e.rel, "\n\t") {
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
	b = contract.AppendString(b, shown, contract.EscapeJQ)
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
	b = appendEscaped(b, n.rel[strings.LastIndexByte(n.rel, '/')+1:])
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
