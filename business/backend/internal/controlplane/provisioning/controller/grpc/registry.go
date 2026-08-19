// Package grpc adapts the published Platform registry Proto to the
// provisioning application boundary. It shares no type with the HTTP contract.
package grpc

import (
	"context"

	platformv1 "github.com/liveshop-platform/contracts/gen/go/platform/v1"
	"github.com/liveshop-platform/contracts/modulemanifest"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/controlplane/provisioning/service"
	grpclib "google.golang.org/grpc"
)

type RegistryController struct {
	platformv1.UnimplementedPlatformRegistryServiceServer
	service service.Provisioning
}

func NewRegistry(application service.Provisioning) *RegistryController {
	return &RegistryController{service: application}
}

func Register(server grpclib.ServiceRegistrar, application service.Provisioning) {
	platformv1.RegisterPlatformRegistryServiceServer(server, NewRegistry(application))
}

func (c *RegistryController) GetRouteSnapshot(ctx context.Context, _ *platformv1.GetRouteSnapshotRequest) (*platformv1.GetRouteSnapshotResponse, error) {
	snapshot, err := c.service.Routes(ctx)
	if err != nil {
		return nil, failure(err)
	}
	response := &platformv1.GetRouteSnapshotResponse{
		Revision: snapshot.Revision,
		Routes:   make([]*platformv1.ActiveRoute, 0, len(snapshot.Routes)),
	}
	for _, route := range snapshot.Routes {
		encoded := &platformv1.ActiveRoute{
			ModuleId: route.ModuleID, Surface: route.Surface, Prefix: route.Prefix,
			Service: route.Service, Origin: route.Origin,
		}
		for _, operation := range route.Operations {
			encoded.Operations = append(encoded.Operations, &platformv1.ActiveRouteOperation{
				Method: operation.Method, Path: operation.Path, Authentication: operation.Authentication,
			})
		}
		response.Routes = append(response.Routes, encoded)
	}
	return response, nil
}

func (c *RegistryController) GetActiveCapabilitySnapshot(ctx context.Context, _ *platformv1.GetActiveCapabilitySnapshotRequest) (*platformv1.GetActiveCapabilitySnapshotResponse, error) {
	snapshot, err := c.service.ActiveCapabilities(ctx)
	if err != nil {
		return nil, failure(err)
	}
	response := &platformv1.GetActiveCapabilitySnapshotResponse{
		RegistryRevision: snapshot.RegistryRevision,
		Modules:          make([]*platformv1.ActiveModuleCapability, 0, len(snapshot.Modules)),
	}
	for _, item := range snapshot.Modules {
		encoded := &platformv1.ActiveModuleCapability{
			ModuleId: item.ModuleID, ModuleName: item.ModuleName,
			ReleaseVersion: item.ReleaseVersion, ReleaseDigest: item.ReleaseDigest,
			Backend: &platformv1.Backend{Service: item.Backend.Service, Origin: item.Backend.Origin},
		}
		for _, permission := range item.Permissions {
			encoded.Permissions = append(encoded.Permissions, &platformv1.PermissionDefinition{
				Code: permission.Code, Name: permission.Name, Resource: permission.Resource,
				Action: permission.Action, Description: permission.Description,
			})
		}
		for _, contribution := range item.Contributions {
			encoded.Contributions = append(encoded.Contributions, activeContribution(contribution))
		}
		if item.Backend.GRPC != nil {
			encoded.Grpc = grpcContract(*item.Backend.GRPC)
		}
		for _, route := range item.Backend.HTTPRoutes {
			for _, operation := range route.Operations {
				encoded.HttpOperations = append(encoded.HttpOperations, &platformv1.HttpOperation{
					OperationId: operation.ID, Surface: route.Surface, Method: operation.Method,
					Path: operation.Path, Summary: operation.Summary, Description: operation.Description,
					Authentication: operation.Authentication, Idempotency: operation.Idempotency,
					RequiredPermissions: append([]string(nil), operation.RequiredPermissions...),
					RequestFields:       capabilityFields(operation.RequestFields),
					Responses:           capabilityResponses(operation.Responses),
				})
			}
		}
		response.Modules = append(response.Modules, encoded)
	}
	return response, nil
}

func activeContribution(source modulemanifest.Contribution) *platformv1.ActiveContribution {
	output := &platformv1.ActiveContribution{
		ContributionId: source.ID, Surface: source.Surface, Kind: source.Kind, Route: source.Route,
		Outlet: source.Outlet, Title: source.Title, Description: source.Description, Icon: source.Icon,
		Sort: int32(source.Sort), RequiredPermissions: append([]string(nil), source.RequiredPermissions...),
		Artifact: &platformv1.Artifact{
			Type: source.Artifact.Type, Name: source.Artifact.Name, Version: source.Artifact.Version,
			Entry: source.Artifact.Entry, ExportName: source.Artifact.ExportName, Integrity: source.Artifact.Integrity,
		},
	}
	if source.Navigation != nil {
		output.Navigation = &platformv1.Navigation{
			GroupId: source.Navigation.GroupID, GroupTitle: source.Navigation.GroupTitle,
			GroupSort: int32(source.Navigation.GroupSort),
		}
	}
	for _, route := range source.AllowedRoutes {
		output.AllowedRoutes = append(output.AllowedRoutes, &platformv1.AllowedRoute{
			Methods: append([]string(nil), route.Methods...), Prefix: route.Prefix,
			RequiredPermissions: append([]string(nil), route.RequiredPermissions...),
		})
	}
	for _, action := range source.Frontend.Actions {
		output.FrontendActions = append(output.FrontendActions, frontendAction(action))
	}
	output.Frontend = &platformv1.FrontendContract{
		Component: source.Frontend.Component,
		Props:     capabilityFields(source.Frontend.Props),
		Actions:   make([]*platformv1.FrontendAction, 0, len(source.Frontend.Actions)),
	}
	for _, event := range source.Frontend.Events {
		output.Frontend.Events = append(output.Frontend.Events, &platformv1.FrontendEvent{
			Name: event.Name, Description: event.Description, Payload: capabilityFields(event.Payload),
		})
	}
	for _, action := range source.Frontend.Actions {
		output.Frontend.Actions = append(output.Frontend.Actions, frontendAction(action))
	}
	return output
}

func frontendAction(action modulemanifest.FrontendAction) *platformv1.FrontendAction {
	return &platformv1.FrontendAction{
		ActionId: action.ID, Label: action.Label, Description: action.Description,
		Invocation: action.Invocation, Target: action.Target,
		RequiredPermissions: append([]string(nil), action.RequiredPermissions...),
		Parameters:          capabilityFields(action.Parameters),
	}
}

func capabilityFields(fields []modulemanifest.CapabilityField) []*platformv1.CapabilityField {
	output := make([]*platformv1.CapabilityField, 0, len(fields))
	for _, field := range fields {
		output = append(output, &platformv1.CapabilityField{
			Name: field.Name, Location: field.Location, Type: field.Type, Format: field.Format,
			Required: field.Required, Description: field.Description, Example: field.Example,
		})
	}
	return output
}

func capabilityResponses(responses []modulemanifest.CapabilityResponse) []*platformv1.CapabilityResponse {
	output := make([]*platformv1.CapabilityResponse, 0, len(responses))
	for _, response := range responses {
		output = append(output, &platformv1.CapabilityResponse{
			Status: int32(response.Status), Description: response.Description, Fields: capabilityFields(response.Fields),
		})
	}
	return output
}

func grpcContract(source modulemanifest.GRPC) *platformv1.GrpcContract {
	output := &platformv1.GrpcContract{
		Service: source.Service, ContractVersion: source.ContractVersion,
		Endpoint: source.Endpoint, TransportSecurity: source.TransportSecurity,
	}
	for _, method := range source.Methods {
		output.Methods = append(output.Methods, &platformv1.GrpcMethod{
			Name: method.Name, FullMethod: method.FullMethod, Summary: method.Summary,
			Description: method.Description, Invocation: method.Invocation, Idempotency: method.Idempotency,
			RecommendedDeadlineMs: int32(method.RecommendedDeadlineMS),
			RequiredPermissions:   append([]string(nil), method.RequiredPermissions...),
			RequestFields:         capabilityFields(method.RequestFields), ResponseFields: capabilityFields(method.ResponseFields),
		})
	}
	return output
}
