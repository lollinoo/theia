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
  const [canvasMaps, setCanvasMaps] = useState<CanvasMap[]>([]);
  const [canvasMapsLoading, setCanvasMapsLoading] = useState(false);
  const [canvasMapsError, setCanvasMapsError] = useState<string | null>(null);
  const [createMapDialogOpen, setCreateMapDialogOpen] = useState(false);
  const [createMapSourceArea, setCreateMapSourceArea] = useState<Area | null>(null);
  const [duplicateMapSource, setDuplicateMapSource] = useState<CanvasMap | null>(null);
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

  const handleViewChange = useCallback((view: ActiveView) => {
    setActiveView(view);
  }, []);

  const handleOpenGlobal = useCallback(() => {
    setSelectedMapId(null);
    setSelectedMapName('Default');
    setSelectedAreaId(null);
    setActiveView('canvas');
  }, []);

  const handleOpenMap = useCallback((map: CanvasMap) => {
    setSelectedMapId(map.is_default ? null : map.id);
    setSelectedMapName(map.name);
    setSelectedAreaId(null);
    setActiveView('canvas');
  }, []);

  const handleAreaSelect = useCallback((areaId: string | null) => {
    setSelectedMapId(null);
    setSelectedMapName('Default');
    setSelectedAreaId(areaId);
    setActiveView('canvas');
  }, []);

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
        const createdMap = await createCanvasMap({
          name,
          source_area_id: sourceArea?.id ?? null,
          filter: sourceArea ? mapFilterForArea(sourceArea) : {},
        });
        setCreateMapDialogOpen(false);
        setCreateMapSourceArea(null);
        setCanvasMaps((maps) => upsertCanvasMap(maps, createdMap));
        handleOpenMap(createdMap);
        void loadCanvasMaps();
      } catch (error) {
        setCanvasMapsError(canvasMapErrorMessage(error, 'Failed to create map'));
      }
    },
    [handleOpenMap, loadCanvasMaps],
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

  const handleDeleteMap = useCallback(
    async (map: CanvasMap) => {
      if (!enableSavedMaps || map.is_default) {
        return;
      }

      if (!window.confirm(`Delete map "${map.name}"?`)) {
        return;
      }

      setCanvasMapsError(null);

      try {
        await deleteCanvasMap(map.id);
        setCanvasMaps((maps) => maps.filter((candidate) => candidate.id !== map.id));
        if (selectedMapId === map.id) {
          setSelectedMapId(null);
          setSelectedMapName('Default');
        }
        void loadCanvasMaps();
      } catch (error) {
        setCanvasMapsError(canvasMapErrorMessage(error, 'Failed to delete map'));
      }
    },
    [loadCanvasMaps, selectedMapId],
  );

  const mapsForHub = enableSavedMaps ? canvasMaps : [];
  const mapsLoadingForHub = enableSavedMaps ? canvasMapsLoading : false;
  const mapsErrorForHub = enableSavedMaps ? canvasMapsError : null;

  return (
    <ThemeProvider>
      <div className="h-screen w-screen overflow-hidden bg-bg text-on-bg">
        <NavigationPill
          activeView={activeView}
          selectedAreaId={selectedAreaId}
          areas={areas}
          onViewChange={handleViewChange}
          onAreaSelect={handleAreaSelect}
        />
        {/* All views stay mounted; inactive ones hidden via CSS */}
        <div className={activeView === 'hub' ? 'h-full overflow-y-auto' : 'hidden'}>
          <TopologyHub
            devices={canvasDevices}
            areas={areas}
            links={canvasLinks}
            snapshot={snapshot}
            maps={mapsForHub}
            mapsLoading={mapsLoadingForHub}
            mapsError={mapsErrorForHub}
            savedMapsEnabled={enableSavedMaps}
            onOpenGlobal={handleOpenGlobal}
            onOpenArea={(areaId) => handleAreaSelect(areaId)}
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
          <Watermark activeView={activeView} selectedAreaId={selectedAreaId} areas={areas} />
          <ReactFlowProvider>
            <Canvas
              snapshot={snapshot}
              alerts={alerts}
              reconnecting={reconnecting}
              prometheusStatus={prometheusStatus}
              selectedAreaId={selectedAreaId}
              mapId={selectedMapId}
              mapName={selectedMapName}
              maps={canvasMaps}
              areas={areas}
              onDevicesChange={handleCanvasDevicesChange}
              onLinksChange={handleCanvasLinksChange}
              onAreaSelect={handleAreaSelect}
              onMapSelect={handleOpenMap}
              onManageMaps={() => setActiveView('hub')}
              onAreasChange={handleAreasChange}
              onDetailDeviceChange={setDetailDeviceId}
              onInteractionActiveChange={setCanvasInteractionActive}
            />
          </ReactFlowProvider>
        </div>
        <div className={activeView === 'dashboard' ? 'h-full' : 'hidden'}>
          <Dashboard devices={canvasDevices} areas={areas} snapshot={snapshot} />
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
          </>
        )}
      </div>
    </ThemeProvider>
  );
}

export default App;
