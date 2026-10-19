// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package chrome

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The expected outputs are `out=$(jq -r PROG 2>/dev/null)` of jq 1.7.
func TestJQ(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"version", `{"version":"1.2"}`, "1.2"},
		{"version", `{"version":null}`, ""},
		{"version", `{"version":false}`, ""},
		{"version", `{"version":3}`, "3"},
		{"version", `{"version":{"a":[1,2]}}`, "{\n  \"a\": [\n    1,\n    2\n  ]\n}"},
		{"version", `[1]`, ""},
		{"version", `{"version":"a"} {"version":"b"}`, "a\nb"},
		{"version", `[1] {"version":"b"}`, "b"},
		{"version", `{"version":"a"} garbage {"version":"b"}`, "a"},
		{"version", `"str"`, ""},
		{"version", ``, ""},
		{"version", `{"version":"x\ty\n"}`, "x\ty"},

		{"shell", `{"browsers":[{"name":"x"},{"name":"chromium-headless-shell","browserVersion":"1.2.3.4"}]}`, "1.2.3.4"},
		{"shell", `{"browsers":[1,{"name":"chromium-headless-shell","browserVersion":"1.2.3.4"}]}`, ""},
		{"shell", `{"browsers":[{"name":"chromium-headless-shell","browserVersion":"a"},5]}`, "a"},
		{"shell", `{"browsers":null}`, ""},
		{"shell", `{"browsers":{"k":{"name":"chromium-headless-shell","browserVersion":"7"}}}`, "7"},
		{"shell", `{"browsers":[{"name":"chromium-headless-shell"}]}`, ""},
		{"shell", `{"browsers":[{"name":"chromium-headless-shell","browserVersion":"a"},{"name":"chromium-headless-shell","browserVersion":"b"}]}`, "a\nb"},

		{"linux64", `{"downloads":{"chrome-headless-shell":[{"platform":"mac","url":"m"},{"platform":"linux64","url":"u"}]}}`, "u"},
		{"linux64", `{"downloads":5}`, ""},
		{"linux64", `5`, ""},
		{"linux64", `{"downloads":{"chrome-headless-shell":[{"platform":"linux64"}]}}`, "null"},
		{"linux64", `{"downloads":{"chrome-headless-shell":[3,{"platform":"linux64","url":"u"}]}}`, ""},
		{"linux64", `{"downloads":{"chrome-headless-shell":[{"platform":"linux64","url":"u"},3]}}`, "u"},
		{"linux64", `{"downloads":{"chrome-headless-shell":[{"platform":"linux64","url":1}]}}`, "1"},

		{"known", `{"versions":[{"version":"1","downloads":{"chrome-headless-shell":[{"platform":"linux64","url":"u"}]}}]}`, "u"},
		{"known", `{"versions":[{"version":"2"},{"version":"1","downloads":5}]}`, ""},
		{"known", `{"versions":[{"version":"1","downloads":{"chrome-headless-shell":[{"platform":"linux64","url":"a"}]}},4,{"version":"1"}]}`, "a"},
		{"known", `{"versions":5}`, ""},
		// `[]?` drops the iteration error only: an entry whose downloads
		// cannot be indexed ends the text.
		{"known", `{"versions":[{"version":"1","downloads":5},{"version":"1","downloads":{"chrome-headless-shell":[{"platform":"linux64","url":"u"}]}}]}`, ""},
		{"known", `{"versions":[{"version":"1","downloads":{"chrome-headless-shell":5}},{"version":"1","downloads":{"chrome-headless-shell":[{"platform":"linux64","url":"u"}]}}]}`, "u"},
		{"known", `{"versions":[{"version":1,"downloads":{"chrome-headless-shell":[{"platform":"linux64","url":"u"}]}}]}`, ""},
	} {
		prog := map[string]func([]byte) string{
			"version": func(b []byte) string { return jqRun(b, jqVersion) },
			"shell":   func(b []byte) string { return jqRun(b, jqShell("browserVersion")) },
			"linux64": func(b []byte) string { return jqRun(b, jqLinux64) },
			"known":   func(b []byte) string { return jqRun(b, jqKnownGood("1")) },
		}[tc.name]
		require.Equal(t, tc.want, prog([]byte(tc.body)), tc.name+" "+tc.body)
	}
}
