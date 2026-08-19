package model

import "github.com/lvtuopen-ai/kernel-go/apperror"

// ErrUnavailable marks a capability whose backing dependency was not
// assembled. Transports translate it into their own unavailability signal.
var ErrUnavailable = apperror.New("platform.dependency_unavailable", "a platform dependency is unavailable")
