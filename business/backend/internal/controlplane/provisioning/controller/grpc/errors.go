package grpc

import (
	"errors"

	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/biz/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// domainCode is the gRPC projection of the domain outcomes. It is derived from
// the domain error, never from an HTTP status.
var domainCode = []struct {
	err  error
	code codes.Code
}{
	{model.ErrReleaseNotFound, codes.NotFound},
	{model.ErrReleaseInvalid, codes.InvalidArgument},
	{model.ErrReleaseImmutable, codes.Aborted},
	{model.ErrRouteConflict, codes.Aborted},
	{model.ErrNavigationGroupConflict, codes.Aborted},
	{model.ErrPlatformSelfDeactivation, codes.PermissionDenied},
	{model.ErrUnavailable, codes.Unavailable},
}

// failure keeps an unrecognised error opaque so storage and driver details are
// never disclosed to a peer workload.
func failure(err error) error {
	if err == nil {
		return nil
	}
	for _, entry := range domainCode {
		if errors.Is(err, entry.err) {
			return status.Error(entry.code, err.Error())
		}
	}
	return status.Error(codes.Internal, "registry request failed")
}
