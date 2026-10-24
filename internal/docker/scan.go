// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docker

import (
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
)

// scan is docker_compose_scan [BASEDIR] [--depth N] [--json]
// [--no-color]: the compose files down to depth N, with their services.
// An unknown option is refused, -h/--help is help only as the first word
// (LEDGER DOCK-10); the depth is a non-negative integer, printed as one,
// and the JSON is encoded (DOCK-09); files come in byte order (DOCK-27).
func (m *Module) scan(c *invoke.Call) int {
	if isHelp(c) {
		return helpPage(c)
	}
	x := m.begin(c)
	basedir, depth := ".", "3"
	jsonOut, noColor := false, false
	for _, a := range c.Args {
		if a == "--json" || a == "-j" {
			jsonOut = true
		}
	}
	for args := c.Args; len(args) > 0; {
		switch a := args[0]; {
		case a == "--depth" || a == "-d":
			if len(args) < 2 {
				x.p.Error(a + " needs a value")
				return 1
			}
			depth, args = args[1], args[2:]
		case a == "--json" || a == "-j":
			args = args[1:]
		case a == "--no-color":
			noColor, args = true, args[1:]
		case strings.HasPrefix(a, "-"):
			if jsonOut {
				return c.Fail(contract.New("unknown_option", a, "Use a directory, --depth N, --json or --no-color"))
			}
			x.p.Error("Unknown option: " + a)
			return 1
		default:
			basedir, args = a, args[1:]
		}
	}
	max, ok := maxDepth(depth)
	if !ok {
		if jsonOut {
			return c.Fail(contract.New("invalid_depth", depth, "Use --depth N with N a non-negative integer"))
		}
		x.p.Error("Invalid --depth: " + depth + " (use a non-negative integer)")
		return 1
	}
	depth = strconv.Itoa(max)
	x.colors(noColor)
	w, s := c.Stdout, x.s
	put := func(t string) { _, _ = io.WriteString(w, t) }

	raw := basedir
	basedir, ok = x.resolveDir(raw)
	if !ok {
		return x.refuseDir(raw, jsonOut)
	}
	files := findCompose(basedir, max)
	if len(files) == 0 {
		if jsonOut {
			put(`{"base_path":` + q(basedir) + `,"depth":` + depth + `,"compose_files":[],"total_found":0}` + "\n")
		} else {
			put("No docker-compose files found in " + basedir + " (depth: " + depth + ")\n")
		}
		return 0
	}
	found := make([]parsed, len(files))
	for i, file := range files {
		found[i] = x.parseServices(file, false)
		if found[i].badName != "" {
			return x.refuseBadName(found[i].badName, file, jsonOut)
		}
	}

	if jsonOut {
		var b strings.Builder
		b.WriteString(`{"base_path":` + q(basedir) + `,"depth":` + depth + `,"compose_files":[`)
		for i, file := range files {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(`{"path":` + q(dirname(file)) + `,"file":` + q(basename(file)) + `,"services":[`)
			for j, svc := range found[i].services {
				if j > 0 {
					b.WriteByte(',')
				}
				b.WriteString(q(svc.name))
			}
			b.WriteString(`]}`)
		}
		b.WriteString(`],"total_found":` + strconv.Itoa(len(files)) + "}\n")
		put(b.String())
		return 0
	}

	bold, off := "", ""
	if s.Color {
		bold, off = s.P.Bold, s.P.Off
	}
	put("\n")
	s.Heading(w, "Scanning for Docker Compose files in "+basedir+" (depth: "+depth+")")
	put("\n")
	for i, file := range files {
		s.Print(w, "accent", dirname(file))
		var items []string
		for _, svc := range found[i].services {
			items = append(items, svc.name+": "+svc.image)
		}
		if len(items) > 0 {
			s.List(w, items...)
		}
		put("\n")
	}
	put("Found " + bold + strconv.Itoa(len(files)) + off + " compose file(s)\n")
	return 0
}

// maxDepth is a --depth value: decimal digits (^[0-9]+$) up to INT_MAX,
// the -maxdepth GNU find accepts.
func maxDepth(v string) (int, bool) {
	if v == "" || strings.Trim(v, "0123456789") != "" {
		return 0, false
	}
	t := strings.TrimLeft(v, "0")
	if len(t) > 10 {
		return 0, false
	}
	n, err := strconv.ParseInt("0"+t, 10, 64)
	if err != nil || n > 2147483647 {
		return 0, false
	}
	return int(n), true
}

// composeName reports one of the four names find looks for.
func composeName(n string) bool {
	switch n {
	case "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml":
		return true
	}
	return false
}

// findCompose is $(find BASEDIR -maxdepth DEPTH -type f \( -name ... \)
// 2>/dev/null | sort): regular files (symlinks not followed) with one
// of the compose names, the lines in byte order (LEDGER DOCK-27).
// Unreadable directories are skipped.
func findCompose(root string, max int) []string {
	st, err := os.Lstat(root)
	if err != nil {
		return nil
	}
	var found []string
	if st.Mode().IsRegular() && composeName(basename(root)) {
		found = append(found, root)
	}
	var walk func(dir string, d int)
	walk = func(dir string, d int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		prefix := strings.TrimSuffix(dir, "/") + "/"
		for _, e := range entries {
			p := prefix + e.Name()
			switch {
			case e.Type().IsRegular():
				if composeName(e.Name()) {
					found = append(found, p)
				}
			case e.IsDir() && d < max:
				walk(p, d+1)
			}
		}
	}
	if st.IsDir() && max > 0 {
		walk(root, 1)
	}
	if len(found) == 0 {
		return nil
	}
	ls := strings.Split(strings.Join(found, "\n"), "\n")
	sort.Strings(ls)
	out := ls[:0]
	for _, l := range ls {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
