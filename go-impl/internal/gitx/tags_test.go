// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const setupTags = setupOrigin + ` &&
git tag v1 && git tag -a v2 -m "two" && git push -q origin v2 2>/dev/null &&
git tag -a v0.9 -m "old" HEAD~1 && git push -q origin v0.9 2>/dev/null && git tag -d v0.9 >/dev/null`

func TestListTagsMatches2x(t *testing.T) {
	cases := []struct {
		name, setup, cwd string
		m                mode
		args             []string
	}{
		{"outside a repository", "", ".", plain, nil},
		{"no origin", setupRepo, "w", plain, nil},
		{"no tags", setupOrigin, "w", plain, nil},
		{"tags", setupTags, "w", plain, nil},
		{"tags in color", setupTags, "w", color, nil},
		{"extra words ignored", setupTags, "w", plain, []string{"x", "y"}},
		{"help", "", ".", plain, []string{"-h"}},
		{"help in color", "", ".", color, []string{"--help"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			newScene(t, tc.setup).same(tc.m, tc.cwd, "git_list_tags", tc.args...)
		})
	}
}

func TestAddTagMatches2x(t *testing.T) {
	cases := []struct {
		name, setup string
		args        []string
		rc          int
	}{
		{"outside a repository", "", nil, 1},
		{"no origin", setupRepo, []string{"v1"}, 1},
		{"missing name", setupOrigin, nil, 1},
		{"empty name", setupOrigin, []string{""}, 1},
		{"invalid name", setupOrigin, []string{"bad..name"}, 1},
		{"invalid name with a space", setupOrigin, []string{"a b"}, 1},
		{"exists locally", setupTags, []string{"v1"}, 1},
		{"exists on origin only", setupTags, []string{"v0.9"}, 1},
		{"prefix is not a match", setupTags, []string{"v"}, 0},
		{"default message", setupOrigin, []string{"v3"}, 0},
		{"own message", setupOrigin, []string{"v3", "Third release"}, 0},
		{"empty message is the default", setupOrigin, []string{"v3", ""}, 0},
		{"extra words ignored", setupOrigin, []string{"v3", "m", "x"}, 0},
		{"help", "", []string{"-h"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newScene(t, tc.setup).same(plain, cwdOf(tc.setup), "git_add_tag", tc.args...)
			require.Equal(t, tc.rc, r.RC)
		})
	}
	t.Run("color", func(t *testing.T) {
		newScene(t, setupOrigin).same(color, "w", "git_add_tag", "v3")
	})
}

func TestDeleteTagMatches2x(t *testing.T) {
	cases := []struct {
		name, setup string
		args        []string
		rc          int
	}{
		{"outside a repository", "", []string{"v1"}, 1},
		{"no origin", setupRepo, []string{"v1"}, 1},
		{"missing name", setupOrigin, nil, 1},
		{"invalid name", setupOrigin, []string{"x..y"}, 1},
		{"unknown", setupTags, []string{"v9"}, 1},
		{"local only", setupTags, []string{"v1"}, 0},
		{"local and origin", setupTags, []string{"v2"}, 0},
		{"origin only", setupTags, []string{"v0.9"}, 0},
		{"extra words ignored", setupTags, []string{"v2", "--yes"}, 0},
		{"help", "", []string{"--help"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newScene(t, tc.setup).same(plain, cwdOf(tc.setup), "git_delete_tag", tc.args...)
			require.Equal(t, tc.rc, r.RC)
		})
	}
	t.Run("color", func(t *testing.T) {
		newScene(t, setupTags).same(color, "w", "git_delete_tag", "v2")
	})
	t.Run("origin refuses the deletion", func(t *testing.T) {
		// A pre-receive hook on origin rejects every push.
		setup := setupTags + ` && printf '#!/bin/sh\nexit 1\n' >../origin.git/hooks/pre-receive &&
chmod +x ../origin.git/hooks/pre-receive`
		r := newScene(t, setup).same(plain, "w", "git_delete_tag", "v2")
		require.Equal(t, 1, r.RC)
		require.Contains(t, r.Err, "Could not delete remote tag 'v2'.")
	})
}

func TestRemoteTagLines(t *testing.T) {
	require.Equal(t, "", remoteTagLines(nil))
	require.Equal(t, "v1\nv1^{}\n\nx\n", remoteTagLines([]byte(
		"h1\trefs/tags/v1\nh2\trefs/tags/v1^{}\nonefield\n  h3  refs/tags/x  extra")))
	require.Equal(t, "a/refs/tags/b\n", remoteTagLines([]byte("h refs/tags/a/refs/tags/b\n")))
}

// cwdOf is the directory a case runs in: the work tree when the setup
// made one.
func cwdOf(setup string) string {
	if setup == "" {
		return "."
	}
	return "w"
}
