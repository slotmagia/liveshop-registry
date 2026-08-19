package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/liveshop-platform/contracts/modulemanifest"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/biz"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/biz/model"
)

const (
	registryReadTimeout  = 5 * time.Second
	registryWriteTimeout = 10 * time.Second
)

type ReleaseRepository struct{ db *sql.DB }

var _ biz.ReleaseRepository = (*ReleaseRepository)(nil)

// NewReleaseRepository fails fast when the registry state row is missing, so a
// process never starts against an unmigrated control-plane database.
func NewReleaseRepository(ctx context.Context, db *sql.DB) (*ReleaseRepository, error) {
	if db == nil {
		return nil, errors.New("mysql: database is required")
	}
	if _, err := loadRegistryState(ctx, db, false); err != nil {
		return nil, err
	}
	return &ReleaseRepository{db: db}, nil
}

func (r *ReleaseRepository) Snapshot(ctx context.Context) (*model.RegistryState, error) {
	ctx, cancel := context.WithTimeout(ctx, registryReadTimeout)
	defer cancel()
	return loadRegistryState(ctx, r.db, false)
}

func (r *ReleaseRepository) Register(ctx context.Context, manifest modulemanifest.Manifest) (string, error) {
	var digest string
	err := transaction(ctx, r.db, registryWriteTimeout, func(ctx context.Context, tx *sql.Tx) error {
		state, err := loadRegistryState(ctx, tx, true)
		if err != nil {
			return err
		}
		digest, err = state.Register(manifest)
		if err != nil {
			return err
		}
		return persistRegistryState(ctx, tx, state)
	})
	return digest, err
}

func (r *ReleaseRepository) Activate(ctx context.Context, actor *model.RegistryAuditActor, moduleID, version string) error {
	return transaction(ctx, r.db, registryWriteTimeout, func(ctx context.Context, tx *sql.Tx) error {
		state, err := loadRegistryState(ctx, tx, true)
		if err != nil {
			return err
		}
		beforeRevision := state.Revision
		if err := state.Activate(moduleID, version); err != nil {
			return err
		}
		// Duplicate activation still runs the aggregate validations above so a
		// deployment cannot preserve an invalid pre-existing active snapshot.
		// Once validated, it remains an idempotent no-op without catalog writes
		// or a duplicate audit event.
		if state.Revision == beforeRevision {
			return nil
		}
		manifest, ok := state.ManifestFor(moduleID, version)
		if !ok {
			return model.ErrReleaseNotFound
		}
		if err := syncPermissionCatalog(ctx, tx, manifest); err != nil {
			return err
		}
		return persistRegistryState(ctx, tx, state)
	})
}

func (r *ReleaseRepository) Deactivate(ctx context.Context, actor *model.RegistryAuditActor, moduleID string) error {
	return transaction(ctx, r.db, registryWriteTimeout, func(ctx context.Context, tx *sql.Tx) error {
		state, err := loadRegistryState(ctx, tx, true)
		if err != nil {
			return err
		}
		if err := state.Deactivate(moduleID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE platform_permission_catalog SET active=FALSE,updated_at=NOW() WHERE module_id=?`, moduleID); err != nil {
			return err
		}
		return persistRegistryState(ctx, tx, state)
	})
}

type rowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadRegistryState(ctx context.Context, query rowQuerier, forUpdate bool) (*model.RegistryState, error) {
	statement := `SELECT revision, releases, active FROM platform_registry_state WHERE id = 1`
	if forUpdate {
		statement += ` FOR UPDATE`
	}
	var revision uint64
	var releasesJSON, activeJSON []byte
	if err := query.QueryRowContext(ctx, statement).Scan(&revision, &releasesJSON, &activeJSON); err != nil {
		return nil, err
	}
	state := &model.RegistryState{
		Revision: revision,
		Releases: map[string]map[string]model.Release{},
		Active:   map[string]string{},
	}
	if err := json.Unmarshal(releasesJSON, &state.Releases); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(activeJSON, &state.Active); err != nil {
		return nil, err
	}
	return state, nil
}

func persistRegistryState(ctx context.Context, tx *sql.Tx, state *model.RegistryState) error {
	releases, err := json.Marshal(state.Releases)
	if err != nil {
		return err
	}
	active, err := json.Marshal(state.Active)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE platform_registry_state SET revision = ?, releases = ?, active = ?, updated_at = NOW() WHERE id = 1`, state.Revision, releases, active)
	if err != nil {
		return err
	}
	// MySQL reports 0 affected rows when every column is unchanged (no
	// CLIENT_FOUND_ROWS). Re-registering the same digest hits that path.
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 1 {
		return nil
	}
	if rows == 0 {
		var id int
		if err := tx.QueryRowContext(ctx, `SELECT id FROM platform_registry_state WHERE id = 1`).Scan(&id); err == nil {
			return nil
		}
	}
	return errors.New("registry state update did not affect exactly one row")
}

// syncPermissionCatalog republishes the permissions owned by the activated
// release and retires the codes that release no longer declares.
func syncPermissionCatalog(ctx context.Context, tx *sql.Tx, manifest modulemanifest.Manifest) error {
	codes := make([]string, 0, len(manifest.Spec.Permissions))
	for _, permission := range manifest.Spec.Permissions {
		codes = append(codes, permission.Code)
		// ON DUPLICATE KEY UPDATE has no conditional WHERE like PostgreSQL's
		// ON CONFLICT DO UPDATE ... WHERE. Keep foreign ownership inert by
		// only rewriting columns when module_id already matches, then verify.
		if _, err := tx.ExecContext(ctx, `INSERT INTO platform_permission_catalog(module_id,permission_code,name,resource_code,action,description,active,release_version,updated_at)
			VALUES(?,?,?,?,?,?,TRUE,?,NOW())
			ON DUPLICATE KEY UPDATE
				name=IF(module_id=VALUES(module_id),VALUES(name),name),
				resource_code=IF(module_id=VALUES(module_id),VALUES(resource_code),resource_code),
				action=IF(module_id=VALUES(module_id),VALUES(action),action),
				description=IF(module_id=VALUES(module_id),VALUES(description),description),
				active=IF(module_id=VALUES(module_id),TRUE,active),
				release_version=IF(module_id=VALUES(module_id),VALUES(release_version),release_version),
				updated_at=IF(module_id=VALUES(module_id),NOW(),updated_at)`,
			manifest.Metadata.ID, permission.Code, permission.Name, permission.Resource, permission.Action, permission.Description, manifest.Metadata.Version); err != nil {
			return err
		}
		var owner string
		if err := tx.QueryRowContext(ctx, `SELECT module_id FROM platform_permission_catalog WHERE permission_code=?`, permission.Code).Scan(&owner); err != nil {
			return err
		}
		if owner != manifest.Metadata.ID {
			return fmt.Errorf("permission %s is owned by another module", permission.Code)
		}
	}
	if len(codes) == 0 {
		_, err := tx.ExecContext(ctx, `UPDATE platform_permission_catalog SET active=FALSE,updated_at=NOW() WHERE module_id=?`, manifest.Metadata.ID)
		return err
	}
	placeholders := make([]string, len(codes))
	args := make([]any, 0, 1+len(codes))
	args = append(args, manifest.Metadata.ID)
	for i, code := range codes {
		placeholders[i] = "?"
		args = append(args, code)
	}
	_, err := tx.ExecContext(ctx, `UPDATE platform_permission_catalog SET active=FALSE,updated_at=NOW() WHERE module_id=? AND permission_code NOT IN (`+strings.Join(placeholders, ",")+`)`, args...)
	return err
}

