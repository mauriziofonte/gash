// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docgen

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// linkRe is an inline Markdown link or image: [text](target "title").
var linkRe = regexp.MustCompile(`!?\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)

// htmlAnchorRe is an explicit anchor: <a id="x"> or <a name="x">.
var htmlAnchorRe = regexp.MustCompile(`<a\s+(?:id|name)="([^"]+)"`)

// markdownFiles are the pages the links test reads: the root pages, docs/
// and dev/ (but the port archive, kept as written then).
func markdownFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			switch rel {
			case ".git", "node_modules", "testdata", filepath.Join("dev", "port"):
				return filepath.SkipDir
			}
			if strings.HasPrefix(d.Name(), ".") && rel != "." && rel != ".github" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".md") {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// pageText is a page without its fenced code blocks and inline code,
// where a link is an example rather than a link.
func pageText(src string) string {
	var b strings.Builder
	fence := ""
	for _, line := range strings.Split(src, "\n") {
		trim := strings.TrimLeft(line, " ")
		if fence == "" && (strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "~~~")) {
			fence = trim[:3]
			b.WriteString("\n")
			continue
		}
		if fence != "" {
			if strings.HasPrefix(trim, fence) {
				fence = ""
			}
			b.WriteString("\n")
			continue
		}
		b.WriteString(stripInlineCode(line) + "\n")
	}
	return b.String()
}

func stripInlineCode(line string) string {
	var b strings.Builder
	for {
		i := strings.Index(line, "`")
		if i < 0 {
			b.WriteString(line)
			return b.String()
		}
		n := len(line[i:]) - len(strings.TrimLeft(line[i:], "`"))
		fence := line[i : i+n]
		j := strings.Index(line[i+n:], fence)
		if j < 0 {
			b.WriteString(line)
			return b.String()
		}
		b.WriteString(line[:i])
		line = line[i+n+j+n:]
	}
}

// anchors are the heading anchors of a page as GitHub makes them
// (duplicates get -1, -2, ...), plus its explicit HTML anchors.
func anchors(src string) map[string]bool {
	out := map[string]bool{}
	seen := map[string]int{}
	fence := ""
	for _, line := range strings.Split(src, "\n") {
		trim := strings.TrimLeft(line, " ")
		if fence == "" && (strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "~~~")) {
			fence = trim[:3]
			continue
		}
		if fence != "" {
			if strings.HasPrefix(trim, fence) {
				fence = ""
			}
			continue
		}
		for _, m := range htmlAnchorRe.FindAllStringSubmatch(line, -1) {
			out[m[1]] = true
		}
		if !strings.HasPrefix(line, "#") {
			continue
		}
		text := strings.TrimSpace(strings.TrimLeft(line, "#"))
		// Inline markup does not reach the anchor.
		text = strings.NewReplacer("`", "", "*", "").Replace(text)
		text = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`).ReplaceAllString(text, "$1")
		a := anchor(text)
		if n := seen[a]; n > 0 {
			out[a+"-"+strconv.Itoa(n)] = true
		} else {
			out[a] = true
		}
		seen[a]++
	}
	return out
}

// TestLinks: every relative link between the pages points to a file that
// exists, and every #anchor to a heading or anchor of its page.
func TestLinks(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cache := map[string]map[string]bool{}
	anchorsOf := func(p string) map[string]bool {
		if a, ok := cache[p]; ok {
			return a
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		cache[p] = anchors(string(src))
		return cache[p]
	}
	for _, page := range markdownFiles(t, root) {
		src, err := os.ReadFile(page)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, page)
		for _, m := range linkRe.FindAllStringSubmatch(pageText(string(src)), -1) {
			target := m[1]
			if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			file, frag, _ := strings.Cut(target, "#")
			dest := page
			if file != "" {
				if strings.HasPrefix(file, "/") {
					dest = filepath.Join(root, file)
				} else {
					dest = filepath.Join(filepath.Dir(page), file)
				}
				if _, err := os.Stat(dest); err != nil {
					t.Errorf("%s: broken link %s (no %s)", rel, target, file)
					continue
				}
			}
			if frag == "" || !strings.HasSuffix(dest, ".md") {
				continue
			}
			if a := anchorsOf(dest); !a[frag] {
				t.Errorf("%s: broken anchor %s", rel, target)
			}
		}
	}
}
