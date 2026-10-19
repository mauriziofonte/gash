// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
)

// isRegular is [[ -f PATH ]]: a regular file after symlinks.
func isRegular(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular()
}

// isDir is [[ -d PATH ]]: a directory after symlinks.
func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// pathExists is [[ -e PATH ]].
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// fileContains is `grep -q SUB FILE 2>/dev/null` for a SUB with no regex
// metacharacters: a byte-substring match, false when the file is unreadable.
func fileContains(path, sub string) bool {
	data, err := os.ReadFile(path)
	return err == nil && bytes.Contains(data, []byte(sub))
}

// findTestGo is `find PATH -name "*_test.go" -print -quit`: whether any
// entry named *_test.go exists under PATH.
func findTestGo(root string) bool {
	found := false
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasSuffix(d.Name(), "_test.go") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// project is llm_project (project.sh __llm_project_impl): detect the project
// type and emit the info object. Detection reproduces the 2.x precedence and
// its quirks (poetry whenever pyproject.toml exists, case-sensitive flask,
// framework grep anywhere in package.json) — LEDGER LLM-27.
func (m *Module) project(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	path, ok := parsePathArg(c, "Use a project directory or no argument")
	if !ok {
		return 1
	}
	safe, ok := m.validatePath(c, path)
	if !ok {
		return 1
	}
	if !pathExists(safe) {
		return fail(c, "path_not_found", safe, "Provide an existing project directory")
	}
	if !isDir(safe) {
		return fail(c, "not_a_directory", safe, "Provide a project directory")
	}

	ptype, framework, pm, entry := "unknown", "", "", ""
	hasTests := false
	switch {
	case isRegular(safe + "/composer.json"):
		ptype, pm = "php", "composer"
		switch {
		case isRegular(safe + "/artisan"):
			framework, entry = "laravel", "public/index.php"
		case isDir(safe+"/symfony") || fileContains(safe+"/composer.json", `"symfony/`):
			framework, entry = "symfony", "public/index.php"
		}
		if isDir(safe + "/tests") {
			hasTests = true
		}
	case isRegular(safe + "/package.json"):
		ptype, pm = "javascript", "npm"
		if isRegular(safe + "/yarn.lock") {
			pm = "yarn"
		}
		if isRegular(safe + "/pnpm-lock.yaml") {
			pm = "pnpm"
		}
		switch {
		case fileContains(safe+"/package.json", `"next"`):
			framework = "nextjs"
		case fileContains(safe+"/package.json", `"react"`):
			framework = "react"
		case fileContains(safe+"/package.json", `"vue"`):
			framework = "vue"
		case fileContains(safe+"/package.json", `"express"`):
			framework = "express"
		}
		if isDir(safe+"/tests") || isDir(safe+"/__tests__") {
			hasTests = true
		}
	case isRegular(safe+"/requirements.txt") || isRegular(safe+"/pyproject.toml"):
		ptype, pm = "python", "pip"
		if isRegular(safe + "/pyproject.toml") {
			pm = "poetry"
		}
		switch {
		case isRegular(safe + "/manage.py"):
			framework = "django"
		case fileContains(safe+"/requirements.txt", "flask"):
			framework = "flask"
		}
		if isDir(safe + "/tests") {
			hasTests = true
		}
	case isRegular(safe + "/go.mod"):
		ptype, pm = "go", "go"
		if isDir(safe+"/tests") || findTestGo(safe) {
			hasTests = true
		}
	case isRegular(safe + "/Cargo.toml"):
		ptype, pm = "rust", "cargo"
		if isDir(safe + "/tests") {
			hasTests = true
		}
	}

	var b bytes.Buffer
	b.WriteString("{\n  \"type\": \"")
	b.WriteString(ptype)
	b.WriteString("\",\n  \"framework\": \"")
	b.WriteString(framework)
	b.WriteString("\",\n  \"package_manager\": \"")
	b.WriteString(pm)
	b.WriteString("\",\n  \"has_tests\": ")
	if hasTests {
		b.WriteString("true")
	} else {
		b.WriteString("false")
	}
	b.WriteString(",\n  \"entry_point\": \"")
	b.WriteString(entry)
	b.WriteString("\",\n  \"path\": ")
	b.Write(contract.AppendString(nil, safe, contract.EscapeBash))
	b.WriteString("\n}\n")
	_, _ = c.Stdout.Write(b.Bytes())
	return 0
}

// deps is llm_deps (project.sh __llm_deps_impl): the project dependencies as
// {manager, dependencies} or, with --dev, {manager, prod, dev}. The exact
// 2.x jq programs run through the sanctioned runner (jq is a hard dependency
// here: jq_not_found FATAL without it), so the jq output — key sorting,
// array manifests yielding index lists, the raw parse error and exit 5 on a
// malformed manifest (LEDGER LLM-05), the go.mod `//` entry (LEDGER LLM-14)
// — is reproduced byte for byte.
func (m *Module) deps(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	pt := pathTaker{path: "."}
	includeDev := false
	args := c.Args
	for len(args) > 0 {
		a := args[0]
		switch {
		case a == "--dev":
			includeDev = true
			args = args[1:]
		case strings.HasPrefix(a, "-"):
			return fail(c, "unknown_option", a, "Use --dev or a path")
		default:
			if !pt.take(c, a) {
				return 1
			}
			args = args[1:]
		}
	}
	safe, ok := m.validatePath(c, pt.path)
	if !ok {
		return 1
	}
	jq, ok := m.lookPath(c, "jq")
	if !ok {
		return fail(c, "jq_not_found", "jq is required for JSON output", "Install jq: sudo apt install jq")
	}

	shape := "{manager: $m, dependencies: $prod}"
	if includeDev {
		shape = "{manager: $m, prod: $prod, dev: $dev}"
	}
	switch {
	case isRegular(safe + "/composer.json"):
		prog := `(.require // {} | keys) as $prod | (."require-dev" // {} | keys) as $dev | ` + shape
		return m.runJQ(c, jq, nil, "-c", "--arg", "m", "composer", prog, safe+"/composer.json")
	case isRegular(safe + "/package.json"):
		prog := `(.dependencies // {} | keys) as $prod | (.devDependencies // {} | keys) as $dev | ` + shape
		return m.runJQ(c, jq, nil, "-c", "--arg", "m", "npm", prog, safe+"/package.json")
	case isRegular(safe + "/requirements.txt"):
		data, _ := os.ReadFile(safe + "/requirements.txt")
		prog := `[inputs | sub("#.*"; "") | gsub("^\\s+|\\s+$"; "") | select(length > 0 and (startswith("-") | not)) | sub("[ <>=!~;\\[@].*"; "")] as $prod | [] as $dev | ` + shape
		return m.runJQ(c, jq, bytes.NewReader(data), "-cRn", "--arg", "m", "pip", prog)
	case isRegular(safe + "/go.mod"):
		data, _ := os.ReadFile(safe + "/go.mod")
		prog := `[inputs | select(length > 0)] as $prod | [] as $dev | ` + shape
		return m.runJQ(c, jq, bytes.NewReader(goModRequires(data)), "-cRn", "--arg", "m", "go", prog)
	default:
		return fail(c, "no_package_file",
			"No composer.json, package.json, requirements.txt or go.mod found",
			"Not a package-managed project")
	}
}

// runJQ runs jq with its stdout and stderr on the caller's streams (the 2.x
// pipeline left jq's stderr on the function's stderr), returning jq's exit
// status. stdin is nil (/dev/null) when the program reads a file argument.
func (m *Module) runJQ(c *invoke.Call, jq string, stdin io.Reader, args ...string) int {
	res, _ := m.run.Run(c.Ctx, runner.Cmd{Path: jq, Args: args, Options: runner.Options{
		Env: c.Env.Child(), Stdin: stdin, Stdout: c.Stdout, Stderr: c.Stderr,
	}})
	return res.ExitCode
}

var (
	goReqOpen  = regexp.MustCompile(`^require[[:space:]]*\(`)
	goReqClose = regexp.MustCompile(`^\)`)
	goReqLine  = regexp.MustCompile(`^require[[:space:]]+[^(]`)
)

// goModRequires is the awk program of the go.mod branch: inside a
// `require (` block it prints the first field of each line with two or more
// fields; a one-line `require MODULE VERSION` prints the module. It keeps
// the 2.x behavior, a `// comment` line in a block yielding a `//` entry
// (LEDGER LLM-14). Its output is awk-independent, feeding the same jq
// program the shell piped it to.
func goModRequires(data []byte) []byte {
	content := string(data)
	lines := strings.Split(content, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" && strings.HasSuffix(content, "\n") {
		lines = lines[:n-1]
	}
	var out bytes.Buffer
	block := false
	for _, line := range lines {
		if goReqOpen.MatchString(line) {
			block = true
			continue
		}
		if block && goReqClose.MatchString(line) {
			block = false
			continue
		}
		if block {
			if f := awkFields(line); len(f) >= 2 {
				out.WriteString(f[0])
				out.WriteByte('\n')
			}
		}
		if goReqLine.MatchString(line) {
			out.WriteString(field(awkFields(line), 2))
			out.WriteByte('\n')
		}
	}
	return out.Bytes()
}
