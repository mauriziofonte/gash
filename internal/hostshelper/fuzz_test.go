// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hostshelper

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

// The desired-block grammar: what ParseBlock accepts holds only loopback
// addresses and valid names, and formats back to the same bytes.
func FuzzParseBlock(f *testing.F) {
	for _, s := range []string{
		"127.0.0.1 shop.test\n",
		"::1 shop.test www.shop.test\n127.0.0.1 a.test\n",
		"127.0.0.1  shop.test\n",
		"127.0.0.2 shop.test\n",
		"127.0.0.1 127.1\n",
		"127.0.0.1 0x7f.1\n",
		"127.0.0.1 -a.test\n",
		"127.0.0.1 a.test",
		"",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		entries, err := ParseBlock(input)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.Address != "127.0.0.1" && e.Address != "::1" {
				t.Fatalf("%q: accepted address %q", input, e.Address)
			}
			for _, n := range e.Names {
				if !ValidHostname(n) || strings.ContainsAny(n, " \n#\t") {
					t.Fatalf("%q: accepted name %q", input, n)
				}
			}
		}
		if out := FormatBlock(entries); !bytes.Equal(out, input) {
			t.Fatalf("%q formats back as %q", input, out)
		}
	})
}

// Splice and Strip touch only the managed region: whatever the hosts file
// holds, a block spliced in is what Extract reads back, splicing again
// changes nothing, and stripping it gives back the other bytes.
func FuzzSplice(f *testing.F) {
	hostsSeeds := []string{
		"",
		"127.0.0.1 localhost\n",
		"127.0.0.1 localhost",
		"127.0.0.1 localhost\n" + BeginMarker + "\n127.0.0.1 old.test\n" + EndMarker + "\n::1 localhost\n",
		BeginMarker + "\n" + EndMarker,
		EndMarker + "\n" + BeginMarker + "\n",
		BeginMarker + "\n" + BeginMarker + "\n" + EndMarker + "\n",
		"# " + BeginMarker + "\n",
		BeginMarker + " \n" + EndMarker + "\n",
	}
	for _, h := range hostsSeeds {
		f.Add([]byte(h), []byte("127.0.0.1 shop.test\n"))
		f.Add([]byte(h), []byte(""))
	}
	f.Fuzz(func(t *testing.T, hosts, input []byte) {
		entries, err := ParseBlock(input)
		if err != nil {
			return
		}
		block := FormatBlock(entries)
		out, err := Splice(hosts, block)
		_, _, hadRegion, rerr := region(hosts)
		if err != nil {
			if rerr == nil {
				t.Fatalf("Splice(%q) failed (%v) on a hosts file whose markers are fine", hosts, err)
			}
			return
		}
		got, found, err := Extract(out)
		if err != nil || !found || !bytes.Equal(got, block) {
			t.Fatalf("Splice(%q, %q) = %q: Extract = %q, %v, %v", hosts, block, out, got, found, err)
		}
		if _, err := ParseBlock(got); err != nil {
			t.Fatalf("the spliced block %q does not parse: %v", got, err)
		}
		again, err := Splice(out, block)
		if err != nil || !bytes.Equal(again, out) {
			t.Fatalf("splicing %q twice: %q, %v", block, again, err)
		}
		stripped, err := Strip(out)
		if err != nil {
			t.Fatalf("Strip(%q): %v", out, err)
		}
		want := hosts
		if hadRegion {
			want, _ = Strip(hosts)
		} else if len(hosts) > 0 && hosts[len(hosts)-1] != '\n' {
			want = append(append([]byte{}, hosts...), '\n')
		}
		if !reflect.DeepEqual(stripped, want) && !(len(stripped) == 0 && len(want) == 0) {
			t.Fatalf("Strip(Splice(%q)) = %q, want %q", hosts, stripped, want)
		}
	})
}
