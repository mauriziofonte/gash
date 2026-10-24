// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/mauriziofonte/gash/internal/invoke"
)

// configMaxBytes is the default --max-bytes of llm_config.
const configMaxBytes = "1048576"

// config is llm_config (fs.sh __llm_config_impl).
func (m *Module) config(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	file, maxBytes := "", configMaxBytes
	args := c.Args
	for len(args) > 0 {
		a := args[0]
		switch {
		case a == "--max-bytes":
			if len(args) < 2 {
				return fail(c, "missing_value", a, a+" needs a value")
			}
			maxBytes = args[1]
			args = args[2:]
		case strings.HasPrefix(a, "-"):
			return fail(c, "unknown_option", a, "Use a file path and --max-bytes N")
		default:
			if file != "" {
				return fail(c, "unexpected_argument", a, "Pass one file per call")
			}
			file = a
			args = args[1:]
		}
	}
	if file == "" {
		return fail(c, "missing_file", "", "Provide a file path as argument")
	}
	if !validUint(c, "size", maxBytes) {
		return 1
	}
	if m.guard(c).IsSecretFile(file) {
		return fail(c, "secret_file_blocked", "Cannot read .env or credential files", "This file contains secrets and cannot be read")
	}
	safe, ok := m.validatePath(c, file)
	if !ok {
		return 1
	}
	if st, err := os.Stat(safe); err != nil || !st.Mode().IsRegular() {
		return fail(c, "file_not_found", safe, "Check the file path and try again")
	}
	if unix.Faccessat(unix.AT_FDCWD, safe, unix.R_OK, unix.AT_EACCESS) != nil {
		return fail(c, "path_unreadable", safe, "The file is not readable by this user")
	}
	if st, err := os.Stat(safe); err == nil && st.Size() > int64(atoi(maxBytes)) {
		return fail(c, "file_too_large", safe+" is "+strconv.FormatInt(st.Size(), 10)+" bytes",
			"Over the --max-bytes limit ("+maxBytes+"): raise --max-bytes, or read part of it with llm_exec \"head -c 65536 FILE\"")
	}
	if strings.HasSuffix(safe, ".yaml") || strings.HasSuffix(safe, ".yml") {
		data, err := m.readFile(safe)
		if err != nil {
			return fail(c, "path_unreadable", safe, "The file could not be read")
		}
		// LEDGER LLM-35: JSON as python-yq and jq printed it; the raw
		// text for a file that does not convert or holds no document.
		if docs, err := yamlDocs(data); err == nil && len(docs) > 0 {
			_ = writeYAMLJSON(c.Stdout, docs)
			return 0
		}
		_, _ = c.Stdout.Write(data)
		return 0
	}
	f, err := os.Open(safe)
	if err != nil {
		return fail(c, "path_unreadable", safe, "The file could not be read")
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(c.Stdout, f); err != nil {
		return fail(c, "path_unreadable", safe, "The file could not be read")
	}
	return 0
}
