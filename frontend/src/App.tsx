import { ReactFlowProvider } from '@xyflow/react';
import { useCallback, useEffect, useState } from 'react';
import {
  createCanvasMap,
  deleteCanvasMap,
  duplicateCanvasMap,
  fetchAreas,
  fetchCanvasMaps,
  setCanvasMapPrimary,
  updateCanvasMap,
} from './api/client';
import Canvas from './components/Canvas';
import { Dashboard } from './components/Dashboard';
import NavigationPill from './components/NavigationPill';
import { Watermark } from './components/Watermark';
import {
  CreateMapDialog,
  type CreateMapDialogSubmit,
} from './components/topology-hub/CreateMapDialog';
import DeleteMapDialog from './components/topology-hub/DeleteMapDialog';
import {
  DuplicateMapDialog,
  type DuplicateMapDialogSubmit,
} from './components/topology-hub/DuplicateMapDialog';
import {
  RenameMapDialog,
  type RenameMapDialogSubmit,
} from './components/topology-hub/RenameMapDialog';
import TopologyHub from './components/topology-hub/TopologyHub';
import { ThemeProvider } from './contexts/ThemeContext';
import { useWebSocket } from './hooks/useWebSocket';
import type { Area, CanvasMap, CanvasMapFilter, Device, Link } from './types/api';

export type ActiveView = 'hub' | 'canvas' | 'dashboard';

const runtimeUpdatePauseIdleDelayMs = 1500;
const enableSavedMaps = true;
const viewLayerBaseClass = 'absolute inset-0 h-full w-full';

function viewLayerClass(active: boolean, className = ''): string {
  const activeClass = active
    ? 'opacity-100 pointer-events-auto z-10'
    : 'opacity-0 pointer-events-none z-0';
  return `${viewLayerBaseClass} ${activeClass} ${className}`.trim();
}

function viewLayerStateProps(active: boolean): { 'aria-hidden': boolean; inert?: '' } {
  return active ? { 'aria-hidden': false } : { 'aria-hidden': true, inert: '' };
}

function canvasMapErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

function mapFilterForArea(area: Area): CanvasMapFilter {
  return {
    area_id: area.id,
    include_cross_area_links: true,
    include_ghost_devices: true,
  };
}

function upsertCanvasMap(maps: CanvasMap[], map: CanvasMap): CanvasMap[] {
  const existingIndex = maps.findIndex((candidate) => candidate.id === map.id);
  if (existingIndex === -1) {
    return [...maps, map];
  }

  return maps.map((candidate, index) => (index === existingIndex ? map : candidate));
}

function fallbackCanvasMap(maps: CanvasMap[]): CanvasMap | null {
  return maps.find((map) => map.is_default) ?? maps[0] ?? null;
}

function setPrimaryCanvasMap(maps: CanvasMap[], primaryMap: CanvasMap): CanvasMap[] {
  let found = false;
  const nextMaps = maps.map((map) => {
    if (map.id === primaryMap.id) {
      found = true;
      return { ...primaryMap, is_default: true };
    }
    return map.is_default ? { ...map, is_default: false } : map;
  });

  if (!found) {
    return [...nextMaps, { ...primaryMap, is_default: true }];
  }
  return nextMaps;
}

function App() {
  const [activeView, setActiveView] = useState<ActiveView>('canvas');
  const [selectedAreaId, setSelectedAreaId] = useState<string | null>(null);
  const [selectedMapId, setSelectedMapId] = useState<string | null>(null);
  const [selectedMapName, setSelectedMapName] = useState('Default');
  const [detailDeviceId, setDetailDeviceId] = useState<string | null>(null);
  const [canvasDevices, setCanvasDevices] = useState<Device[]>([]);
  const [canvasLinks, setCanvasLinks] = useState<Link[]>([]);
  const [, setAreas] = useState<Area[]>([]);
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
  const [canvasInteractionActive, setCanvasInteractionActive] = useState(false);
  const [runtimeUpdatesPaused, setRuntimeUpdatesPaused] = useState(false);

  useEffect(() => {
    if (canvasInteractionActive) {
      setRuntimeUpdatesPaused(true);
      return;
    }

    if (!runtimeUpdatesPaused) {
      return;
    }

    const timer = window.setTimeout(() => {
      setRuntimeUpdatesPaused(false);
    }, runtimeUpdatePauseIdleDelayMs);

    return () => {
      window.clearTimeout(timer);
    };
  }, [canvasInteractionActive, runtimeUpdatesPaused]);

  const { snapshot, alerts, reconnecting, prometheusStatus } = useWebSocket(
    '/api/v1/ws',
    detailDeviceId,
    { requireRuntimeBootstrap: true, runtimeUpdatesPaused },
  );

  // Fetch areas on mount
  useEffect(() => {
    fetchAreas()
      .then(setAreas)
      .catch(() => {
        // Area fetch failure is non-fatal; Hub will show empty state
      });
  }, []);

  const loadCanvasMaps = useCallback(async (): Promise<CanvasMap[] | null> => {
    if (!enableSavedMaps) {
      return null;
    }

    setCanvasMapsLoading(true);
    setCanvasMapsError(null);

    try {
      const maps = await fetchCanvasMaps();
      setCanvasMaps(maps);
      setCanvasMapsLoaded(true);
      return maps;
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

  // Re-fetch areas/maps when switching to hub view (pick up changes from settings)
  useEffect(() => {
    if (activeView === 'hub') {
      fetchAreas()
        .then(setAreas)
        .catch(() => {});
      if (!canvasMapsLoaded && !canvasMapsLoading) {
        void loadCanvasMaps();
      }
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

  const openCanvasView = useCallback(() => {
    setActiveView('canvas');
    requestCanvasFitView();
  }, [requestCanvasFitView]);

  const handleViewChange = useCallback(
    (view: ActiveView) => {
      setActiveView(view);
      if (view === 'canvas') {
        requestCanvasFitView();
      }
    },
    [requestCanvasFitView],
  );

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
    (map: CanvasMap) => {
      handleSelectMapContext(map);
      openCanvasView();
    },
    [handleSelectMapContext, openCanvasView],
  );

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
      return;
    }

    fetchAreas()
      .then(setAreas)
      .catch(() => {});
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

  return (
    <ThemeProvider>
      <div className="relative h-screen w-screen overflow-hidden bg-bg text-on-bg">
        <NavigationPill
          activeView={activeView}
          selectedAreaId={selectedAreaId}
          selectedMapId={selectedMapId}
          selectedMapName={selectedMapName}
          maps={canvasMaps}
          areas={navigationAreas}
          onViewChange={handleViewChange}
          onAreaSelect={handleNavigationAreaSelect}
          onMapSelect={handleNavigationMapSelect}
          onManageMaps={() => {
            setActiveView('hub');
          }}
        />
        {/* All views stay mounted; inactive ones keep dimensions for React Flow. */}
        <div
          {...viewLayerStateProps(activeView === 'hub')}
          className={viewLayerClass(activeView === 'hub', 'overflow-y-auto')}
        >
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
            onOpenSettings={() => {
              setActiveView('canvas');
            }}
          />
        </div>
        <div
          {...viewLayerStateProps(activeView === 'canvas')}
          className={viewLayerClass(activeView === 'canvas', 'overflow-hidden')}
        >
          <Watermark
            activeView={activeView}
            selectedAreaId={selectedAreaId}
            areas={navigationAreas}
          />
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
              onDevicesChange={handleCanvasDevicesChange}
              onLinksChange={handleCanvasLinksChange}
              onAreaSelect={handleAreaFilterSelect}
              onTopologyAreasChange={setCanvasTopologyAreas}
              onTopologyLoadingChange={setCanvasTopologyLoading}
              onDetailDeviceChange={setDetailDeviceId}
              onInteractionActiveChange={setCanvasInteractionActive}
            />
          </ReactFlowProvider>
        </div>
        <div
          {...viewLayerStateProps(activeView === 'dashboard')}
          className={viewLayerClass(activeView === 'dashboard', 'overflow-hidden')}
        >
          <Dashboard
            devices={canvasDevices}
            areas={navigationAreas}
            snapshot={snapshot}
            selectedAreaId={selectedAreaId}
            onAreaSelect={handleAreaFilterSelect}
            onOpenMap={openCanvasView}
            loading={canvasTopologyLoading}
          />
        </div>
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
      </div>
    </ThemeProvider>
  );
}

export default App;
