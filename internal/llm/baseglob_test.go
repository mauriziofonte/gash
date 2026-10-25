// Gash 3.1. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The wildcard comparison of a **/P glob gives the regexp's verdict, case
// folding included, on every path; a name that is not ASCII goes to the
// regexp under a case-insensitive glob.
func TestBaseGlobMatchesRegexp(t *testing.T) {
	globs := append(append([]string{}, noiseGlobs...), secretIglobs()...)
	globs = append(globs, "*.go", "a*b*c", "*", "**", "x*", "*x", "**/*.md", "src/*.go", "?.txt", "[ab].c", "{a,b}.d", "**/foo/**")
	cands := []string{
		"", "/", "a", "x", ".env", ".ENV", ".env.local", "dir/.env", "/abs/.Env.prod", "prod.env", "a/b/c.PEM",
		"id_rsa", "id_rsa.pub", "ID_RSA_old", "k/my_ed25519", "credentials", "Credentials.json", "credentials-x",
		"node_modules", "a/node_modules", "Node_Modules", "vendor/x.go", ".git", "a/.git", "main.go", "main.GO",
		"abc", "aXbXc", "acb", "ab", "c", "x.md", "a/b/x.md", "src/a.go", "src/b/a.go", "1.txt", "a.c", "b.d",
		"foo/bar", "a/foo/b", ".ssh/id", "home/.ssh/config", ".config/gcloud/x", "ssh_host_rsa_key",
		".Kube/config", "sKecret", "Kubeconfig", "kubeconfig", "secrets", "ſecrets", "日本.env",
		"x.tfstate.backup", "x.TFVARS.json", "a/.gash_env", "_netrc", "x\xffy.pem", "\xff.env",
	}
	shipped, fast := len(noiseGlobs)+len(secretIglobs()), 0
	for i, gl := range globs {
		for _, icase := range []bool{false, true} {
			g := &gitignore{root: "."}
			if g.add(gl, icase) != nil || len(g.globs) == 0 {
				continue
			}
			ig := g.globs[0]
			if i < shipped && !icase && ig.fast != nil {
				fast++
			}
			for _, c := range cands {
				want := ig.re.MatchString(c)
				slow := ig
				slow.fast = nil
				one := &gitignore{root: ".", globs: []ignoreGlob{ig}}
				two := &gitignore{root: ".", globs: []ignoreGlob{slow}}
				require.Equal(t, two.matched(c, false), one.matched(c, false), "%q icase=%v on %q (regexp %v)", gl, icase, c, want)
			}
		}
	}
	// The directory globs (**/.ssh/** and the like) by the text they need.
	require.Equal(t, shipped, fast)
}
