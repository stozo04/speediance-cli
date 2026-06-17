package cli

import "fmt"

// Exit codes. GOAL.md §12 freezes two of these as part of the external
// contract: auth failure must be 2, success must be 0. The rest are our own
// convention (generic failure is 1).
const (
	ExitOK         = 0
	ExitFailure    = 1
	ExitAuth       = 2 // AuthError — preserved from the Python tool.
	ExitUsage      = 64
	ExitConfig     = 78 // missing credentials / bad config.
	ExitUnresolved = 65 // bad plan / unresolved exercise id.
)

// ExitError carries an explicit process exit code alongside an error. main
// unwraps it with errors.As to choose the exit status, keeping a single exit
// point (GOAL.md §12) instead of scattering os.Exit calls through commands.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit code %d", e.Code)
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error { return e.Err }

// withCode wraps err so main exits with the given code. A nil err yields nil so
// callers can `return withCode(code, doThing())` without a guard.
func withCode(code int, err error) error {
	if err == nil {
		return nil
	}
	return &ExitError{Code: code, Err: err}
}
