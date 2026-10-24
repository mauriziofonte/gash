// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package pathguard

import "strings"

// asciiLower lowercases ASCII letters only, matching bash ${var,,} under the
// C locale (the reproductions run with LANG=C.UTF-8 / C).
func asciiLower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] |= 0x20
		}
	}
	return string(b)
}

// SecretName reports whether a path's base name matches a secret pattern
// (case-insensitively, with the exceptions winning) or whether the path lies
// under a secret directory. It touches no filesystem. Ported from
// __llm_secret_name.
func SecretName(path string) bool {
	path = strings.TrimSuffix(path, "/") // ${path%/}: one trailing slash
	base := path
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if SecretException(base) {
		return false
	}
	base = asciiLower(base)
	for _, pat := range SecretPatterns {
		if matchGlob(pat, base) {
			return true
		}
	}
	lower := asciiLower(path)
	for _, dir := range SecretDirs {
		if strings.Contains("/"+lower+"/", "/"+dir+"/") {
			return true
		}
	}
	return false
}

// SecretException reports whether a base name matches one of the
// SecretExceptions (case-insensitively): a template that shares a secret
// name but holds no secrets, which wins over the patterns and the secret
// directories. Walks use it to search such files (LEDGER LLM-15).
func SecretException(name string) bool {
	name = asciiLower(name)
	for _, pat := range SecretExceptions {
		if matchGlob(pat, name) {
			return true
		}
	}
	return false
}

// SecretExcludes builds the exclusion arguments that keep secret files out of
// a content search, for tool "rg" or "grep". Ported from
// __llm_secret_excludes (used by llm_grep/llm_find and the llm_exec prelude).
func SecretExcludes(tool string) []string {
	var out []string
	for _, pat := range SecretPatterns {
		if tool == "rg" {
			out = append(out, "--iglob", "!"+pat)
		} else {
			out = append(out, "--exclude="+IcaseGlob(pat))
		}
	}
	for _, dir := range SecretDirs {
		if tool == "rg" {
			out = append(out, "--iglob", "!**/"+dir+"/**")
		} else if !strings.Contains(dir, "/") {
			out = append(out, "--exclude-dir="+IcaseGlob(dir))
		}
	}
	return out
}

// ExecSecretExcludes is SecretExcludes for the llm_exec wrappers: the same
// secret directories, and the patterns with the exceptions taken out
// (SecretExcludeGlobs), so grep, rg and diff inside llm_exec search a
// .env.example or an id_rsa.pub as the path guard allows (F-05).
func ExecSecretExcludes(tool string) []string {
	var out []string
	for _, g := range SecretExcludeGlobs(tool != "rg") {
		if tool == "rg" {
			out = append(out, "--iglob", "!"+g)
		} else {
			out = append(out, "--exclude="+g)
		}
	}
	for _, dir := range SecretDirs {
		if tool == "rg" {
			out = append(out, "--iglob", "!**/"+dir+"/**")
		} else if !strings.Contains(dir, "/") {
			out = append(out, "--exclude-dir="+IcaseGlob(dir))
		}
	}
	return out
}
