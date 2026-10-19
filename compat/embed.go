// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package compat holds the templates of the generated Bash files (see
// internal/compatgen). Templates use the delimiters {{% and %}}, which
// never occur in Bash.
package compat

import "embed"

// FS holds every template.
//
//go:embed *.tmpl
var FS embed.FS
