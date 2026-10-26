// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package winbridge

// The contract error codes the bridge raises. They are added to
// internal/contract/codes.go and documented in AGENTS.md section 5.
const (
	// CodeUnavailable: interop is off, or no Windows is reachable. STOP.
	CodeUnavailable = "interop_unavailable"
	// CodeTimeout: a Windows call did not answer in time. RETRY.
	CodeTimeout = "interop_timeout"
	// CodeAccessDenied: Windows refused the read (NTFS ACL, or the data
	// needs an elevated terminal). STOP.
	CodeAccessDenied = "windows_access_denied"
	// CodeElevation: the operation needs an administrator. STOP.
	CodeElevation = "elevation_required"
	// CodeNotFound: the Windows program or object was not found. STOP.
	CodeNotFound = "windows_not_found"
	// CodeBadArgument: an argument failed Gash's validation before any
	// Windows program ran. RETRY.
	CodeBadArgument = "invalid_windows_argument"
)

// Error is a Windows-side failure, carrying the contract code, the detail
// to show and a hint. It never carries a Windows program's raw stderr as
// the code: the code is one of the fixed set above.
type Error struct {
	Code    string
	Details string
	Hint    string
}

func (e *Error) Error() string { return e.Code + ": " + e.Details }
