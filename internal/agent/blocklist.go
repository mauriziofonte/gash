// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

import (
	"github.com/mauriziofonte/gash/internal/lazyre"
)

// writeBlocklist is what no agent_* command line may do, whatever the
// target: wipe a root or a home, change the owner or mode of a whole
// filesystem, write a disk or kernel memory, format, stop the machine,
// pipe a download into a shell, read the shadow file or a private key,
// tamper with the history. It is llm_exec's dangerous-command list with
// each root pattern anchored, so that a write below a root, the daily
// work of a write tool (chown -R www-data /var/www/storage, rm -rf
// /var/www/cache), is not taken for the root itself.
var writeBlocklist = []*lazyre.Regexp{
	lazyre.MustCompile(`(^|[\s;&|(])rm\s+(-[a-zA-Z-]*\s+)*(/|/\*|/\.\*|~|~/|~/\*|\$HOME|\$\{HOME\}|\*)(\s|$|;)`),
	lazyre.MustCompile(`(^|[\s;&|(])ch(own|mod|grp)\s+(-[a-zA-Z]*\s+)*-[a-zA-Z]*R[a-zA-Z]*\s+(-[a-zA-Z]*\s+)*(\S+\s+)?(/|/\*)(\s|$|;)`),
	lazyre.MustCompile(`(^|[\s;&|(])ch(own|mod)\s+(-[a-zA-Z]*\s+)*(\S+\s+)(/)(\s|$|;)`),
	lazyre.MustCompile(`(^|[\s;&|(])mkfs(\.|\s|$)`),
	lazyre.MustCompile(`(^|[\s;&|(])dd\s.*\bof=/dev/`),
	lazyre.MustCompile(`>\s*/dev/(sd|nvme|vd|hd|xvd|mmcblk|mem|kmem|port)`),
	lazyre.MustCompile(`:\(\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`),
	lazyre.MustCompile(`(^|[\s;&|(])(shutdown|reboot|halt|poweroff)(\s|$|;)`),
	lazyre.MustCompile(`(^|[\s;&|(])(init|telinit)\s+[06](\s|$|;)`),
	lazyre.MustCompile(`(^|[\s;&|(])(curl|wget)\s.*\|\s*(sudo\s+)?(ba|z|da)?sh(\s|$)`),
	lazyre.MustCompile(`/etc/shadow`),
	lazyre.MustCompile(`\.ssh/id_`),
	lazyre.MustCompile(`(^|[\s;&|(])history\s+-[cw]`),
	lazyre.MustCompile(`(^|[\s;&|(])HISTFILE=`),
}

// writeBlocked reports whether line matches writeBlocklist.
func writeBlocked(line string) bool {
	for _, re := range writeBlocklist {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}
