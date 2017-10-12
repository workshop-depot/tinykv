package tinykv

import (
	"fmt"
)

//-----------------------------------------------------------------------------

// errorf value type (string) error
func errorf(format string, a ...interface{}) error {
	return errString(fmt.Sprintf(format, a...))
}

//-----------------------------------------------------------------------------
