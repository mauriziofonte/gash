// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package winbridge

import "strings"

// Script is one fixed, versioned PowerShell script Gash sends to Windows.
// The body reads the caller's arguments from $a (the JSON object Gash puts
// on standard input) and assigns the value to serialize to $result; the
// wrapper forces UTF-8 output and prints $result as compact JSON at Depth.
//
// A script holds only read verbs (Get-*, queries, conversions). The test
// TestScriptsReadOnly checks every registered script against writeVerbs,
// so a write cmdlet can never reach Windows through the bridge: writes to
// Windows exist only in the human win fix path, behind a UAC prompt.
type Script struct {
	Name  string
	Depth int
	Body  string
}

// scriptText wraps a script body in the fixed preamble and the
// ConvertTo-Json tail.
func scriptText(s Script) string {
	depth := s.Depth
	if depth <= 0 {
		depth = 4
	}
	var b strings.Builder
	b.WriteString("$ErrorActionPreference='Stop';")
	b.WriteString("$ProgressPreference='SilentlyContinue';")
	b.WriteString("[Console]::OutputEncoding=[Text.Encoding]::UTF8;")
	b.WriteString("$__in=[Console]::In.ReadToEnd();")
	b.WriteString("$a=if($__in){$__in|ConvertFrom-Json}else{[pscustomobject]@{}};")
	b.WriteString("$result=$null;")
	b.WriteString(s.Body)
	b.WriteString(";ConvertTo-Json -InputObject $result -Compress -Depth ")
	b.WriteString(itoa(depth))
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}

// Registry is every PowerShell script the bridge can run, for the
// read-only audit test.
var Registry = []Script{}

// register adds s to the registry and returns it, so a script is declared
// and audited in one place.
func register(s Script) Script {
	Registry = append(Registry, s)
	return s
}

// writeVerbs are PowerShell verbs and operators that change state. The
// audit test rejects any script whose body contains one as a word.
var writeVerbs = []string{
	"Set-", "New-", "Remove-", "Add-", "Clear-", "Rename-", "Move-", "Copy-",
	"Start-", "Stop-", "Restart-", "Suspend-", "Resume-", "Disable-", "Enable-",
	"Install-", "Uninstall-", "Register-", "Unregister-", "Write-", "Out-File",
	"Export-", "Import-", "Invoke-Expression", "iex", "Invoke-Command", "icm",
	"Invoke-WebRequest", "iwr", "Invoke-RestMethod", "irm",
}
