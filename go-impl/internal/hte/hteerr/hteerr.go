// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package hteerr is the error type of the HTE packages: a code with its
// fixed action (design/hte.md section 9), details and a hint. It stands in
// for internal/contract until that package exists; the codes and actions
// are the ones the contract catalog registers.
package hteerr

import (
	"errors"
	"fmt"
	"slices"
)

// Action is what the caller should do with an error (AGENTS.md section 5).
type Action string

// The four actions of the Gash error contract.
const (
	Retry    Action = "RETRY"
	Continue Action = "CONTINUE"
	Stop     Action = "STOP"
	Fatal    Action = "FATAL"
)

// Code is a machine-readable error code.
type Code string

// Codes of design/hte.md section 9, plus missing_value of the general
// contract.
const (
	HTENotInstalled        Code = "hte_not_installed"
	SiteNotFound           Code = "site_not_found"
	SiteExists             Code = "site_exists"
	InvalidDomain          Code = "invalid_domain"
	InvalidDocroot         Code = "invalid_docroot"
	DocrootNotFound        Code = "docroot_not_found"
	PHPVersionNotInstalled Code = "php_version_not_installed"
	InvalidProfile         Code = "invalid_profile"
	ConfirmationRequired   Code = "confirmation_required"
	LockTimeout            Code = "lock_timeout"
	PortInUse              Code = "port_in_use"
	PrivilegedStepRequired Code = "privileged_step_required"
	ConfigTestFailed       Code = "config_test_failed"
	ReloadFailed           Code = "reload_failed"
	ApacheNotFound         Code = "apache_not_found"
	ApacheModuleMissing    Code = "apache_module_missing"
	PlatformUnsupported    Code = "platform_unsupported"
	ReceiptMismatch        Code = "receipt_mismatch"
	RootNotAllowed         Code = "root_not_allowed"
	MissingValue           Code = "missing_value"

	// ConflictingOptions is a flag pair that contradicts itself, such as
	// --force-https with --no-https (LEDGER HTE-01).
	ConflictingOptions Code = "conflicting_options"
	// InvalidState is a site TOML or hte.toml that does not parse, has
	// unknown or missing keys, or holds a value the strict validation
	// refuses, and a php_ini value that cannot be written into a pool. A
	// human must fix the file.
	InvalidState Code = "invalid_state"
)

var actions = map[Code]Action{
	HTENotInstalled:        Stop,
	SiteNotFound:           Retry,
	SiteExists:             Retry,
	InvalidDomain:          Retry,
	InvalidDocroot:         Retry,
	DocrootNotFound:        Retry,
	PHPVersionNotInstalled: Retry,
	InvalidProfile:         Retry,
	ConfirmationRequired:   Retry,
	LockTimeout:            Retry,
	PortInUse:              Stop,
	PrivilegedStepRequired: Stop,
	ConfigTestFailed:       Stop,
	ReloadFailed:           Stop,
	ApacheNotFound:         Fatal,
	ApacheModuleMissing:    Stop,
	PlatformUnsupported:    Fatal,
	ReceiptMismatch:        Fatal,
	RootNotAllowed:         Fatal,
	MissingValue:           Retry,
	ConflictingOptions:     Retry,
	InvalidState:           Stop,
}

// ActionOf returns the fixed action of code; an unknown code is STOP, as
// the contract prescribes.
func ActionOf(code Code) Action {
	if a, ok := actions[code]; ok {
		return a
	}
	return Stop
}

// Codes returns every code of the catalog, sorted.
func Codes() []Code {
	out := make([]Code, 0, len(actions))
	for c := range actions {
		out = append(out, c)
	}
	slices.Sort(out)
	return out
}

// Error is one HTE error. Details say what failed (the value, the file),
// Hint what to do; both may be empty.
type Error struct {
	Code    Code
	Details string
	Hint    string
}

// New returns an error with the given code, details and hint.
func New(code Code, details, hint string) *Error {
	return &Error{Code: code, Details: details, Hint: hint}
}

// Newf is New with formatted details.
func Newf(code Code, hint, format string, args ...any) *Error {
	return &Error{Code: code, Details: fmt.Sprintf(format, args...), Hint: hint}
}

// Action returns the fixed action of the error's code.
func (e *Error) Action() Action { return ActionOf(e.Code) }

// Recoverable is false exactly for FATAL.
func (e *Error) Recoverable() bool { return e.Action() != Fatal }

func (e *Error) Error() string {
	if e.Details == "" {
		return string(e.Code)
	}
	return string(e.Code) + ": " + e.Details
}

// As returns the *Error in err's chain.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

// CodeOf returns the code of the *Error in err's chain, or "".
func CodeOf(err error) Code {
	if e, ok := As(err); ok {
		return e.Code
	}
	return ""
}
