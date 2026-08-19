package service

import (
	"context"

	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/biz/model"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/controlplane/provisioning/appmodel"
)

type Provisioning interface {
	RegisterRelease(ctx context.Context, document []byte) (appmodel.RegisteredRelease, error)
	Activate(ctx context.Context, activation appmodel.Activation) error
	Deactivate(ctx context.Context, moduleID string) error
	Routes(ctx context.Context) (appmodel.RouteSnapshot, error)
	Modules(ctx context.Context) ([]model.ModuleInfo, error)
	Capabilities(ctx context.Context) (appmodel.CapabilityCatalog, error)
	ActiveCapabilities(ctx context.Context) (appmodel.ActiveCapabilitySnapshot, error)
}
