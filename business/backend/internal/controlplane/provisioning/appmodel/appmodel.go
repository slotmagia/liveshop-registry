// Package appmodel holds the transport-neutral input and output of the
// provisioning surface, shared by its HTTP and gRPC transports.
package appmodel

import (
	"github.com/liveshop-platform/contracts/modulemanifest"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/biz/model"
)

type RegisteredRelease struct {
	Digest string
}

type Activation struct {
	ModuleID string
	Version  string
}

type RouteSnapshot struct {
	Revision uint64
	Routes   []modulemanifest.ActiveRoute
}

type CapabilityCatalog struct {
	Revision uint64
	Items    []model.ModuleCapabilityCatalog
}

type ActiveCapabilitySnapshot struct {
	RegistryRevision uint64
	Modules          []model.ActiveModuleCapability
}
