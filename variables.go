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
	ErrCASCond = sentinel.Errorf("CAS_COND_FAILED")
)
