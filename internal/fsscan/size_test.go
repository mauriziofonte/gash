// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import "testing"

// TestParseSize pins the size grammar against the live 2.x
// __gash_fs_parse_size oracle (numfmt --from=iec, then the pure-bash
// fallback). These replace the INTERNAL parse_size spec tests, which cannot
// reach the Go binary (files_spec.sh calls the private Bash helper).
func TestParseSize(t *testing.T) {
	cases := []struct {
		in      string
		ok      bool
		bytes   int64
		big     bool
		wrapped bool
	}{
		{"1024", true, 1024, false, false},
		{"1K", true, 1024, false, false},
		{"1M", true, 1048576, false, false},
		{"1G", true, 1073741824, false, false},
		{"2GiB", true, 2147483648, false, false},
		{"20M", true, 20971520, false, false},
		{"500K", true, 512000, false, false},
		{"1.5K", true, 1536, false, false},
		{"0.5K", true, 512, false, false},
		{"1.9999K", true, 2048, false, false},
		{"1.5G", true, 1610612736, false, false},
		{"1E", true, 1152921504606846976, false, false},
		// fallback cases (numfmt refuses, pure-bash takes over)
		{"1K2", true, 12288, false, false},
		{"K1", true, 1024, false, false},
		{"iB5", true, 5, false, false},
		{"5i", true, 5, false, false},
		{"1 0K", true, 10240, false, false},
		{"1kB", true, 1024, false, false},
		{"1Ki", true, 1024, false, false},
		{"1KiB", true, 1024, false, false},
		// overflow classifications (FS-22)
		{"8E", true, maxInt64, true, false},
		{"9999999P", true, maxInt64, true, true},
		// errors
		{"", false, 0, false, false},
		{"5KIB", false, 0, false, false},
		{"1kb", false, 0, false, false},
		{"nonsense", false, 0, false, false},
	}
	for _, c := range cases {
		got, ok := ParseSize(c.in)
		if ok != c.ok {
			t.Errorf("ParseSize(%q) ok = %v, want %v", c.in, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if got.Bytes != c.bytes || got.Big != c.big || got.Wrapped != c.wrapped {
			t.Errorf("ParseSize(%q) = %+v, want bytes=%d big=%v wrapped=%v",
				c.in, got, c.bytes, c.big, c.wrapped)
		}
	}
}
