// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package migrate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// JSON encodes the plan for --json: compact, with &, < and > kept as they
// are (paths and Apache text are not HTML).
func (p *Plan) JSON() ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(p); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// Text renders the plan for a human reader.
func (p *Plan) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "gash hte migrate: plan for %s (dry run: nothing is changed)\n", p.Platform)

	b.WriteString("\nSites\n")
	if len(p.Sites) == 0 {
		b.WriteString("  no vhost found\n")
	}
	for _, s := range p.Sites {
		domain := s.Domain
		if domain == "" {
			domain = "-"
		}
		fmt.Fprintf(&b, "  %-15s %-20s %-13s %s\n", s.Action, domain, s.Class, s.Path)
		detail := func(format string, a ...any) { fmt.Fprintf(&b, "  %-15s "+format+"\n", append([]any{""}, a...)...) }
		if s.Reason != "" {
			detail("%s", s.Reason)
		}
		if st := s.Site; st != nil {
			detail("%s", siteSummary(s))
			if len(st.Aliases) > 0 {
				detail("aliases: %s", strings.Join(st.Aliases, " "))
			}
		}
		if pp := s.Pool; pp != nil && s.Site != nil && pp.Path != "" {
			if pp.Found {
				detail("pool: %s", pp.Path)
			}
			for _, d := range pp.Diffs {
				detail("pool difference: %s = %q (profile: %q)", d.Key, d.Pool, d.Profile)
			}
			if len(pp.Kept) > 0 {
				detail("kept in [php_ini]: %s", strings.Join(pp.Kept, ", "))
			}
			if len(pp.NotKept) > 0 {
				detail("not kept (no php_ini form): %s", strings.Join(pp.NotKept, ", "))
			}
		}
		if c := s.Cert; c != nil {
			switch c.Action {
			case CertCopy:
				detail("certificate: copy %s and %s (the key by the root step)", c.Cert, c.Key)
			default:
				detail("certificate: a new pair in %s", c.Dest)
			}
		}
		for _, n := range s.Notes {
			detail("note: %s", n)
		}
		for _, w := range s.Warnings {
			detail("warning: %s", w)
		}
	}

	b.WriteString("\nHosts block\n")
	if p.Hosts.Block == "" {
		b.WriteString("  (empty)\n")
	}
	for _, line := range strings.Split(strings.TrimSuffix(p.Hosts.Block, "\n"), "\n") {
		if line != "" {
			b.WriteString("  " + line + "\n")
		}
	}
	if n := len(p.Hosts.Legacy); n > 0 {
		var lines []string
		for _, l := range p.Hosts.Legacy {
			lines = append(lines, fmt.Sprint(l.Line))
		}
		fmt.Fprintf(&b, "  the \"# hte-cli\" lines (%s) stay untouched\n", strings.Join(lines, ", "))
	}

	section(&b, "Root steps (one sudo for --apply)", len(p.RootOps), func() {
		for _, op := range p.RootOps {
			fmt.Fprintf(&b, "  %s %s -> %s\n", op.Op, op.Src, op.Dst)
		}
	})
	section(&b, "Reported", len(p.Reports), func() {
		for _, r := range p.Reports {
			fmt.Fprintf(&b, "  %s: %s\n", r.Kind, r.Message)
			for _, path := range r.Paths {
				fmt.Fprintf(&b, "    %s\n", path)
			}
		}
	})
	section(&b, "No longer served after --apply", len(p.NoLongerServed), func() {
		for _, path := range p.NoLongerServed {
			fmt.Fprintf(&b, "  %s\n", path)
		}
	})
	section(&b, "Blocking --apply (import or exclude each one first)", len(p.Blocking), func() {
		for _, path := range p.Blocking {
			fmt.Fprintf(&b, "  %s\n", path)
		}
	})
	return b.String()
}

func section(b *strings.Builder, title string, n int, body func()) {
	if n == 0 {
		return
	}
	b.WriteString("\n" + title + "\n")
	body()
}

func siteSummary(s SitePlan) string {
	st := s.Site
	var parts []string
	if st.PHP != "" {
		parts = append(parts, "PHP "+st.PHP)
		src := ""
		if s.Pool != nil {
			src = " (" + string(s.Pool.Source) + ")"
		}
		parts = append(parts, "profile "+st.Profile+src)
	}
	switch {
	case !st.HTTPS:
		parts = append(parts, "HTTP only")
	case st.ForceHTTPS && st.ForceHTTPSBypassLoopback:
		parts = append(parts, "HTTPS forced (307, loopback bypass on)")
	case st.ForceHTTPS:
		parts = append(parts, "HTTPS forced (307)")
	default:
		parts = append(parts, "HTTPS")
	}
	if st.Enabled {
		parts = append(parts, "enabled")
	} else {
		parts = append(parts, "disabled")
	}
	if len(st.PHPIni) > 0 {
		parts = append(parts, fmt.Sprintf("%d php_ini values", len(st.PHPIni)))
	}
	return strings.Join(parts, ", ")
}
