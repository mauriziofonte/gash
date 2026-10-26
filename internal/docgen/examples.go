// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docgen

import (
	"regexp"
	"strings"
)

// An Example is a verified example of a guide: a ```console block right
// after an HTML comment that starts with "<!-- example". The comment's
// other lines are a setup script, run hidden first in an empty directory
// (the sandbox HOME); then each "$ " line of the block runs with Gash
// loaded, and the lines up to the next "$ " are its output (stdout and
// stderr). go test ./internal/docgen -update writes the real output back.
// A comment line "mask: REGEX" is not setup: text that matches REGEX (a
// time, a duration) may differ between the page and the run.
//
//	<!-- example
//	mkdir -p src && printf 'x\n' >src/a.txt
//	mask: Scan time: +[0-9]+ms
//	-->
//	```console
//	$ tree_stats src
//	...
//	```
type Example struct {
	// Line is the 1-based line of the ```console fence.
	Line  int
	Setup string
	Masks []*regexp.Regexp
	Steps []Step
	// start and end are the line indexes of the block body (fences
	// excluded).
	start, end int
}

// A Step is one command of an example and the output the page shows.
type Step struct {
	Command string
	Output  string
}

// Examples finds the verified examples of a page.
func Examples(src string) []Example {
	lines := strings.Split(src, "\n")
	var out []Example
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "<!-- example") {
			continue
		}
		// The comment: its first line may end it ("<!-- example -->").
		var setup []string
		var masks []*regexp.Regexp
		j := i
		if !strings.HasSuffix(strings.TrimSpace(lines[i]), "-->") {
			for j = i + 1; j < len(lines) && strings.TrimSpace(lines[j]) != "-->"; j++ {
				if m, ok := strings.CutPrefix(lines[j], "mask: "); ok {
					if re, err := regexp.Compile(m); err == nil {
						masks = append(masks, re)
						continue
					}
				}
				setup = append(setup, lines[j])
			}
		}
		if j+1 >= len(lines) || strings.TrimSpace(lines[j+1]) != "```console" {
			continue
		}
		ex := Example{Line: j + 2, Setup: strings.Join(setup, "\n"), Masks: masks, start: j + 2}
		k := j + 2
		for ; k < len(lines) && strings.TrimSpace(lines[k]) != "```"; k++ {
			l := lines[k]
			if strings.HasPrefix(l, "$ ") {
				ex.Steps = append(ex.Steps, Step{Command: strings.TrimPrefix(l, "$ ")})
				continue
			}
			if n := len(ex.Steps); n > 0 {
				if strings.HasSuffix(ex.Steps[n-1].Command, `\`) && ex.Steps[n-1].Output == "" {
					// A continued command line.
					ex.Steps[n-1].Command += "\n" + l
					continue
				}
				ex.Steps[n-1].Output += l + "\n"
			}
		}
		ex.end = k
		out = append(out, ex)
		i = k
	}
	return out
}

// Same reports whether a run's output matches the page's, the masked
// parts aside.
func (ex Example) Same(page, run string) bool {
	norm := func(s string) string {
		s = strings.TrimRight(s, "\n")
		for _, re := range ex.Masks {
			s = re.ReplaceAllString(s, "\x00")
		}
		return s
	}
	return norm(page) == norm(run)
}

// WithOutputs returns the page with the outputs of its examples replaced:
// outputs[n][s] is the output of step s of example n.
func WithOutputs(src string, outputs [][]string) string {
	lines := strings.Split(src, "\n")
	exs := Examples(src)
	// From the last example back, so earlier line numbers stay valid.
	for n := len(exs) - 1; n >= 0; n-- {
		ex := exs[n]
		var body []string
		for s, st := range ex.Steps {
			body = append(body, "$ "+st.Command)
			if out := strings.TrimRight(outputs[n][s], "\n"); out != "" {
				body = append(body, strings.Split(out, "\n")...)
			}
		}
		lines = append(lines[:ex.start], append(body, lines[ex.end:]...)...)
	}
	return strings.Join(lines, "\n")
}
