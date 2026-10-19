// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ssh

import (
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
	"mvdan.cc/sh/v3/syntax"

	"github.com/mauriziofonte/gash/internal/runner"
)

// agentSock is the name of the one agent socket per user.
const agentSock = "gash-ssh-agent.sock"

// startAgent is __gash_try_start_ssh_agent: the agent on the fixed socket
// when it answers, else a new one (on that socket, else anywhere), whose
// variables are set as 2.x's eval set them.
func (m *Module) startAgent(s *session) {
	if !m.found(s, "ssh-agent") {
		return
	}
	home := s.c.Env.Get("HOME")
	dir := s.c.Env.Get("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = home + "/.ssh"
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() || unix.Faccessat(unix.AT_FDCWD, dir, unix.W_OK, unix.AT_EACCESS) != nil {
		dir = home + "/.ssh"
	}
	// `mkdir -p "$dir" && chmod 700 "$dir"`, also on a directory that was
	// there (SYS-22).
	if os.MkdirAll(dir, 0o777) == nil {
		_ = os.Chmod(dir, 0o700)
	}
	sock := dir + "/" + agentSock

	if st, err := os.Stat(sock); err == nil && st.Mode()&os.ModeSocket != 0 {
		// ssh-add -l: 0 = keys, 1 = no keys (agent alive), 2 = no agent.
		rc := 0
		res, err := m.run.Run(s.ctx(), runner.Cmd{Path: "ssh-add", Args: []string{"-l"},
			Options: runner.Options{Env: s.with("SSH_AUTH_SOCK=" + sock), Stdout: io.Discard, Stderr: io.Discard}})
		switch se := err.(type) {
		case nil:
			rc = res.ExitCode
		case *runner.StartError:
			rc = se.Status
		default:
			rc = res.ExitCode
		}
		if rc != 2 {
			s.set("SSH_AUTH_SOCK", sock)
			return
		}
		_ = os.Remove(sock)
	}

	out := m.agentOutput(s, "-a", sock, "-s")
	if out == "" {
		out = m.agentOutput(s, "-s")
	}
	if out == "" {
		return
	}
	for _, a := range agentAssignments(out) {
		s.set(a[0], a[1])
	}
}

// agentOutput is `$(ssh-agent ARGS 2>/dev/null | sed '/^echo Agent pid /d')`.
func (m *Module) agentOutput(s *session, args ...string) string {
	res, _ := m.run.Run(s.ctx(), runner.Cmd{Path: "ssh-agent", Args: args,
		Options: runner.Options{Env: s.env, Stderr: io.Discard}})
	var kept []string
	for _, l := range strings.SplitAfter(string(res.Stdout), "\n") {
		if !strings.HasPrefix(l, "echo Agent pid ") {
			kept = append(kept, l)
		}
	}
	return runner.CommandSubst([]byte(strings.Join(kept, "")))
}

// agentAssignments are the SSH_AUTH_SOCK=VALUE and SSH_AGENT_PID=DIGITS
// assignments of an agent's output, in order, read with the shell
// grammar where 2.x evaluated the whole output (SYS-39): a statement that
// is not a plain assignment (or export NAME=VALUE) of those names with a
// literal value is ignored, never run.
func agentAssignments(out string) [][2]string {
	var stmts []*syntax.Stmt
	if f, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(out), ""); err == nil {
		stmts = f.Stmts
	} else {
		// A line that does not parse is skipped; the others are read.
		for _, line := range strings.Split(out, "\n") {
			if f, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(line), ""); err == nil {
				stmts = append(stmts, f.Stmts...)
			}
		}
	}
	var got [][2]string
	for _, st := range stmts {
		if st.Background || st.Coprocess || st.Negated || len(st.Redirs) > 0 {
			continue
		}
		var assigns []*syntax.Assign
		switch x := st.Cmd.(type) {
		case *syntax.CallExpr:
			if len(x.Args) == 0 {
				assigns = x.Assigns
			}
		case *syntax.DeclClause:
			if x.Variant.Value == "export" {
				assigns = x.Args
			}
		}
		for _, a := range assigns {
			if a.Append || a.Naked || a.Index != nil || a.Array != nil || a.Name == nil {
				continue
			}
			name := a.Name.Value
			if name != "SSH_AUTH_SOCK" && name != "SSH_AGENT_PID" {
				continue
			}
			v, ok := literal(a.Value)
			if !ok || (name == "SSH_AGENT_PID" && !digits(v)) {
				continue
			}
			got = append(got, [2]string{name, v})
		}
	}
	return got
}

// literal is the value of a word without expansions or escapes.
func literal(w *syntax.Word) (string, bool) {
	if w == nil {
		return "", true
	}
	var b strings.Builder
	for _, part := range w.Parts {
		switch x := part.(type) {
		case *syntax.Lit:
			if strings.ContainsRune(x.Value, '\\') {
				return "", false
			}
			b.WriteString(x.Value)
		case *syntax.SglQuoted:
			if x.Dollar {
				return "", false
			}
			b.WriteString(x.Value)
		case *syntax.DblQuoted:
			if x.Dollar {
				return "", false
			}
			for _, q := range x.Parts {
				lit, ok := q.(*syntax.Lit)
				if !ok || strings.ContainsRune(lit.Value, '\\') {
					return "", false
				}
				b.WriteString(lit.Value)
			}
		default:
			return "", false
		}
	}
	return b.String(), true
}

func digits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
