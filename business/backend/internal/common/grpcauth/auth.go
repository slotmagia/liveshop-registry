// Package grpcauth authorizes Platform gRPC calls by verified SPIFFE identity.
package grpcauth

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type Workload struct {
	SPIFFEID    string
	Subject     string
	Permissions []string
}

type identity struct {
	subject     string
	permissions map[string]struct{}
}

type Authorizer struct {
	identities   map[string]identity
	requirements map[string]string
}

func New(workloads []Workload, requirements map[string]string) (*Authorizer, error) {
	identities := make(map[string]identity, len(workloads))
	for _, workload := range workloads {
		spiffeID := strings.TrimSpace(workload.SPIFFEID)
		subject := strings.TrimSpace(workload.Subject)
		if spiffeID == "" || subject == "" || len(workload.Permissions) == 0 {
			return nil, errors.New("registry gRPC workload SPIFFE ID, subject and permissions are required")
		}
		identityURI, err := url.Parse(spiffeID)
		if err != nil || identityURI.Scheme != "spiffe" || identityURI.Host == "" || identityURI.User != nil || identityURI.RawQuery != "" || identityURI.Fragment != "" {
			return nil, fmt.Errorf("registry gRPC workload %s has invalid SPIFFE ID", subject)
		}
		if _, exists := identities[spiffeID]; exists {
			return nil, fmt.Errorf("registry gRPC duplicate SPIFFE ID %s", spiffeID)
		}
		permissions := make(map[string]struct{}, len(workload.Permissions))
		for _, permission := range workload.Permissions {
			if permission = strings.TrimSpace(permission); permission != "" {
				permissions[permission] = struct{}{}
			}
		}
		identities[spiffeID] = identity{subject: subject, permissions: permissions}
	}
	return &Authorizer{identities: identities, requirements: requirements}, nil
}

func (a *Authorizer) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		subject, err := a.authorize(ctx, info.FullMethod)
		if err != nil {
			return nil, err
		}
		return handler(withSubject(ctx, subject), request)
	}
}

func (a *Authorizer) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(service any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		subject, err := a.authorize(stream.Context(), info.FullMethod)
		if err != nil {
			return err
		}
		return handler(service, &subjectStream{ServerStream: stream, ctx: withSubject(stream.Context(), subject)})
	}
}

type subjectKey struct{}

func withSubject(ctx context.Context, subject string) context.Context {
	return context.WithValue(ctx, subjectKey{}, subject)
}

func Subject(ctx context.Context) string {
	value, _ := ctx.Value(subjectKey{}).(string)
	return value
}

type subjectStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *subjectStream) Context() context.Context { return s.ctx }

func (a *Authorizer) authorize(ctx context.Context, method string) (string, error) {
	spiffeID, err := verifiedSPIFFEID(ctx)
	if err != nil {
		return "", status.Error(codes.Unauthenticated, err.Error())
	}
	current, trusted := a.identities[spiffeID]
	if !trusted {
		return "", status.Error(codes.PermissionDenied, "workload SPIFFE identity is not trusted")
	}
	if strings.HasPrefix(method, "/grpc.health.v1.Health/") {
		return current.subject, nil
	}
	required, declared := a.requirements[method]
	if !declared || required == "" {
		return "", status.Error(codes.PermissionDenied, "gRPC method is not authorized")
	}
	if _, granted := current.permissions[required]; !granted {
		return "", status.Error(codes.PermissionDenied, "workload permission is not granted")
	}
	return current.subject, nil
}

func verifiedSPIFFEID(ctx context.Context) (string, error) {
	remote, ok := peer.FromContext(ctx)
	if !ok || remote.AuthInfo == nil {
		return "", errors.New("verified client certificate is required")
	}
	var state tls.ConnectionState
	switch info := remote.AuthInfo.(type) {
	case credentials.TLSInfo:
		state = info.State
	case *credentials.TLSInfo:
		state = info.State
	default:
		return "", errors.New("verified TLS client certificate is required")
	}
	if len(state.VerifiedChains) == 0 || len(state.VerifiedChains[0]) == 0 {
		return "", errors.New("verified TLS client certificate is required")
	}
	leaf := state.VerifiedChains[0][0]
	for _, uri := range leaf.URIs {
		if uri != nil && uri.Scheme == "spiffe" {
			return uri.String(), nil
		}
	}
	return "", errors.New("client certificate SPIFFE ID is required")
}
