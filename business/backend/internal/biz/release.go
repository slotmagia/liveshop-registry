package biz

import (
	"context"
	"fmt"
	"strings"

	"github.com/liveshop-platform/contracts/modulemanifest"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/biz/model"
)

// ReleaseRepository loads and mutates the module registry aggregate. Reads go
// through Snapshot so every projection is derived from one consistent state.
type ReleaseRepository interface {
	Snapshot(ctx context.Context) (*model.RegistryState, error)
	Register(ctx context.Context, manifest modulemanifest.Manifest) (string, error)
	Activate(ctx context.Context, actor *model.RegistryAuditActor, moduleID, version string) error
	Deactivate(ctx context.Context, actor *model.RegistryAuditActor, moduleID string) error
}

type Release struct{ repository ReleaseRepository }

func NewRelease(repository ReleaseRepository) *Release { return &Release{repository: repository} }

// RegisterManifest accepts the raw manifest document published by the release
// pipeline. Decoding stays here so the transport never owns the wire format.
func (a *Release) RegisterManifest(ctx context.Context, document []byte) (string, error) {
	manifest, err := modulemanifest.Decode(document)
	if err != nil {
		return "", fmt.Errorf("%w: %s", model.ErrReleaseInvalid, err.Error())
	}
	return a.repository.Register(ctx, manifest)
}

func (a *Release) Activate(ctx context.Context, moduleID, version string) error {
	if strings.TrimSpace(moduleID) == "" || strings.TrimSpace(version) == "" {
		return model.ErrReleaseInvalid
	}
	return a.repository.Activate(ctx, nil, moduleID, version)
}

func (a *Release) ActivateAudited(ctx context.Context, actor model.RegistryAuditActor, moduleID, version string) error {
	if strings.TrimSpace(moduleID) == "" || strings.TrimSpace(version) == "" {
		return model.ErrReleaseInvalid
	}
	return a.repository.Activate(ctx, &actor, moduleID, version)
}

func (a *Release) Deactivate(ctx context.Context, moduleID string) error {
	if moduleID == model.PlatformModuleID {
		return model.ErrPlatformSelfDeactivation
	}
	if strings.TrimSpace(moduleID) == "" {
		return model.ErrReleaseInvalid
	}
	return a.repository.Deactivate(ctx, nil, moduleID)
}

// DeactivateAudited refuses to remove the control-plane module, whose own
// route serves the request performing the change.
func (a *Release) DeactivateAudited(ctx context.Context, actor model.RegistryAuditActor, moduleID string) error {
	if moduleID == model.PlatformModuleID {
		return model.ErrPlatformSelfDeactivation
	}
	if strings.TrimSpace(moduleID) == "" {
		return model.ErrReleaseInvalid
	}
	return a.repository.Deactivate(ctx, &actor, moduleID)
}

func (a *Release) Routes(ctx context.Context) (uint64, []modulemanifest.ActiveRoute, error) {
	state, err := a.repository.Snapshot(ctx)
	if err != nil {
		return 0, nil, err
	}
	revision, routes := state.Routes()
	return revision, routes, nil
}

func (a *Release) Capabilities(ctx context.Context) (uint64, []model.ModuleCapabilityCatalog, error) {
	state, err := a.repository.Snapshot(ctx)
	if err != nil {
		return 0, nil, err
	}
	return state.CapabilityCatalogs()
}

func (a *Release) ActiveCapabilities(ctx context.Context) (uint64, []model.ActiveModuleCapability, error) {
	state, err := a.repository.Snapshot(ctx)
	if err != nil {
		return 0, nil, err
	}
	return state.ActiveCapabilitySnapshot()
}

func (a *Release) Modules(ctx context.Context) ([]model.ModuleInfo, error) {
	state, err := a.repository.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	return state.Modules(), nil
}

func (a *Release) Contributions(ctx context.Context, surface string) (uint64, []modulemanifest.RuntimeContribution, error) {
	state, err := a.repository.Snapshot(ctx)
	if err != nil {
		return 0, nil, err
	}
	revision, items := state.Contributions(surface)
	return revision, items, nil
}

func (a *Release) Contribution(ctx context.Context, moduleID, version, contributionID string) (modulemanifest.Contribution, error) {
	state, err := a.repository.Snapshot(ctx)
	if err != nil {
		return modulemanifest.Contribution{}, err
	}
	return state.Contribution(moduleID, version, contributionID)
}
