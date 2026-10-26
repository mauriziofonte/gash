// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"sort"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/remote"
)

// treeFallback is the POSIX sh llm_tree for a host without the gash
// binary ([PATH] [--depth N] [--stats] [--text]): the remote side checks
// the root as the local function does and lists the entries below it
// (type, lstat size, path, link target), find's -printf where it has one,
// a loop over -print and ls -ldn elsewhere; the tree is folded here, in
// the walk's order, with the local emitters, so the output is the local
// one. With PROTECT:all paths for the host, a link the host resolves into
// one lists "[protected]" as its target.
func treeFallback(_ *invoke.Call, nonce string, args []string) (*remote.Plan, bool) {
	textMode, withStats := false, false
	maxDepth, path, havePath := "", ".", false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--text":
			textMode = true
		case a == "--stats":
			withStats = true
		case a == "--depth" || a == "--max-depth":
			if i+1 >= len(args) {
				return argError("missing_value", a, a+" needs a value")
			}
			maxDepth = args[i+1]
			i++
		case strings.HasPrefix(a, "-"):
			return nil, false
		case havePath:
			return argError("unexpected_argument", a, "Pass at most one PATH")
		default:
			path, havePath = a, true
		}
	}
	if maxDepth == "" {
		maxDepth = "1"
		if textMode {
			maxDepth = "3"
		}
	}
	if !uintRE.MatchString(maxDepth) || atoi(maxDepth) < 1 {
		return argError("invalid_depth", "Value '"+maxDepth+"' is not an integer from 1 to 999999999", "Provide a depth of 1 or more")
	}
	depth := atoi(maxDepth)
	all := "0"
	if withStats && !textMode {
		all = "1"
	}
	script := fbHeader("P", path, "D", strconv.Itoa(depth), "ALL", all) + `
abs=$(gash_resolve "$P") || exit 1
gash_forbidden "$abs" && gash_err forbidden_path protected "$abs"
[ -d "$abs" ] || gash_err not_a_directory dir "$abs"
gash_forbidden "$abs" names && gash_err forbidden_path subdir "$abs"
gash_tools find
printf '%s root %s\n' "$N" "$abs" >&2
if [ "$ALL" = 1 ]; then set -- -mindepth 1; else set -- -mindepth 1 -maxdepth "$D"; fi
set -- "$@" \( -name node_modules -o -name vendor -o -name .git -o -name __pycache__ -o -name .cache -o -name "*$NL*" -o -name "*$TAB*" \) -prune -o
PF=; find "$abs" -maxdepth 0 -printf '' >/dev/null 2>&1 && PF=1
[ -n "$PF" ] || gash_tools ls
gash_list() {
  if [ -n "$PF" ]; then
    find "$abs" "$@" -printf '%y\t%s\t%P\t%l\n' 2>/dev/null
  else
    find "$abs" "$@" -print 2>/dev/null | while IFS= read -r p; do
      if [ -L "$p" ]; then t=l; elif [ -d "$p" ]; then t=d; elif [ -f "$p" ]; then t=f; else t=x; fi
      set -f; set -- $(ls -ldn -- "$p" 2>/dev/null); set +f
      l=; [ "$t" = l ] && { gash_tools readlink; l=$(readlink -- "$p" 2>/dev/null); }
      printf '%s\t%s\t%s\t%s\n' "$t" "${5:-0}" "${p#"$abs"/}" "$l"
    done
  fi
}
if [ -n "$GP_ALL" ]; then
  gash_list "$@" | while IFS="$TAB" read -r t s p l; do
    [ "$t" = l ] && r=$(gash_resolve "$abs/$p" 2>/dev/null) && gash_in "$GP_ALL" "$r" && l=` + remote.Quote(protectedTarget) + `
    printf '%s\t%s\t%s\t%s\n' "$t" "$s" "$p" "$l"
  done
else
  gash_list "$@"
fi
exit 0`
	finish := func(_ remote.Host, out, errOut []byte, st int) ([]byte, []byte, int) {
		if env, ok := fbError(errOut, nonce, nil); ok {
			return nil, env, 1
		}
		root, ok := fbFrame(errOut, nonce, "root")
		if !ok || st != 0 {
			return out, nil, st
		}
		ents := parseListing(root, out)
		if textMode {
			return treeTextList(root, ents), nil, 0
		}
		return treeJSONList(root, depth, withStats, ents), nil, 0
	}
	return &remote.Plan{Script: script, Finish: finish}, true
}

// listEnt is one line of the remote listing.
type listEnt struct {
	typ    byte // f, d, l or ? (find's %y; anything else is ?)
	size   int64
	rel    string
	parts  []string
	target string
}

// parseListing reads "TYPE\tSIZE\tREL\tTARGET" lines and returns them in
// the local walk's order: each directory's entries in byte order of their
// names, a directory before what is below it.
func parseListing(root string, out []byte) []listEnt {
	var ents []listEnt
	for _, l := range strings.Split(string(out), "\n") {
		f := strings.SplitN(l, "\t", 4)
		if len(f) < 4 || f[2] == "" || f[0] == "" {
			continue
		}
		size, _ := strconv.ParseInt(f[1], 10, 64)
		t := f[0][0]
		if t != 'f' && t != 'd' && t != 'l' {
			t = '?'
		}
		ents = append(ents, listEnt{typ: t, size: size, rel: f[2], parts: strings.Split(f[2], "/"), target: f[3]})
	}
	sort.SliceStable(ents, func(i, j int) bool { return lessParts(ents[i].parts, ents[j].parts) })
	return ents
}

func lessParts(a, b []string) bool {
	for k := 0; k < len(a) && k < len(b); k++ {
		if a[k] != b[k] {
			return a[k] < b[k]
		}
	}
	return len(a) < len(b)
}

// treeJSONList is treeJSON over a listing: the same fold (the cumulative
// sizes, the real children counts, the 500-entry cap in walk order) and
// the same emitters.
func treeJSONList(safe string, maxDepth int, withStats bool, ents []listEnt) []byte {
	total := map[string]int64{}
	nchild := map[string]int{}
	nodes := map[string]*treeNode{}
	root := &treeNode{}
	nodes[""] = root
	listed, truncated := 0, false
	for _, e := range ents {
		if e.typ == '?' {
			continue
		}
		n := len(e.parts)
		if !withStats && n > maxDepth {
			continue
		}
		par := ""
		if n > 1 {
			par = strings.Join(e.parts[:n-1], "/")
		}
		if e.typ != 'd' {
			total[""] += e.size
			anc := ""
			for i := 1; i < n && i <= maxDepth; i++ {
				if i == 1 {
					anc = e.parts[0]
				} else {
					anc += "/" + e.parts[i-1]
				}
				total[anc] += e.size
			}
		}
		nchild[par]++
		if n > maxDepth {
			continue
		}
		if listed >= treeCap {
			truncated = true
			continue
		}
		listed++
		node := &treeNode{rel: e.rel, size: e.size}
		switch e.typ {
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
	}
	var b []byte
	b = append(b, `{"path":`...)
	b = contract.AppendString(b, safe, contract.EscapeJQ)
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

// treeTextList is treeText over a listing: the root, then the files,
// directories and links below it, cut at 200 lines.
func treeTextList(safe string, ents []listEnt) []byte {
	var out bytes.Buffer
	name := basename(safe)
	if name != "/" {
		name += "/"
	}
	echo(&out, name)
	n := 1
	for _, e := range ents {
		if n >= textCap {
			break
		}
		if e.typ == '?' {
			continue
		}
		n++
		indent := strings.Repeat("  ", len(e.parts)-1)
		name := e.parts[len(e.parts)-1]
		switch e.typ {
		case 'd':
			name += "/"
		case 'l':
			name += " -> " + e.target
		}
		echo(&out, indent+name)
	}
	return out.Bytes()
}
