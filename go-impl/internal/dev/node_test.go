// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package dev

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// nvmTree makes an NVM_DIR with the given node version directories under
// versions/node and returns it.
func nvmTree(t *testing.T, versions ...string) string {
	t.Helper()
	dir := phys(t)
	for _, v := range versions {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "versions", "node", v), 0o755))
	}
	return dir
}

// field reads a one-line-JSON string field.
func field(out, name string) string {
	key := `"` + name + `":"`
	i := strings.Index(out, key)
	if i < 0 {
		return ""
	}
	rest := out[i+len(key):]
	return rest[:strings.Index(rest, `"`)]
}

func TestNodeDetect(t *testing.T) {
	m := devMod(t, "")

	// .nvmrc wins over package.json; full_version is the newest patch.
	nvm := nvmTree(t, "v18.18.0", "v20.9.0", "v20.11.1")
	proj := phys(t)
	mkfile(t, filepath.Join(proj, ".nvmrc"), "18\n")
	mkfile(t, filepath.Join(proj, "package.json"), `{"engines":{"node":"20"}}`)
	env := baseEnv(phys(t), map[string]string{"NVM_DIR": nvm})
	r := call(m, "", env, "node_detect_version", proj, "--llm")
	require.Equal(t, "18", field(r.out, "version"))
	require.Equal(t, "18.18.0", field(r.out, "full_version"))
	require.Equal(t, "node18", field(r.out, "node_command"))
	require.Equal(t, "npm18", field(r.out, "npm_command"))
	require.Equal(t, "npx18", field(r.out, "npx_command"))
	require.Equal(t, 0, r.rc)

	// package.json engines.node, then newest installed.
	nvm2 := nvmTree(t, "v20.9.0")
	proj2 := phys(t)
	mkfile(t, filepath.Join(proj2, "package.json"), `{"engines":{"node":">=20"}}`)
	require.Equal(t, "20", field(call(m, "", baseEnv(phys(t), map[string]string{"NVM_DIR": nvm2}), "node_detect_version", proj2, "--llm").out, "version"))

	nvm3 := nvmTree(t, "v18.18.0", "v22.14.0")
	r = call(m, "", baseEnv(phys(t), map[string]string{"NVM_DIR": nvm3}), "node_detect_version", phys(t), "--llm")
	require.Equal(t, "22", field(r.out, "version"))
	require.Equal(t, "22.14.0", field(r.out, "full_version"))

	// Missing NVM_DIR errors, rc 1.
	r = call(m, "", baseEnv(phys(t), map[string]string{"NVM_DIR": "/nonexistent-xyz"}), "node_detect_version")
	require.Equal(t, 1, r.rc)
	require.Contains(t, r.err, "NVM directory not found")

	// No installed versions at all: error, rc 1.
	r = call(m, "", baseEnv(phys(t), map[string]string{"NVM_DIR": nvmTree(t)}), "node_detect_version", "--llm")
	require.Equal(t, 1, r.rc)
	require.Contains(t, r.err, "No Node.js versions installed in NVM")

	// Human output under --width 15.
	human := call(m, "", baseEnv(phys(t), map[string]string{"NVM_DIR": nvm, "GASH_HEADLESS": ""}), "node_detect_version", proj)
	require.Contains(t, human.err, "Node.js version for")
	require.Contains(t, human.out, "node_command")
	require.NotContains(t, human.out, `"version"`)
}

// TestLedger_DEV05_2x: every entry of versions/node counts (unfiltered):
// a stray name becomes the newest, and a prerelease beats its release.
func TestLedger_DEV05_2x(t *testing.T) {
	m := devMod(t, "")

	nvm := nvmTree(t, "v20.11.1", "zzz-broken")
	r := call(m, "", baseEnv(phys(t), map[string]string{"NVM_DIR": nvm}), "node_detect_version", phys(t), "--llm")
	require.Equal(t, "zzz-broken", field(r.out, "version"))
	require.Equal(t, "nodezzz-broken", field(r.out, "node_command"))
	require.Contains(t, r.err, "Node zzz-broken not installed. Using zzz-broken.")

	nvm2 := nvmTree(t, "v20.11.1", "v20.12.0-rc.1")
	r = call(m, "", baseEnv(phys(t), map[string]string{"NVM_DIR": nvm2}), "node_detect_version", phys(t), "--llm")
	require.Equal(t, "20", field(r.out, "version"))
	require.Equal(t, "20.12.0-rc.1", field(r.out, "full_version"))
}

// TestLedger_DEV19_2x: an exact pin resolves to the newest installed
// patch of its major.
func TestLedger_DEV19_2x(t *testing.T) {
	m := devMod(t, "")
	nvm := nvmTree(t, "v20.9.0", "v20.11.1")
	proj := phys(t)
	mkfile(t, filepath.Join(proj, ".node-version"), "v20.9.0\n")
	r := call(m, "", baseEnv(phys(t), map[string]string{"NVM_DIR": nvm}), "node_detect_version", proj, "--llm")
	require.Equal(t, "20", field(r.out, "version"))
	require.Equal(t, "20.11.1", field(r.out, "full_version"))
}

// TestNodeLTS: lts/<name> resolves through the nvm alias files, with the
// codename table as a fallback (the node_upgrade_spec vectors).
func TestNodeLTS(t *testing.T) {
	m := devMod(t, "")
	dir := phys(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "alias", "lts"), 0o755))
	mkfile(t, filepath.Join(dir, "alias", "lts", "futurename"), "v26.1.0\n")
	mkfile(t, filepath.Join(dir, "alias", "lts", "*"), "v24.10.0\n")
	require.Equal(t, "26", m.parseToMajor(dir, "lts/futurename"))
	require.Equal(t, "24", m.parseToMajor(dir, "lts/*"))
	require.Equal(t, "24", m.parseToMajor(dir, "lts/krypton")) // codename table
	require.Equal(t, "18", m.parseToMajor(dir, "lts/hydrogen"))
}
