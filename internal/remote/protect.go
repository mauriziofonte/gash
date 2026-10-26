// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"strings"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/pathguard"
)

// The paths the user protects on a host (PROTECT: lines without a host,
// and those for its alias). They are judged three times: here, on the
// path as written, before anything connects (protectedLexical); on the
// host, by the gash binary's own guard (pathguard.ProtectVar carries the
// rules, every script exports it); and by the POSIX fallbacks, on the
// resolved path (gash_forbidden and gash_secret consult ShProtect).

// protectExport is the sh prefix that exports the host's rules for the
// script after it; "" when there are none.
func protectExport(h Host) string {
	if len(h.Rules) == 0 {
		return ""
	}
	return pathguard.ProtectVar + "=" + Quote(pathguard.EncodeRules(h.Rules)) + "; export " + pathguard.ProtectVar + "; "
}

// protectedLexical is the rule of hosts a normalized path (normalizePath:
// ~/x is x, below the remote home) falls in, as written: the mode of the
// first rule it is at or below, "" for none.
func protectedLexical(norm string, hosts []Host) string {
	mode := ""
	for _, h := range hosts {
		for _, r := range h.Rules {
			rn := normalizePath(r.Path)
			in := under(norm, rn) || rn == "/" || (rn == "." && !strings.HasPrefix(norm, "/"))
			if !in {
				continue
			}
			if r.Mode == config.ProtectAll {
				return r.Mode
			}
			mode = r.Mode
		}
	}
	return mode
}

// protectedRead refuses a read of a PROTECT:all path of hosts.
func protectedRead(norm string, hosts []Host) *contract.Envelope {
	if protectedLexical(norm, hosts) == config.ProtectAll {
		e := contract.New("forbidden_path", norm, "This path is protected and cannot be accessed")
		return &e
	}
	return nil
}

// ShProtect are the sh helpers of the host's rules, for gash_forbidden and
// gash_secret: gash_protect_init reads GASH__PROTECT once into GP_ALL and
// GP_CONTENT (each rule resolved on the host, and as written, one per
// line), gash_in LIST PATH succeeds when PATH is at or below an entry,
// gash_above LIST PATH when an entry lies below PATH.
func ShProtect() string {
	return `GP_NL='
'
gash_protect_init() {
  GP_INIT=1; GP_ALL=; GP_CONTENT=
  gp_i=$IFS; IFS=$GP_NL; set -f
  for gp_l in ${` + pathguard.ProtectVar + `-}; do
    gp_m=${gp_l%% *}; gp_p=${gp_l#* }
    case $gp_p in '~') gp_p=$HOME ;; '~/'*) gp_p=$HOME/${gp_p#'~/'} ;; /*) ;; *) continue ;; esac
    gp_r=$(gash_resolve "$gp_p" 2>/dev/null) || gp_r=$gp_p
    gp_p=${gp_p%/}
    case $gp_m in
      all) GP_ALL=$GP_ALL$gp_r$GP_NL${gp_p:-/}$GP_NL ;;
      content) GP_CONTENT=$GP_CONTENT$gp_r$GP_NL${gp_p:-/}$GP_NL ;;
    esac
  done
  set +f; IFS=$gp_i
}
gash_in() {
  gp_j=$IFS; IFS=$GP_NL; set -f
  for gp_t in $1; do
    case $2 in "$gp_t"|"$gp_t"/*) IFS=$gp_j; set +f; return 0 ;; esac
    [ "$gp_t" = / ] && { IFS=$gp_j; set +f; return 0; }
  done
  IFS=$gp_j; set +f; return 1
}
gash_above() {
  gp_j=$IFS; IFS=$GP_NL; set -f
  for gp_t in $1; do
    case $gp_t in "$2"/*) IFS=$gp_j; set +f; return 0 ;; esac
    [ "$2" = / ] && { IFS=$gp_j; set +f; return 0; }
  done
  IFS=$gp_j; set +f; return 1
}
`
}
