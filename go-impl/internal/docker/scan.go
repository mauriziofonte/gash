// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docker

import (
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
)

const scanHelp = `docker_compose_scan - Scan for Docker Compose projects

USAGE:
  docker_compose_scan [BASEDIR] [OPTIONS]

OPTIONS:
  BASEDIR     Base directory to scan (default: current dir)
  --depth N   Maximum search depth (default: 3)
  --json      Output in JSON format
  --no-color  Disable ANSI colors
  -h          Show this help

EXAMPLES:
  docker_compose_scan                     Scan current directory
  docker_compose_scan ~ --depth 2         Scan home with depth 2
  docker_compose_scan /projects --json    JSON output
`

// scan is docker_compose_scan [BASEDIR] [--depth N] [--json]
// [--no-color]: the compose files down to depth N, with their services.
// Unknown options are ignored (LEDGER DOCK-10, kept); the JSON keeps
// the raw names, [""] and the depth text, and a depth find refuses gives
// the empty shape (DOCK-09, kept); files come in byte order (DOCK-27).
func (m *Module) scan(c *invoke.Call) int {
	if isHelp(c) {
		_, _ = io.WriteString(c.Stdout, scanHelp)
		return 0
	}
	x := m.begin(c)
	basedir, depth := ".", "3"
	jsonOut, noColor := false, false
	for args := c.Args; len(args) > 0; {
		switch a := args[0]; {
		case a == "--depth" || a == "-d":
			if len(args) < 2 {
				x.p.Error(a + " needs a value")
				return 1
			}
			depth, args = args[1], args[2:]
		case a == "--json" || a == "-j":
			jsonOut, args = true, args[1:]
		case a == "--no-color":
			noColor, args = true, args[1:]
		case strings.HasPrefix(a, "-"):
			args = args[1:]
		default:
			basedir, args = a, args[1:]
		}
	}
	x.colors(noColor)
	w, s := c.Stdout, x.s
	put := func(t string) { _, _ = io.WriteString(w, t) }

	basedir = x.resolveDir(basedir)
	files := findCompose(basedir, depth)
	if files == "" {
		if jsonOut {
			put(`{"base_path":` + q(basedir) + `,"compose_files":[],"total_found":0}` + "\n")
		} else {
			put("No docker-compose files found in " + basedir + " (depth: " + depth + ")\n")
		}
		return 0
	}

	if jsonOut {
		var b strings.Builder
		b.WriteString(`{"base_path":` + q(basedir) + `,"depth":` + depth + `,"compose_files":[`)
		total := 0
		for _, file := range hereLines(files) {
			total++
			if total > 1 {
				b.WriteByte(',')
			}
			// parse | cut -d'|' -f1 | tr '\n' ',' | sed 's/,$//'
			var names []string
			if text := x.parseServices(file); text != "" {
				for _, l := range strings.Split(text, "\n") {
					name, _, _ := strings.Cut(l, "|")
					names = append(names, name)
				}
			}
			joined := strings.ReplaceAll(strings.Join(names, ","), ",", `","`)
			if reEchoOpt.MatchString(joined) { // echo -n "$services"
				joined = ""
			}
			b.WriteString(`{"path":` + q(dirname(file)) + `,"file":` + q(basename(file)) + `,"services":["` + joined + `"]}`)
		}
		b.WriteString(`],"total_found":` + strconv.Itoa(total) + "}\n")
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
	total := 0
	for _, file := range hereLines(files) {
		total++
		s.Print(w, "accent", dirname(file))
		var items []string
		for _, line := range hereLines(x.parseServices(file)) {
			if f := readFields(line, 2); f[0] != "" {
				items = append(items, f[0]+": "+f[1])
			}
		}
		if len(items) > 0 {
			s.List(w, items...)
		}
		put("\n")
	}
	put("Found " + bold + strconv.Itoa(total) + off + " compose file(s)\n")
	return 0
}

// maxDepth is the -maxdepth argument GNU find accepts: decimal digits up
// to INT_MAX.
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
// of the compose names, the lines in byte order (LEDGER DOCK-27); "" when
// find refuses the depth or finds nothing. Unreadable directories are
// skipped.
func findCompose(root, depth string) string {
	max, ok := maxDepth(depth)
	if !ok {
		return ""
	}
	st, err := os.Lstat(root)
	if err != nil {
		return ""
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
		return ""
	}
	ls := strings.Split(strings.Join(found, "\n"), "\n")
	sort.Strings(ls)
	return strings.TrimRight(strings.Join(ls, "\n"), "\n")
}
