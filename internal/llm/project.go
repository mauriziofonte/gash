// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/registry"
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

// manifestDep reports whether the JSON manifest at path (one JSON object)
// has a key under its prodKey or devKey object that match accepts. An
// unreadable or malformed manifest has none.
func (m *Module) manifestDep(path, prodKey, devKey string, match func(string) bool) bool {
	data, err := m.readManifest(path)
	if err != nil {
		return false
	}
	top, err := jsonObject(data)
	if err != nil {
		return false
	}
	for _, key := range []string{prodKey, devKey} {
		obj, err := jsonObject(top[key])
		if err != nil {
			continue
		}
		for k := range obj {
			if match(k) {
				return true
			}
		}
	}
	return false
}

// poetryProject reports whether pyproject.toml declares a [tool.poetry]
// table (or one of its subtables), the mark of a Poetry project.
func (m *Module) poetryProject(path string) bool {
	data, err := m.readManifest(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") {
			continue
		}
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		h := strings.ReplaceAll(strings.Trim(line, "[]"), " ", "")
		h = strings.ReplaceAll(h, "\t", "")
		if h == "tool.poetry" || strings.HasPrefix(h, "tool.poetry.") {
			return true
		}
	}
	return false
}

// requirement reports whether requirements.txt names the distribution
// name (compared as PEP 503 normalizes names: case, and runs of -_. alike).
func (m *Module) requirement(path, name string) bool {
	data, err := m.readManifest(path)
	if err != nil {
		return false
	}
	norm := func(s string) string {
		s = strings.ToLower(s)
		return strings.NewReplacer("_", "-", ".", "-").Replace(s)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "-") {
			continue
		}
		end := strings.IndexFunc(line, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.')
		})
		if end >= 0 {
			line = line[:end]
		}
		if norm(line) == norm(name) {
			return true
		}
	}
	return false
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
// type and emit the info object, with the 2.x precedence. 3.0 (LEDGER
// LLM-27): the manifests are parsed, not grepped: poetry only with a
// [tool.poetry] table in pyproject.toml, flask as a requirement name in any
// case, and the JS and Symfony frameworks only as keys of the dependency
// objects.
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
		case isDir(safe+"/symfony") || m.manifestDep(safe+"/composer.json", "require", "require-dev", func(k string) bool { return strings.HasPrefix(k, "symfony/") }):
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
		for _, fw := range [][2]string{{"next", "nextjs"}, {"react", "react"}, {"vue", "vue"}, {"express", "express"}} {
			if m.manifestDep(safe+"/package.json", "dependencies", "devDependencies", func(k string) bool { return k == fw[0] }) {
				framework = fw[1]
				break
			}
		}
		if isDir(safe+"/tests") || isDir(safe+"/__tests__") {
			hasTests = true
		}
	case isRegular(safe+"/requirements.txt") || isRegular(safe+"/pyproject.toml"):
		ptype, pm = "python", "pip"
		if m.poetryProject(safe + "/pyproject.toml") {
			pm = "poetry"
		}
		switch {
		case isRegular(safe + "/manage.py"):
			framework = "django"
		case m.requirement(safe+"/requirements.txt", "flask"):
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
	b.Write(contract.AppendString(nil, safe, contract.EscapeJQ))
	b.WriteString("\n}\n")
	_, _ = c.Stdout.Write(b.Bytes())
	return 0
}

// deps is llm_deps (project.sh __llm_deps_impl): the project dependencies as
// {manager, dependencies} or, with --dev, {manager, prod, dev}.
// composer.json and package.json are read natively (3.0: LEDGER LLM-05 and
// LLM-14 fixed): a manifest that is not one JSON object, empty included, is
// invalid_manifest (2.x: jq's raw parse error and exit 5, or nothing and
// exit 0), and a dependency map that is not an object counts as empty
// (2.x: jq's keys gave an array's indices). requirements.txt and go.mod
// are read natively too, as the 2.x jq programs read them (3.4: no jq is
// needed; 2.x and 3.x before failed with jq_not_found, FATAL, for every
// manifest); a go.mod comment line is no dependency (LLM-14).
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
			return fail(c, "unknown_option", a, registry.OptionsHint(c.Name))
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
	switch {
	case isRegular(safe + "/composer.json"):
		return m.jsonDeps(c, safe+"/composer.json", "composer", "require", "require-dev", includeDev)
	case isRegular(safe + "/package.json"):
		return m.jsonDeps(c, safe+"/package.json", "npm", "dependencies", "devDependencies", includeDev)
	case isRegular(safe + "/requirements.txt"):
		// An unreadable file lists nothing, as jq read nothing from it.
		data, _ := os.ReadFile(safe + "/requirements.txt")
		_, _ = c.Stdout.Write(depsJSON("pip", pipRequires(data), includeDev))
		return 0
	case isRegular(safe + "/go.mod"):
		data, _ := os.ReadFile(safe + "/go.mod")
		var mods []string
		for _, l := range rawLines(goModRequires(data)) {
			if l != "" {
				mods = append(mods, l)
			}
		}
		_, _ = c.Stdout.Write(depsJSON("go", mods, includeDev))
		return 0
	default:
		return fail(c, "no_package_file",
			"No composer.json, package.json, requirements.txt or go.mod found",
			"Not a package-managed project")
	}
}

// jsonDeps prints the dependency names of a JSON manifest: the keys of
// its prodKey and devKey objects, sorted bytewise as jq's keys sorts them,
// encoded as jq -c writes them.
func (m *Module) jsonDeps(c *invoke.Call, file, manager, prodKey, devKey string, includeDev bool) int {
	data, err := m.readManifest(file)
	var top map[string]json.RawMessage
	if err == nil {
		top, err = jsonObject(data)
	}
	if err != nil {
		return fail(c, "invalid_manifest", file, "The manifest is not a JSON object; fix it or use llm_config to read it")
	}
	b := []byte(`{"manager":`)
	b = contract.AppendString(b, manager, contract.EscapeJQ)
	if includeDev {
		b = append(b, `,"prod":`...)
		b = appendKeys(b, top[prodKey])
		b = append(b, `,"dev":`...)
		b = appendKeys(b, top[devKey])
	} else {
		b = append(b, `,"dependencies":`...)
		b = appendKeys(b, top[prodKey])
	}
	b = append(b, "}\n"...)
	_, _ = c.Stdout.Write(b)
	return 0
}

func (m *Module) readManifest(file string) ([]byte, error) {
	if m.readFile != nil {
		return m.readFile(file)
	}
	return os.ReadFile(file)
}

// jsonObject decodes data as exactly one JSON object; anything else (an
// empty file, another value, a second value after it) is an error.
func jsonObject(data []byte) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 || raw[0] != '{' {
		return nil, errNotObject
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errNotObject
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

var errNotObject = errors.New("not one JSON object")

// appendKeys appends the sorted keys of v when it is an object, [] for
// anything else (missing, null, an array, a string, ...).
func appendKeys(b []byte, v json.RawMessage) []byte {
	var keys []string
	if obj, err := jsonObject(v); err == nil {
		for k := range obj {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	b = append(b, '[')
	for i, k := range keys {
		if i > 0 {
			b = append(b, ',')
		}
		b = contract.AppendString(b, k, contract.EscapeJQ)
	}
	return append(b, ']')
}

// depsJSON is the {manager, dependencies} line of llm_deps (with dev,
// {manager, prod, dev} and no dev dependencies), as jq -c wrote it.
func depsJSON(manager string, prod []string, dev bool) []byte {
	b := []byte(`{"manager":`)
	b = contract.AppendString(b, manager, contract.EscapeJQ)
	if dev {
		b = append(b, `,"prod":`...)
	} else {
		b = append(b, `,"dependencies":`...)
	}
	b = append(b, '[')
	for i, p := range prod {
		if i > 0 {
			b = append(b, ',')
		}
		b = contract.AppendString(b, p, contract.EscapeJQ)
	}
	b = append(b, ']')
	if dev {
		b = append(b, `,"dev":[]`...)
	}
	return append(b, "}\n"...)
}

// rawLines are the inputs of jq -R: the text split at each newline, a
// last line without one included, no empty input after a final newline.
func rawLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// pipRequires is the 2.x jq program of requirements.txt, natively: each
// line without its # comment and the blanks around it (Unicode white
// space, as jq's \s), empty lines and options (-r, -e, --index-url) left
// out, cut at the first of ` <>=!~;[@`.
func pipRequires(data []byte) []string {
	var out []string
	for _, l := range rawLines(data) {
		if i := strings.IndexByte(l, '#'); i >= 0 {
			l = l[:i]
		}
		l = strings.TrimFunc(l, unicode.IsSpace)
		if l == "" || strings.HasPrefix(l, "-") {
			continue
		}
		if i := strings.IndexAny(l, " <>=!~;[@"); i >= 0 {
			l = l[:i]
		}
		out = append(out, l)
	}
	return out
}

var (
	goReqOpen  = lazyre.MustCompile(`^require[[:space:]]*\(`)
	goReqClose = lazyre.MustCompile(`^\)`)
	goReqLine  = lazyre.MustCompile(`^require[[:space:]]+[^(]`)
)

// goModRequires is the awk program of the go.mod branch: inside a
// `require (` block it prints the first field of each line with two or more
// fields, except a `// comment` line (2.x listed its `//`, LEDGER LLM-14);
// a one-line `require MODULE VERSION` prints the module. Its output feeds
// the jq program the shell piped it to.
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
			// A comment line is no dependency (3.0: LEDGER LLM-14 fixed;
			// 2.x listed its "//").
			if f := awkFields(line); len(f) >= 2 && !strings.HasPrefix(f[0], "//") {
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
