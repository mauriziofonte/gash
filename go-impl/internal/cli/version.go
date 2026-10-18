// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package cli

import (
	"io"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/mauriziofonte/gash/internal/buildinfo"
	"github.com/mauriziofonte/gash/internal/contract"
)

// versionCmd is `gash version [--json]`: the version, os/arch and the
// path of the binary, used by the update smoke test.
//
//	gash 3.0.0 linux/amd64 /home/u/.local/share/gash/versions/3.0.0/gash
//	{"version":"3.0.0","os":"linux","arch":"amd64","path":"/home/u/..."}
func versionCmd(p *Process, status *int) *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "version",
		Short: "Print the version, platform and path of this binary",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			path, err := p.Executable()
			if err != nil {
				path = ""
			}
			writeVersion(p.Stdout, asJSON, buildinfo.Version, runtime.GOOS, runtime.GOARCH, path)
			*status = 0
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "print one JSON object")
	return c
}

func writeVersion(w io.Writer, asJSON bool, version, goos, goarch, path string) {
	if asJSON {
		var o contract.Object
		o.String("version", version).String("os", goos).String("arch", goarch).String("path", path)
		if b, err := o.Append(nil, contract.EscapeJQ); err == nil {
			_, _ = w.Write(append(b, '\n'))
		}
		return
	}
	_, _ = io.WriteString(w, "gash "+version+" "+goos+"/"+goarch+" "+path+"\n")
}
