// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// requireJQ skips when jq is not installed.
func requireJQ(t *testing.T) {
	t.Helper()
	if _, err := lookPathTest("jq"); err != nil {
		t.Skip("jq is not installed")
	}
}

func TestProjectMatches2x(t *testing.T) {
	m := New()

	// php / laravel
	php := physTemp(t)
	mustWrite(t, filepath.Join(php, "composer.json"), `{"require":{"monolog/monolog":"^3.0","symfony/console":"^6.0"},"require-dev":{"phpunit/phpunit":"^10.0"}}`+"\n")
	mustWrite(t, filepath.Join(php, "artisan"), "#!/usr/bin/env php\n")
	mustMkdir(t, filepath.Join(php, "tests"))

	// php / symfony (by composer.json grep)
	sym := physTemp(t)
	mustWrite(t, filepath.Join(sym, "composer.json"), `{"require":{"symfony/framework-bundle":"^6.0"}}`+"\n")

	// javascript / next, pnpm
	js := physTemp(t)
	mustWrite(t, filepath.Join(js, "package.json"), `{"dependencies":{"next":"14.0.0","react":"18.0.0"},"devDependencies":{"jest":"29.0.0"}}`+"\n")
	mustWrite(t, filepath.Join(js, "pnpm-lock.yaml"), "lockfileVersion: 6.0\n")
	mustMkdir(t, filepath.Join(js, "__tests__"))

	// javascript / express, yarn
	expr := physTemp(t)
	mustWrite(t, filepath.Join(expr, "package.json"), `{"dependencies":{"express":"4.0.0"}}`+"\n")
	mustWrite(t, filepath.Join(expr, "yarn.lock"), "# yarn\n")

	// python / django; 3.0: LLM-27 fixed, a pyproject.toml without
	// [tool.poetry] is pip (2.x said poetry for any pyproject.toml)
	py := physTemp(t)
	mustWrite(t, filepath.Join(py, "requirements.txt"), "flask==3.0.0\n")
	mustWrite(t, filepath.Join(py, "pyproject.toml"), "[build-system]\nrequires = [\"setuptools\"]\n")
	mustWrite(t, filepath.Join(py, "manage.py"), "# django\n")

	// python / flask (requirements only, pip)
	flask := physTemp(t)
	mustWrite(t, filepath.Join(flask, "requirements.txt"), "Flask-login\nflask==3.0\n")

	// go with a _test.go
	gop := physTemp(t)
	mustWrite(t, filepath.Join(gop, "go.mod"), "module example.com/x\n\ngo 1.22\n")
	mustWrite(t, filepath.Join(gop, "pkg", "unit_test.go"), "package pkg\n")

	// rust
	rust := physTemp(t)
	mustWrite(t, filepath.Join(rust, "Cargo.toml"), "[package]\nname = \"x\"\n")
	mustMkdir(t, filepath.Join(rust, "tests"))

	// unknown
	unknown := physTemp(t)
	mustWrite(t, filepath.Join(unknown, "readme.txt"), "nothing\n")

	for _, tc := range []struct {
		name string
		cwd  string
		args []string
	}{
		{"php-laravel", php, nil},
		{"php-symfony", sym, nil},
		{"js-next-pnpm", js, nil},
		{"js-express-yarn", expr, nil},
		{"py-django-poetry", py, nil},
		{"py-flask-pip", flask, nil},
		{"go", gop, nil},
		{"rust", rust, nil},
		{"unknown", unknown, nil},
		{"path-arg", physTemp(t), []string{php}},
		{"not-found", physTemp(t), []string{"does-not-exist"}},
		{"help", php, []string{"--help"}},
		{"help-h", php, []string{"-h"}},
		{"unknown-option", php, []string{"--bogus"}},
		{"second-path", php, []string{".", "."}},
		{"traversal", php, []string{"../etc"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := physTemp(t)
			same(t, m, tc.cwd, baseEnv(home, nil), "llm_project", tc.args...)
		})
	}

	// not_a_directory: a regular file as PATH
	t.Run("not-a-directory", func(t *testing.T) {
		d := physTemp(t)
		mustWrite(t, filepath.Join(d, "f.txt"), "x\n")
		home := physTemp(t)
		same(t, m, d, baseEnv(home, nil), "llm_project", "f.txt")
	})
}

func TestDepsMatches2x(t *testing.T) {
	requireJQ(t)
	m := New()

	composer := physTemp(t)
	mustWrite(t, filepath.Join(composer, "composer.json"), `{"require":{"symfony/console":"^6.0","monolog/monolog":"^3.0"},"require-dev":{"phpunit/phpunit":"^10.0"}}`+"\n")

	node := physTemp(t)
	mustWrite(t, filepath.Join(node, "package.json"), `{"dependencies":{"react":"18","axios":"1.0"},"devDependencies":{"jest":"29","eslint":"8"}}`+"\n")

	reqs := physTemp(t)
	mustWrite(t, filepath.Join(reqs, "requirements.txt"),
		"# a comment\nFlask==3.0.0\n  requests>=2.0  \ndjango~=4.0\n-e .\nnumpy ; python_version>'3.6'\npkg[extra]==1.0\n\n")

	gomod := physTemp(t)
	mustWrite(t, filepath.Join(gomod, "go.mod"),
		"module example.com/x\n\ngo 1.22\n\nrequire (\n\tgithub.com/pkg/errors v0.9.1\n\t// pinned for CVE\n\tgolang.org/x/sys v0.1.0 // indirect\n)\n\nrequire gopkg.in/yaml.v3 v3.0.1\n")

	none := physTemp(t)
	mustWrite(t, filepath.Join(none, "readme.txt"), "x\n")

	for _, tc := range []struct {
		name string
		cwd  string
		args []string
	}{
		{"composer", composer, nil},
		{"composer-dev", composer, []string{"--dev"}},
		{"node", node, nil},
		{"node-dev", node, []string{"--dev"}},
		{"pip", reqs, nil},
		{"pip-dev", reqs, []string{"--dev"}},
		{"go", gomod, nil},
		{"go-dev", gomod, []string{"--dev"}},
		{"path-arg", physTemp(t), []string{composer}},
		{"no-package-file", none, nil},
		{"help", composer, []string{"--help"}},
		{"unknown-option", composer, []string{"--bogus"}},
		{"second-path", composer, []string{".", "."}},
		{"traversal", composer, []string{"../etc"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := physTemp(t)
			same(t, m, tc.cwd, baseEnv(home, nil), "llm_deps", tc.args...)
		})
	}
}

// TestLedger_LLM05: a JSON manifest that is not one JSON object (broken,
// an array, empty, two values) is invalid_manifest (RETRY), one JSON line
// on stderr, exit 1; 2.x printed jq's raw parse error with exit 5, or
// nothing with exit 0 for an empty file.
func TestLedger_LLM05(t *testing.T) {
	m := New()
	for _, body := range []string{`{"dependencies": {`, `["a"]`, "", "  \n", `{"a":1} {"b":2}`, "null", `"x"`} {
		for _, name := range []string{"package.json", "composer.json"} {
			d := physTemp(t)
			mustWrite(t, filepath.Join(d, name), body)
			r := runGo(t, m, d, baseEnv(physTemp(t), nil), "llm_deps")
			require.Equal(t, result{"", `{"error":"invalid_manifest","details":"` + filepath.Join(d, name) +
				`","action":"RETRY","recoverable":true,"hint":"The manifest is not a JSON object; fix it or use llm_config to read it"}` + "\n", 1}, r, "%s %q", name, body)
		}
	}
}

// TestLedger_LLM14: a go.mod comment line is no dependency, and a
// dependency map that is not an object counts as empty (2.x: "//" and
// the array's indices).
func TestLedger_LLM14(t *testing.T) {
	m := New()
	env := baseEnv(physTemp(t), nil)
	gomod := physTemp(t)
	mustWrite(t, filepath.Join(gomod, "go.mod"),
		"module example.com/x\n\ngo 1.22\n\nrequire (\n\tgithub.com/pkg/errors v0.9.1\n\t// pinned for CVE\n\t//nospace x\n\tgolang.org/x/sys v0.1.0 // indirect\n)\n\nrequire gopkg.in/yaml.v3 v3.0.1\n")
	require.Equal(t, result{`{"manager":"go","dependencies":["github.com/pkg/errors","golang.org/x/sys","gopkg.in/yaml.v3"]}` + "\n", "", 0},
		runGo(t, m, gomod, env, "llm_deps"))

	node := physTemp(t)
	mustWrite(t, filepath.Join(node, "package.json"), `{"dependencies":["react","vue"],"devDependencies":{"jest":"29","eslint":"8","é\u0001":"1"}}`)
	require.Equal(t, result{`{"manager":"npm","dependencies":[]}` + "\n", "", 0}, runGo(t, m, node, env, "llm_deps"))
	require.Equal(t, result{`{"manager":"npm","prod":[],"dev":["eslint","jest","é\u0001"]}` + "\n", "", 0}, runGo(t, m, node, env, "llm_deps", "--dev"))

	composer := physTemp(t)
	mustWrite(t, filepath.Join(composer, "composer.json"), `{"require":{"symfony/console":"^6.0","monolog/monolog":"^3.0","php":">=8"},"require-dev":"x"}`)
	require.Equal(t, result{`{"manager":"composer","prod":["monolog/monolog","php","symfony/console"],"dev":[]}` + "\n", "", 0},
		runGo(t, m, composer, env, "llm_deps", "--dev"))
	none := physTemp(t)
	mustWrite(t, filepath.Join(none, "package.json"), `{"name":"x"}`)
	require.Equal(t, result{`{"manager":"npm","dependencies":[]}` + "\n", "", 0}, runGo(t, m, none, env, "llm_deps"))
}

// TestDepsWithoutJQ: requirements.txt and go.mod are read without jq,
// line for line as the 2.x jq programs read them (2.x and 3.x before:
// jq_not_found, FATAL, for every manifest without jq). With jq installed
// the 2.x programs are run on the same files for comparison.
func TestDepsWithoutJQ(t *testing.T) {
	m := New()
	m.lookPathFn = func(string) (string, bool) { return "", false }
	env := baseEnv(physTemp(t), map[string]string{"PATH": "/nonexistent"})
	reqs := "# a comment\nFlask==3.0.0\n  requests>=2.0  \ndjango~=4.0\n-e .\nnumpy ; python_version>'3.6'\npkg[extra]==1.0\n\n" +
		"\u00a0nbsp\u00a0\nx\t>=1\nweird\xffname\n--index-url x\nlast-line-no-nl"
	gomod := "module example.com/x\n\ngo 1.22\n\nrequire (\n\tgithub.com/pkg/errors v0.9.1\n\t// pinned\n\tgolang.org/x/sys v0.1.0 // indirect\n)\n\nrequire gopkg.in/yaml.v3 v3.0.1\n"
	shape := map[bool]string{false: "{manager: $m, dependencies: $prod}", true: "{manager: $m, prod: $prod, dev: $dev}"}
	for _, tc := range []struct {
		file, body, manager, prog, want string
	}{
		{"requirements.txt", reqs, "pip",
			`[inputs | sub("#.*"; "") | gsub("^\\s+|\\s+$"; "") | select(length > 0 and (startswith("-") | not)) | sub("[ <>=!~;\\[@].*"; "")] as $prod | [] as $dev | `,
			`["Flask","requests","django","numpy","pkg","nbsp","x\t","weird�name","last-line-no-nl"]`},
		{"go.mod", gomod, "go", `[inputs | select(length > 0)] as $prod | [] as $dev | `,
			`["github.com/pkg/errors","golang.org/x/sys","gopkg.in/yaml.v3"]`},
	} {
		d := physTemp(t)
		mustWrite(t, filepath.Join(d, tc.file), tc.body)
		for _, dev := range []bool{false, true} {
			args := []string{}
			want := `{"manager":"` + tc.manager + `","dependencies":` + tc.want + "}\n"
			if dev {
				args = []string{"--dev"}
				want = `{"manager":"` + tc.manager + `","prod":` + tc.want + `,"dev":[]}` + "\n"
			}
			require.Equal(t, result{want, "", 0}, runGo(t, m, d, env, "llm_deps", args...), tc.file)
			jq, err := lookPathTest("jq")
			if err != nil {
				continue
			}
			in := []byte(tc.body)
			if tc.file == "go.mod" {
				in = goModRequires(in)
			}
			cmd := exec.Command(jq, "-cRn", "--arg", "m", tc.manager, tc.prog+shape[dev])
			cmd.Stdin = strings.NewReader(string(in))
			out, err := cmd.Output()
			require.NoError(t, err)
			require.Equal(t, want, string(out), "jq on %s", tc.file)
		}
	}
}

// TestLedger_LLM27: llm_project reads the manifests: a pyproject.toml without
// [tool.poetry] is pip, with it poetry; "next" in a description or a script
// sets no framework, under devDependencies it does; Flask in any case in
// requirements.txt is flask, a flask- package is not; symfony/ only counts
// under require or require-dev.
func TestLedger_LLM27(t *testing.T) {
	m := New()
	env := baseEnv(physTemp(t), nil)
	info := func(d string) (v struct{ Type, Framework, Package_manager string }) {
		t.Helper()
		r := runGo(t, m, d, env, "llm_project")
		require.Equal(t, 0, r.RC, r.Stderr)
		require.NoError(t, json.Unmarshal([]byte(r.Stdout), &v), r.Stdout)
		return v
	}
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "pyproject.toml"), "[project]\nname = \"x\"\n# [tool.poetry]\n[tool.setuptools]\n")
	require.Equal(t, "pip", info(d).Package_manager)
	mustWrite(t, filepath.Join(d, "pyproject.toml"), "[project]\nname = \"x\"\n[ tool.poetry.dependencies ]\npython = \"^3.11\"\n")
	require.Equal(t, "poetry", info(d).Package_manager)

	d = physTemp(t)
	mustWrite(t, filepath.Join(d, "requirements.txt"), "Flask-Login\n# flask\n")
	require.Equal(t, "", info(d).Framework)
	mustWrite(t, filepath.Join(d, "requirements.txt"), "Flask-Login\nFlask>=3 ; python_version > \"3\"\n")
	require.Equal(t, "flask", info(d).Framework)

	d = physTemp(t)
	mustWrite(t, filepath.Join(d, "package.json"), `{"description":"not \"next\" at all","scripts":{"vue":"x"},"dependencies":{"left-pad":"1"}}`)
	require.Equal(t, "", info(d).Framework)
	mustWrite(t, filepath.Join(d, "package.json"), `{"dependencies":{"react":"18"},"devDependencies":{"next":"14"}}`)
	require.Equal(t, "nextjs", info(d).Framework)
	mustWrite(t, filepath.Join(d, "package.json"), `{"dependencies":{"express":"4","vue":"3"}}`)
	require.Equal(t, "vue", info(d).Framework)
	mustWrite(t, filepath.Join(d, "package.json"), `not json "next"`)
	v := info(d)
	require.Equal(t, "javascript", v.Type)
	require.Equal(t, "", v.Framework)

	d = physTemp(t)
	mustWrite(t, filepath.Join(d, "composer.json"), `{"description":"like symfony/console","require":{"monolog/monolog":"3"}}`)
	require.Equal(t, "", info(d).Framework)
	mustWrite(t, filepath.Join(d, "composer.json"), `{"require-dev":{"symfony/var-dumper":"6"}}`)
	require.Equal(t, "symfony", info(d).Framework)
}
