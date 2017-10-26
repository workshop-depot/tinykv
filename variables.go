package tinykv

import (
	"time"

	"github.com/dc0d/errgo/sentinel"
)

// constants
const (
	DefaultTimeout = time.Minute * 3
)

// errors
var (
	// ErrNotFound             = sentinel.Errorf("NOT_FOUND")
	ErrCASCond              = sentinel.Errorf("CAS_COND_FAILED")
	ErrNegativeExpiresAfter = sentinel.Errorf("ExpiresAfter MUST BE GREATER THAN ZERO")
)
