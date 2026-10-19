// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"os/exec"
	"strings"
	"testing"
)

// TestBodyMatchesJq checks that the request bodies are byte-for-byte what
// 2.x built with jq (key order and the compacted schema), when jq is
// present. Skipped otherwise.
func TestBodyMatchesJq(t *testing.T) {
	jq, err := exec.LookPath("jq")
	if err != nil {
		t.Skip("no jq")
	}
	got, err := claudeBody("claude-haiku-5-5", "sys prompt", "hi there", responseSchema, 1024, "low")
	if err != nil {
		t.Fatal(err)
	}
	want := jqBytes(t, jq,
		[]string{"--arg", "model", "claude-haiku-5-5", "--arg", "system", "sys prompt",
			"--arg", "user", "hi there", "--argjson", "max_tokens", "1024",
			"--argjson", "schema", responseSchema, "--arg", "effort", "low"},
		`{model: $model, max_tokens: $max_tokens, system: $system, `+
			`messages: [{role: "user", content: $user}], `+
			`output_config: ({format: {type: "json_schema", schema: $schema}} `+
			`+ (if $effort == "" then {} else {effort: $effort} end))}`)
	if string(got) != want {
		t.Fatalf("claude body:\n got: %s\nwant: %s", got, want)
	}

	gg, err := geminiBody("sys prompt", "hi there", responseSchema, 1024)
	if err != nil {
		t.Fatal(err)
	}
	wantG := jqBytes(t, jq,
		[]string{"--arg", "system", "sys prompt", "--arg", "user", "hi there",
			"--argjson", "max_tokens", "1024", "--argjson", "schema", responseSchema},
		`{systemInstruction: {parts: [{text: $system}]}, `+
			`contents: [{role: "user", parts: [{text: $user}]}], `+
			`generationConfig: {maxOutputTokens: $max_tokens, `+
			`responseMimeType: "application/json", responseJsonSchema: $schema}}`)
	if string(gg) != wantG {
		t.Fatalf("gemini body:\n got: %s\nwant: %s", gg, wantG)
	}
}

func jqBytes(t *testing.T, jq string, args []string, prog string) string {
	t.Helper()
	out, err := exec.Command(jq, append(append([]string{"-cn"}, args...), prog)...).Output()
	if err != nil {
		t.Fatalf("jq: %v", err)
	}
	return strings.TrimRight(string(out), "\n")
}
