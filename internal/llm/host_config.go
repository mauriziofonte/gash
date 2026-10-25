// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/pathguard"
	"github.com/mauriziofonte/gash/internal/remote"
)

// configFallback is the POSIX sh llm_config for a host without the gash
// binary (FILE [--max-bytes N]): the remote side applies the file checks
// in the local order (secret name of the resolved file, protected path,
// regular file, readable, the size cap) and sends the file; the YAML
// conversion runs here, with the local code, so the output is the local
// one. A secret file never leaves the host. Other options need the binary.
func configFallback(_ *invoke.Call, nonce string, args []string) (*remote.Plan, bool) {
	file, maxBytes := "", configMaxBytes
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--max-bytes":
			if i+1 >= len(args) {
				return argError("missing_value", a, a+" needs a value")
			}
			maxBytes = args[i+1]
			i++
		case strings.HasPrefix(a, "-"):
			return nil, false
		case file != "":
			return argError("unexpected_argument", a, "Pass one file per call")
		default:
			file = a
		}
	}
	if file == "" {
		return argError("missing_file", "", "Provide a file path as argument")
	}
	if !uintRE.MatchString(maxBytes) {
		return argError("invalid_size", "Value '"+maxBytes+"' is not a non-negative integer below 1000000000", "Provide a plain integer for size")
	}
	if pathguard.SecretName(file) {
		return argError("secret_file_blocked", "Cannot read .env or credential files", "This file contains secrets and cannot be read")
	}
	script := fbHeader("P", file, "MAX", maxBytes) + `
abs=$(gash_resolve "$P")
gash_secret "$abs" && gash_err secret_file_blocked secret -
gash_forbidden "$abs" && gash_err forbidden_path protected "$abs"
[ -f "$abs" ] || gash_err file_not_found file "$abs"
[ -r "$abs" ] || gash_err path_unreadable notreadable "$abs"
set -f; set -- $(ls -lnd -- "$abs" 2>/dev/null); set +f
[ "${5:-0}" -gt "$MAX" ] && gash_err file_too_large toolarge "$abs is $5 bytes"
printf '%s meta %s\n' "$N" "$abs" >&2
cat -- "$abs" || gash_err path_unreadable couldnot "$abs"
exit 0`
	hint := func(code, key, details string) (string, string) {
		switch key {
		case "secret":
			return "Cannot read .env or credential files", "This file contains secrets and cannot be read"
		case "toolarge":
			return details, "Over the --max-bytes limit (" + maxBytes + "): raise --max-bytes, or read part of it with llm_exec \"head -c 65536 FILE\""
		}
		return details, fbHints[key]
	}
	finish := func(_ remote.Host, out, errOut []byte, st int) ([]byte, []byte, int) {
		if env, ok := fbError(errOut, nonce, hint); ok {
			return nil, env, 1
		}
		abs, ok := fbFrame(errOut, nonce, "meta")
		if !ok || st != 0 {
			return out, nil, st
		}
		if strings.HasSuffix(abs, ".yaml") || strings.HasSuffix(abs, ".yml") {
			if docs, err := yamlDocs(out); err == nil && len(docs) > 0 {
				var b bytes.Buffer
				_ = writeYAMLJSON(&b, docs)
				return b.Bytes(), nil, 0
			}
		}
		return out, nil, 0
	}
	return &remote.Plan{Script: script, Finish: finish}, true
}
