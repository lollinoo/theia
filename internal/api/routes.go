package api

// This file defines routes API routing, middleware, and permission policy behavior.

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/lollinoo/theia/internal/domain"
)

// routeAuthMode declares whether a route bypasses auth, requires RBAC, or needs WebSocket upgrade handling.
type routeAuthMode int

const (
	routeAuthPublic routeAuthMode = iota + 1
	routeAuthProtected
	routeAuthWebSocket
)

// routeMiddlewareProfile selects the request wrapper used after route metadata has matched.
// Binary and restore routes use dedicated profiles because body limits and response writers differ from JSON APIs.
type routeMiddlewareProfile int

const (
	routeMiddlewareNormalJSON routeMiddlewareProfile = iota + 1
	routeMiddlewarePublicJSONSmallBody
	routeMiddlewareBinaryDownload
	routeMiddlewareRestoreUpload
	routeMiddlewareDeviceImportUpload
	routeMiddlewareWebSocketUpgrade
)

// routeHandlerKey is the stable join key between route metadata and constructed handler instances.
type routeHandlerKey string

const (
	routeHandlerAdmin                       routeHandlerKey = "admin"
	routeHandlerAuth                        routeHandlerKey = "auth"
	routeHandlerAreaCollection              routeHandlerKey = "areaCollection"
	routeHandlerAreaItem                    routeHandlerKey = "areaItem"
	routeHandlerBackupBulkDownload          routeHandlerKey = "backupBulkDownload"
	routeHandlerBackupBulkRunCollection     routeHandlerKey = "backupBulkRunCollection"
	routeHandlerBackupBulkRunItem           routeHandlerKey = "backupBulkRunItem"
	routeHandlerBackupBulkRunLatest         routeHandlerKey = "backupBulkRunLatest"
	routeHandlerBackupBulkStatus            routeHandlerKey = "backupBulkStatus"
	routeHandlerBackupFile                  routeHandlerKey = "backupFile"
	routeHandlerBackupJob                   routeHandlerKey = "backupJob"
	routeHandlerBridgeConnectorLaunch       routeHandlerKey = "bridgeConnectorLaunch"
	routeHandlerBridgeDownload              routeHandlerKey = "bridgeDownload"
	routeHandlerBridgeLaunchRequest         routeHandlerKey = "bridgeLaunchRequest"
	routeHandlerBridgeToken                 routeHandlerKey = "bridgeToken"
	routeHandlerCanvas                      routeHandlerKey = "canvas"
	routeHandlerCanvasMapCollection         routeHandlerKey = "canvasMapCollection"
	routeHandlerCanvasMapItem               routeHandlerKey = "canvasMapItem"
	routeHandlerCredentialProfileCollection routeHandlerKey = "credentialProfileCollection"
	routeHandlerCredentialProfileItem       routeHandlerKey = "credentialProfileItem"
	routeHandlerDeviceCollection            routeHandlerKey = "deviceCollection"
	routeHandlerDeviceBatch                 routeHandlerKey = "deviceBatch"
	routeHandlerDeviceImport                routeHandlerKey = "deviceImport"
	routeHandlerDeviceItem                  routeHandlerKey = "deviceItem"
	routeHandlerDeviceOrphans               routeHandlerKey = "deviceOrphans"
	routeHandlerGrafanaDeviceOverride       routeHandlerKey = "grafanaDeviceOverride"
	routeHandlerGrafanaProfileCollection    routeHandlerKey = "grafanaProfileCollection"
	routeHandlerGrafanaProfileItem          routeHandlerKey = "grafanaProfileItem"
	routeHandlerHealth                      routeHandlerKey = "health"
	routeHandlerInstanceBackupCollection    routeHandlerKey = "instanceBackupCollection"
	routeHandlerInstanceBackupItem          routeHandlerKey = "instanceBackupItem"
	routeHandlerInstanceBackupRestore       routeHandlerKey = "instanceBackupRestore"
	routeHandlerInstanceBackupRestoreStatus routeHandlerKey = "instanceBackupRestoreStatus"
	routeHandlerLinkCollection              routeHandlerKey = "linkCollection"
	routeHandlerLinkItem                    routeHandlerKey = "linkItem"
	routeHandlerPositionCollection          routeHandlerKey = "positionCollection"
	routeHandlerPrometheusHealth            routeHandlerKey = "prometheusHealth"
	routeHandlerRuntimeOverview             routeHandlerKey = "runtimeOverview"
	routeHandlerSettingsCollection          routeHandlerKey = "settingsCollection"
	routeHandlerSettingsBridge              routeHandlerKey = "settingsBridge"
	routeHandlerSettingsBridgeConnector     routeHandlerKey = "settingsBridgeConnector"
	routeHandlerSettingsBridgeDownload      routeHandlerKey = "settingsBridgeDownload"
	routeHandlerSettingsBridgeSecret        routeHandlerKey = "settingsBridgeSecret"
	routeHandlerSettingsBridgeSecretRevoke  routeHandlerKey = "settingsBridgeSecretRevoke"
	routeHandlerSettingsItem                routeHandlerKey = "settingsItem"
	routeHandlerSettingsMe                  routeHandlerKey = "settingsMe"
	routeHandlerSNMPProfileCollection       routeHandlerKey = "snmpProfileCollection"
	routeHandlerSNMPProfileItem             routeHandlerKey = "snmpProfileItem"
	routeHandlerTopologyCanvas              routeHandlerKey = "topologyCanvas"
	routeHandlerVendorCollection            routeHandlerKey = "vendorCollection"
	routeHandlerVendorItem                  routeHandlerKey = "vendorItem"
	routeHandlerWebSocket                   routeHandlerKey = "webSocket"
)

// routeAuthEndpoint identifies public auth routes so middleware can apply endpoint-specific session semantics.
type routeAuthEndpoint int

const (
	routeAuthEndpointNone routeAuthEndpoint = iota
	routeAuthEndpointLogin
	routeAuthEndpointLogout
	routeAuthEndpointMe
	routeAuthEndpointPasswordChange
	routeAuthEndpointPasswordReset
	routeAuthEndpointLegacySession
)

// apiRouteSpec is the source of truth for API route registration, auth mode, middleware, and permissions.
// The metadata registry validates ordering so broad patterns cannot shadow more specific routes.
type apiRouteSpec struct {
	name              string
	pattern           string
	serveMuxPattern   string
	handlerKey        routeHandlerKey
	authEndpoint      routeAuthEndpoint
	authMode          routeAuthMode
	middlewareProfile routeMiddlewareProfile
	methodPolicies    map[string][]string
}

// apiRouteMetadataRegistry provides route lookup by method/path without coupling middleware to handler structs.
type apiRouteMetadataRegistry struct {
	specs []apiRouteSpec
}

// routeMuxRegistration collapses route specs that share one net/http ServeMux pattern and handler.
type routeMuxRegistration struct {
	pattern string
	handler http.Handler
}

// routeMuxRegistrations verifies that every route metadata entry has an assembled handler.
func routeMuxRegistrations(specs []apiRouteSpec, handlers map[routeHandlerKey]http.Handler) ([]routeMuxRegistration, error) {
	seen := make(map[string]routeHandlerKey, len(specs))
	registrations := make([]routeMuxRegistration, 0, len(specs))
	for _, spec := range specs {
		if existing, ok := seen[spec.serveMuxPattern]; ok {
			if existing != spec.handlerKey {
				return nil, fmt.Errorf("serve mux pattern %s has handlers %s and %s", spec.serveMuxPattern, existing, spec.handlerKey)
			}
			continue
		}
		handler := handlers[spec.handlerKey]
		if handler == nil {
			return nil, fmt.Errorf("api route %s handler %s is not configured", spec.name, spec.handlerKey)
		}
		seen[spec.serveMuxPattern] = spec.handlerKey
		registrations = append(registrations, routeMuxRegistration{
			pattern: spec.serveMuxPattern,
			handler: handler,
		})
	}
	return registrations, nil
}

// registerAPIRouteHandlers registers validated route metadata with the standard library mux.
func registerAPIRouteHandlers(mux *http.ServeMux, specs []apiRouteSpec, handlers map[routeHandlerKey]http.Handler) error {
	registrations, err := routeMuxRegistrations(specs, handlers)
	if err != nil {
		return err
	}
	for _, registration := range registrations {
		mux.Handle(registration.pattern, registration.handler)
	}
	return nil
}

// newAPIRouteMetadataRegistry snapshots specs so tests and middleware cannot mutate global route metadata.
func newAPIRouteMetadataRegistry(specs []apiRouteSpec) apiRouteMetadataRegistry {
	return apiRouteMetadataRegistry{specs: append([]apiRouteSpec(nil), specs...)}
}

func (r apiRouteMetadataRegistry) match(method, path string) (apiRouteSpec, bool) {
	for _, spec := range r.specs {
		if !matchRoutePattern(path, spec.pattern) {
			continue
		}
		if method == "" || spec.supportsMethod(method) {
			return spec, true
		}
	}
	return apiRouteSpec{}, false
}

func (r apiRouteMetadataRegistry) matchPath(path string) (apiRouteSpec, bool) {
	for _, spec := range r.specs {
		if matchRoutePattern(path, spec.pattern) {
			return spec, true
		}
	}
	return apiRouteSpec{}, false
}

func (r apiRouteMetadataRegistry) isPublicAuthPath(path string) bool {
	spec, ok := r.matchPath(path)
	return ok && spec.authMode == routeAuthPublic && spec.handlerKey == routeHandlerAuth
}

// validate enforces route metadata invariants before handlers are exposed.
// It catches missing permission policies and route ordering that would make narrower paths unreachable.
func (r apiRouteMetadataRegistry) validate() error {
	seenPatterns := make(map[string]struct{}, len(r.specs))
	for i, spec := range r.specs {
		if strings.TrimSpace(spec.name) == "" {
			return fmt.Errorf("api route at index %d has no name", i)
		}
		if strings.TrimSpace(spec.pattern) == "" {
			return fmt.Errorf("api route %s has no pattern", spec.name)
		}
		if strings.TrimSpace(spec.serveMuxPattern) == "" {
			return fmt.Errorf("api route %s has no serve mux pattern", spec.name)
		}
		if spec.handlerKey == "" {
			return fmt.Errorf("api route %s has no handler key", spec.name)
		}
		if spec.handlerKey == routeHandlerAuth && spec.authEndpoint == routeAuthEndpointNone {
			return fmt.Errorf("api auth route %s has no auth endpoint", spec.name)
		}
		if spec.authMode == 0 {
			return fmt.Errorf("api route %s has no auth mode", spec.name)
		}
		if spec.middlewareProfile == 0 {
			return fmt.Errorf("api route %s has no middleware profile", spec.name)
		}
		if _, exists := seenPatterns[spec.pattern]; exists {
			return fmt.Errorf("duplicate api route pattern %s", spec.pattern)
		}
		seenPatterns[spec.pattern] = struct{}{}
		if spec.authMode == routeAuthProtected || spec.authMode == routeAuthWebSocket {
			if len(spec.methodPolicies) == 0 {
				return fmt.Errorf("protected api route %s has no method policies", spec.pattern)
			}
			for method, permissions := range spec.methodPolicies {
				if strings.TrimSpace(method) == "" {
					return fmt.Errorf("protected api route %s has an empty method", spec.pattern)
				}
				if len(nonEmptyPermissions(permissions...)) == 0 {
					return fmt.Errorf("protected api route %s %s has no permissions", method, spec.pattern)
				}
			}
		}
		for _, previous := range r.specs[:i] {
			if previous.authMode == routeAuthPublic && spec.authMode != routeAuthPublic {
				continue
			}
			if matchRoutePattern(spec.pattern, previous.pattern) {
				return fmt.Errorf("api route pattern %s is shadowed by earlier pattern %s", spec.pattern, previous.pattern)
			}
		}
	}
	return nil
}

func (s apiRouteSpec) supportsMethod(method string) bool {
	if len(s.methodPolicies) == 0 {
		return true
	}
	_, ok := s.methodPolicies[method]
	return ok
}

// protectedRoutePermissionSpecs converts route metadata into middleware permission rules.
func protectedRoutePermissionSpecs(specs []apiRouteSpec) []routePermissionSpec {
	out := make([]routePermissionSpec, 0, len(specs))
	for _, spec := range specs {
		if spec.authMode != routeAuthProtected && spec.authMode != routeAuthWebSocket {
			continue
		}
		out = append(out, routePermissionSpec{
			pattern:     spec.pattern,
			permissions: routePermissionsByMethod(copyMethodPolicies(spec.methodPolicies)),
		})
	}
	return out
}

func publicRoute(name, pattern string, methods []string, handlerKey routeHandlerKey) apiRouteSpec {
	return apiRouteSpec{
		name:              name,
		pattern:           pattern,
		serveMuxPattern:   pattern,
		handlerKey:        handlerKey,
		authMode:          routeAuthPublic,
		middlewareProfile: routeMiddlewarePublicJSONSmallBody,
		methodPolicies:    methodsWithoutPermissions(methods),
	}
}

func publicAuthRoute(name, pattern string, methods []string, endpoint routeAuthEndpoint) apiRouteSpec {
	spec := publicRoute(name, pattern, methods, routeHandlerAuth)
	spec.authEndpoint = endpoint
	return spec
}

func protectedRoute(name, pattern, serveMuxPattern string, handlerKey routeHandlerKey, profile routeMiddlewareProfile, methodPolicies map[string][]string) apiRouteSpec {
	return apiRouteSpec{
		name:              name,
		pattern:           pattern,
		serveMuxPattern:   serveMuxPattern,
		handlerKey:        handlerKey,
		authMode:          routeAuthProtected,
		middlewareProfile: profile,
		methodPolicies:    copyMethodPolicies(methodPolicies),
	}
}

func websocketRoute(name, pattern string, methodPolicies map[string][]string) apiRouteSpec {
	return apiRouteSpec{
		name:              name,
		pattern:           pattern,
		serveMuxPattern:   pattern,
		handlerKey:        routeHandlerWebSocket,
		authMode:          routeAuthWebSocket,
		middlewareProfile: routeMiddlewareWebSocketUpgrade,
		methodPolicies:    copyMethodPolicies(methodPolicies),
	}
}

func methodsWithoutPermissions(methods []string) map[string][]string {
	out := make(map[string][]string, len(methods))
	for _, method := range methods {
		out[method] = nil
	}
	return out
}

func copyMethodPolicies(input map[string][]string) map[string][]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string][]string, len(input))
	for method, permissions := range input {
		out[method] = append([]string(nil), permissions...)
	}
	return out
}

func readPolicy(permissions ...string) map[string][]string {
	return map[string][]string{
		http.MethodGet:  permissions,
		http.MethodHead: permissions,
	}
}

func postPolicy(permissions ...string) map[string][]string {
	return map[string][]string{http.MethodPost: permissions}
}

func putPolicy(permissions ...string) map[string][]string {
	return map[string][]string{http.MethodPut: permissions}
}

func patchPolicy(permissions ...string) map[string][]string {
	return map[string][]string{http.MethodPatch: permissions}
}

func deletePolicy(permissions ...string) map[string][]string {
	return map[string][]string{http.MethodDelete: permissions}
}

func policy(entries ...routeMethodPolicyEntry) map[string][]string {
	out := make(map[string][]string, len(entries))
	for _, entry := range entries {
		out[entry.method] = append([]string(nil), entry.permissions...)
	}
	return out
}

type routeMethodPolicyEntry struct {
	method      string
	permissions []string
}

func methodPolicy(method string, permissions ...string) routeMethodPolicyEntry {
	return routeMethodPolicyEntry{method: method, permissions: permissions}
}

var apiRouteSpecs = []apiRouteSpec{
	publicAuthRoute("auth login", "/api/v1/auth/login", []string{http.MethodPost}, routeAuthEndpointLogin),
	publicAuthRoute("auth logout", "/api/v1/auth/logout", []string{http.MethodPost, http.MethodDelete}, routeAuthEndpointLogout),
	publicAuthRoute("auth me", "/api/v1/auth/me", []string{http.MethodGet}, routeAuthEndpointMe),
	publicAuthRoute("legacy me", "/api/v1/me", []string{http.MethodGet}, routeAuthEndpointMe),
	publicAuthRoute("auth password change", "/api/v1/auth/password/change", []string{http.MethodPost}, routeAuthEndpointPasswordChange),
	publicAuthRoute("auth password reset", "/api/v1/auth/password/reset", []string{http.MethodPost}, routeAuthEndpointPasswordReset),
	publicAuthRoute("legacy session", "/api/v1/session", []string{http.MethodGet, http.MethodDelete, http.MethodPost}, routeAuthEndpointLegacySession),
	publicRoute("bridge connector launch", "/api/v1/bridge/connector/launch", []string{http.MethodPost}, routeHandlerBridgeConnectorLaunch),

	protectedRoute("settings me", "/api/v1/settings/me", "/api/v1/settings/me", routeHandlerSettingsMe, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionAccountManage),
		methodPolicy(http.MethodHead, domain.PermissionAccountManage),
		methodPolicy(http.MethodPatch, domain.PermissionAccountManage),
	)),
	protectedRoute("settings bridge", "/api/v1/settings/bridge", "/api/v1/settings/bridge", routeHandlerSettingsBridge, routeMiddlewareNormalJSON, readPolicy(domain.PermissionAccountManage)),
	protectedRoute("settings bridge secret", "/api/v1/settings/bridge/secret", "/api/v1/settings/bridge/secret", routeHandlerSettingsBridgeSecret, routeMiddlewareNormalJSON, postPolicy(domain.PermissionAccountManage)),
	protectedRoute("settings bridge secret rotate", "/api/v1/settings/bridge/secret/rotate", "/api/v1/settings/bridge/secret/rotate", routeHandlerSettingsBridgeSecret, routeMiddlewareNormalJSON, postPolicy(domain.PermissionAccountManage)),
	protectedRoute("settings bridge secret revoke", "/api/v1/settings/bridge/secret/revoke", "/api/v1/settings/bridge/secret/revoke", routeHandlerSettingsBridgeSecretRevoke, routeMiddlewareNormalJSON, postPolicy(domain.PermissionAccountManage)),
	protectedRoute("settings bridge connector config", "/api/v1/settings/bridge/connector/config", "/api/v1/settings/bridge/connector/config", routeHandlerSettingsBridgeConnector, routeMiddlewareNormalJSON, readPolicy(domain.PermissionAccountManage)),
	protectedRoute("settings bridge connector download", "/api/v1/settings/bridge/connector/download/{os}/{arch}", "/api/v1/settings/bridge/connector/download/", routeHandlerSettingsBridgeDownload, routeMiddlewareNormalJSON, readPolicy(domain.PermissionAccountManage)),
	protectedRoute("settings collection", "/api/v1/settings", "/api/v1/settings", routeHandlerSettingsCollection, routeMiddlewareNormalJSON, readPolicy(domain.PermissionSettingsRead)),
	protectedRoute("settings item", "/api/v1/settings/{key}", "/api/v1/settings/", routeHandlerSettingsItem, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionSettingsRead),
		methodPolicy(http.MethodHead, domain.PermissionSettingsRead),
		methodPolicy(http.MethodPut, domain.PermissionSettingsUpdate),
	)),
	protectedRoute("runtime overview", "/api/v1/runtime/overview", "/api/v1/runtime/overview", routeHandlerRuntimeOverview, routeMiddlewareNormalJSON, readPolicy(domain.PermissionTopologyRead)),

	protectedRoute("topology canvas", "/api/v1/topology/canvas", "/api/v1/topology/canvas", routeHandlerTopologyCanvas, routeMiddlewareNormalJSON, readPolicy(domain.PermissionTopologyRead)),
	protectedRoute("canvas", "/api/v1/canvas", "/api/v1/canvas", routeHandlerCanvas, routeMiddlewareNormalJSON, readPolicy(domain.PermissionTopologyRead)),
	protectedRoute("canvas maps", "/api/v1/canvas/maps", "/api/v1/canvas/maps", routeHandlerCanvasMapCollection, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionTopologyRead),
		methodPolicy(http.MethodHead, domain.PermissionTopologyRead),
		methodPolicy(http.MethodPost, domain.PermissionTopologyUpdate),
	)),
	protectedRoute("canvas map", "/api/v1/canvas/maps/{mapID}", "/api/v1/canvas/maps/", routeHandlerCanvasMapItem, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionTopologyRead),
		methodPolicy(http.MethodHead, domain.PermissionTopologyRead),
		methodPolicy(http.MethodPatch, domain.PermissionTopologyUpdate),
		methodPolicy(http.MethodDelete, domain.PermissionTopologyUpdate),
	)),
	protectedRoute("canvas map duplicate", "/api/v1/canvas/maps/{mapID}/duplicate", "/api/v1/canvas/maps/", routeHandlerCanvasMapItem, routeMiddlewareNormalJSON, postPolicy(domain.PermissionTopologyUpdate)),
	protectedRoute("canvas map primary", "/api/v1/canvas/maps/{mapID}/primary", "/api/v1/canvas/maps/", routeHandlerCanvasMapItem, routeMiddlewareNormalJSON, postPolicy(domain.PermissionTopologyUpdate)),
	protectedRoute("canvas map topology", "/api/v1/canvas/maps/{mapID}/topology", "/api/v1/canvas/maps/", routeHandlerCanvasMapItem, routeMiddlewareNormalJSON, readPolicy(domain.PermissionTopologyRead)),
	protectedRoute("canvas map bootstrap", "/api/v1/canvas/maps/{mapID}/bootstrap", "/api/v1/canvas/maps/", routeHandlerCanvasMapItem, routeMiddlewareNormalJSON, readPolicy(domain.PermissionTopologyRead)),
	protectedRoute("canvas map positions", "/api/v1/canvas/maps/{mapID}/positions", "/api/v1/canvas/maps/", routeHandlerCanvasMapItem, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionTopologyRead),
		methodPolicy(http.MethodHead, domain.PermissionTopologyRead),
		methodPolicy(http.MethodPut, domain.PermissionTopologyUpdate),
	)),
	protectedRoute("canvas map device areas", "/api/v1/canvas/maps/{mapID}/device-areas", "/api/v1/canvas/maps/", routeHandlerCanvasMapItem, routeMiddlewareNormalJSON, putPolicy(domain.PermissionTopologyUpdate)),
	protectedRoute("canvas map areas", "/api/v1/canvas/maps/{mapID}/areas", "/api/v1/canvas/maps/", routeHandlerCanvasMapItem, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionTopologyRead),
		methodPolicy(http.MethodHead, domain.PermissionTopologyRead),
		methodPolicy(http.MethodPost, domain.PermissionTopologyUpdate),
	)),
	protectedRoute("canvas map area", "/api/v1/canvas/maps/{mapID}/areas/{areaID}", "/api/v1/canvas/maps/", routeHandlerCanvasMapItem, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodPut, domain.PermissionTopologyUpdate),
		methodPolicy(http.MethodDelete, domain.PermissionTopologyUpdate),
	)),
	protectedRoute("canvas map device", "/api/v1/canvas/maps/{mapID}/devices/{deviceID}", "/api/v1/canvas/maps/", routeHandlerCanvasMapItem, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodPost, domain.PermissionTopologyUpdate),
		methodPolicy(http.MethodPatch, domain.PermissionTopologyUpdate),
		methodPolicy(http.MethodDelete, domain.PermissionTopologyUpdate),
	)),
	protectedRoute("canvas map link route", "/api/v1/canvas/maps/{mapID}/link-routes/{linkID}", "/api/v1/canvas/maps/", routeHandlerCanvasMapItem, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodPut, domain.PermissionTopologyUpdate),
		methodPolicy(http.MethodDelete, domain.PermissionTopologyUpdate),
	)),

	protectedRoute("devices", "/api/v1/devices", "/api/v1/devices", routeHandlerDeviceCollection, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionDevicesRead),
		methodPolicy(http.MethodHead, domain.PermissionDevicesRead),
		methodPolicy(http.MethodPost, domain.PermissionDevicesCreate, domain.PermissionDevicesUpdate),
	)),
	protectedRoute("devices batch", "/api/v1/devices/batch", "/api/v1/devices/batch", routeHandlerDeviceBatch, routeMiddlewareNormalJSON, postPolicy(domain.PermissionDevicesCreate, domain.PermissionDevicesUpdate)),
	protectedRoute("devices orphans", "/api/v1/devices/orphans", "/api/v1/devices/orphans", routeHandlerDeviceOrphans, routeMiddlewareNormalJSON, readPolicy(domain.PermissionDevicesRead)),
	protectedRoute("device winbox credentials reveal", "/api/v1/devices/{deviceID}/winbox-credentials/reveal", "/api/v1/devices/", routeHandlerDeviceItem, routeMiddlewareNormalJSON, postPolicy(domain.PermissionCredentialsReveal)),
	protectedRoute("device credential profile", "/api/v1/devices/{deviceID}/credential-profiles/{profileID}", "/api/v1/devices/", routeHandlerDeviceItem, routeMiddlewareNormalJSON, deletePolicy(domain.PermissionCredentialsUpdate)),
	protectedRoute("device credential profiles", "/api/v1/devices/{deviceID}/credential-profiles", "/api/v1/devices/", routeHandlerDeviceItem, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionCredentialsRead),
		methodPolicy(http.MethodHead, domain.PermissionCredentialsRead),
		methodPolicy(http.MethodPost, domain.PermissionCredentialsUpdate),
	)),
	protectedRoute("device winbox profile", "/api/v1/devices/{deviceID}/winbox-profile", "/api/v1/devices/", routeHandlerDeviceItem, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodPut, domain.PermissionCredentialsUpdate),
		methodPolicy(http.MethodDelete, domain.PermissionCredentialsUpdate),
	)),
	protectedRoute("device winbox credentials", "/api/v1/devices/{deviceID}/winbox-credentials", "/api/v1/devices/", routeHandlerDeviceItem, routeMiddlewareNormalJSON, readPolicy(domain.PermissionCredentialsRead)),
	protectedRoute("device latest backup", "/api/v1/devices/{deviceID}/backups/latest", "/api/v1/devices/", routeHandlerDeviceItem, routeMiddlewareNormalJSON, readPolicy(domain.PermissionBackupsRead)),
	protectedRoute("device backups", "/api/v1/devices/{deviceID}/backups", "/api/v1/devices/", routeHandlerDeviceItem, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionBackupsRead),
		methodPolicy(http.MethodHead, domain.PermissionBackupsRead),
		methodPolicy(http.MethodPost, domain.PermissionBackupsUpdate),
	)),
	protectedRoute("device interfaces", "/api/v1/devices/{deviceID}/interfaces", "/api/v1/devices/", routeHandlerDeviceItem, routeMiddlewareNormalJSON, readPolicy(domain.PermissionTopologyRead)),
	protectedRoute("device probe", "/api/v1/devices/{deviceID}/probe", "/api/v1/devices/", routeHandlerDeviceItem, routeMiddlewareNormalJSON, postPolicy(domain.PermissionDevicesUpdate)),
	protectedRoute("device snmp test", "/api/v1/devices/{deviceID}/snmp-test", "/api/v1/devices/", routeHandlerDeviceItem, routeMiddlewareNormalJSON, postPolicy(domain.PermissionDevicesUpdate)),
	protectedRoute("device topology discovery", "/api/v1/devices/{deviceID}/topology-discovery", "/api/v1/devices/", routeHandlerDeviceItem, routeMiddlewareNormalJSON, postPolicy(domain.PermissionTopologyUpdate)),
	protectedRoute("device address reachability", "/api/v1/devices/{deviceID}/addresses/reachability", "/api/v1/devices/", routeHandlerDeviceItem, routeMiddlewareNormalJSON, postPolicy(domain.PermissionDevicesUpdate)),
	protectedRoute("device ssh credential test", "/api/v1/devices/{deviceID}/ssh-credentials/test", "/api/v1/devices/", routeHandlerDeviceItem, routeMiddlewareNormalJSON, postPolicy(domain.PermissionDevicesCreate, domain.PermissionDevicesUpdate)),
	protectedRoute("device ssh host key reset", "/api/v1/devices/{deviceID}/ssh-host-key/reset", "/api/v1/devices/", routeHandlerDeviceItem, routeMiddlewareNormalJSON, postPolicy(domain.PermissionBackupsUpdate)),
	protectedRoute("device", "/api/v1/devices/{deviceID}", "/api/v1/devices/", routeHandlerDeviceItem, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionDevicesRead),
		methodPolicy(http.MethodHead, domain.PermissionDevicesRead),
		methodPolicy(http.MethodPut, domain.PermissionDevicesUpdate),
		methodPolicy(http.MethodDelete, domain.PermissionDevicesDelete),
	)),

	protectedRoute("links", "/api/v1/links", "/api/v1/links", routeHandlerLinkCollection, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionTopologyRead),
		methodPolicy(http.MethodHead, domain.PermissionTopologyRead),
		methodPolicy(http.MethodPost, domain.PermissionTopologyUpdate),
	)),
	protectedRoute("link", "/api/v1/links/{linkID}", "/api/v1/links/", routeHandlerLinkItem, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodPut, domain.PermissionTopologyUpdate),
		methodPolicy(http.MethodDelete, domain.PermissionTopologyUpdate),
	)),
	protectedRoute("positions", "/api/v1/positions", "/api/v1/positions", routeHandlerPositionCollection, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionTopologyRead),
		methodPolicy(http.MethodHead, domain.PermissionTopologyRead),
		methodPolicy(http.MethodPut, domain.PermissionTopologyUpdate),
	)),

	protectedRoute("grafana dashboard profiles", "/api/v1/grafana/dashboard-profiles", "/api/v1/grafana/dashboard-profiles", routeHandlerGrafanaProfileCollection, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionSettingsRead),
		methodPolicy(http.MethodHead, domain.PermissionSettingsRead),
		methodPolicy(http.MethodPost, domain.PermissionSettingsUpdate),
	)),
	protectedRoute("grafana dashboard profile", "/api/v1/grafana/dashboard-profiles/{profileID}", "/api/v1/grafana/dashboard-profiles/", routeHandlerGrafanaProfileItem, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodPut, domain.PermissionSettingsUpdate),
		methodPolicy(http.MethodDelete, domain.PermissionSettingsUpdate),
	)),
	protectedRoute("grafana device override", "/api/v1/grafana/device-overrides/{deviceID}", "/api/v1/grafana/device-overrides/", routeHandlerGrafanaDeviceOverride, routeMiddlewareNormalJSON, putPolicy(domain.PermissionSettingsUpdate)),

	protectedRoute("snmp profile reveal", "/api/v1/snmp-profiles/{profileID}/reveal", "/api/v1/snmp-profiles/", routeHandlerSNMPProfileItem, routeMiddlewareNormalJSON, postPolicy(domain.PermissionCredentialsReveal)),
	protectedRoute("snmp profiles", "/api/v1/snmp-profiles", "/api/v1/snmp-profiles", routeHandlerSNMPProfileCollection, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionCredentialsRead),
		methodPolicy(http.MethodHead, domain.PermissionCredentialsRead),
		methodPolicy(http.MethodPost, domain.PermissionCredentialsUpdate),
	)),
	protectedRoute("snmp profile", "/api/v1/snmp-profiles/{profileID}", "/api/v1/snmp-profiles/", routeHandlerSNMPProfileItem, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionCredentialsRead),
		methodPolicy(http.MethodHead, domain.PermissionCredentialsRead),
		methodPolicy(http.MethodPut, domain.PermissionCredentialsUpdate),
		methodPolicy(http.MethodDelete, domain.PermissionCredentialsUpdate),
	)),
	protectedRoute("credential profile test", "/api/v1/credential-profiles/{profileID}/test", "/api/v1/credential-profiles/", routeHandlerCredentialProfileItem, routeMiddlewareNormalJSON, postPolicy(domain.PermissionCredentialsUpdate)),
	protectedRoute("credential profiles", "/api/v1/credential-profiles", "/api/v1/credential-profiles", routeHandlerCredentialProfileCollection, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionCredentialsRead),
		methodPolicy(http.MethodHead, domain.PermissionCredentialsRead),
		methodPolicy(http.MethodPost, domain.PermissionCredentialsUpdate),
	)),
	protectedRoute("credential profile", "/api/v1/credential-profiles/{profileID}", "/api/v1/credential-profiles/", routeHandlerCredentialProfileItem, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionCredentialsRead),
		methodPolicy(http.MethodHead, domain.PermissionCredentialsRead),
		methodPolicy(http.MethodPut, domain.PermissionCredentialsUpdate),
		methodPolicy(http.MethodDelete, domain.PermissionCredentialsUpdate),
	)),

	protectedRoute("areas", "/api/v1/areas", "/api/v1/areas", routeHandlerAreaCollection, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionTopologyRead),
		methodPolicy(http.MethodHead, domain.PermissionTopologyRead),
		methodPolicy(http.MethodPost, domain.PermissionTopologyUpdate),
	)),
	protectedRoute("area", "/api/v1/areas/{areaID}", "/api/v1/areas/", routeHandlerAreaItem, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionTopologyRead),
		methodPolicy(http.MethodHead, domain.PermissionTopologyRead),
		methodPolicy(http.MethodPut, domain.PermissionTopologyUpdate),
		methodPolicy(http.MethodDelete, domain.PermissionTopologyUpdate),
	)),

	protectedRoute("backup bulk status", "/api/v1/backups/bulk/status", "/api/v1/backups/bulk/status", routeHandlerBackupBulkStatus, routeMiddlewareNormalJSON, readPolicy(domain.PermissionBackupsRead)),
	protectedRoute("backup bulk run latest", "/api/v1/backups/bulk-runs/latest", "/api/v1/backups/bulk-runs/latest", routeHandlerBackupBulkRunLatest, routeMiddlewareNormalJSON, readPolicy(domain.PermissionBackupsRead)),
	protectedRoute("backup bulk runs", "/api/v1/backups/bulk-runs", "/api/v1/backups/bulk-runs", routeHandlerBackupBulkRunCollection, routeMiddlewareNormalJSON, postPolicy(domain.PermissionBackupsUpdate)),
	protectedRoute("backup bulk run pause", "/api/v1/backups/bulk-runs/{runID}/pause", "/api/v1/backups/bulk-runs/", routeHandlerBackupBulkRunItem, routeMiddlewareNormalJSON, postPolicy(domain.PermissionBackupsUpdate)),
	protectedRoute("backup bulk run resume", "/api/v1/backups/bulk-runs/{runID}/resume", "/api/v1/backups/bulk-runs/", routeHandlerBackupBulkRunItem, routeMiddlewareNormalJSON, postPolicy(domain.PermissionBackupsUpdate)),
	protectedRoute("backup bulk run cancel", "/api/v1/backups/bulk-runs/{runID}/cancel", "/api/v1/backups/bulk-runs/", routeHandlerBackupBulkRunItem, routeMiddlewareNormalJSON, postPolicy(domain.PermissionBackupsUpdate)),
	protectedRoute("backup bulk run", "/api/v1/backups/bulk-runs/{runID}", "/api/v1/backups/bulk-runs/", routeHandlerBackupBulkRunItem, routeMiddlewareNormalJSON, readPolicy(domain.PermissionBackupsRead)),
	protectedRoute("backup bulk download", "/api/v1/backups/bulk-download", "/api/v1/backups/bulk-download", routeHandlerBackupBulkDownload, routeMiddlewareNormalJSON, postPolicy(domain.PermissionBackupsUpdate)),
	protectedRoute("backup job", "/api/v1/backup-jobs/{jobID}", "/api/v1/backup-jobs/", routeHandlerBackupJob, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionBackupsRead),
		methodPolicy(http.MethodHead, domain.PermissionBackupsRead),
		methodPolicy(http.MethodDelete, domain.PermissionBackupsUpdate),
	)),
	protectedRoute("backup file download", "/api/v1/backup-files/{fileID}/download", "/api/v1/backup-files/", routeHandlerBackupFile, routeMiddlewareBinaryDownload, readPolicy(domain.PermissionBackupsRead)),
	protectedRoute("backup file content", "/api/v1/backup-files/{fileID}/content", "/api/v1/backup-files/", routeHandlerBackupFile, routeMiddlewareNormalJSON, readPolicy(domain.PermissionBackupsRead)),

	protectedRoute("vendors", "/api/v1/vendors", "/api/v1/vendors", routeHandlerVendorCollection, routeMiddlewareNormalJSON, readPolicy(domain.PermissionSettingsRead)),
	protectedRoute("vendor", "/api/v1/vendors/{vendorID}", "/api/v1/vendors/", routeHandlerVendorItem, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionSettingsRead),
		methodPolicy(http.MethodHead, domain.PermissionSettingsRead),
		methodPolicy(http.MethodPut, domain.PermissionSettingsUpdate),
	)),

	protectedRoute("instance backups", "/api/v1/instance-backups", "/api/v1/instance-backups", routeHandlerInstanceBackupCollection, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionBackupsRead),
		methodPolicy(http.MethodHead, domain.PermissionBackupsRead),
		methodPolicy(http.MethodPost, domain.PermissionBackupsUpdate),
	)),
	protectedRoute("instance backup restore status", "/api/v1/instance-backups/restore-status", "/api/v1/instance-backups/restore-status", routeHandlerInstanceBackupRestoreStatus, routeMiddlewareNormalJSON, readPolicy(domain.PermissionBackupsRead)),
	protectedRoute("instance backup restore", "/api/v1/instance-backups/restore", "/api/v1/instance-backups/restore", routeHandlerInstanceBackupRestore, routeMiddlewareRestoreUpload, postPolicy(domain.PermissionBackupsUpdate)),
	protectedRoute("instance backup download", "/api/v1/instance-backups/{backupID}/download", "/api/v1/instance-backups/", routeHandlerInstanceBackupItem, routeMiddlewareBinaryDownload, readPolicy(domain.PermissionBackupsRead)),
	protectedRoute("instance backup cancel", "/api/v1/instance-backups/{backupID}/cancel", "/api/v1/instance-backups/", routeHandlerInstanceBackupItem, routeMiddlewareNormalJSON, postPolicy(domain.PermissionBackupsUpdate)),
	protectedRoute("instance backup", "/api/v1/instance-backups/{backupID}", "/api/v1/instance-backups/", routeHandlerInstanceBackupItem, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionBackupsRead),
		methodPolicy(http.MethodHead, domain.PermissionBackupsRead),
		methodPolicy(http.MethodDelete, domain.PermissionBackupsUpdate),
	)),

	protectedRoute("bridge download", "/api/v1/bridge/download/{os}/{arch}", "/api/v1/bridge/download/", routeHandlerBridgeDownload, routeMiddlewareBinaryDownload, readPolicy(domain.PermissionSettingsRead)),
	protectedRoute("bridge launch request", "/api/v1/bridge/launch-requests/{deviceID}", "/api/v1/bridge/launch-requests/", routeHandlerBridgeLaunchRequest, routeMiddlewareNormalJSON, postPolicy(domain.PermissionBridgeTokenCreate)),
	protectedRoute("bridge token", "/api/v1/bridge/token/{deviceID}", "/api/v1/bridge/token/", routeHandlerBridgeToken, routeMiddlewareNormalJSON, postPolicy(domain.PermissionBridgeTokenCreate)),

	protectedRoute("device import preview", "/api/v1/admin/device-imports/preview", "/api/v1/admin/device-imports/preview", routeHandlerDeviceImport, routeMiddlewareDeviceImportUpload, postPolicy(domain.PermissionAdminDashboard)),
	protectedRoute("device import commit", "/api/v1/admin/device-imports/commit", "/api/v1/admin/device-imports/commit", routeHandlerDeviceImport, routeMiddlewareDeviceImportUpload, postPolicy(domain.PermissionAdminDashboard)),
	protectedRoute("admin dashboard", "/api/v1/admin/dashboard", "/api/v1/admin/", routeHandlerAdmin, routeMiddlewareNormalJSON, readPolicy(domain.PermissionAdminDashboard)),
	protectedRoute("admin users", "/api/v1/admin/users", "/api/v1/admin/", routeHandlerAdmin, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionUsersRead),
		methodPolicy(http.MethodHead, domain.PermissionUsersRead),
		methodPolicy(http.MethodPost, domain.PermissionUsersCreate, domain.PermissionUsersUpdate),
	)),
	protectedRoute("admin user status", "/api/v1/admin/users/{userID}/status", "/api/v1/admin/", routeHandlerAdmin, routeMiddlewareNormalJSON, patchPolicy(domain.PermissionUsersUpdate)),
	protectedRoute("admin user role", "/api/v1/admin/users/{userID}/roles/{roleID}", "/api/v1/admin/", routeHandlerAdmin, routeMiddlewareNormalJSON, deletePolicy(domain.PermissionRolesAssign)),
	protectedRoute("admin user roles", "/api/v1/admin/users/{userID}/roles", "/api/v1/admin/", routeHandlerAdmin, routeMiddlewareNormalJSON, postPolicy(domain.PermissionRolesAssign)),
	protectedRoute("admin user password reset", "/api/v1/admin/users/{userID}/password-reset", "/api/v1/admin/", routeHandlerAdmin, routeMiddlewareNormalJSON, postPolicy(domain.PermissionUsersUpdate)),
	protectedRoute("admin user", "/api/v1/admin/users/{userID}", "/api/v1/admin/", routeHandlerAdmin, routeMiddlewareNormalJSON, policy(
		methodPolicy(http.MethodGet, domain.PermissionUsersRead),
		methodPolicy(http.MethodHead, domain.PermissionUsersRead),
		methodPolicy(http.MethodPatch, domain.PermissionUsersUpdate),
	)),
	protectedRoute("admin role permissions", "/api/v1/admin/roles/{roleID}/permissions", "/api/v1/admin/", routeHandlerAdmin, routeMiddlewareNormalJSON, patchPolicy(domain.PermissionRolesUpdate)),
	protectedRoute("admin roles", "/api/v1/admin/roles", "/api/v1/admin/", routeHandlerAdmin, routeMiddlewareNormalJSON, readPolicy(domain.PermissionRolesRead)),
	protectedRoute("admin permissions", "/api/v1/admin/permissions", "/api/v1/admin/", routeHandlerAdmin, routeMiddlewareNormalJSON, readPolicy(domain.PermissionRolesRead)),
	protectedRoute("admin audit logs", "/api/v1/admin/audit-logs", "/api/v1/admin/", routeHandlerAdmin, routeMiddlewareNormalJSON, readPolicy(domain.PermissionAuditLogsRead)),

	protectedRoute("health", "/api/v1/health", "/api/v1/health", routeHandlerHealth, routeMiddlewareNormalJSON, readPolicy(domain.PermissionSettingsRead)),
	protectedRoute("prometheus health", "/api/v1/prometheus/health", "/api/v1/prometheus/health", routeHandlerPrometheusHealth, routeMiddlewareNormalJSON, readPolicy(domain.PermissionSettingsRead)),
	websocketRoute("websocket", "/api/v1/ws", readPolicy(domain.PermissionTopologyRead)),
}

var apiRouteMetadata = newAPIRouteMetadataRegistry(apiRouteSpecs)
