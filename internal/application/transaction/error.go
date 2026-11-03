package transaction

import "errors"

// ErrTimeout is returned when the host does not answer before the deadline.
var ErrTimeout = errors.New("host did not respond in time")
