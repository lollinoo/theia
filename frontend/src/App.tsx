import { ReactFlowProvider } from '@xyflow/react';
import { useCallback, useEffect, useState } from 'react';
import {
  createCanvasMap,
  deleteCanvasMap,
  duplicateCanvasMap,
  fetchAreas,
  fetchCanvasMaps,
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
import TopologyHub from './components/topology-hub/TopologyHub';
import { ThemeProvider } from './contexts/ThemeContext';
import { useWebSocket } from './hooks/useWebSocket';
import type { Area, CanvasMap, CanvasMapFilter, Device, Link } from './types/api';

export type ActiveView = 'hub' | 'canvas' | 'dashboard';

const runtimeUpdatePauseIdleDelayMs = 1500;
const enableSavedMaps = true;

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

function App() {
  const [activeView, setActiveView] = useState<ActiveView>('canvas');
  const [selectedAreaId, setSelectedAreaId] = useState<string | null>(null);
  const [selectedMapId, setSelectedMapId] = useState<string | null>(null);
  const [selectedMapName, setSelectedMapName] = useState('Default');
  const [detailDeviceId, setDetailDeviceId] = useState<string | null>(null);
  const [canvasDevices, setCanvasDevices] = useState<Device[]>([]);
  const [canvasLinks, setCanvasLinks] = useState<Link[]>([]);
  const [areas, setAreas] = useState<Area[]>([]);
  const [canvasTopologyAreas, setCanvasTopologyAreas] = useState<Area[]>([]);
  const [canvasTopologyLoading, setCanvasTopologyLoading] = useState(true);
  const [canvasMaps, setCanvasMaps] = useState<CanvasMap[]>([]);
  const [canvasMapsLoading, setCanvasMapsLoading] = useState(false);
  const [canvasMapsError, setCanvasMapsError] = useState<string | null>(null);
  const [createMapDialogOpen, setCreateMapDialogOpen] = useState(false);
  const [createMapSourceArea, setCreateMapSourceArea] = useState<Area | null>(null);
  const [duplicateMapSource, setDuplicateMapSource] = useState<CanvasMap | null>(null);
  const [deleteMapSource, setDeleteMapSource] = useState<CanvasMap | null>(null);
  const [deleteMapLoading, setDeleteMapLoading] = useState(false);
  const [fitViewRevision, setFitViewRevision] = useState(0);
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
      return maps;
    } catch (error) {
      setCanvasMapsError(canvasMapErrorMessage(error, 'Failed to load maps'));
      return null;
    } finally {
      setCanvasMapsLoading(false);
    }
  }, []);

  // Re-fetch areas/maps when switching to hub view (pick up changes from settings)
  useEffect(() => {
    if (activeView === 'hub') {
      fetchAreas()
        .then(setAreas)
        .catch(() => {});
      void loadCanvasMaps();
    }
  }, [activeView, loadCanvasMaps]);

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

  const handleOpenGlobal = useCallback(() => {
    setSelectedMapId(null);
    setSelectedMapName('Default');
    setSelectedAreaId(null);
    setCanvasTopologyAreas(areas);
    openCanvasView();
  }, [areas, openCanvasView]);

  const handleSelectMapContext = useCallback(
    (map: CanvasMap) => {
      setSelectedMapId(map.is_default ? null : map.id);
      setSelectedMapName(map.name);
      setSelectedAreaId(null);
      setCanvasTopologyAreas(map.is_default ? areas : []);
    },
    [areas],
  );

  const handleOpenMap = useCallback(
    (map: CanvasMap) => {
      handleSelectMapContext(map);
      openCanvasView();
    },
    [handleSelectMapContext, openCanvasView],
  );

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
    fetchAreas()
      .then(setAreas)
      .catch(() => {});
  }, []);

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

  const handleCreateMap = useCallback(
    async ({ name, sourceArea }: CreateMapDialogSubmit) => {
      if (!enableSavedMaps) {
        return;
      }

      setCanvasMapsError(null);

      try {
        const sourceMapId = sourceArea && selectedMapId ? selectedMapId : null;
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
    [handleOpenMap, loadCanvasMaps, selectedMapId],
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
        setCanvasMaps((maps) => maps.filter((candidate) => candidate.id !== map.id));
        if (selectedMapId === map.id) {
          setSelectedMapId(null);
          setSelectedMapName('Default');
          setSelectedAreaId(null);
          setCanvasTopologyAreas(areas);
        }
      } catch (error) {
        setCanvasMapsError(canvasMapErrorMessage(error, 'Failed to delete map'));
      } finally {
        setDeleteMapLoading(false);
      }
    },
    [areas, selectedMapId],
  );

  const mapsForHub = enableSavedMaps ? canvasMaps : [];
  const mapsLoadingForHub = enableSavedMaps ? canvasMapsLoading : false;
  const mapsErrorForHub = enableSavedMaps ? canvasMapsError : null;
  const navigationAreas = canvasTopologyAreas;

  return (
    <ThemeProvider>
      <div className="h-screen w-screen overflow-hidden bg-bg text-on-bg">
        <NavigationPill
          activeView={activeView}
          selectedAreaId={selectedAreaId}
          selectedMapId={selectedMapId}
          selectedMapName={selectedMapName}
          maps={canvasMaps}
          areas={navigationAreas}
          onViewChange={handleViewChange}
          onAreaSelect={handleNavigationAreaSelect}
          onMapSelect={handleSelectMapContext}
          onManageMaps={() => {
            setActiveView('hub');
          }}
        />
        {/* All views stay mounted; inactive ones hidden via CSS */}
        <div className={activeView === 'hub' ? 'h-full overflow-y-auto' : 'hidden'}>
          <TopologyHub
            devices={canvasDevices}
            areas={navigationAreas}
            links={canvasLinks}
            snapshot={snapshot}
            maps={mapsForHub}
            mapsLoading={mapsLoadingForHub}
            mapsError={mapsErrorForHub}
            savedMapsEnabled={enableSavedMaps}
            onOpenGlobal={handleOpenGlobal}
            onOpenArea={(areaId) => handleOpenArea(areaId)}
            onOpenMap={handleOpenMap}
            onCreateEmptyMap={handleCreateEmptyMap}
            onCreateMapFromArea={handleCreateMapFromArea}
            onDuplicateMap={handleDuplicateMap}
            onDeleteMap={handleDeleteMap}
            onOpenSettings={() => {
              setActiveView('canvas');
            }}
          />
        </div>
        <div className={activeView === 'canvas' ? 'relative h-full' : 'hidden'}>
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
              fitViewRevision={fitViewRevision}
              onDevicesChange={handleCanvasDevicesChange}
              onLinksChange={handleCanvasLinksChange}
              onAreaSelect={handleAreaFilterSelect}
              onTopologyAreasChange={setCanvasTopologyAreas}
              onTopologyLoadingChange={setCanvasTopologyLoading}
              onAreasChange={handleAreasChange}
              onDetailDeviceChange={setDetailDeviceId}
              onInteractionActiveChange={setCanvasInteractionActive}
            />
          </ReactFlowProvider>
        </div>
        <div className={activeView === 'dashboard' ? 'h-full' : 'hidden'}>
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
