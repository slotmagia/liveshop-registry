package memory

import (
	"context"
	"sync"

	"github.com/liveshop-platform/contracts/modulemanifest"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/biz"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/biz/model"
)

type ReleaseRepository struct {
	mu    sync.RWMutex
	state *model.RegistryState
}

var _ biz.ReleaseRepository = (*ReleaseRepository)(nil)

func NewReleaseRepository() *ReleaseRepository {
	return &ReleaseRepository{state: model.NewRegistryState()}
}

// Snapshot copies the aggregate maps so a reader can project the state without
// holding the write lock. Manifests inside a release are immutable.
func (r *ReleaseRepository) Snapshot(context.Context) (*model.RegistryState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	copied := &model.RegistryState{
		Revision: r.state.Revision,
		Releases: make(map[string]map[string]model.Release, len(r.state.Releases)),
		Active:   make(map[string]string, len(r.state.Active)),
	}
	for moduleID, versions := range r.state.Releases {
		clone := make(map[string]model.Release, len(versions))
		for version, release := range versions {
			clone[version] = release
		}
		copied.Releases[moduleID] = clone
	}
	for moduleID, version := range r.state.Active {
		copied.Active[moduleID] = version
	}
	return copied, nil
}

func (r *ReleaseRepository) Register(_ context.Context, manifest modulemanifest.Manifest) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state.Register(manifest)
}

func (r *ReleaseRepository) Activate(_ context.Context, _ *model.RegistryAuditActor, moduleID, version string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state.Activate(moduleID, version)
}

func (r *ReleaseRepository) Deactivate(_ context.Context, _ *model.RegistryAuditActor, moduleID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state.Deactivate(moduleID)
}
