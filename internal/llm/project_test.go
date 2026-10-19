// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"path/filepath"
	"testing"
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

	// python / django, poetry (pyproject.toml forces poetry per LLM-27)
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
