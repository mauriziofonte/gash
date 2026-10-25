// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

import "sort"

// functions is the whole catalog: the entries of catalog_gen.go and the
// 3.4 agent-toolkit entries, kept one file per area (catalog34_*.go) so
// that parallel work on separate areas never edits the same lines; merged
// and sorted by name once, at package initialization.
var functions = mergeCatalog(baseFunctions,
	catalogSearch, catalogText, catalogData, catalogGit, catalogNet,
	catalogProc, catalogRuntime, catalogRemote, catalogAgent)

func mergeCatalog(parts ...[]Function) []Function {
	n := 0
	for _, p := range parts {
		n += len(p)
	}
	out := make([]Function, 0, n)
	for _, p := range parts {
		out = append(out, p...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
