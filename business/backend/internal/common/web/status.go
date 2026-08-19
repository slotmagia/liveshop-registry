package web

import (
	"errors"
	"net/http"

	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/biz/model"
)

var domainStatus = []struct {
	err    error
	status int
}{
	{model.ErrReleaseNotFound, http.StatusNotFound},
	{model.ErrReleaseInvalid, http.StatusBadRequest},
	{model.ErrReleaseImmutable, http.StatusConflict},
	{model.ErrRouteConflict, http.StatusConflict},
	{model.ErrNavigationGroupConflict, http.StatusConflict},
	{model.ErrPlatformSelfDeactivation, http.StatusForbidden},
	{model.ErrUnavailable, http.StatusServiceUnavailable},
}

func StatusFor(err error) (int, bool) {
	for _, entry := range domainStatus {
		if errors.Is(err, entry.err) {
			return entry.status, true
		}
	}
	return 0, false
}
