package exitcode

import (
	"errors"
	"flag"
	"os"
)

// Coder is an interface to control what value Get returns.
type Coder interface {
	error
	ExitCode() int
}

// Get gets the exit code associated with an error. Cases:
//
//     nil => 0
//     errors implementing Coder => value returned by ExitCode
//     flag.ErrHelp => 2
//     all other errors => 1
func Get(err error) int {
	if err == nil {
		return 0
	}

	if coder := Coder(nil); errors.As(err, &coder) {
		return coder.ExitCode()
	}

	if errors.Is(err, flag.ErrHelp) {
		return 2
	}

	return 1
}

// Set wraps an error in a Coder, setting its error code.
func Set(err error, code int) error {
	if err == nil {
		return nil
	}
	return coder{err, code}
}

var _ Coder = coder{}

type coder struct {
	error
	int
}

func (co coder) ExitCode() int {
	return co.int
}

func (co coder) Unwrap() error {
	return co.error
}

// Exit is a convenience function that calls os.Exit
// with the exit code associated with err.
func Exit(err error) {
	os.Exit(Get(err))
}
