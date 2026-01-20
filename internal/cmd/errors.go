package cmd

import "fmt"

// RuntimeError marks errors that should map to exit code 2.
type RuntimeError struct {
	Err error
}

func (e RuntimeError) Error() string {
	if e.Err == nil {
		return "runtime error"
	}
	return e.Err.Error()
}

func (e RuntimeError) Unwrap() error {
	return e.Err
}

// NewRuntimeError wraps an error so callers can signal a runtime exit code.
func NewRuntimeError(err error) error {
	if err == nil {
		return nil
	}
	return RuntimeError{Err: fmt.Errorf("%w", err)}
}
