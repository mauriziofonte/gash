// Gash 3.5. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package cli

import (
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mauriziofonte/gash/internal/complete"
	"github.com/mauriziofonte/gash/internal/runner"
)

// completeTree answers `gash __complete WORDS... PARTIAL` from the command
// tree (internal/complete). It returns false, with p.Args rewritten,
// when the words belong to a 3.x command whose cobra tree completes them.
func (p *Process) completeTree() bool {
	if len(p.Args) < 2 || (p.Args[0] != cobra.ShellCompRequestCmd && p.Args[0] != cobra.ShellCompNoDescRequestCmd) {
		return false
	}
	words, partial := p.Args[1:len(p.Args)-1], p.Args[len(p.Args)-1]
	dir, _ := os.Getwd()
	r := complete.Words(words, partial, complete.Env{
		Ctx: p.Ctx, Get: p.Env.Get, Delegate: p.delegateWords,
		Run: runner.Exec{}, Environ: p.Env.Child(), Dir: dir,
	})
	if r.Relay != nil {
		p.Args = append([]string{p.Args[0]}, r.Relay...)
		return false
	}
	complete.Write(p.Stdout, r, p.Args[0] == cobra.ShellCompRequestCmd)
	return true
}

// delegateWords are the completions of the 3.x command name for args, as
// its cobra tree gives them.
func (p *Process) delegateWords(name string, args []string) []complete.Candidate {
	var out bytes.Buffer
	q := *p
	q.Args = append([]string{cobra.ShellCompRequestCmd, name}, args...)
	q.Stdout, q.Stderr = &out, io.Discard
	status := 0
	root := newRoot(&q, &status)
	root.SetArgs(q.Args)
	root.SetOut(&out)
	if err := root.ExecuteContext(q.Ctx); err != nil {
		return nil
	}
	comps, _ := parseCompletion(out.String())
	var list []complete.Candidate
	for _, c := range comps {
		value, desc, _ := strings.Cut(c, "\t")
		if strings.HasPrefix(value, "-") || value == "help" || value == "completion" {
			continue
		}
		list = append(list, complete.Candidate{Value: value, Desc: desc})
	}
	return list
}
