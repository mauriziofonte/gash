// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// noComposeV2 is a docker without the compose subcommand (Compose v1
// comes from the docker-compose mock).
const noComposeV2 = "#!/bin/sh\necho \"docker $*\" >>\"$MOCK_DOCKER_LOG\"\n[ \"$1\" = compose ] && exit 1\nexec \"$(dirname \"$0\")/../mock-docker\" \"$@\"\n"

// mixed is a compose file with every kind of reference the mocks know.
const mixed = `version: "3"
services:
  web:
    image: nginx:latest
  db:
    image: postgres:15.3.1
  cache:
    image: redis:7
  pinned:
    image: redis@sha256:789abc123def456abc123def456abc123def456abc123def456abc123def4567
  ui:
    image: ghcr.io/open-webui/open-webui:main
  app:
    image: registry.example.com/team/app:v1
  hub:
    image: docker.io/alpine
  llm:
    image: ollama/ollama
`

func withV1(s *scene) *scene {
	s.stubs["docker"] = noComposeV2
	s.prep = chain(s.prep, func(string) {
		_ = os.Symlink(filepath.Join(absRoot, "tests/mocks/bin/docker"), filepath.Join(s.base, "mock-docker"))
	})
	return s
}

func chain(fs ...func(string)) func(string) {
	return func(w string) {
		for _, f := range fs {
			if f != nil {
				f(w)
			}
		}
	}
}

func TestCheckMatches2x(t *testing.T) {
	for _, mode := range []struct{ color, utf8 bool }{{false, false}, {true, false}, {true, true}} {
		s := newScene(t)
		s.color, s.utf8 = mode.color, mode.utf8
		s.files["docker-compose.yml"] = mixed
		r := s.same("docker_compose_check")
		require.Contains(t, r.Out, "NOT_PULLED")
		s.same("docker_compose_check", "--json")
		s.same("docker_compose_check", ".", "--no-color")
		for _, knob := range []string{"MOCK_CURL_REGISTRY_RATE_LIMITED=1", "MOCK_CURL_REGISTRY_NOT_FOUND=1",
			"MOCK_CURL_REGISTRY_UNAUTHORIZED=1", "MOCK_CURL_REGISTRY_AUTH_FAIL=1", "MOCK_CURL_REGISTRY_TIMEOUT=1",
			"MOCK_CURL_ERROR=1", "MOCK_CURL_ERROR=1 MOCK_CURL_ERROR_CODE=418", "MOCK_CURL_ERROR=1 MOCK_CURL_ERROR_CODE=302",
			"MOCK_CURL_TIMEOUT=1", "GITHUB_TOKEN=gh-secret"} {
			s.env = strings.Fields(knob)
			s.same("docker_compose_check")
			s.same("docker_compose_check", "--json")
		}
		s.env = nil
	}
}

func TestCheckArgsMatch2x(t *testing.T) {
	s := newScene(t)
	s.files["p/compose.yaml"] = "services:\n  w:\n    image: nginx:latest\n"
	s.files["my \"pro\"\\ject/docker-compose.yaml"] = "services:\n  w:\n    image: nginx:latest\n"
	s.files["empty/compose.yml"] = "version: \"3\"\n# No services defined\n"
	s.files["build/compose.yml"] = "services:\n  x:\n    build: .\n"
	s.prep = func(w string) { require.NoError(t, os.Symlink(w+"/p", w+"/link")) }
	for _, args := range [][]string{
		{"-h"}, {"--help"}, {"p", "-h"}, {"--bogus"}, {"--bogus", "--json"}, {"-j", "-x"}, {"-"}, {"p", "--json"},
		{"p", "-j"}, {"p"}, {"link", "--json"}, {"link/../p", "--json"}, {"nowhere"}, {"nowhere", "--json"},
		{"empty"}, {"empty", "--json"}, {"build", "--json"}, {"my \"pro\"\\ject", "--json"}, {""}, {"", "--json"},
		{"p", "empty"}, {"empty", "p", "--json"}, {"p/compose.yaml"},
	} {
		s.same("docker_compose_check", args...)
	}
	s.same("docker_compose_check", "p", "--json", "--no-color")
	// No docker, no compose.
	n := newScene(t).noDocker()
	n.files["compose.yml"] = "services:\n  w:\n    image: nginx\n"
	n.same("docker_compose_check")
	n.same("docker_compose_check", "--json")
	n.same("docker_compose_check", "--no-color")
	n.stubs["docker"] = noComposeV2 // docker without compose and no docker-compose
	n.prep = func(string) {
		_ = os.Symlink(filepath.Join(absRoot, "tests/mocks/bin/docker"), filepath.Join(n.base, "mock-docker"))
	}
	n.same("docker_compose_check")
	n.same("docker_compose_check", "--json")
	// Compose v1: the YAML-subset reader.
	v := withV1(newScene(t))
	v.files["compose.yml"] = mixed
	v.same("docker_compose_check", "--json")
}

func TestCheckEnvMatches2x(t *testing.T) {
	s := newScene(t)
	s.files["docker-compose.yml"] = "services:\n  app:\n    image: ${REGISTRY:-docker.io}/myapp:${TAG:-latest}\n" +
		"  b:\n    image: \"${IMG}:${MISSING-dflt}\"\n  c:\n    image: ${LOOP}\n  d:\n    image: ${OPT}\n" +
		"  e:\n    image: redis:${EMPTY:-x}${HOME_TAG}\n  f:\n    image: $PLAIN/x\n"
	s.files[".env"] = "# comment\nREGISTRY=ghcr.io/myorg\nTAG=v1.0\nIMG='nginx'\nLOOP=${LOOP}\nOPT=-n\nEMPTY=\n" +
		"  SPACED=1\nexport X=1\nTAG=\"v2.0\"\nlast=no-newline"
	s.env = []string{"HOME_TAG=-from-env", "TAG=shell"}
	r := s.same("docker_compose_check", "--json")
	require.Contains(t, r.Out, "ghcr.io/myorg/myapp:v2.0")
	require.Contains(t, r.Err, "Unresolvable variable reference (circular?) in: ${LOOP}")
	s.color = true
	s.same("docker_compose_check")
}

func TestCheckConfigJSONMatches2x(t *testing.T) {
	s := newScene(t)
	s.files["compose.yml"] = "services:\n  web:\n    image: ${TAG}\n"
	for _, js := range []string{
		`{"services":{"web":{"image":"nginx:1.27"},"app":{"build":{"context":"."}},"cache":{"image":"redis:7"}}}`,
		`{"services":{}}`, `{"name":"x"}`, `null`, `{"services":null}`, `[1]`, `{"services":"x"}`, `not json`,
		`{"services":{"a":{"image":5},"b":{"image":false},"c":null}}`, `{"services":{"a":{"image":"x"}}} {"services":{"b":{"image":"y"}}}`,
	} {
		s.env = []string{"MOCK_COMPOSE_CONFIG_JSON=" + js}
		s.same("docker_compose_check", "--json")
		s.same("docker_compose_scan", "--json")
	}
}

func TestUpgradeMatches2x(t *testing.T) {
	for _, mode := range []struct{ color, utf8 bool }{{false, false}, {true, false}, {true, true}} {
		s := newScene(t)
		s.color, s.utf8 = mode.color, mode.utf8
		s.files["docker-compose.yml"] = mixed
		s.files["pinned/compose.yml"] = "services:\n  db:\n    image: postgres:15.2.1\n"
		s.files["build/compose.yml"] = "services:\n  x:\n    build: .\n"
		s.files["two/docker-compose.yml"] = "services:\n  old:\n    image: nginx:latest\n"
		s.files["two/compose.yaml"] = "services:\n  new:\n    image: nginx:latest\n"
		s.files["opt/compose.yml"] = "services:\n  --help:\n    image: nginx:latest\n"
		for _, args := range [][]string{
			{"-h"}, {"--help"}, {"--dry-run"}, {"--dry-run", "--force"}, {"pinned", "--dry-run"}, {"pinned"},
			{"pinned", "-f", "--dry-run"}, {"build", "--dry-run"}, {"two", "--dry-run"}, {"opt", "--dry-run"},
			{"-n"}, {"--bogus"}, {"nowhere"}, {""}, {"--yes"}, {"-y", "--no-color"}, {"build", "-y"}, {},
			{"--no-color"},
		} {
			s.same("docker_compose_upgrade", args...)
		}
		s.env = []string{"GASH_HEADLESS=1"}
		s.same("docker_compose_upgrade")
		s.same("docker_compose_upgrade", "--yes")
		s.env = nil
	}
	n := newScene(t).noDocker()
	n.files["compose.yml"] = mixed
	n.same("docker_compose_upgrade", "-n")
	n.same("docker_compose_upgrade", "--dry-run")
	v := withV1(newScene(t))
	v.files["compose.yml"] = mixed
	v.same("docker_compose_upgrade", "--dry-run")
	v.same("docker_compose_upgrade", "--yes")
}

// A failing pull, stop and up.
func TestUpgradeFailuresMatch2x(t *testing.T) {
	for _, fail := range []string{"pull", "stop", "up", "ps", "image"} {
		s := withV1(newScene(t))
		s.stubs["docker"] = "#!/bin/sh\necho \"docker $*\" >>\"$MOCK_DOCKER_LOG\"\nfor a in \"$@\"; do [ \"$a\" = " + fail +
			" ] && { echo \"$a failed\" >&2; exit 4; }; done\nexec \"$(dirname \"$0\")/../mock-docker\" \"$@\"\n"
		s.files["compose.yml"] = "services:\n  web:\n    image: nginx:latest\n  db:\n    image: redis:7\n"
		r := s.same("docker_compose_upgrade", "--yes")
		if fail == "pull" || fail == "up" || fail == "ps" {
			require.NotEqual(t, 0, r.RC, fail)
		}
	}
}

func TestScanMatches2x(t *testing.T) {
	for _, mode := range []struct{ color, utf8 bool }{{false, false}, {true, false}, {true, true}} {
		s := newScene(t).noDocker()
		s.color, s.utf8 = mode.color, mode.utf8
		s.files["project1/docker-compose.yml"] = "services:\n  app:\n    image: nginx\n"
		s.files["project2/compose.yaml"] = "services:\n  db:\n    image: postgres\n  cache:\n    image: redis\n"
		s.files["l1/l2/l3/docker-compose.yml"] = "services:\n  b:\n    image: nginx\n"
		s.files["B/compose.yml"] = "services: {}\n"
		s.files["a-b/compose.yml"] = "services:\n  'a\"b':\n    image: nginx:latest\n  'c,d':\n    image: x\n"
		s.files["a_b/compose.yml"] = "services:\n  x:\n    build: .\n"
		s.files["a/docker-compose.yaml"] = "services:\n  -n:\n    image: x\n"
		s.files["p3/compose.yml"] = "services:\n  a|b:\n    image: c|d\n"
		s.files["other.yml"] = "services:\n  z:\n    image: z\n"
		s.prep = func(w string) {
			require.NoError(t, os.Symlink(w+"/project1", w+"/dirlink"))
			require.NoError(t, os.Symlink(w+"/project1/docker-compose.yml", w+"/compose.yml"))
		}
		for _, args := range [][]string{
			{}, {"--json"}, {"-h"}, {"--help"}, {".", "--help"}, {"--depth", "2", "--json"}, {"-d", "1"},
			{"--depth", "05", "--json"}, {"--depth", "abc", "--json"}, {"--depth", "abc"}, {"--depth", "0"},
			{"--depth", "-1", "--json"}, {"--depth", "99999999999", "--json"}, {"--depth"}, {"-d"},
			{"--bogus", "--json"}, {"nowhere"}, {"nowhere", "--json"}, {"project2/compose.yaml", "--json"},
			{"project2"}, {"", "--json"}, {"--json", "--no-color"}, {"--no-color"}, {"l1", "--depth", "3"},
		} {
			s.same("docker_compose_scan", args...)
		}
	}
	e := newScene(t).noDocker()
	e.same("docker_compose_scan", "--json")
	e.same("docker_compose_scan")
	d := newScene(t)
	d.files["x/compose.yml"] = "services:\n  app:\n    image: nginx\n"
	d.same("docker_compose_scan", "--json")
	d.same("docker_compose_scan")
}
