// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/pathguard"
)

// TestCheckProtected: the host's PROTECT: paths as written, before
// anything connects: a read of an "all" path is forbidden_path, a write
// of a "content" path secret_file_blocked; ~/x and x are the same path
// (below the remote home); another host's lines do not count.
func TestCheckProtected(t *testing.T) {
	web := Host{Alias: "web", Rules: []pathguard.Rule{{Mode: "all", Path: "~/private"}, {Mode: "content", Path: "/srv/app/storage"}}}
	other := Host{Alias: "db"}
	for p, want := range map[string]string{
		"private": "forbidden_path", "~/private/x": "forbidden_path", "./private//x": "forbidden_path",
		"privateer": "", "/srv/app/storage/k": "", "/srv/app": "",
	} {
		e := CheckRead(p, web)
		if want == "" {
			require.Nil(t, e, p)
			continue
		}
		require.NotNil(t, e, p)
		require.Equal(t, want, e.Code, p)
		require.Nil(t, CheckRead(p, other), p)
	}
	for p, want := range map[string]string{
		"private/x": "forbidden_path", "/srv/app/storage/k": "secret_file_blocked", "/srv/app/x": "",
	} {
		e := CheckWrite(p, true, web)
		if want == "" {
			require.Nil(t, e, p)
			continue
		}
		require.NotNil(t, e, p)
		require.Equal(t, want, e.Code, p)
	}
	require.Equal(t, "", protectExport(other))
	require.Equal(t, "GASH__PROTECT='all ~/private\ncontent /srv/app/storage\n'; export GASH__PROTECT; ", protectExport(web))
}
