// Package web provides the Platform HTTP response envelope and the single
// mapping from application errors to HTTP status codes.
package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/biz/model"
	"github.com/lvtuopen-ai/kernel-go/logctx"
)

// RawBody returns the immutable request bytes captured by GoFrame. Registry
// release registration signs and canonicalizes this exact document.
func RawBody(ctx context.Context) []byte {
	request := ghttp.RequestFromCtx(ctx)
	if request == nil {
		return nil
	}
	return append([]byte(nil), request.GetBody()...)
}

type HTTPError struct {
	Status int
	Cause  error
	// Internal carries the disclosed-to-logs-only origin of the failure.
	Internal error
}

func (e *HTTPError) Error() string { return e.Cause.Error() }
func (e *HTTPError) Unwrap() error { return e.Cause }

// Error states the status explicitly. Use it only where the transport itself
// decides the outcome, such as a missing cookie.
func Error(status int, err error) error {
	if err == nil {
		err = errors.New(http.StatusText(status))
	}
	return &HTTPError{Status: status, Cause: err}
}

// Failure maps an application error to its transport status. Anything that is
// not a known domain outcome becomes an unavailable dependency so storage and
// driver details never reach the client.
func Failure(err error) error {
	if err == nil {
		return nil
	}
	var httpError *HTTPError
	if errors.As(err, &httpError) {
		return err
	}
	if status, ok := StatusFor(err); ok {
		return &HTTPError{Status: status, Cause: err}
	}
	return &HTTPError{Status: http.StatusServiceUnavailable, Cause: model.ErrUnavailable, Internal: err}
}

type noContent interface{ NoContent() bool }
type noData interface{ NoData() bool }

func ResponseHandler(request *ghttp.Request) {
	request.Middleware.Next()
	if request.Response.BufferLength() > 0 {
		return
	}
	if err := request.GetError(); err != nil {
		status := http.StatusInternalServerError
		var httpError *HTTPError
		if errors.As(err, &httpError) {
			status = httpError.Status
			if httpError.Internal != nil {
				logctx.FromContext(request.GetCtx()).Error("registry request failed",
					"status", status, "method", request.Method, "path", request.URL.Path, "error", httpError.Internal)
			}
		}
		WriteFailure(request, status, err)
		return
	}
	data := request.GetHandlerResponse()
	if marker, ok := data.(noContent); ok && marker.NoContent() {
		request.Response.WriteStatus(http.StatusNoContent)
		return
	}
	request.Response.Header().Set("Content-Type", "application/json; charset=utf-8")
	if marker, ok := data.(noData); ok && marker.NoData() {
		request.Response.WriteJson(map[string]any{"code": 0})
		return
	}
	request.Response.WriteJson(map[string]any{"code": 0, "data": data})
}

func WriteFailure(request *ghttp.Request, status int, err error) {
	request.Response.ClearBuffer()
	request.Response.Header().Set("Content-Type", "application/json; charset=utf-8")
	request.Response.WriteHeader(status)
	message, reason, args := localizeFailure(request, err)
	response := map[string]any{"code": status * 100, "message": message}
	if reason != "" {
		response["reason"] = reason
	}
	if len(args) > 0 {
		response["args"] = args
	}
	request.Response.WriteJson(response)
}
