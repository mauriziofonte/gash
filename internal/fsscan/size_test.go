// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import "testing"

// TestParseSize pins the 3.0 size grammar (FS-11, FS-22): the numfmt
// --from=iec values rc.A emulated stay, the pure-bash fallback oddities are
// refused, units are case-insensitive and values above 2^63-1 are refused.
func TestParseSize(t *testing.T) {
	cases := []struct {
		in    string
		ok    bool
		bytes int64
	}{
		{"1024", true, 1024},
		{"0", true, 0},
		{"08", true, 8},
		{"1K", true, 1024},
		{"1M", true, 1048576},
		{"1G", true, 1073741824},
		{"2GiB", true, 2147483648},
		{"20M", true, 20971520},
		{"500K", true, 512000},
		{"1.5K", true, 1536},
		{"0.5K", true, 512},
		{"1.9999K", true, 2048},
		{"1.5G", true, 1610612736},
		{"1E", true, 1152921504606846976},
		{"7E", true, 7 << 60},
		{"1.5", true, 2},
		{"100B", true, 100},
		{"1kB", true, 1024},
		{"1Ki", true, 1024},
		{"1KiB", true, 1024},
		// 3.0: FS-11 fixed: units are case-insensitive.
		{"1kb", true, 1024},
		{"1kib", true, 1024},
		{"5KIB", true, 5120},
		{"9223372036854775807", false, 0}, // 19 digits: above the 15-digit bound
		{"999999999999999", true, 999999999999999},
		// 3.0: FS-11 fixed: the fallback oddities are refused.
		{"1K2", false, 0},
		{"K1", false, 0},
		{"iB5", false, 0},
		{"5i", false, 0},
		{"1 0K", false, 0},
		{" 1K", false, 0},
		// FS-22: above 2^63-1.
		{"8E", false, 0},
		{"8192P", false, 0},
		{"9999999P", false, 0},
		// errors
		{"", false, 0},
		{"nonsense", false, 0},
		{"1.2.3K", false, 0},
		{"1.1234567K", false, 0},
		{"1e3", false, 0},
		{"-1K", false, 0},
		{"١", false, 0},
	}
	for _, c := range cases {
		got, ok := ParseSize(c.in)
		if ok != c.ok || (ok && got != c.bytes) {
			t.Errorf("ParseSize(%q) = %d, %v; want %d, %v", c.in, got, ok, c.bytes, c.ok)
		}
	}
}
