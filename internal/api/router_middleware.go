package api

import (
	"net/http"

	"github.com/lollinoo/theia/internal/service"
)

type routerMiddlewareSet struct {
	normal          http.Handler
	binaryDownload  http.Handler
	restoreUpload   http.Handler
	publicByHandler map[routeHandlerKey]http.Handler
}

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
		publicByHandler: map[routeHandlerKey]http.Handler{
			routeHandlerAuth:                  applyPublicMiddleware(routeHandlers[routeHandlerAuth], routerOpts.security, true, 16<<10),
			routeHandlerBridgeConnectorLaunch: applyPublicMiddleware(routeHandlers[routeHandlerBridgeConnectorLaunch], routerOpts.security, true, 16<<10),
		},
	}
}

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
	default:
		return false
	}
}

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
