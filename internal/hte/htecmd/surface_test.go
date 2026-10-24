// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package htecmd

import (
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestLedger_HTE53: the command tree and the flags of gash hte are the
// table of design/hte.md 9, with no compatibility layer for hte-cli
// (option a): no fix command, no --hosts, --interactive, --no-normalize
// or --dry-run. Every command takes --json and --help. 3.0: F-01 fixed:
// site create and modify take --http2 as well as --no-http2.
func TestLedger_HTE53(t *testing.T) {
	site := []string{"alias", "docroot", "domain", "extra-http-port", "extra-https-port", "force-https",
		"force-https-bypass", "http2", "https", "no-force-https", "no-force-https-bypass", "no-http2", "no-https", "no-www",
		"php", "profile"}
	want := map[string][]string{
		"hte":                        nil,
		"hte install":                {"autostart", "no-restore", "ports", "restore-stage", "yes"},
		"hte uninstall":              {"yes"},
		"hte doctor":                 nil,
		"hte migrate":                {"apply", "exclude", "keep-pool-values", "reissue-certs", "site", "yes"},
		"hte site":                   nil,
		"hte site create":            site,
		"hte site modify":            site,
		"hte site remove":            {"domain", "yes"},
		"hte site list":              nil,
		"hte site show":              {"domain"},
		"hte site enable":            {"domain"},
		"hte site disable":           {"domain"},
		"hte php":                    nil,
		"hte php versions":           nil,
		"hte php ini":                nil,
		"hte php ini get":            {"domain"},
		"hte php ini set":            {"domain"},
		"hte php ini unset":          {"domain"},
		"hte php global":             nil,
		"hte php global get":         {"version"},
		"hte php global set":         {"version"},
		"hte php global unset":       {"version"},
		"hte apache":                 nil,
		"hte apache config":          nil,
		"hte apache config path":     nil,
		"hte apache modules":         nil,
		"hte apache modules list":    nil,
		"hte apache modules enable":  nil,
		"hte apache modules disable": nil,
		"hte service":                nil,
		"hte service start":          nil,
		"hte service stop":           nil,
		"hte service restart":        nil,
		"hte service reload":         nil,
		"hte service status":         nil,
		"hte service logs":           {"lines", "site"},
		"hte cert":                   nil,
		"hte cert show":              {"domain"},
		"hte cert renew":             {"domain"},
		"hte hosts":                  nil,
		"hte hosts list":             nil,
		"hte hosts apply":            nil,
		"hte hosts check":            nil,
		"hte hosts windows":          nil,
	}
	// The hidden commands the system runs (sudo, the unit, gash init bash).
	hidden := []string{"hte __autostart", "hte __root", "hte __unit"}

	got := map[string][]string{}
	var gotHidden []string
	var walk func(c *cobra.Command, path string)
	walk = func(c *cobra.Command, path string) {
		if c.Hidden {
			gotHidden = append(gotHidden, path)
			return
		}
		var fl []string
		c.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) { fl = append(fl, f.Name) })
		got[path] = fl
		if c.Flags().Lookup("json") == nil && c.InheritedFlags().Lookup("json") == nil {
			t.Errorf("%s: no --json", path)
		}
		for _, s := range c.Commands() {
			walk(s, path+" "+s.Name())
		}
	}
	walk((&run{}).tree(), "hte")

	for _, p := range slices.Sorted(maps.Keys(want)) {
		g, ok := got[p]
		if !ok {
			t.Errorf("missing command %q", p)
			continue
		}
		if !slices.Equal(g, want[p]) {
			t.Errorf("%s: flags %v, want %v", p, g, want[p])
		}
	}
	for _, p := range slices.Sorted(maps.Keys(got)) {
		if _, ok := want[p]; !ok {
			t.Errorf("command %q is not in the design table", p)
		}
	}
	slices.Sort(gotHidden)
	if !slices.Equal(gotHidden, hidden) {
		t.Errorf("hidden commands %v, want %v", gotHidden, hidden)
	}
	// hte-cli's commands and flags have no counterpart: each is a usage
	// error naming the command's help.
	for _, args := range [][]string{
		{"fix", "--domain", "a.test"},
		{"create", "--domain=a.test"},
		{"details"},
		{"site", "create", "--domain", "a.test", "--docroot", "/srv", "--hosts=no"},
		{"site", "modify", "--domain", "a.test", "--interactive"},
		{"site", "remove", "--domain", "a.test", "--no-normalize"},
		{"site", "remove", "--domain", "a.test", "--force"},
		{"site", "create", "--domain", "a.test", "--docroot", "/srv", "--dry-run"},
	} {
		stdout, stderr, status := call(nil, args...)
		if status != 1 || stdout != "" ||
			!(strings.HasPrefix(stderr, `{"error":"unknown_option"`) || strings.HasPrefix(stderr, `{"error":"unexpected_argument"`)) {
			t.Errorf("%v: status %d stdout %q stderr %q", args, status, stdout, stderr)
		}
	}
}

// TestFollowup_F01: --http2 turns HTTP/2 back on where --no-http2 turned
// it off; neither leaves it as it is; both together contradict.
func TestFollowup_F01(t *testing.T) {
	parse := func(args ...string) (*bool, error) {
		c := &cobra.Command{Use: "modify"}
		sf := &siteFlags{}
		(&run{}).addSiteFlags(c, sf)
		if err := c.Flags().Parse(args); err != nil {
			t.Fatal(err)
		}
		f, err := sf.flags(c)
		return f.HTTP2, err
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--http2"}, "true"}, {[]string{"--no-http2"}, "false"}, {nil, "unset"},
	} {
		v, err := parse(tc.args...)
		got := "unset"
		if v != nil {
			got = strconv.FormatBool(*v)
		}
		if err != nil || got != tc.want {
			t.Errorf("%v: %s %v, want %s", tc.args, got, err, tc.want)
		}
	}
	if _, err := parse("--http2", "--no-http2"); err == nil || !strings.Contains(err.Error(), "contradict") {
		t.Errorf("--http2 --no-http2: %v", err)
	}
}

// TestFollowup_F02: every visible command of gash hte has its long help,
// every help text belongs to a command, and a flag a text names is one
// the command, its parents or its subcommands have.
func TestFollowup_F02(t *testing.T) {
	flagRE := regexp.MustCompile(`--[a-z0-9][a-z0-9-]*`)
	seen := map[string]bool{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Hidden {
			return
		}
		path := c.CommandPath()
		seen[path] = true
		if strings.TrimSpace(c.Long) == "" || c.Long != helpLong[path] {
			t.Errorf("%s: no long help", path)
		}
		has := func(name string) bool {
			if c.Flags().Lookup(name) != nil || c.InheritedFlags().Lookup(name) != nil {
				return true
			}
			found := false
			var sub func(*cobra.Command)
			sub = func(d *cobra.Command) {
				if d.Flags().Lookup(name) != nil {
					found = true
				}
				for _, e := range d.Commands() {
					sub(e)
				}
			}
			sub(c)
			return found
		}
		for _, f := range flagRE.FindAllString(c.Long, -1) {
			if !has(f[2:]) {
				t.Errorf("%s: the help names %s, which it does not have", path, f)
			}
		}
		for _, s := range c.Commands() {
			walk(s)
		}
	}
	walk((&run{}).tree())
	for p := range helpLong {
		if !seen[p] {
			t.Errorf("help text for %q, which is not a visible command", p)
		}
	}
}
