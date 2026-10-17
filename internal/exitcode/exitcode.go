// Package exitcode maps errors to process exit codes (family pattern from kwd/kzero).
package exitcode

import "errors"

// Exit codes for the kpm CLI (SPEC §12).
const (
	Success = 0 // command succeeded
	Failure = 1 // general failure
	Cluster = 2 // cluster / Metrics API / RBAC failure
	Store   = 3 // empty / insufficient store data (recommend later)
	NoFit   = 4 // the question was answered, and the answer is no (capacity fit)
)

// codedError wraps an error with an explicit exit code.
type codedError struct {
	code int
	err  error
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

// ExitCode returns the wrapped exit code.
func (e *codedError) ExitCode() int { return e.code }

// New wraps err with an explicit exit code.
func New(code int, err error) error {
	return &codedError{code: code, err: err}
}

// Of maps an error to its process exit code. A nil error is Success. If the
// error (or any wrapped cause) carries an ExitCode, that code is returned;
// otherwise Failure.
func Of(err error) int {
	if err == nil {
		return Success
	}
	var c *codedError
	if errors.As(err, &c) && c.code != 0 {
		return c.code
	}
	return Failure
}
