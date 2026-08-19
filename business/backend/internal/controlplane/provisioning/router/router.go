// Package router registers the provisioning surface transports. Both the HTTP
// group and the gRPC service are mounted here, never by a controller.
package router

import (
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/common/middleware"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/common/web"
	provisioninggrpc "github.com/lvtuopen-ai/liveshop-registry/backend/internal/controlplane/provisioning/controller/grpc"
	provisioninghttp "github.com/lvtuopen-ai/liveshop-registry/backend/internal/controlplane/provisioning/controller/http"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/controlplane/provisioning/service"
	"github.com/lvtuopen-ai/kernel-go/workloadidentity"
	grpclib "google.golang.org/grpc"
)

const Prefix = "/internal/v1/module-registry"

type Deps struct {
	Application service.Provisioning
	Workloads   *workloadidentity.Verifier
}

// RegisterHTTP binds each controller under the single workload permission that
// authorizes it; read and write capabilities never share a group.
func RegisterHTTP(root *ghttp.RouterGroup, deps Deps) {
	bind := func(permission string, target any) {
		root.Group(Prefix, func(group *ghttp.RouterGroup) {
			group.Middleware(web.ResponseHandler)
			group.Middleware(middleware.Workload(deps.Workloads, permission))
			group.Bind(target)
		})
	}
	bind("registry.release.write", provisioninghttp.NewRelease(deps.Application))
	bind("registry.activation.write", provisioninghttp.NewActivation(deps.Application))
	bind("registry.routes.read", provisioninghttp.NewRoutes(deps.Application))
}

// RegisterGRPC mounts the published registry service. Peer authorization is
// enforced by the mTLS/SPIFFE interceptor installed by the composition root.
func RegisterGRPC(server grpclib.ServiceRegistrar, application service.Provisioning) {
	provisioninggrpc.Register(server, application)
}
