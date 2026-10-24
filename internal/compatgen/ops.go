// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package compatgen

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
)

// Register binds the compat operations of the generator:
//
//	gash __compat generate --out DIR [--check] [--stage]
//	gash __compat stage --out DIR [--root ROOT] [--bin PATH]
func Register(t *invoke.Table) {
	t.HandleCompat("generate", generateOp)
	t.HandleCompat("stage", stageOp)
}

func usage(c *invoke.Call, err error) int {
	code := "unexpected_argument"
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "flag provided but not defined"):
		code = "unknown_option"
	case strings.HasPrefix(msg, "flag needs an argument"):
		code = "missing_value"
	}
	return c.Fail(contract.New(code, msg, "Run: gash "+c.Name+" --help"))
}

func generateOp(c *invoke.Call) int {
	fl := flag.NewFlagSet("generate", flag.ContinueOnError)
	fl.SetOutput(io.Discard)
	out := fl.String("out", "", "directory to write into (or to compare with --check)")
	check := fl.Bool("check", false, "compare instead of writing; status 1 on any difference")
	stage := fl.Bool("stage", false, "include the module shims of a staged tree")
	if err := fl.Parse(c.Args); err != nil {
		return usage(c, err)
	}
	if *out == "" || fl.NArg() > 0 {
		return usage(c, errors.New("usage: gash __compat generate --out DIR [--check] [--stage]"))
	}
	files, err := Generate(*stage)
	if err != nil {
		return c.Fail(contract.New("internal_error", err.Error(), ""))
	}
	if *check {
		diff := Check(files, *out)
		for _, d := range diff {
			_, _ = fmt.Fprintln(c.Stdout, d)
		}
		if len(diff) > 0 {
			return 1
		}
		return 0
	}
	if err := Write(files, *out); err != nil {
		return c.Fail(contract.New("internal_error", err.Error(), ""))
	}
	return 0
}

// Write writes files under dir.
func Write(files []File, dir string) error {
	for _, f := range files {
		p := filepath.Join(dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, f.Data, fs.FileMode(f.Mode)); err != nil {
			return err
		}
		if err := os.Chmod(p, fs.FileMode(f.Mode)); err != nil {
			return err
		}
	}
	return nil
}

// Check returns the paths under dir that differ from files ("missing P",
// "differs P").
func Check(files []File, dir string) []string {
	var out []string
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f.Path)))
		switch {
		case err != nil:
			out = append(out, "missing "+f.Path)
		case !bytes.Equal(b, f.Data):
			out = append(out, "differs "+f.Path)
		}
	}
	return out
}

func stageOp(c *invoke.Call) int {
	fl := flag.NewFlagSet("stage", flag.ContinueOnError)
	fl.SetOutput(io.Discard)
	out := fl.String("out", "", "directory of the staged tree (created, must be empty)")
	root := fl.String("root", "", "the checkout to stage (default: found from the current directory)")
	bin := fl.String("bin", "", "the binary to place (default: this one)")
	if err := fl.Parse(c.Args); err != nil {
		return usage(c, err)
	}
	if *out == "" || fl.NArg() > 0 {
		return usage(c, errors.New("usage: gash __compat stage --out DIR [--root ROOT] [--bin PATH]"))
	}
	if *root == "" {
		wd, err := os.Getwd()
		if err == nil {
			*root, err = FindRoot(wd)
		}
		if err != nil {
			return c.Fail(contract.New("path_not_found", "no Gash checkout above the current directory", "Pass --root ROOT"))
		}
	}
	if *bin == "" {
		exe, err := os.Executable()
		if err != nil {
			return c.Fail(contract.New("internal_error", "cannot locate this binary: "+err.Error(), "Pass --bin PATH"))
		}
		*bin = exe
	}
	binPath, err := Stage(*root, *out, *bin)
	if err != nil {
		return c.Fail(contract.New("internal_error", err.Error(), ""))
	}
	_, _ = fmt.Fprintln(c.Stdout, "GASH_BIN="+binPath)
	return 0
}
