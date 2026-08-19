// Package http adapts the provisioning HTTP contract to its application
// boundary. Each controller is bound under its own workload permission.
package http

import (
	"context"

	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/common/web"
	apiregistry "github.com/lvtuopen-ai/liveshop-registry/backend/internal/controlplane/provisioning/api/http/v1/registry"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/controlplane/provisioning/appmodel"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/controlplane/provisioning/service"
)

type ReleaseController struct{ service service.Provisioning }

func NewRelease(application service.Provisioning) *ReleaseController {
	return &ReleaseController{service: application}
}

func (c *ReleaseController) RegisterRelease(ctx context.Context, _ *apiregistry.RegisterReleaseReq) (*apiregistry.RegisterReleaseRes, error) {
	registered, err := c.service.RegisterRelease(ctx, web.RawBody(ctx))
	if err != nil {
		return nil, web.Failure(err)
	}
	return &apiregistry.RegisterReleaseRes{Digest: registered.Digest}, nil
}

type ActivationController struct{ service service.Provisioning }

func NewActivation(application service.Provisioning) *ActivationController {
	return &ActivationController{service: application}
}

func (c *ActivationController) Activate(ctx context.Context, req *apiregistry.ActivateReq) (*apiregistry.ActivateRes, error) {
	if err := c.service.Activate(ctx, appmodel.Activation{ModuleID: req.ModuleID, Version: req.Version}); err != nil {
		return nil, web.Failure(err)
	}
	return &apiregistry.ActivateRes{}, nil
}

type RoutesController struct{ service service.Provisioning }

func NewRoutes(application service.Provisioning) *RoutesController {
	return &RoutesController{service: application}
}

func (c *RoutesController) Routes(ctx context.Context, _ *apiregistry.RoutesReq) (*apiregistry.RoutesRes, error) {
	snapshot, err := c.service.Routes(ctx)
	if err != nil {
		return nil, web.Failure(err)
	}
	return &apiregistry.RoutesRes{Revision: snapshot.Revision, Routes: snapshot.Routes}, nil
}

func (c *RoutesController) Modules(ctx context.Context, _ *apiregistry.ModulesReq) (*apiregistry.ModulesRes, error) {
	items, err := c.service.Modules(ctx)
	if err != nil {
		return nil, web.Failure(err)
	}
	output := make([]apiregistry.ModuleInfo, 0, len(items))
	for _, item := range items {
		info := apiregistry.ModuleInfo{ID: item.ID, Name: item.Name, ActiveVersion: item.ActiveVersion}
		for _, release := range item.Releases {
			info.Releases = append(info.Releases, apiregistry.ReleaseInfo{Version: release.Version, Digest: release.Digest})
		}
		output = append(output, info)
	}
	return &apiregistry.ModulesRes{Items: output}, nil
}

func (c *RoutesController) Capabilities(ctx context.Context, _ *apiregistry.CapabilitiesReq) (*apiregistry.CapabilitiesRes, error) {
	snapshot, err := c.service.Capabilities(ctx)
	if err != nil {
		return nil, web.Failure(err)
	}
	return &apiregistry.CapabilitiesRes{Revision: snapshot.Revision, Items: snapshot.Items}, nil
}

func (c *ActivationController) Deactivate(ctx context.Context, req *apiregistry.DeactivateReq) (*apiregistry.DeactivateRes, error) {
	if err := c.service.Deactivate(ctx, req.ModuleID); err != nil {
		return nil, web.Failure(err)
	}
	return &apiregistry.DeactivateRes{}, nil
}
