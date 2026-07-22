package api

// This file defines router middleware API routing, middleware, and permission policy behavior.

import (
	"net/http"

	"github.com/lollinoo/theia/internal/service"
)

// routerMiddlewareSet keeps the normal JSON chain separate from routes that cannot use it.
// WebSocket upgrades, binary downloads, and restore uploads each need different body/response handling.
type routerMiddlewareSet struct {
	normal             http.Handler
	binaryDownload     http.Handler
	restoreUpload      http.Handler
	deviceImportUpload http.Handler
	publicByHandler    map[routeHandlerKey]http.Handler
}

// buildRouterMiddlewareSet assembles reusable middleware chains after handlers have been constructed.
// Restore upload limits follow the service archive quota so route handling and validation share one ceiling.
func buildRouterMiddlewareSet(
	mux http.Handler,
	deps routerDependencies,
	routeHandlers map[routeHandlerKey]http.Handler,
	routerOpts routerOptions,
) routerMiddlewareSet {
	restoreLimit := service.DefaultRestoreArchiveLimits.MaxCompressedBytes
	if deps.instanceBackupService != nil {
		restoreLimit = deps.instanceBackupService.RestoreArchiveLimits().MaxCompressedBytes
	}

	return routerMiddlewareSet{
		normal:         applyMiddleware(mux, routerOpts.security, routerOpts.auth, true, 1<<20),
		binaryDownload: applyMiddleware(mux, routerOpts.security, routerOpts.auth, false, 0),
		restoreUpload:  applyMiddleware(mux, routerOpts.security, routerOpts.auth, false, restoreLimit+restoreMultipartEnvelopeOverheadBytes),
		deviceImportUpload: applyMiddleware(
			mux,
			routerOpts.security,
			routerOpts.auth,
			true,
			int64(service.DeviceImportMaxFileBytes)+deviceImportMultipartEnvelopeOverheadBytes,
		),
		publicByHandler: map[routeHandlerKey]http.Handler{
			routeHandlerAuth:                  applyPublicMiddleware(routeHandlers[routeHandlerAuth], routerOpts.security, true, 16<<10),
			routeHandlerBridgeConnectorLaunch: applyPublicMiddleware(routeHandlers[routeHandlerBridgeConnectorLaunch], routerOpts.security, true, 16<<10),
		},
	}
}

// servePublicRoute dispatches explicitly public route specs through small-body public middleware.
func (middleware routerMiddlewareSet) servePublicRoute(w http.ResponseWriter, r *http.Request, spec apiRouteSpec) bool {
	if spec.authMode != routeAuthPublic {
		return false
	}
	publicHandler := middleware.publicByHandler[spec.handlerKey]
	if publicHandler == nil {
		return false
	}
	publicHandler.ServeHTTP(w, r)
	return true
}

// serveRouteProfile handles special middleware profiles and leaves ordinary JSON routes to the normal chain.
func (middleware routerMiddlewareSet) serveRouteProfile(
	w http.ResponseWriter,
	r *http.Request,
	spec apiRouteSpec,
	deps routerDependencies,
	routerOpts routerOptions,
) bool {
	switch spec.middlewareProfile {
	case routeMiddlewareWebSocketUpgrade:
		return serveWebSocketUpgradeRoute(w, r, spec, deps, routerOpts)
	case routeMiddlewareBinaryDownload:
		middleware.binaryDownload.ServeHTTP(w, r)
		return true
	case routeMiddlewareRestoreUpload:
		middleware.restoreUpload.ServeHTTP(w, r)
		return true
	case routeMiddlewareDeviceImportUpload:
		middleware.deviceImportUpload.ServeHTTP(w, r)
		return true
	default:
		return false
	}
}

// serveWebSocketUpgradeRoute authenticates and authorizes a WebSocket request without wrapping the ResponseWriter.
// The upgrade path must preserve http.Hijacker support, so it bypasses the standard logger/JSON middleware.
func serveWebSocketUpgradeRoute(
	w http.ResponseWriter,
	r *http.Request,
	spec apiRouteSpec,
	deps routerDependencies,
	routerOpts routerOptions,
) bool {
	if deps.wsHandler == nil {
		return false
	}
	// WebSocket upgrades must bypass the JSON/logger middleware chain because
	// the wrapped ResponseWriter does not expose the hijacker interface.
	authenticatedRequest, user, _, ok := AuthenticateUserRequest(w, r, routerOpts.auth)
	if !ok {
		return true
	}
	if user.User.User.MustChangePassword {
		writeAuthCodeError(w, http.StatusForbidden, "password_change_required", "password change required")
		return true
	}
	if !requireAnyPermission(w, routerOpts.auth, user, spec.methodPolicies[r.Method]) {
		return true
	}
	deps.wsHandler.ServeHTTP(w, authenticatedRequest)
	return true
}
