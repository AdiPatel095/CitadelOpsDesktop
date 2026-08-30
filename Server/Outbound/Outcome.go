package Outbound

import (
	"errors"
	"fmt"
)

var ErrIndeterminate = errors.New("outbound effect outcome is indeterminate")

type IndeterminateError struct {
	Cause error
}

func (err *IndeterminateError) Error() string {
	if err == nil || err.Cause == nil {
		return ErrIndeterminate.Error()
	}
	return fmt.Sprintf("%s: %v", ErrIndeterminate, err.Cause)
}

func (err *IndeterminateError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func (err *IndeterminateError) Is(target error) bool {
	return target == ErrIndeterminate
}

func MarkIndeterminate(err error) error {
	if err == nil || IsIndeterminate(err) {
		return err
	}
	return &IndeterminateError{Cause: err}
}

func IsIndeterminate(err error) bool {
	return errors.Is(err, ErrIndeterminate)
}
