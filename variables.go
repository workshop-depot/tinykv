package tinykv

import (
	"time"

	"github.com/pkg/errors"
)

// constants
const (
	DefaultTimeout = time.Minute * 3
)

// errors
var (
	ErrCASCond = errors.Errorf("CAS COND FAILED")
)
