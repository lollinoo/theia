package api

// This file defines router dependencies API routing, middleware, and permission policy behavior.

import (
	"database/sql"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/repository/postgres"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/vendor"
	"github.com/lollinoo/theia/internal/ws"
)

// routerDependencies names NewRouter's required collaborators so route assembly
// can be split without changing the public constructor signature.
type routerDependencies struct {
	db                    *sql.DB
	deviceService         *service.DeviceService
	linkRepo              domain.LinkRepository
	positionRepo          domain.PositionRepository
	canvasMapRepo         domain.CanvasMapRepository
	canvasMapPositionRepo domain.CanvasMapPositionRepository
	settingsRepo          domain.SettingsRepository
	snmpProfileRepo       domain.SNMPProfileRepository
	credentialProfileRepo *postgres.CredentialProfileRepo
	areaRepo              domain.AreaRepository
	backupService         *service.BackupService
	vendorRegistry        *vendor.Registry
	vendorConfigRepo      domain.VendorConfigRepository
	poller                statusProvider
	instanceBackupService *service.InstanceBackupService
	restoreRestarter      func()
	bridgeBinariesDir     string
	runtimeSnapshotFunc   func() (*ws.SnapshotPayload, uint64)
	wsHandler             *ws.Handler
}
