package app

import (
	"context"
	"fmt"

	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/common/grpcauth"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/common/grpcserver"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/common/server"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/config"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/controlplane/provisioning"
)

type instance struct {
	httpAddress string
	deps        Dependencies
	httpServer  *server.Server
	grpcServer  *grpcserver.Server
}

func bootstrap(ctx context.Context) (*instance, error) {
	cfg, err := config.Load(ctx)
	if err != nil {
		return nil, err
	}
	deps, err := NewDependencies(cfg)
	if err != nil {
		return nil, fmt.Errorf("registry: assemble dependencies: %w", err)
	}
	surface := provisioning.New(provisioning.Config{Release: deps.Release, Workloads: deps.Workloads})
	httpServer := server.New(server.Config{
		AllowedOrigins: cfg.HTTP.AllowedOrigins,
		Ready:          deps.Ready,
	}, surface)
	httpServer.SetAddr(cfg.Server.HTTP)
	grpcServer, err := grpcserver.New(grpcserver.Config{
		Address: cfg.Server.GRPC,
		TLS: grpcserver.TLSConfig{
			CertificateFile: cfg.GRPC.TLS.CertificateFile,
			PrivateKeyFile:  cfg.GRPC.TLS.PrivateKeyFile,
			ClientCAFile:    cfg.GRPC.TLS.ClientCAFile,
		},
		Workloads: []grpcauth.Workload{
			{SPIFFEID: cfg.WorkloadIdentity.GRPC.Gateway.SPIFFEID, Subject: cfg.WorkloadIdentity.GRPC.Gateway.Subject, Permissions: cfg.WorkloadIdentity.GRPC.Gateway.Permissions},
			{SPIFFEID: cfg.WorkloadIdentity.GRPC.Identity.SPIFFEID, Subject: cfg.WorkloadIdentity.GRPC.Identity.Subject, Permissions: cfg.WorkloadIdentity.GRPC.Identity.Permissions},
		},
	}, surface)
	if err != nil {
		_ = deps.Close()
		return nil, err
	}
	return &instance{httpAddress: cfg.Server.HTTP, deps: deps, httpServer: httpServer, grpcServer: grpcServer}, nil
}
