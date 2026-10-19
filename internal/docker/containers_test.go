// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// failingDocker is a docker whose daemon cannot be reached.
const failingDocker = "#!/bin/sh\n[ -n \"$MOCK_DOCKER_LOG\" ] && echo \"docker $*\" >>\"$MOCK_DOCKER_LOG\"\necho \"Cannot connect to the Docker daemon\" >&2\nexit 1\n"

func TestStartStopMatches2x(t *testing.T) {
	for _, fn := range []string{"docker_stop_all", "docker_start_all"} {
		for _, color := range []bool{false, true} {
			s := newScene(t)
			s.color = color
			s.same(fn)
			s.env = []string{"MOCK_DOCKER_CONTAINERS=1"}
			r := s.same(fn)
			require.Contains(t, r.Err, "Docker containers...")
			s.same(fn, "--bogus", "x")
			s.same(fn, "--help")
			s.same(fn, "-h")
			s.noDocker().same(fn)
		}
	}
}

func TestStartStopIDs(t *testing.T) {
	// IDs are split on blanks and newlines, as an unquoted $containers.
	s := newScene(t)
	s.stubs["docker"] = "#!/bin/sh\necho \"docker $*\" >>\"$MOCK_DOCKER_LOG\"\ncase $1 in ps) printf ' a1\\tb2\\n\\nc3  \\n' ;; stop) echo stopped; exit 3 ;; esac\n"
	r := s.same("docker_stop_all")
	require.Equal(t, 0, r.RC)
	require.Equal(t, "docker ps -q\ndocker stop a1 b2 c3\n", r.Docker)
	require.Equal(t, "stopped\n", r.Out)
}

func TestPruneMatches2x(t *testing.T) {
	for _, color := range []bool{false, true} {
		s := newScene(t)
		s.color = color
		s.env = []string{"MOCK_DOCKER_CONTAINERS=1", "MOCK_DOCKER_CUSTOM_NETWORKS=1"}
		r := s.same("docker_prune_all", "--dry-run")
		require.Contains(t, r.Err, "To remove: 2 container(s), 2 image(s), 1 volume(s), 2 network(s)")
		r = s.same("docker_prune_all", "-y")
		require.Contains(t, r.Docker, "docker network rm my_app_network")
		require.Contains(t, r.Docker, "docker system prune --volumes -a --force")
		s.same("docker_prune_all", "--yes", "--dry-run")
		// No terminal for the question: refused after the warning.
		r = s.same("docker_prune_all")
		require.Equal(t, 1, r.RC)
		s.env = append(s.env, "GASH_HEADLESS=1")
		r = s.same("docker_prune_all")
		require.Equal(t, 1, r.RC)
		s.same("docker_prune_all", "--bogus")
		s.same("docker_prune_all", "-y", "-n")
		s.same("docker_prune_all", "--help")
		s.noDocker().same("docker_prune_all", "-y")
	}
	s := newScene(t)
	r := s.same("docker_prune_all", "-y")
	require.NotContains(t, r.Docker, "docker stop")
	s.stubs["docker"] = failingDocker
	s.same("docker_prune_all", "-y")
}
