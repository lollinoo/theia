package api

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/repository/postgres"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/vendor"
	"github.com/lollinoo/theia/internal/ws"
)

// routerOptions collects optional services that alter middleware or route behavior.
type routerOptions struct {
	security      SecurityConfig
	auth          authProvider
	bridgeService *service.BridgeService
	auditLogs     domain.AuditLogRepository
}

// RouterOption customizes router middleware behavior.
type RouterOption func(*routerOptions)

// WithSecurity configures operator authentication and browser origin policy.
func WithSecurity(config SecurityConfig) RouterOption {
	return func(options *routerOptions) {
		options.security = config
	}
}

// WithAuthService configures password-session authentication and RBAC.
func WithAuthService(authService *service.AuthService) RouterOption {
	return func(options *routerOptions) {
		options.auth = authService
	}
}

// WithBridgeService configures bridge launch and connector endpoints.
func WithBridgeService(bridgeService *service.BridgeService) RouterOption {
	return func(options *routerOptions) {
		options.bridgeService = bridgeService
	}
}

// WithAuditLogRepository configures audit logging for backup routes.
func WithAuditLogRepository(auditLogs domain.AuditLogRepository) RouterOption {
	return func(options *routerOptions) {
		options.auditLogs = auditLogs
	}
}

// withAuthProvider injects a test auth provider without requiring the concrete service.
func withAuthProvider(auth authProvider) RouterOption {
	return func(options *routerOptions) {
		options.auth = auth
	}
}

// NewRouter creates the HTTP handler with all /api/v1/ routes registered.
// Route metadata remains the source of truth for auth, middleware, and permissions;
// the returned handler only selects public, special-profile, or normal middleware.
func NewRouter(
	db *sql.DB,
	deviceService *service.DeviceService,
	linkRepo domain.LinkRepository,
	positionRepo domain.PositionRepository,
	canvasMapRepo domain.CanvasMapRepository,
	canvasMapPositionRepo domain.CanvasMapPositionRepository,
	settingsRepo domain.SettingsRepository,
	snmpProfileRepo domain.SNMPProfileRepository,
	credentialProfileRepo *postgres.CredentialProfileRepo,
	areaRepo domain.AreaRepository,
	backupService *service.BackupService,
	vendorRegistry *vendor.Registry,
	vendorConfigRepo domain.VendorConfigRepository,
	poller statusProvider,
	instanceBackupService *service.InstanceBackupService,
	restoreRestarter func(),
	bridgeBinariesDir string,
	runtimeSnapshotFunc func() (*ws.SnapshotPayload, uint64),
	wsHandler *ws.Handler,
	opts ...RouterOption,
) http.Handler {
	routerOpts := routerOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&routerOpts)
		}
	}
	deps := routerDependencies{
		db:                    db,
		deviceService:         deviceService,
		linkRepo:              linkRepo,
		positionRepo:          positionRepo,
		canvasMapRepo:         canvasMapRepo,
		canvasMapPositionRepo: canvasMapPositionRepo,
		settingsRepo:          settingsRepo,
		snmpProfileRepo:       snmpProfileRepo,
		credentialProfileRepo: credentialProfileRepo,
		areaRepo:              areaRepo,
		backupService:         backupService,
		vendorRegistry:        vendorRegistry,
		vendorConfigRepo:      vendorConfigRepo,
		poller:                poller,
		instanceBackupService: instanceBackupService,
		restoreRestarter:      restoreRestarter,
		bridgeBinariesDir:     bridgeBinariesDir,
		runtimeSnapshotFunc:   runtimeSnapshotFunc,
		wsHandler:             wsHandler,
	}

	mux := http.NewServeMux()
	routeHandlers := buildRouteHandlers(deps, routerOpts)
	if err := registerAPIRouteHandlers(mux, apiRouteSpecs, routeHandlers); err != nil {
		panic(err)
	}
	middleware := buildRouterMiddlewareSet(mux, deps, routeHandlers, routerOpts)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if spec, ok := apiRouteMetadata.matchPath(r.URL.Path); ok && spec.authMode == routeAuthPublic {
			if middleware.servePublicRoute(w, r, spec) {
				return
			}
		}

		if spec, ok := apiRouteMetadata.match(r.Method, r.URL.Path); ok {
			if middleware.serveRouteProfile(w, r, spec, deps, routerOpts) {
				return
			}
		}

		middleware.normal.ServeHTTP(w, r)
	})
}

// isAuthRoute identifies public auth routes that bypass protected RBAC middleware.
func isAuthRoute(path string) bool {
	return apiRouteMetadata.isPublicAuthPath(path)
}

// parseCanvasMapRoute extracts the map ID and action suffix from saved-map routes.
func parseCanvasMapRoute(path string) (uuid.UUID, string, bool) {
	const prefix = "/api/v1/canvas/maps/"
	suffix, ok := strings.CutPrefix(path, prefix)
	if !ok || suffix == "" {
		return uuid.Nil, "", false
	}

	parts := strings.Split(suffix, "/")
	if len(parts) == 0 || parts[0] == "" {
		return uuid.Nil, "", false
	}
	mapID, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, "", false
	}
	action := strings.Join(parts[1:], "/")
	if strings.Contains(action, "//") {
		return uuid.Nil, "", false
	}
	return mapID, action, true
}

// isWinboxCredentialsRevealPath matches the explicit credential reveal endpoint shape.
func isWinboxCredentialsRevealPath(path string) bool {
	const prefix = "/api/v1/devices/"
	suffix, ok := strings.CutPrefix(path, prefix)
	if !ok {
		return false
	}
	parts := strings.Split(suffix, "/")
	return len(parts) == 3 &&
		parts[0] != "" &&
		parts[1] == "winbox-credentials" &&
		parts[2] == "reveal"
}

// indexOf returns the first byte offset of a substring or -1 when absent.
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
