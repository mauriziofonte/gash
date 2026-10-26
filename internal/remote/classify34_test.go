// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyLocal(t *testing.T) {
	for path, want := range map[string]string{
		"/etc/passwd":            "deny:/etc/passwd",
		"/usr/bin/env":           "deny:/usr/bin",
		"/":                      "deny:/",
		"/etc/hosts":             "ask:/etc",
		"/var/www/x":             "ask:/var",
		"/home/u/.ssh/config":    "ask:.ssh",
		"/home/u/app/a$b(1).php": "allow",
		"/tmp/x":                 "allow",
		"/var/tmp/x":             "allow",
	} {
		require.Equal(t, want, ClassifyLocal(path), path)
	}
}
