package model

import (
	"errors"
	"strings"
	"testing"

	"github.com/liveshop-platform/contracts/modulemanifest"
)

func fixture(id, prefix string) modulemanifest.Manifest {
	permission := id + ".item.read"
	return modulemanifest.Manifest{
		APIVersion: modulemanifest.APIVersion,
		Kind:       modulemanifest.KindModuleRelease,
		Metadata:   modulemanifest.Metadata{ID: id, Name: id, Version: "1.0.0"},
		Spec: modulemanifest.Spec{
			Backend: modulemanifest.Backend{Service: id, Origin: "http://" + id, HTTPRoutes: []modulemanifest.HTTPRoute{{
				Surface: "admin", Prefix: prefix, Operations: []modulemanifest.HTTPOperation{{
					ID: id + ".item.list", Method: "GET", Path: prefix, Summary: "List items", Description: "Lists module items.", Authentication: "module-session", Idempotency: "safe", RequiredPermissions: []string{permission},
					Responses: []modulemanifest.CapabilityResponse{{Status: 200, Description: "Item list"}},
				}},
			}}},
			Permissions: []modulemanifest.PermissionDefinition{{Code: permission, Name: "Read items", Resource: id + ".item", Action: "read"}},
		},
	}
}

func withNavigation(manifest modulemanifest.Manifest, surface, groupID, groupTitle string, groupSort int) modulemanifest.Manifest {
	permission := manifest.Metadata.ID + ".item.read"
	manifest.Spec.Backend.HTTPRoutes[0].Surface = surface
	manifest.Spec.Contributions = []modulemanifest.Contribution{{
		ID: manifest.Metadata.ID + ".page", Surface: surface, Kind: "page", Route: "/" + manifest.Metadata.ID,
		Title: manifest.Metadata.Name, Description: manifest.Metadata.Name + " management page.",
		Navigation:          &modulemanifest.Navigation{GroupID: groupID, GroupTitle: groupTitle, GroupSort: groupSort},
		RequiredPermissions: []string{permission},
		AllowedRoutes:       []modulemanifest.AllowedRoute{{Methods: []string{"GET"}, Prefix: manifest.Spec.Backend.HTTPRoutes[0].Prefix, RequiredPermissions: []string{permission}}},
		Artifact: modulemanifest.Artifact{
			Type: "iframe", Name: manifest.Metadata.ID + "-admin", Version: manifest.Metadata.Version,
			Entry: "https://" + manifest.Metadata.ID + ".invalid", Integrity: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		},
		Frontend: modulemanifest.FrontendContract{Component: manifest.Metadata.Name + "Page"},
	}}
	return manifest
}

func TestActivationRejectsRouteConflict(t *testing.T) {
	state := NewRegistryState()
	for _, manifest := range []modulemanifest.Manifest{fixture("catalog", "/admin/items"), fixture("order", "/admin/items")} {
		if _, err := state.Register(manifest); err != nil {
			t.Fatal(err)
		}
	}
	if err := state.Activate("catalog", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := state.Activate("order", "1.0.0"); !errors.Is(err, ErrRouteConflict) {
		t.Fatalf("expected route conflict, got %v", err)
	}
}

func TestActivationRejectsOverlappingRoutePrefix(t *testing.T) {
	state := NewRegistryState()
	for _, manifest := range []modulemanifest.Manifest{fixture("catalog", "/admin/items"), fixture("order", "/admin/items/private")} {
		if _, err := state.Register(manifest); err != nil {
			t.Fatal(err)
		}
	}
	if err := state.Activate("catalog", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := state.Activate("order", "1.0.0"); !errors.Is(err, ErrRouteConflict) {
		t.Fatalf("expected overlapping route conflict, got %v", err)
	}
}

func TestActivationAcceptsConsistentNavigationGroupAcrossModules(t *testing.T) {
	state := NewRegistryState()
	manifests := []modulemanifest.Manifest{
		withNavigation(fixture("catalog", "/admin/catalog"), "admin", "commerce", "Commerce", 20),
		withNavigation(fixture("order", "/admin/orders"), "admin", "commerce", "Commerce", 20),
	}
	for _, manifest := range manifests {
		if _, err := state.Register(manifest); err != nil {
			t.Fatal(err)
		}
		if err := state.Activate(manifest.Metadata.ID, manifest.Metadata.Version); err != nil {
			t.Fatal(err)
		}
	}
}

func TestActivationAllowsNavigationGroupMetadataToDifferAcrossSurfaces(t *testing.T) {
	state := NewRegistryState()
	manifests := []modulemanifest.Manifest{
		withNavigation(fixture("catalog", "/admin/catalog"), "admin", "commerce", "Platform Commerce", 20),
		withNavigation(fixture("order", "/merch/orders"), "merch", "commerce", "Merchant Commerce", 40),
	}
	for _, manifest := range manifests {
		if _, err := state.Register(manifest); err != nil {
			t.Fatal(err)
		}
		if err := state.Activate(manifest.Metadata.ID, manifest.Metadata.Version); err != nil {
			t.Fatal(err)
		}
	}
}

func TestActivationRejectsNavigationGroupConflictWithoutChangingState(t *testing.T) {
	for _, test := range []struct {
		name       string
		groupTitle string
		groupSort  int
	}{
		{name: "title", groupTitle: "Sales", groupSort: 20},
		{name: "sort", groupTitle: "Commerce", groupSort: 30},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := NewRegistryState()
			catalog := withNavigation(fixture("catalog", "/admin/catalog"), "admin", "commerce", "Commerce", 20)
			order := withNavigation(fixture("order", "/admin/orders"), "admin", "commerce", test.groupTitle, test.groupSort)
			for _, manifest := range []modulemanifest.Manifest{catalog, order} {
				if _, err := state.Register(manifest); err != nil {
					t.Fatal(err)
				}
			}
			if err := state.Activate("catalog", "1.0.0"); err != nil {
				t.Fatal(err)
			}
			beforeRevision := state.Revision
			beforeActive := state.ActiveVersion("order")
			err := state.Activate("order", "1.0.0")
			if !errors.Is(err, ErrNavigationGroupConflict) {
				t.Fatalf("expected navigation group conflict, got %v", err)
			}
			for _, detail := range []string{`surface "admin"`, `group "commerce"`, "catalog/catalog.page", "order/order.page"} {
				if !strings.Contains(err.Error(), detail) {
					t.Fatalf("conflict error %q does not identify %q", err, detail)
				}
			}
			if state.Revision != beforeRevision || state.ActiveVersion("order") != beforeActive || state.ActiveVersion("catalog") != "1.0.0" {
				t.Fatalf("failed activation mutated state: revision=%d active=%#v", state.Revision, state.Active)
			}
		})
	}
}

func TestActivationRejectsNavigationGroupConflictWithinRelease(t *testing.T) {
	state := NewRegistryState()
	manifest := withNavigation(fixture("catalog", "/admin/catalog"), "admin", "commerce", "Commerce", 20)
	conflicting := manifest.Spec.Contributions[0]
	conflicting.ID = "catalog.other"
	conflicting.Route = "/catalog/other"
	conflicting.Navigation = &modulemanifest.Navigation{GroupID: "commerce", GroupTitle: "Sales", GroupSort: 20}
	manifest.Spec.Contributions = append(manifest.Spec.Contributions, conflicting)
	if _, err := state.Register(manifest); err != nil {
		t.Fatal(err)
	}
	beforeRevision := state.Revision
	if err := state.Activate("catalog", "1.0.0"); !errors.Is(err, ErrNavigationGroupConflict) {
		t.Fatalf("expected same-release navigation group conflict, got %v", err)
	}
	if state.Revision != beforeRevision || state.ActiveVersion("catalog") != "" {
		t.Fatalf("failed activation mutated state: revision=%d active=%#v", state.Revision, state.Active)
	}
}

func TestActivationAllowsEmptyGroupIconDuringRollout(t *testing.T) {
	state := NewRegistryState()
	older := withNavigation(fixture("identity", "/admin/identity"), "admin", "commerce", "Commerce", 20)
	newer := withNavigation(fixture("catalog", "/admin/catalog"), "admin", "commerce", "Commerce", 20)
	newer.Spec.Contributions[0].Navigation.GroupIcon = "store"
	if _, err := state.Register(older); err != nil {
		t.Fatal(err)
	}
	if _, err := state.Register(newer); err != nil {
		t.Fatal(err)
	}
	if err := state.Activate("identity", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := state.Activate("catalog", "1.0.0"); err != nil {
		t.Fatal(err)
	}
}

func TestDuplicateActivationIsIdempotent(t *testing.T) {
	state := NewRegistryState()
	if _, err := state.Register(fixture("catalog", "/admin/items")); err != nil {
		t.Fatal(err)
	}
	if err := state.Activate("catalog", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	revision, _ := state.Routes()
	if err := state.Activate("catalog", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	after, _ := state.Routes()
	if after != revision {
		t.Fatalf("duplicate activation changed revision: before=%d after=%d", revision, after)
	}
}

func TestRoutesHaveDeterministicOrderForEqualLengthPrefixes(t *testing.T) {
	state := NewRegistryState()
	manifest := fixture("catalog", "/admin/catalog")
	shopRoute := manifest.Spec.Backend.HTTPRoutes[0]
	shopRoute.Operations = append([]modulemanifest.HTTPOperation(nil), shopRoute.Operations...)
	shopRoute.Surface = "merch"
	shopRoute.Prefix = "/merch/catalog"
	shopRoute.Operations[0].ID = "catalog.merch.item.list"
	shopRoute.Operations[0].Path = shopRoute.Prefix
	manifest.Spec.Backend.HTTPRoutes = []modulemanifest.HTTPRoute{shopRoute, manifest.Spec.Backend.HTTPRoutes[0]}
	if _, err := state.Register(manifest); err != nil {
		t.Fatal(err)
	}
	if err := state.Activate("catalog", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	_, routes := state.Routes()
	if len(routes) != 2 || routes[0].Surface != "admin" || routes[1].Surface != "merch" {
		t.Fatalf("equal-length routes are not deterministically ordered: %#v", routes)
	}
	if len(routes[0].Operations) != 1 || routes[0].Operations[0].Method != "GET" || routes[0].Operations[0].Path != "/admin/catalog" || routes[0].Operations[0].Authentication != "module-session" {
		t.Fatalf("route snapshot lost operation authentication metadata: %#v", routes[0].Operations)
	}
}

func TestModuleListingAndDeactivation(t *testing.T) {
	state := NewRegistryState()
	if _, err := state.Register(fixture("catalog", "/admin/items")); err != nil {
		t.Fatal(err)
	}
	if err := state.Activate("catalog", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	items := state.Modules()
	if len(items) != 1 || items[0].ActiveVersion != "1.0.0" {
		t.Fatalf("unexpected modules: %#v", items)
	}
	if err := state.Deactivate("catalog"); err != nil {
		t.Fatal(err)
	}
	if _, routes := state.Routes(); len(routes) != 0 {
		t.Fatalf("deactivated routes remain: %#v", routes)
	}
}

func TestRegisterRejectsChangedImmutableRelease(t *testing.T) {
	state := NewRegistryState()
	manifest := fixture("catalog", "/admin/items")
	if _, err := state.Register(manifest); err != nil {
		t.Fatal(err)
	}
	changed := fixture("catalog", "/admin/other")
	if _, err := state.Register(changed); !errors.Is(err, ErrReleaseImmutable) {
		t.Fatalf("expected immutable release rejection, got %v", err)
	}
}

func TestCapabilityCatalogIsDerivedFromImmutableRelease(t *testing.T) {
	state := NewRegistryState()
	manifest := fixture("catalog", "/admin/catalog")
	manifest.Metadata.Name = "Catalog Capability Module"
	if _, err := state.Register(manifest); err != nil {
		t.Fatal(err)
	}
	if err := state.Activate("catalog", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	revision, catalogs, err := state.CapabilityCatalogs()
	if err != nil || revision != 2 || len(catalogs) != 1 || len(catalogs[0].Releases) != 1 {
		t.Fatalf("unexpected capability catalog: revision=%d catalogs=%#v err=%v", revision, catalogs, err)
	}
	release := catalogs[0].Releases[0]
	if catalogs[0].Name != "Catalog Capability Module" || !release.Active || len(release.Backend.HTTPRoutes) != 1 || release.Backend.HTTPRoutes[0].Operations[0].ID != "catalog.item.list" {
		t.Fatalf("registered capabilities were not preserved: %#v", release)
	}

	// Returned slices are decoded/owned by the immutable release snapshot; a
	// caller must not be able to alter the registry's next discovery response.
	release.Backend.HTTPRoutes[0].Operations[0].Summary = "tampered"
	_, again, err := state.CapabilityCatalogs()
	if err != nil || again[0].Releases[0].Backend.HTTPRoutes[0].Operations[0].Summary != "List items" {
		t.Fatal("capability catalog response mutated registry state")
	}
}

func TestActiveCapabilitySnapshotIsVersionedDeterministicAndSideEffectFree(t *testing.T) {
	state := NewRegistryState()
	active := withNavigation(fixture("catalog", "/admin/catalog"), "admin", "commerce", "Commerce", 20)
	inactive := withNavigation(fixture("order", "/admin/orders"), "admin", "trade", "Trade", 30)
	active.Spec.Contributions[0].Frontend.Actions = []modulemanifest.FrontendAction{{
		ID: "refresh", Label: "Refresh", Description: "Reload products.", Invocation: "http",
		Target: "catalog.item.list", RequiredPermissions: []string{"catalog.item.read"},
	}}
	for _, manifest := range []modulemanifest.Manifest{inactive, active} {
		if _, err := state.Register(manifest); err != nil {
			t.Fatal(err)
		}
	}
	if err := state.Activate("catalog", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	beforeRevision := state.Revision
	revision, modules, err := state.ActiveCapabilitySnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if revision != beforeRevision || len(modules) != 1 || modules[0].ModuleID != "catalog" || modules[0].ReleaseVersion != "1.0.0" {
		t.Fatalf("unexpected active snapshot: revision=%d modules=%#v", revision, modules)
	}
	if len(modules[0].Permissions) != 1 || len(modules[0].Contributions) != 1 || len(modules[0].Backend.HTTPRoutes) != 1 || len(modules[0].Contributions[0].Frontend.Actions) != 1 {
		t.Fatalf("active capability contract is incomplete: %#v", modules[0])
	}

	modules[0].Permissions[0].Name = "tampered"
	modules[0].Contributions[0].AllowedRoutes[0].Prefix = "/tampered"
	againRevision, again, err := state.ActiveCapabilitySnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if againRevision != beforeRevision || state.Revision != beforeRevision || again[0].Permissions[0].Name == "tampered" || again[0].Contributions[0].AllowedRoutes[0].Prefix == "/tampered" {
		t.Fatal("repeated snapshot read mutated registry state")
	}
}

func TestFailedActivationLeavesActiveCapabilitySnapshotUnchanged(t *testing.T) {
	state := NewRegistryState()
	first := withNavigation(fixture("catalog", "/admin/catalog"), "admin", "commerce", "Commerce", 20)
	conflicting := withNavigation(fixture("order", "/admin/orders"), "admin", "commerce", "Sales", 20)
	for _, manifest := range []modulemanifest.Manifest{first, conflicting} {
		if _, err := state.Register(manifest); err != nil {
			t.Fatal(err)
		}
	}
	if err := state.Activate("catalog", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	beforeRevision, before, err := state.ActiveCapabilitySnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Activate("order", "1.0.0"); !errors.Is(err, ErrNavigationGroupConflict) {
		t.Fatalf("expected navigation conflict, got %v", err)
	}
	afterRevision, after, err := state.ActiveCapabilitySnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if afterRevision != beforeRevision || len(after) != 1 || after[0].ModuleID != before[0].ModuleID || after[0].ReleaseDigest != before[0].ReleaseDigest {
		t.Fatalf("failed activation changed active snapshot: before=%#v after=%#v", before, after)
	}
}
