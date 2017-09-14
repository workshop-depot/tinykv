package tinykv

import "fmt"

// errString a string that inplements the error interface
type errString string

func (v errString) Error() string { return string(v) }

// errorf value type (string) error
func errorf(format string, a ...interface{}) error {
	return errString(fmt.Sprintf(format, a...))
}
