package web

import (
	"errors"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/lvtuopen-ai/kernel-go/apperror"
)

func localizeFailure(request *ghttp.Request, err error) (message, reason string, args map[string]any) {
	cause := err
	var httpError *HTTPError
	if errors.As(err, &httpError) && httpError.Cause != nil {
		cause = httpError.Cause
	}
	message = cause.Error()
	applicationError, ok := apperror.As(cause)
	if !ok {
		return message, "", nil
	}
	reason = applicationError.Reason
	if len(applicationError.Args) > 0 {
		args = make(map[string]any, len(applicationError.Args))
		for key, value := range applicationError.Args {
			args[key] = value
		}
	}
	return message, reason, args
}
