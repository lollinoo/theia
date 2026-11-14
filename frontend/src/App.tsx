/**
 * Owns top-level application routing and provider composition.
 * Keeps global layout decisions separate from feature-specific components.
 */
import { ReactFlowProvider } from '@xyflow/react';
import { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react';
import {
  createCanvasMap,
  deleteCanvasMap,
  duplicateCanvasMap,
  fetchCanvasMaps,
  setCanvasMapPrimary,
  updateCanvasMap,
} from './api/client';
import Canvas from './components/Canvas';
import type { ImportedNodePlacementRequest } from './components/canvas/importedNodePlacementRequest';
import { EncryptionKeyOnboardingModal } from './components/EncryptionKeyOnboardingModal';
import NavigationPill from './components/NavigationPill';
import {
  CreateMapDialog,
  type CreateMapDialogSubmit,
} from './components/topology-hub/CreateMapDialog';
import {
  canvasMapErrorMessage,
  fallbackCanvasMap,
  mapFilterForArea,
  setPrimaryCanvasMap,
  upsertCanvasMap,
} from './components/topology-hub/canvasMapState';
import DeleteMapDialog from './components/topology-hub/DeleteMapDialog';
import {
  DuplicateMapDialog,
  type DuplicateMapDialogSubmit,
} from './components/topology-hub/DuplicateMapDialog';
import {
  RenameMapDialog,
  type RenameMapDialogSubmit,
} from './components/topology-hub/RenameMapDialog';
import { Watermark } from './components/Watermark';
import { useAuth } from './contexts/AuthContext';
import { ThemeProvider } from './contexts/ThemeContext';
import { useRuntimeUpdatePause } from './hooks/useRuntimeUpdatePause';
import { useWebSocket } from './hooks/useWebSocket';
import type { Area, CanvasMap, CanvasMapFilter, Device, Link } from './types/api';

const AdminDashboard = lazy(() =>
  import('./components/AdminDashboard').then((m) => ({
    default: m.AdminDashboard,
  })),
);
const Dashboard = lazy(() =>
  import('./components/Dashboard').then((m) => ({
    default: m.Dashboard,
  })),
);
const TopologyHub = lazy(() =>
  import('./components/topology-hub/TopologyHub').then((m) => ({
    default: m.TopologyHub,
  })),
);
const UserSettingsPage = lazy(() =>
  import('./components/settings/UserSettingsPage').then((m) => ({
    default: m.UserSettingsPage,
  })),
);

/** Top-level route-like application views rendered without unmounting the canvas runtime. */
export type ActiveView = 'hub' | 'canvas' | 'dashboard' | 'admin' | 'settings';

const enableSavedMaps = true;
const viewLayerBaseClass = 'absolute inset-0 h-full w-full';
const canvasChromeHiddenStorageKey = 'theia.canvas.chromeHidden';

/** Reads the persisted canvas chrome preference before first render to avoid visible layout churn. */
function initialCanvasChromeHidden(): boolean {
  return window.localStorage.getItem(canvasChromeHiddenStorageKey) === 'true';
}

/** Builds the absolute layer class used to keep inactive views mounted but non-interactive. */
function viewLayerClass(active: boolean, className = ''): string {
  const activeClass = active
    ? 'opacity-100 pointer-events-auto z-10'
    : 'opacity-0 pointer-events-none z-0';
  return `${viewLayerBaseClass} ${activeClass} ${className}`.trim();
}

/** Mirrors layer visibility into accessibility state and inert handling for hidden views. */
function viewLayerStateProps(active: boolean): {
  'aria-hidden': boolean;
  inert?: boolean;
} {
  return active ? { 'aria-hidden': false } : { 'aria-hidden': true, inert: true };
}

/**
 * Coordinates application-level view state, saved topology map ownership, and runtime update pause state.
 * The canvas remains mounted across view changes so WebSocket/runtime state and React Flow state are preserved.
 */
function App() {
  const [activeView, setActiveView] = useState<ActiveView>('canvas');
  const [selectedAreaId, setSelectedAreaId] = useState<string | null>(null);
  const [selectedMapId, setSelectedMapId] = useState<string | null>(null);
  const [selectedMapName, setSelectedMapName] = useState('Default');
  const selectedMapIdRef = useRef<string | null>(null);
  selectedMapIdRef.current = selectedMapId;
  const [detailDeviceId, setDetailDeviceId] = useState<string | null>(null);
  const [canvasDevices, setCanvasDevices] = useState<Device[]>([]);
  const [canvasLinks, setCanvasLinks] = useState<Link[]>([]);
  const [canvasTopologyAreas, setCanvasTopologyAreas] = useState<Area[]>([]);
  const [canvasTopologyLoading, setCanvasTopologyLoading] = useState(true);
  const [canvasMaps, setCanvasMaps] = useState<CanvasMap[]>([]);
  const [canvasMapsLoading, setCanvasMapsLoading] = useState(false);
  const [canvasMapsLoaded, setCanvasMapsLoaded] = useState(false);
  const [canvasMapsError, setCanvasMapsError] = useState<string | null>(null);
  const [createMapDialogOpen, setCreateMapDialogOpen] = useState(false);
  const [createMapSourceArea, setCreateMapSourceArea] = useState<Area | null>(null);
  const [renameMapSource, setRenameMapSource] = useState<CanvasMap | null>(null);
  const [renameMapLoading, setRenameMapLoading] = useState(false);
  const [duplicateMapSource, setDuplicateMapSource] = useState<CanvasMap | null>(null);
  const [deleteMapSource, setDeleteMapSource] = useState<CanvasMap | null>(null);
  const [deleteMapLoading, setDeleteMapLoading] = useState(false);
  const [fitViewRevision, setFitViewRevision] = useState(0);
  const [topologyRefreshRevision, setTopologyRefreshRevision] = useState(0);
  const [importedNodePlacementRequest, setImportedNodePlacementRequest] =
    useState<ImportedNodePlacementRequest | null>(null);
  const [canvasInteractionActive, setCanvasInteractionActive] = useState(false);
  const [canvasChromeHidden, setCanvasChromeHidden] = useState(initialCanvasChromeHidden);
  const runtimeUpdatesPaused = useRuntimeUpdatePause(canvasInteractionActive);
  const { hasPermission, logout, user } = useAuth();
  const canViewAdmin = hasPermission('admin:dashboard:read');
  const canReadSettings = hasPermission('settings:read');
  const canUpdateSettings = hasPermission('settings:update');
  const canOpenSettings = canViewAdmin && canReadSettings && canUpdateSettings;
  const canOpenUserSettings = hasPermission('account:manage');
  const canCoordinateTopologyImport =
    canViewAdmin &&
    hasPermission('devices:read') &&
    hasPermission('devices:create') &&
    hasPermission('topology:read') &&
    hasPermission('topology:update') &&
    hasPermission('credentials:read');

  const { snapshot, alerts, reconnecting, prometheusStatus } = useWebSocket(
    '/api/v1/ws',
    detailDeviceId,
    { requireRuntimeBootstrap: true, runtimeUpdatesPaused },
  );

  const loadCanvasMaps = useCallback(async (): Promise<CanvasMap[] | null> => {
    if (!enableSavedMaps) {
      return null;
    }

    setCanvasMapsLoading(true);
    setCanvasMapsError(null);

    try {
      const nextMaps = await fetchCanvasMaps();
      setCanvasMaps(nextMaps);
      setCanvasMapsLoaded(true);
      const currentSelectedId = selectedMapIdRef.current;
      const defaultMap = fallbackCanvasMap(nextMaps);
      if (
        nextMaps.length > 0 &&
        (currentSelectedId === null || !nextMaps.some((map) => map.id === currentSelectedId))
      ) {
        const fallbackTarget = defaultMap ?? nextMaps[0] ?? null;
        const nextId = fallbackTarget?.id ?? null;
        setSelectedMapId(nextId);
        if (fallbackTarget) {
          setSelectedMapName(fallbackTarget.name);
        }
        if (currentSelectedId !== null && currentSelectedId !== nextId) {
          setSelectedAreaId(null);
          setCanvasTopologyAreas([]);
        }
      }
      return nextMaps;
    } catch (error) {
      setCanvasMapsError(canvasMapErrorMessage(error, 'Failed to load maps'));
      return null;
    } finally {
      setCanvasMapsLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadCanvasMaps();
  }, [loadCanvasMaps]);

  // Re-fetch maps when switching to hub view if not yet loaded
  useEffect(() => {
    if (activeView === 'hub' && !canvasMapsLoaded && !canvasMapsLoading) {
      void loadCanvasMaps();
    }
  }, [activeView, canvasMapsLoaded, canvasMapsLoading, loadCanvasMaps]);

  const handleCanvasDevicesChange = useCallback((devices: Device[]) => {
    setCanvasDevices(devices);
  }, []);

  const handleCanvasLinksChange = useCallback((links: Link[]) => {
    setCanvasLinks(links);
  }, []);

  const requestCanvasFitView = useCallback(() => {
    setFitViewRevision((revision) => revision + 1);
  }, []);

  const handleCanvasChromeHiddenChange = useCallback((hidden: boolean) => {
    setCanvasChromeHidden(hidden);
    window.localStorage.setItem(canvasChromeHiddenStorageKey, String(hidden));
  }, []);

  const openCanvasView = useCallback(() => {
    setActiveView('canvas');
    requestCanvasFitView();
  }, [requestCanvasFitView]);

  const handleViewChange = useCallback(
    (view: ActiveView) => {
      if (view === 'admin' && !canViewAdmin) {
        return;
      }
      if (view === 'settings' && !canOpenUserSettings) {
        return;
      }
      setActiveView(view);
      if (view === 'canvas') {
        requestCanvasFitView();
      }
    },
    [canOpenUserSettings, canViewAdmin, requestCanvasFitView],
  );

  const handleOpenSettings = useCallback(() => {
    if (!canOpenSettings) {
      return;
    }
    setActiveView('admin');
  }, [canOpenSettings]);

  useEffect(() => {
    if (activeView === 'admin' && !canViewAdmin) {
      setActiveView('canvas');
    }
    if (activeView === 'settings' && !canOpenUserSettings) {
      setActiveView('canvas');
    }
  }, [activeView, canOpenUserSettings, canViewAdmin]);

  const handleSelectMapContext = useCallback(
    (map: CanvasMap) => {
      const isSameMap = selectedMapId === map.id;
      setSelectedMapId(map.id);
      setSelectedMapName(map.name);
      if (!isSameMap) {
        setSelectedAreaId(null);
        setCanvasTopologyAreas([]);
      }
    },
    [selectedMapId],
  );

  const handleOpenMap = useCallback(
    (map: CanvasMap, placementRequest?: ImportedNodePlacementRequest) => {
      if (placementRequest?.mapId === map.id) {
        setImportedNodePlacementRequest(placementRequest);
      }
      handleSelectMapContext(map);
      openCanvasView();
    },
    [handleSelectMapContext, openCanvasView],
  );

  const handleImportedNodePlacementConsumed = useCallback((requestId: string) => {
    setImportedNodePlacementRequest((current) =>
      current?.requestId === requestId ? null : current,
    );
  }, []);

  const handleNavigationMapSelect = useCallback(
    (map: CanvasMap) => {
      handleSelectMapContext(map);
      if (activeView === 'hub') {
        openCanvasView();
      } else {
        requestCanvasFitView();
      }
    },
    [activeView, handleSelectMapContext, openCanvasView, requestCanvasFitView],
  );

  useEffect(() => {
    if (!enableSavedMaps || selectedMapId !== null || canvasMaps.length === 0) {
      return;
    }

    const defaultMap = fallbackCanvasMap(canvasMaps);
    if (defaultMap) {
      handleSelectMapContext(defaultMap);
    }
  }, [canvasMaps, handleSelectMapContext, selectedMapId]);

  const handleAreaFilterSelect = useCallback((areaId: string | null) => {
    setSelectedAreaId(areaId);
  }, []);

  const handleNavigationAreaSelect = useCallback(
    (areaId: string | null) => {
      setSelectedAreaId(areaId);
      openCanvasView();
    },
    [openCanvasView],
  );

  const handleOpenArea = useCallback(
    (areaId: string | null) => {
      setSelectedAreaId(areaId);
      openCanvasView();
    },
    [openCanvasView],
  );

  const handleAreasChange = useCallback(() => {
    if (enableSavedMaps && selectedMapId) {
      setTopologyRefreshRevision((revision) => revision + 1);
    }
  }, [selectedMapId]);

  const handleCreateMapFromArea = useCallback((area: Area) => {
    if (!enableSavedMaps) {
      return;
    }

    setCanvasMapsError(null);
    setCreateMapSourceArea(area);
    setCreateMapDialogOpen(true);
  }, []);

  const handleCreateEmptyMap = useCallback(() => {
    if (!enableSavedMaps) {
      return;
    }

    setCanvasMapsError(null);
    setCreateMapSourceArea(null);
    setCreateMapDialogOpen(true);
  }, []);

  const handleDuplicateMap = useCallback((map: CanvasMap) => {
    if (!enableSavedMaps) {
      return;
    }

    setCanvasMapsError(null);
    setDuplicateMapSource(map);
  }, []);

  const handleRenameMap = useCallback((map: CanvasMap) => {
    if (!enableSavedMaps) {
      return;
    }

    setCanvasMapsError(null);
    setRenameMapSource(map);
  }, []);

  const handleCreateMap = useCallback(
    async ({ name, sourceArea }: CreateMapDialogSubmit) => {
      if (!enableSavedMaps) {
        return;
      }

      setCanvasMapsError(null);

      try {
        const sourceMapId =
          sourceArea && enableSavedMaps
            ? (selectedMapId ?? fallbackCanvasMap(canvasMaps)?.id ?? null)
            : null;
        const payload: {
          name: string;
          source_area_id: string | null;
          source_map_id?: string;
          filter: CanvasMapFilter;
        } = {
          name,
          source_area_id: sourceArea?.id ?? null,
          filter: sourceArea ? mapFilterForArea(sourceArea) : {},
        };
        if (sourceMapId) {
          payload.source_map_id = sourceMapId;
        }

        const createdMap = await createCanvasMap(payload);
        setCreateMapDialogOpen(false);
        setCreateMapSourceArea(null);
        setCanvasMaps((maps) => upsertCanvasMap(maps, createdMap));
        handleOpenMap(createdMap);
        void loadCanvasMaps();
      } catch (error) {
        setCanvasMapsError(canvasMapErrorMessage(error, 'Failed to create map'));
      }
    },
    [canvasMaps, handleOpenMap, loadCanvasMaps, selectedMapId],
  );

  const handleDuplicateMapSubmit = useCallback(
    async ({ name, sourceMap }: DuplicateMapDialogSubmit) => {
      if (!enableSavedMaps) {
        return;
      }

      setCanvasMapsError(null);

      try {
        const duplicatedMap = await duplicateCanvasMap(sourceMap.id, { name });
        setDuplicateMapSource(null);
        setCanvasMaps((maps) => upsertCanvasMap(maps, duplicatedMap));
        handleOpenMap(duplicatedMap);
        void loadCanvasMaps();
      } catch (error) {
        setCanvasMapsError(canvasMapErrorMessage(error, 'Failed to duplicate map'));
      }
    },
    [handleOpenMap, loadCanvasMaps],
  );

  const handleRenameMapSubmit = useCallback(
    async ({ name, map }: RenameMapDialogSubmit) => {
      if (!enableSavedMaps) {
        return;
      }

      setCanvasMapsError(null);
      setRenameMapLoading(true);

      try {
        const renamedMap = await updateCanvasMap(map.id, { name });
        setRenameMapSource(null);
        setCanvasMaps((maps) => upsertCanvasMap(maps, renamedMap));
        if (selectedMapId === map.id || (selectedMapId === null && map.is_default)) {
          handleSelectMapContext(renamedMap);
        }
      } catch (error) {
        setCanvasMapsError(canvasMapErrorMessage(error, 'Failed to rename map'));
      } finally {
        setRenameMapLoading(false);
      }
    },
    [handleSelectMapContext, selectedMapId],
  );

  const handleSetPrimaryMap = useCallback(
    async (map: CanvasMap) => {
      if (!enableSavedMaps) {
        return;
      }

      setCanvasMapsError(null);

      try {
        const primaryMap = await setCanvasMapPrimary(map.id);
        setCanvasMaps((maps) => setPrimaryCanvasMap(maps, primaryMap));
        handleSelectMapContext(primaryMap);
      } catch (error) {
        setCanvasMapsError(canvasMapErrorMessage(error, 'Failed to set primary map'));
      }
    },
    [handleSelectMapContext],
  );

  const handleDeleteMap = useCallback((map: CanvasMap) => {
    if (!enableSavedMaps || map.is_default) {
      return;
    }

    setCanvasMapsError(null);
    setDeleteMapSource(map);
  }, []);

  const handleDeleteMapConfirm = useCallback(
    async (map: CanvasMap) => {
      if (!enableSavedMaps || map.is_default) {
        return;
      }

      setCanvasMapsError(null);
      setDeleteMapLoading(true);

      try {
        await deleteCanvasMap(map.id);
        setDeleteMapSource(null);
        const remainingMaps = canvasMaps.filter((candidate) => candidate.id !== map.id);
        setCanvasMaps(remainingMaps);
        if (selectedMapId === map.id) {
          const fallbackMap = fallbackCanvasMap(remainingMaps);
          if (fallbackMap) {
            handleSelectMapContext(fallbackMap);
          } else {
            setSelectedMapId(null);
            setSelectedMapName('Default');
            setSelectedAreaId(null);
            setCanvasTopologyAreas([]);
          }
        }
      } catch (error) {
        setCanvasMapsError(canvasMapErrorMessage(error, 'Failed to delete map'));
      } finally {
        setDeleteMapLoading(false);
      }
    },
    [canvasMaps, handleSelectMapContext, selectedMapId],
  );

  const mapsForHub = enableSavedMaps ? canvasMaps : [];
  const mapsLoadingForHub = enableSavedMaps ? canvasMapsLoading : false;
  const mapsErrorForHub = enableSavedMaps ? canvasMapsError : null;
  const navigationAreas = canvasTopologyAreas;
  const savedMapContextResolved =
    !enableSavedMaps || selectedMapId !== null || (canvasMapsLoaded && canvasMaps.length === 0);

  return (
    <ThemeProvider>
      <div className="relative h-screen w-screen overflow-hidden bg-bg text-on-bg">
        {!(activeView === 'canvas' && canvasChromeHidden) && (
          <NavigationPill
            activeView={activeView}
            selectedAreaId={selectedAreaId}
            selectedMapId={selectedMapId}
            selectedMapName={selectedMapName}
            maps={canvasMaps}
            areas={navigationAreas}
            canViewAdmin={canViewAdmin}
            canViewSettings={canOpenUserSettings}
            userLabel={user?.display_name || user?.username || 'User'}
            onViewChange={handleViewChange}
            onAreaSelect={handleNavigationAreaSelect}
            onMapSelect={handleNavigationMapSelect}
            onManageMaps={() => {
              setActiveView('hub');
            }}
            onLogout={() => {
              void logout();
            }}
          />
        )}
        {/* Inactive top-level views are mounted on demand; Canvas stays mounted to preserve React Flow dimensions and runtime state. */}
        {activeView === 'hub' && (
          <div
            {...viewLayerStateProps(activeView === 'hub')}
            className={viewLayerClass(activeView === 'hub', 'overflow-y-auto')}
          >
            <Suspense fallback={null}>
              <TopologyHub
                devices={canvasDevices}
                areas={navigationAreas}
                links={canvasLinks}
                snapshot={snapshot}
                maps={mapsForHub}
                mapsLoading={mapsLoadingForHub}
                mapsError={mapsErrorForHub}
                selectedMapId={selectedMapId}
                selectedMapName={selectedMapName}
                savedMapsEnabled={enableSavedMaps}
                onOpenArea={(areaId) => handleOpenArea(areaId)}
                onSelectMap={handleSelectMapContext}
                onOpenMap={handleOpenMap}
                onCreateEmptyMap={handleCreateEmptyMap}
                onCreateMapFromArea={handleCreateMapFromArea}
                onAreasChange={handleAreasChange}
                onRenameMap={handleRenameMap}
                onDuplicateMap={handleDuplicateMap}
                onDeleteMap={handleDeleteMap}
                onSetPrimaryMap={handleSetPrimaryMap}
                canOpenSettings={canOpenSettings}
                onOpenSettings={handleOpenSettings}
              />
            </Suspense>
          </div>
        )}
        <div
          {...viewLayerStateProps(activeView === 'canvas')}
          className={viewLayerClass(activeView === 'canvas', 'overflow-hidden')}
        >
          <Watermark
            activeView={activeView}
            selectedAreaId={selectedAreaId}
            areas={navigationAreas}
            mapName={selectedMapName}
            compact={canvasChromeHidden}
          />
          {savedMapContextResolved ? (
            <ReactFlowProvider>
              <Canvas
                snapshot={snapshot}
                alerts={alerts}
                reconnecting={reconnecting}
                prometheusStatus={prometheusStatus}
                selectedAreaId={selectedAreaId}
                mapId={selectedMapId}
                mapName={selectedMapName}
                visible={activeView === 'canvas'}
                fitViewRevision={fitViewRevision}
                topologyRefreshRevision={topologyRefreshRevision}
                topologyImportEnabled={canCoordinateTopologyImport}
                importedNodePlacementRequest={importedNodePlacementRequest}
                onImportedNodePlacementConsumed={handleImportedNodePlacementConsumed}
                chromeHidden={canvasChromeHidden}
                onDevicesChange={handleCanvasDevicesChange}
                onLinksChange={handleCanvasLinksChange}
                onAreaSelect={handleAreaFilterSelect}
                onTopologyAreasChange={setCanvasTopologyAreas}
                onTopologyLoadingChange={setCanvasTopologyLoading}
                onDetailDeviceChange={setDetailDeviceId}
                onInteractionActiveChange={setCanvasInteractionActive}
                onChromeHiddenChange={handleCanvasChromeHiddenChange}
              />
            </ReactFlowProvider>
          ) : (
            <div className="flex h-full w-full items-center justify-center text-sm text-on-bg-secondary">
              {canvasMapsError ?? 'Loading maps'}
            </div>
          )}
        </div>
        {activeView === 'dashboard' && (
          <div
            {...viewLayerStateProps(activeView === 'dashboard')}
            className={viewLayerClass(activeView === 'dashboard', 'overflow-hidden')}
          >
            <Suspense fallback={null}>
              <Dashboard
                devices={canvasDevices}
                areas={navigationAreas}
                snapshot={snapshot}
                selectedAreaId={selectedAreaId}
                onAreaSelect={handleAreaFilterSelect}
                onOpenMap={openCanvasView}
                loading={canvasTopologyLoading}
              />
            </Suspense>
          </div>
        )}
        {canViewAdmin && activeView === 'admin' && (
          <div
            {...viewLayerStateProps(activeView === 'admin')}
            className={viewLayerClass(activeView === 'admin', 'overflow-y-auto')}
          >
            <Suspense fallback={null}>
              <AdminDashboard visible={activeView === 'admin'} onOpenMap={handleOpenMap} />
            </Suspense>
          </div>
        )}
        {canOpenUserSettings && activeView === 'settings' && (
          <div
            {...viewLayerStateProps(activeView === 'settings')}
            className={viewLayerClass(activeView === 'settings', 'overflow-y-auto')}
          >
            <Suspense fallback={null}>
              <UserSettingsPage />
            </Suspense>
          </div>
        )}
        {enableSavedMaps && (
          <>
            <CreateMapDialog
              open={createMapDialogOpen}
              sourceArea={createMapSourceArea}
              onCreate={handleCreateMap}
              onClose={() => {
                setCreateMapDialogOpen(false);
                setCreateMapSourceArea(null);
              }}
            />
            <DuplicateMapDialog
              open={duplicateMapSource !== null}
              sourceMap={duplicateMapSource}
              onDuplicate={handleDuplicateMapSubmit}
              onClose={() => setDuplicateMapSource(null)}
            />
            <RenameMapDialog
              open={renameMapSource !== null}
              map={renameMapSource}
              renaming={renameMapLoading}
              onRename={handleRenameMapSubmit}
              onClose={() => {
                if (!renameMapLoading) {
                  setRenameMapSource(null);
                }
              }}
            />
            <DeleteMapDialog
              open={deleteMapSource !== null}
              map={deleteMapSource}
              deleting={deleteMapLoading}
              onDelete={handleDeleteMapConfirm}
              onClose={() => {
                if (!deleteMapLoading) {
                  setDeleteMapSource(null);
                }
              }}
            />
          </>
        )}
        <EncryptionKeyOnboardingModal
          enabled={canViewAdmin && hasPermission('backups:read') && hasPermission('backups:update')}
        />
      </div>
    </ThemeProvider>
  );
}

export default App;
