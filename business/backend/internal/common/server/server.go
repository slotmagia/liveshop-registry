// Package server is the sole HTTP composition root for Platform surfaces. It
// configures the engine and mounts whatever surfaces it is given; it names no
// surface, owns no business rule and reaches no storage.
package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gogf/gf/v2/net/ghttp"
	commonmw "github.com/lvtuopen-ai/liveshop-registry/backend/internal/common/middleware"
)

var serverSequence atomic.Uint64

// Surface is one security boundary served by this process. Each surface mounts
// its own route groups, so adding one never changes this composition root.
type Surface interface {
	RegisterHTTP(root *ghttp.RouterGroup)
}

type Server struct {
	engine *ghttp.Server
}

type Config struct {
	AllowedOrigins []string
	// Ready reports backing-store readiness; liveness never depends on it.
	Ready func(context.Context) error
}

func New(config Config, surfaces ...Surface) *Server {
	origins := make(map[string]struct{}, len(config.AllowedOrigins))
	for _, origin := range config.AllowedOrigins {
		if origin = strings.TrimSpace(origin); origin != "" {
			origins[origin] = struct{}{}
		}
	}

	engine := ghttp.GetServer(fmt.Sprintf("registry-http-%d", serverSequence.Add(1)))
	engine.SetAddr(":0")
	engine.SetDumpRouterMap(false)
	engine.SetAccessLogEnabled(false)
	engine.SetReadTimeout(15 * time.Second)
	engine.SetWriteTimeout(15 * time.Second)
	engine.SetIdleTimeout(60 * time.Second)
	engine.SetMaxHeaderBytes(1 << 20)
	engine.SetClientMaxBodySize(2 << 20)
	engine.SetGraceful(true)
	engine.SetGracefulShutdownTimeout(10)
	engine.Group("/", func(root *ghttp.RouterGroup) {
		root.Middleware(commonmw.RequestMetadata)
		root.Middleware(commonmw.CORS(origins))
		root.Middleware(commonmw.ValidateJSON)
		registerHealth(root, config.Ready)
		for _, surface := range surfaces {
			surface.RegisterHTTP(root)
		}
	})
	return &Server{engine: engine}
}

func registerHealth(root *ghttp.RouterGroup, ready func(context.Context) error) {
	writeLive := func(request *ghttp.Request) {
		request.Response.Header().Set("Content-Type", "application/json; charset=utf-8")
		request.Response.WriteJson(map[string]string{"status": "ok"})
	}
	writeReady := func(request *ghttp.Request) {
		if ready != nil {
			ctx, cancel := context.WithTimeout(request.GetCtx(), 2*time.Second)
			defer cancel()
			if err := ready(ctx); err != nil {
				request.Response.WriteStatus(http.StatusServiceUnavailable)
				request.Response.WriteJson(map[string]string{"status": "not-ready"})
				return
			}
		}
		writeLive(request)
	}
	root.GET("/livez", writeLive)
	root.GET("/readyz", writeReady)
	root.GET("/health", writeReady)
}

func (s *Server) SetAddr(addr string) { s.engine.SetAddr(addr) }

func (s *Server) Handler() http.Handler { return s.engine }

func (s *Server) Start() error { return s.engine.Start() }

func (s *Server) Shutdown() error { return s.engine.Shutdown() }
