// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runner_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestOnlyRunnerStartsProcesses enforces the invariant of design/layout.md
// ("runner/ the only place that starts processes") across the binary's
// internal packages: no non-test .go file under internal/ other than
// internal/runner may import os/exec or call os.StartProcess, syscall.Exec
// or syscall.ForkExec. The two carve-outs the design names
// (internal/llmexec/probe.go, internal/selfmgmt/proc.go) are allowed here;
// in the current tree both delegate to internal/runner, so the invariant is
// clean, but the guard keeps the sanctioned exceptions explicit.
func TestOnlyRunnerStartsProcesses(t *testing.T) {
	internalDir, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	allowed := func(rel string) bool {
		rel = filepath.ToSlash(rel)
		return rel == "runner" || strings.HasPrefix(rel, "runner/") ||
			rel == "llmexec/probe.go" || rel == "selfmgmt/proc.go"
	}
	// forbidden selector calls, as "pkg.Func".
	forbidden := map[string]bool{
		"os.StartProcess":  true,
		"syscall.Exec":     true,
		"syscall.ForkExec": true,
	}
	fset := token.NewFileSet()
	var offenders []string
	err = filepath.WalkDir(internalDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(internalDir, p)
		if err != nil {
			return err
		}
		if allowed(rel) {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			if imp.Path.Value == `"os/exec"` {
				offenders = append(offenders, filepath.ToSlash(rel)+": imports os/exec")
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if forbidden[pkg.Name+"."+sel.Sel.Name] {
				offenders = append(offenders, filepath.ToSlash(rel)+": calls "+pkg.Name+"."+sel.Sel.Name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Fatalf("only internal/runner (plus llmexec/probe.go, selfmgmt/proc.go) may start processes; offenders:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}
