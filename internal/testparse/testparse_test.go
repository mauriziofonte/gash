// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package testparse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return b
}

// TestParseFixtures: every recorded runner output parses to its counts
// and failures, detected from the output alone and with --runner.
func TestParseFixtures(t *testing.T) {
	for _, tc := range []struct {
		file, runner string
		want         Summary
	}{
		{"phpunit_fail.txt", "phpunit", Summary{Passed: 3, Failed: 2, Skipped: 1, Failures: []Failure{
			{"Tests\\Unit\\UserTest::testCreate", "/work/app/tests/Unit/UserTest.php", 22, "RuntimeException: database unavailable"},
			{"Tests\\Unit\\MathTest::testSum", "/work/app/tests/Unit/MathTest.php", 15, "Failed asserting that 3 is identical to 4."},
		}}},
		{"phpunit_pass.txt", "phpunit", Summary{Passed: 4}},
		{"phpunit9_skipped.txt", "phpunit", Summary{Passed: 2, Skipped: 2}},
		{"pest_fail.txt", "pest", Summary{Passed: 2, Failed: 2, Skipped: 2, Failures: []Failure{
			{"Tests\\Feature\\UserTest > it creates a user", "tests/Feature/UserTest.php", 18, "Failed asserting that 500 is identical to 201."},
			{"Tests\\Feature\\UserTest > it deletes a user", "app/Models/User.php", 44, "RuntimeException: Record not found"},
		}}},
		{"pest_pass.txt", "pest", Summary{Passed: 2}},
		{"phpstan_table.txt", "phpstan", Summary{Failed: 3, Failures: []Failure{
			{"method.notFound", "src/Controller/UserController.php", 24, "Call to an undefined method App\\Repo::findAll()."},
			{"argument.type", "src/Controller/UserController.php", 31, "Parameter #1 $id of method App\\Repo::find() expects int, string given."},
			{"missingType.property", "src/Model/User.php", 12, "Property App\\Model\\User::$name has no type specified."},
		}}},
		{"phpstan_ok.txt", "phpstan", Summary{}},
		{"phpstan_json.txt", "phpstan", Summary{Failed: 2, Failures: []Failure{
			{"missingType.property", "/work/app/src/Model/User.php", 12, "Property App\\Model\\User::$name has no type specified."},
			{"method.notFound", "/work/app/src/Repo.php", 24, "Call to an undefined method App\\Repo::findAll()."},
		}}},
		{"psalm_fail.txt", "psalm", Summary{Failed: 2, Failures: []Failure{
			{"UndefinedMethod", "src/Repo.php", 24, "Method App\\Repo::findAll does not exist"},
			{"InvalidReturnType", "src/User.php", 12, "The declared return type 'int' for App\\User::id is incorrect, got 'string'"},
		}}},
		{"psalm_ok.txt", "psalm", Summary{}},
		{"vitest_fail.txt", "vitest", Summary{Passed: 4, Failed: 1, Skipped: 1, Failures: []Failure{
			{"client > retries on 503", "src/api/client.test.ts", 27, "AssertionError: expected 2 to be 3 // Object.is equality"},
		}}},
		{"vitest_pass.txt", "vitest", Summary{Passed: 3}},
		{"jest_fail.txt", "jest", Summary{Passed: 5, Failed: 2, Skipped: 1, Failures: []Failure{
			{"Cart › applies discount", "src/cart.test.js", 14, "expect(received).toBe(expected) // Object.is equality"},
			{"Cart › handles empty cart", "src/cart.test.js", 19, "TypeError: Cannot read properties of undefined (reading 'price')"},
		}}},
		{"jest_json.txt", "jest", Summary{Passed: 3, Failed: 1, Skipped: 2, Failures: []Failure{
			{"Cart applies discount", "/work/web/src/cart.test.js", 14, "Error: expect(received).toBe(expected) // Object.is equality"},
		}}},
		{"mocha_fail.txt", "mocha", Summary{Passed: 3, Failed: 2, Skipped: 1, Failures: []Failure{
			{"Cart applies discount", "test/cart.spec.js", 14, "AssertionError [ERR_ASSERTION]: Expected values to be strictly equal:"},
			{"Parser rejects bad input", "test/parser.spec.js", 9, "Error: expected parse to throw"},
		}}},
		{"pytest_fail.txt", "pytest", Summary{Passed: 3, Failed: 2, Skipped: 1, Failures: []Failure{
			{"test_div", "tests/test_math.py", 12, "ZeroDivisionError: division by zero"},
			{"test_read_file", "tests/test_io.py", 7, "OSError: disk full"},
		}}},
		{"pytest_pass.txt", "pytest", Summary{Passed: 5, Skipped: 1}},
		{"gotest_fail.txt", "go", Summary{Failed: 2, NoPassCounts: true, Failures: []Failure{
			{"TestParse/empty", "parse_test.go", 21, "Parse(\"\") = nil, want error"},
			{"TestSum", "sum_test.go", 9, "Sum(1, 2) = 4, want 3"},
		}}},
		{"gotest_verbose.txt", "go", Summary{Passed: 2, Skipped: 1}},
		// TestOne passes, TestTwo fails (t.Fatalf), TestThree logs, TestFour
		// has a failing subtest, TestP1 and TestP2 run in parallel (3.4.2:
		// without -v the passed count is unknown, not the ok packages; under
		// -v a test's lines come before its --- FAIL line, so the next test's
		// log is not its message).
		{"gotest_mixed.txt", "go", Summary{Failed: 3, NoPassCounts: true, Failures: []Failure{
			{"TestTwo", "x_test.go", 7, "boom two"},
			{"TestFour/b", "x_test.go", 14, "b bad"},
			{"TestP1", "y_test.go", 7, "p1 log"},
		}}},
		{"gotest_mixed_verbose.txt", "go", Summary{Passed: 4, Failed: 3, Failures: []Failure{
			{"TestTwo", "x_test.go", 7, "boom two"},
			{"TestFour/b", "x_test.go", 14, "b bad"},
			{"TestP1", "y_test.go", 7, "p1 log"},
		}}},
		{"gotest_json.txt", "go", Summary{Passed: 1, Failed: 1, Skipped: 1, Failures: []Failure{
			{"TestSum", "sum_test.go", 9, "Sum(1, 2) = 4, want 3"},
		}}},
		{"cargo_fail.txt", "cargo", Summary{Passed: 1, Failed: 2, Skipped: 1, Failures: []Failure{
			{"tests::subtracts", "src/lib.rs", 21, "assertion `left == right` failed"},
			{"tests::divides", "src/lib.rs", 27, "attempt to divide by zero"},
		}}},
		{"cargo_pass.txt", "cargo", Summary{Passed: 3}},
		{"playwright_fail.txt", "playwright", Summary{Passed: 2, Failed: 1, Skipped: 1, Failures: []Failure{
			{"[chromium] login › rejects bad password", "tests/login.spec.ts", 14, "Error: Timed out 5000ms waiting for expect(locator).toBeVisible()"},
		}}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			out := fixture(t, tc.file)
			want := tc.want
			want.Runner = tc.runner
			got, ok := Parse(out, "auto", nil)
			require.True(t, ok, "detected")
			require.Equal(t, want, got)
			got, ok = Parse(out, tc.runner, nil)
			require.True(t, ok, "named")
			require.Equal(t, want, got)
		})
	}
}

// TestJestConsoleBlock: the "● Console" block of a suite's console output
// is not a failure, and its frames are not a failure's location (3.4.2).
func TestJestConsoleBlock(t *testing.T) {
	out := strings.Replace(string(fixture(t, "jest_fail.txt")), " FAIL  src/cart.test.js\n",
		" FAIL  src/cart.test.js\n  ● Console\n\n    console.log src/cart.test.js:31\n      three ok\n\n", 1)
	s, ok := Parse([]byte(out), "jest", nil)
	require.True(t, ok)
	require.Len(t, s.Failures, 2)
	require.Equal(t, Failure{"Cart › applies discount", "src/cart.test.js", 14, "expect(received).toBe(expected) // Object.is equality"}, s.Failures[0])
}

func TestParseUnknownAndWrongRunner(t *testing.T) {
	_, ok := Parse(fixture(t, "unknown.txt"), "auto", nil)
	require.False(t, ok)
	_, ok = Parse(fixture(t, "pytest_fail.txt"), "jest", nil)
	require.False(t, ok)
	_, ok = Parse(fixture(t, "pytest_fail.txt"), "nope", nil)
	require.False(t, ok)
	require.Equal(t, []string{"checking links... 2 broken", "make: *** [check] Error 1"}, Tail(fixture(t, "unknown.txt"), 2))
	require.Equal(t, []string{}, Tail([]byte("\n\n"), 3))
}

func TestHintAndClean(t *testing.T) {
	for argv, want := range map[string][]string{
		"pest":       {"vendor/bin/pest", "--filter", "x"},
		"phpunit":    {"php", "vendor/bin/phpunit"},
		"vitest":     {"npx", "vitest", "run"},
		"pytest":     {"python3", "-m", "pytest", "-q"},
		"go":         {"go", "test", "./..."},
		"cargo":      {"cargo", "test"},
		"playwright": {"npx", "playwright", "test"},
		"":           {"npm", "test"},
	} {
		if argv == "pest" {
			require.Equal(t, "pest", Hint([]string{"php", "artisan", "test"}))
		}
		require.Equal(t, argv, Hint(want), want)
	}
	require.Equal(t, "", Hint([]string{"go", "build"}))
	require.Equal(t, "ok\nB\n", Clean([]byte("\x1b[32mok\x1b[0m\r\nA\rB\n")))
	// The JSON reporter of vitest is jest's shape; the command names it.
	s, ok := Parse(fixture(t, "jest_json.txt"), "auto", []string{"npx", "vitest", "run", "--reporter=json"})
	require.True(t, ok)
	require.Equal(t, "vitest", s.Runner)
	// A colored run parses as the plain one.
	s, ok = Parse([]byte("\x1b[1m  Tests:    \x1b[31m1 failed\x1b[39m, \x1b[32m1 passed\x1b[39m (2 assertions)\x1b[0m\n"), "auto", nil)
	require.True(t, ok)
	require.Equal(t, Summary{Runner: "pest", Passed: 1, Failed: 1}, s)
}
