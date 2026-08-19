package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/lvtuopen-ai/kernel-go/logctx"
	"github.com/lvtuopen-ai/kernel-go/requestmeta"
	"github.com/lvtuopen-ai/kernel-go/workloadidentity"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/common/authctx"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/common/web"
)

func RequestMetadata(request *ghttp.Request) {
	requestID := requestmeta.Ensure(request.Request)
	request.Response.Header().Set(requestmeta.HeaderRequestID, requestID)
	request.SetCtx(requestmeta.Context(request.GetCtx(), requestID))
	request.Middleware.Next()
}

func CORS(allowedOrigins map[string]struct{}) ghttp.HandlerFunc {
	return func(request *ghttp.Request) {
		origin := request.Header.Get("Origin")
		if origin != "" {
			if _, ok := allowedOrigins[origin]; !ok {
				web.WriteFailure(request, http.StatusForbidden, errors.New("origin is not allowed"))
				request.ExitAll()
				return
			}
			request.Response.Header().Set("Access-Control-Allow-Origin", origin)
			request.Response.Header().Set("Access-Control-Allow-Credentials", "true")
			request.Response.Header().Set("Vary", "Origin")
		}
		request.Response.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Liveshop-Surface,X-Locale")
		request.Response.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		if request.Method == http.MethodOptions {
			request.Response.WriteStatus(http.StatusNoContent)
			request.ExitAll()
			return
		}
		request.Middleware.Next()
	}
}

func ValidateJSON(request *ghttp.Request) {
	body := bytes.TrimSpace(request.GetBody())
	if len(body) > 0 && strings.Contains(strings.ToLower(request.Header.Get("Content-Type")), "application/json") && !json.Valid(body) {
		web.WriteFailure(request, http.StatusBadRequest, errors.New("invalid JSON request body"))
		request.ExitAll()
		return
	}
	request.Middleware.Next()
}

func Workload(verifier *workloadidentity.Verifier, permission string) ghttp.HandlerFunc {
	return func(request *ghttp.Request) {
		token, ok := workloadidentity.Bearer(request.Header.Get("Authorization"))
		if !ok || verifier == nil {
			logctx.FromContext(request.GetCtx()).Warn("workload authorization denied", "workload", "unknown", "permission", permission, "method", request.Method, "path", request.URL.Path, "decision", "deny")
			reject(request, http.StatusUnauthorized, "workload identity is required")
			return
		}
		claims, err := verifier.Authorize(token, permission)
		if err != nil {
			logctx.FromContext(request.GetCtx()).Warn("workload authorization denied", "workload", "unverified", "permission", permission, "method", request.Method, "path", request.URL.Path, "decision", "deny")
			reject(request, http.StatusForbidden, "workload is not authorized")
			return
		}
		logctx.FromContext(request.GetCtx()).Info("workload authorization allowed", "workload", claims.Subject, "permission", permission, "method", request.Method, "path", request.URL.Path, "decision", "allow")
		request.SetCtx(authctx.WithWorkloadSubject(request.GetCtx(), claims.Subject))
		request.Middleware.Next()
	}
}

func reject(request *ghttp.Request, status int, message string) {
	web.WriteFailure(request, status, errors.New(message))
	request.ExitAll()
}
