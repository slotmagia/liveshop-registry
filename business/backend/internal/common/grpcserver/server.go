// Package grpcserver is the Platform gRPC composition root. It mounts whatever
// surfaces it is given and names none of them.
package grpcserver

import (
	"context"
	"fmt"

	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/common/grpcauth"
	"github.com/lvtuopen-ai/kernel-go/grpcx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// Surface is one gRPC-serving security boundary. Besides mounting its services
// it declares their names and the workload permission each method requires, so
// this composition root needs no knowledge of any published contract.
type Surface interface {
	RegisterGRPC(registrar grpc.ServiceRegistrar)
	GRPCServiceNames() []string
	GRPCMethodPermissions() map[string]string
}

type Config struct {
	Address   string
	TLS       TLSConfig
	Workloads []grpcauth.Workload
}

type Server struct {
	transport *grpcx.Server
	health    *health.Server
	services  []string
}

func New(config Config, surfaces ...Surface) (*Server, error) {
	transportCredentials, err := serverCredentials(config.TLS)
	if err != nil {
		return nil, err
	}
	permissions := map[string]string{}
	var services []string
	for _, surface := range surfaces {
		services = append(services, surface.GRPCServiceNames()...)
		for method, permission := range surface.GRPCMethodPermissions() {
			permissions[method] = permission
		}
	}
	authorizer, err := grpcauth.New(config.Workloads, permissions)
	if err != nil {
		return nil, err
	}
	transport, err := grpcx.NewServer(config.Address, grpcx.ServerOptions{
		TransportCredentials: transportCredentials,
		UnaryInterceptors: []grpc.UnaryServerInterceptor{
			authorizer.UnaryServerInterceptor(),
		},
		StreamInterceptors: []grpc.StreamServerInterceptor{
			authorizer.StreamServerInterceptor(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("registry: create gRPC server: %w", err)
	}
	server := &Server{transport: transport, health: health.NewServer(), services: services}
	server.setServingStatus(grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	grpc_health_v1.RegisterHealthServer(transport.Engine(), server.health)
	for _, surface := range surfaces {
		surface.RegisterGRPC(transport.Engine())
	}
	return server, nil
}

func (s *Server) setServingStatus(status grpc_health_v1.HealthCheckResponse_ServingStatus) {
	if s == nil || s.health == nil {
		return
	}
	s.health.SetServingStatus("", status)
	for _, service := range s.services {
		s.health.SetServingStatus(service, status)
	}
}

func (s *Server) Address() string {
	if s == nil || s.transport == nil {
		return ""
	}
	return s.transport.Address()
}

func (s *Server) Serve() error {
	if s == nil || s.transport == nil || s.health == nil {
		return fmt.Errorf("registry: gRPC server is not initialized")
	}
	s.setServingStatus(grpc_health_v1.HealthCheckResponse_SERVING)
	err := s.transport.Serve()
	s.setServingStatus(grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	return err
}

func (s *Server) Stop(ctx context.Context) error {
	if s == nil || s.transport == nil {
		return nil
	}
	s.setServingStatus(grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	return s.transport.Stop(ctx)
}
