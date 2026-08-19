package app

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/lvtuopen-ai/kernel-go/workloadidentity"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/biz"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/config"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/data/mysql"
)

const assemblyTimeout = 10 * time.Second

const workloadAudience = "liveshop-platform-internal"

type Dependencies struct {
	Release   *biz.Release
	Workloads *workloadidentity.Verifier
	Ready     func(context.Context) error
	shutdown  func() error
}

func (d Dependencies) Close() error {
	if d.shutdown == nil {
		return nil
	}
	return d.shutdown()
}

func NewDependencies(cfg *config.Config) (Dependencies, error) {
	ctx, cancel := context.WithTimeout(context.Background(), assemblyTimeout)
	defer cancel()

	workloads, err := workloadidentity.NewVerifier(trustedWorkloads(cfg), cfg.WorkloadIdentity.Issuer, workloadAudience)
	if err != nil {
		return Dependencies{}, fmt.Errorf("registry: workload_identity keys: %w", err)
	}
	database, err := openDatabase(cfg)
	if err != nil {
		return Dependencies{}, err
	}
	if err := mysql.Verify(ctx, database); err != nil {
		_ = database.Close()
		return Dependencies{}, fmt.Errorf("registry: database is unreachable: %w", err)
	}
	releaseStore, err := mysql.NewReleaseRepository(ctx, database)
	if err != nil {
		_ = database.Close()
		return Dependencies{}, err
	}
	return Dependencies{
		Release:   biz.NewRelease(releaseStore),
		Workloads: workloads,
		Ready:     database.PingContext,
		shutdown:  database.Close,
	}, nil
}

func trustedWorkloads(cfg *config.Config) map[string]workloadidentity.TrustedWorkload {
	peers := map[string]workloadidentity.TrustedWorkload{}
	for _, peer := range []config.BearerWorkload{cfg.WorkloadIdentity.HTTP.Gateway, cfg.WorkloadIdentity.HTTP.Release, cfg.WorkloadIdentity.HTTP.Platform} {
		peers[peer.KeyID] = workloadidentity.TrustedWorkload{
			PublicKey:   peer.PublicKey,
			Subject:     peer.Subject,
			Permissions: peer.Permissions,
		}
	}
	return peers
}

func openDatabase(cfg *config.Config) (*sql.DB, error) {
	database, err := sql.Open("mysql", cfg.Database.URL)
	if err != nil {
		return nil, fmt.Errorf("registry: open database: %w", err)
	}
	database.SetMaxOpenConns(cfg.Database.MaxOpenConnections)
	database.SetMaxIdleConns(cfg.Database.MaxIdleConnections)
	database.SetConnMaxLifetime(cfg.ConnectionMaxLifetime())
	database.SetConnMaxIdleTime(cfg.ConnectionMaxIdleTime())
	return database, nil
}
