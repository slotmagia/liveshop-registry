package model

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/liveshop-platform/contracts/modulemanifest"
	"github.com/lvtuopen-ai/kernel-go/apperror"
)

var (
	ErrReleaseNotFound          = apperror.New("platform.registry.release_not_found", "module release not found")
	ErrReleaseInvalid           = apperror.New("platform.registry.release_invalid", "module release manifest is invalid")
	ErrReleaseImmutable         = apperror.New("platform.registry.release_immutable", "immutable module release content differs")
	ErrRouteConflict            = apperror.New("platform.registry.route_conflict", "activated module routes overlap")
	ErrNavigationGroupConflict  = apperror.New("platform.registry.navigation_group_conflict", "activated module navigation group metadata conflicts")
	ErrPlatformSelfDeactivation = apperror.New("platform.registry.self_deactivation_forbidden", "the platform control-plane module cannot deactivate itself")
)

// PlatformModuleID names the control-plane module, which may never deactivate
// itself because doing so would remove the route that performs the change.
const PlatformModuleID = "platform"

// Release is one immutable registered manifest. Field names are part of the
// persisted registry state encoding and must not be renamed or tagged.
type Release struct {
	Manifest modulemanifest.Manifest
	Digest   string
}

type ReleaseInfo struct {
	Version string
	Digest  string
}

type ModuleInfo struct {
	ID            string
	Name          string
	ActiveVersion string
	Releases      []ReleaseInfo
}

// CapabilityRelease is the immutable capability contract published by one
// module release. The registry manifest remains the sole source of truth.
type CapabilityRelease struct {
	Version       string
	Digest        string
	Active        bool
	Backend       modulemanifest.Backend
	Permissions   []modulemanifest.PermissionDefinition
	Contributions []modulemanifest.Contribution
}

type ModuleCapabilityCatalog struct {
	ID            string
	Name          string
	ActiveVersion string
	Releases      []CapabilityRelease
}

// ActiveModuleCapability is the authorization input published to Identity for
// one active immutable release. It intentionally excludes inactive releases:
// Identity must never grant a capability that cannot be reached at the same
// Registry revision.
type ActiveModuleCapability struct {
	ModuleID       string
	ModuleName     string
	ReleaseVersion string
	ReleaseDigest  string
	Backend        modulemanifest.Backend
	Permissions    []modulemanifest.PermissionDefinition
	Contributions  []modulemanifest.Contribution
}

type RegistryAuditActor struct {
	Realm      string
	MerchantID int64
	Subject    string
}

func (a RegistryAuditActor) Valid() bool {
	return a.Realm != "" && a.MerchantID >= 0 && a.Subject != ""
}

// RegistryState is the module registry aggregate: every registered release
// plus the currently active version of each module. Storage adapters load it,
// apply one domain operation and persist it again.
type RegistryState struct {
	Revision uint64
	Releases map[string]map[string]Release
	Active   map[string]string
}

func NewRegistryState() *RegistryState {
	return &RegistryState{Revision: 1, Releases: map[string]map[string]Release{}, Active: map[string]string{}}
}

func (s *RegistryState) Register(manifest modulemanifest.Manifest) (string, error) {
	if err := manifest.Validate(); err != nil {
		return "", fmt.Errorf("%w: %s", ErrReleaseInvalid, err.Error())
	}
	digest, err := manifest.Digest()
	if err != nil {
		return "", err
	}
	versions := s.Releases[manifest.Metadata.ID]
	if versions == nil {
		versions = map[string]Release{}
		if s.Releases == nil {
			s.Releases = map[string]map[string]Release{}
		}
		s.Releases[manifest.Metadata.ID] = versions
	}
	if existing, ok := versions[manifest.Metadata.Version]; ok {
		if existing.Digest != digest {
			return "", ErrReleaseImmutable
		}
		return digest, nil
	}
	versions[manifest.Metadata.Version] = Release{Manifest: manifest, Digest: digest}
	return digest, nil
}

func (s *RegistryState) Activate(moduleID, version string) error {
	if _, ok := s.Releases[moduleID][version]; !ok {
		return ErrReleaseNotFound
	}
	next := make(map[string]string, len(s.Active)+1)
	for id, current := range s.Active {
		next[id] = current
	}
	next[moduleID] = version
	if err := s.validateActiveRoutes(next); err != nil {
		return err
	}
	if err := s.validateActiveNavigationGroups(next); err != nil {
		return err
	}
	if s.Active[moduleID] == version {
		return nil
	}
	s.Active = next
	s.Revision++
	return nil
}

func (s *RegistryState) Deactivate(moduleID string) error {
	if _, ok := s.Active[moduleID]; !ok {
		return ErrReleaseNotFound
	}
	delete(s.Active, moduleID)
	s.Revision++
	return nil
}

func (s *RegistryState) ActiveVersion(moduleID string) string { return s.Active[moduleID] }

func (s *RegistryState) ManifestFor(moduleID, version string) (modulemanifest.Manifest, bool) {
	release, ok := s.Releases[moduleID][version]
	return release.Manifest, ok
}

func (s *RegistryState) Modules() []ModuleInfo {
	output := make([]ModuleInfo, 0, len(s.Releases))
	for moduleID, versions := range s.Releases {
		item := ModuleInfo{ID: moduleID, ActiveVersion: s.Active[moduleID]}
		if active, ok := versions[item.ActiveVersion]; ok {
			item.Name = active.Manifest.Metadata.Name
		}
		for version, current := range versions {
			item.Releases = append(item.Releases, ReleaseInfo{Version: version, Digest: current.Digest})
		}
		sort.Slice(item.Releases, func(i, j int) bool { return item.Releases[i].Version < item.Releases[j].Version })
		if item.Name == "" && len(item.Releases) > 0 {
			item.Name = versions[item.Releases[len(item.Releases)-1].Version].Manifest.Metadata.Name
		}
		output = append(output, item)
	}
	sort.Slice(output, func(i, j int) bool { return output[i].ID < output[j].ID })
	return output
}

// CapabilityCatalogs returns an immutable, machine-readable view of every
// registered module release. It never probes a running module, so discovery
// cannot drift from the release that was registered and activated.
func (s *RegistryState) CapabilityCatalogs() (uint64, []ModuleCapabilityCatalog, error) {
	output := make([]ModuleCapabilityCatalog, 0, len(s.Releases))
	for moduleID, versions := range s.Releases {
		item := ModuleCapabilityCatalog{ID: moduleID, ActiveVersion: s.Active[moduleID]}
		if active, ok := versions[item.ActiveVersion]; ok {
			item.Name = active.Manifest.Metadata.Name
		}
		for version, current := range versions {
			spec, err := cloneSpec(current.Manifest.Spec)
			if err != nil {
				return 0, nil, err
			}
			item.Releases = append(item.Releases, CapabilityRelease{
				Version:       version,
				Digest:        current.Digest,
				Active:        s.Active[moduleID] == version,
				Backend:       spec.Backend,
				Permissions:   spec.Permissions,
				Contributions: spec.Contributions,
			})
		}
		sort.Slice(item.Releases, func(i, j int) bool { return item.Releases[i].Version < item.Releases[j].Version })
		if item.Name == "" && len(item.Releases) > 0 {
			item.Name = versions[item.Releases[len(item.Releases)-1].Version].Manifest.Metadata.Name
		}
		output = append(output, item)
	}
	sort.Slice(output, func(i, j int) bool { return output[i].ID < output[j].ID })
	return s.Revision, output, nil
}

// ActiveCapabilitySnapshot returns a deterministic deep copy of the exact
// active capability set at Revision. Registry state is the sole source of
// truth; the projection performs no network probes and has no side effects.
func (s *RegistryState) ActiveCapabilitySnapshot() (uint64, []ActiveModuleCapability, error) {
	moduleIDs := make([]string, 0, len(s.Active))
	for moduleID := range s.Active {
		moduleIDs = append(moduleIDs, moduleID)
	}
	sort.Strings(moduleIDs)
	output := make([]ActiveModuleCapability, 0, len(moduleIDs))
	for _, moduleID := range moduleIDs {
		version := s.Active[moduleID]
		release, ok := s.Releases[moduleID][version]
		if !ok {
			return 0, nil, fmt.Errorf("%w: active release %s@%s is missing", ErrReleaseInvalid, moduleID, version)
		}
		spec, err := cloneSpec(release.Manifest.Spec)
		if err != nil {
			return 0, nil, err
		}
		output = append(output, ActiveModuleCapability{
			ModuleID:       moduleID,
			ModuleName:     release.Manifest.Metadata.Name,
			ReleaseVersion: version,
			ReleaseDigest:  release.Digest,
			Backend:        spec.Backend,
			Permissions:    spec.Permissions,
			Contributions:  spec.Contributions,
		})
	}
	return s.Revision, output, nil
}

func (s *RegistryState) Routes() (uint64, []modulemanifest.ActiveRoute) {
	output := make([]modulemanifest.ActiveRoute, 0)
	for moduleID, version := range s.Active {
		manifest := s.Releases[moduleID][version].Manifest
		for _, route := range manifest.Spec.Backend.HTTPRoutes {
			operations := make([]modulemanifest.ActiveRouteOperation, 0, len(route.Operations))
			for _, operation := range route.Operations {
				operations = append(operations, modulemanifest.ActiveRouteOperation{
					Method: operation.Method, Path: operation.Path, Authentication: operation.Authentication,
				})
			}
			output = append(output, modulemanifest.ActiveRoute{
				ModuleID:   moduleID,
				Surface:    route.Surface,
				Prefix:     strings.TrimRight(route.Prefix, "/"),
				Service:    manifest.Spec.Backend.Service,
				Origin:     manifest.Spec.Backend.Origin,
				Operations: operations,
			})
		}
	}
	sort.Slice(output, func(i, j int) bool {
		if len(output[i].Prefix) != len(output[j].Prefix) {
			return len(output[i].Prefix) > len(output[j].Prefix)
		}
		if output[i].ModuleID != output[j].ModuleID {
			return output[i].ModuleID < output[j].ModuleID
		}
		if output[i].Surface != output[j].Surface {
			return output[i].Surface < output[j].Surface
		}
		if output[i].Prefix != output[j].Prefix {
			return output[i].Prefix < output[j].Prefix
		}
		if output[i].Service != output[j].Service {
			return output[i].Service < output[j].Service
		}
		return output[i].Origin < output[j].Origin
	})
	return s.Revision, output
}

func (s *RegistryState) Contributions(surface string) (uint64, []modulemanifest.RuntimeContribution) {
	output := make([]modulemanifest.RuntimeContribution, 0)
	for moduleID, version := range s.Active {
		manifest := s.Releases[moduleID][version].Manifest
		for _, contribution := range manifest.Spec.Contributions {
			if contribution.Surface == surface {
				output = append(output, modulemanifest.RuntimeContribution{ModuleID: moduleID, ModuleVersion: version, Contribution: contribution})
			}
		}
	}
	sort.Slice(output, func(i, j int) bool {
		left, right := output[i].Contribution, output[j].Contribution
		if left.Sort == right.Sort {
			return left.ID < right.ID
		}
		return left.Sort < right.Sort
	})
	return s.Revision, output
}

func (s *RegistryState) Contribution(moduleID, version, contributionID string) (modulemanifest.Contribution, error) {
	if s.Active[moduleID] != version {
		return modulemanifest.Contribution{}, ErrReleaseNotFound
	}
	release, ok := s.Releases[moduleID][version]
	if !ok {
		return modulemanifest.Contribution{}, ErrReleaseNotFound
	}
	for _, contribution := range release.Manifest.Spec.Contributions {
		if contribution.ID == contributionID {
			return contribution, nil
		}
	}
	return modulemanifest.Contribution{}, ErrReleaseNotFound
}

func (s *RegistryState) validateActiveRoutes(active map[string]string) error {
	type ownedRoute struct {
		surface string
		prefix  string
		owner   string
	}
	seen := make([]ownedRoute, 0)
	for moduleID, version := range active {
		release := s.Releases[moduleID][version]
		for _, route := range release.Manifest.Spec.Backend.HTTPRoutes {
			if route.Surface == "internal" {
				continue
			}
			prefix := normalizedRoutePrefix(route.Prefix)
			for _, current := range seen {
				if current.surface == route.Surface && current.owner != moduleID && routePrefixesOverlap(current.prefix, prefix) {
					return fmt.Errorf("%w: %s:%s overlaps %s owned by %s", ErrRouteConflict, route.Surface, prefix, current.prefix, current.owner)
				}
			}
			seen = append(seen, ownedRoute{surface: route.Surface, prefix: prefix, owner: moduleID})
		}
	}
	return nil
}

// validateActiveNavigationGroups keeps Host navigation deterministic. Host
// resolves one menu tree per surface, so a group ID may be reused across
// surfaces but every page in the same (surface, group ID) must publish exactly
// the same presentation metadata.
func (s *RegistryState) validateActiveNavigationGroups(active map[string]string) error {
	type ownedGroup struct {
		title        string
		icon         string
		sort         int
		moduleID     string
		contribution string
	}
	seen := map[string]ownedGroup{}
	moduleIDs := make([]string, 0, len(active))
	for moduleID := range active {
		moduleIDs = append(moduleIDs, moduleID)
	}
	sort.Strings(moduleIDs)
	for _, moduleID := range moduleIDs {
		version := active[moduleID]
		release := s.Releases[moduleID][version]
		for _, contribution := range release.Manifest.Spec.Contributions {
			navigation := contribution.Navigation
			if navigation == nil {
				continue
			}
			key := contribution.Surface + "\x00" + navigation.GroupID
			current, ok := seen[key]
			if ok && (current.title != navigation.GroupTitle || current.sort != navigation.GroupSort || iconsConflict(current.icon, navigation.GroupIcon)) {
				return fmt.Errorf(
					"%w: surface %q group %q is %q icon %q sort %d in %s/%s but %q icon %q sort %d in %s/%s",
					ErrNavigationGroupConflict,
					contribution.Surface,
					navigation.GroupID,
					current.title,
					current.icon,
					current.sort,
					current.moduleID,
					current.contribution,
					navigation.GroupTitle,
					navigation.GroupIcon,
					navigation.GroupSort,
					moduleID,
					contribution.ID,
				)
			}
			if !ok {
				seen[key] = ownedGroup{
					title:        navigation.GroupTitle,
					icon:         navigation.GroupIcon,
					sort:         navigation.GroupSort,
					moduleID:     moduleID,
					contribution: contribution.ID,
				}
			} else if current.icon == "" && navigation.GroupIcon != "" {
				current.icon = navigation.GroupIcon
				seen[key] = current
			}
		}
	}
	return nil
}

func iconsConflict(left, right string) bool {
	return left != "" && right != "" && left != right
}

func normalizedRoutePrefix(value string) string {
	value = strings.TrimRight(value, "/")
	if value == "" {
		return "/"
	}
	return value
}

func routePrefixesOverlap(left, right string) bool {
	return left == right || left == "/" || right == "/" || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

func cloneSpec(source modulemanifest.Spec) (modulemanifest.Spec, error) {
	encoded, err := json.Marshal(source)
	if err != nil {
		return modulemanifest.Spec{}, err
	}
	var clone modulemanifest.Spec
	if err := json.Unmarshal(encoded, &clone); err != nil {
		return modulemanifest.Spec{}, err
	}
	return clone, nil
}
