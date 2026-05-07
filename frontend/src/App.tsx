import { ReactFlowProvider } from '@xyflow/react';
import { useCallback, useEffect, useState } from 'react';
import { fetchAreas, fetchCanvasMaps } from './api/client';
import Canvas from './components/Canvas';
import { Dashboard } from './components/Dashboard';
import NavigationPill from './components/NavigationPill';
import TopologyHub from './components/topology-hub/TopologyHub';
import { Watermark } from './components/Watermark';
import { ThemeProvider } from './contexts/ThemeContext';
import { useWebSocket } from './hooks/useWebSocket';
import type { Area, CanvasMap, Device, Link } from './types/api';

export type ActiveView = 'hub' | 'canvas' | 'dashboard';

const runtimeUpdatePauseIdleDelayMs = 1500;
const enableSavedMaps = false;

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
  const [canvasInteractionActive, setCanvasInteractionActive] = useState(false);
  const [runtimeUpdatesPaused, setRuntimeUpdatesPaused] = useState(false);

  void selectedMapId;
  void selectedMapName;

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

  const loadCanvasMaps = useCallback(async () => {
    if (!enableSavedMaps) {
      return;
    }

    setCanvasMapsLoading(true);
    setCanvasMapsError(null);

    try {
      setCanvasMaps(await fetchCanvasMaps());
    } catch (error) {
      setCanvasMapsError(error instanceof Error ? error.message : 'Failed to load maps');
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
    void area;
  }, []);

  const handleDuplicateMap = useCallback((map: CanvasMap) => {
    void map;
  }, []);

  const handleDeleteMap = useCallback((map: CanvasMap) => {
    void map;
  }, []);

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
              areas={areas}
              onDevicesChange={handleCanvasDevicesChange}
              onLinksChange={handleCanvasLinksChange}
              onAreaSelect={handleAreaSelect}
              onAreasChange={handleAreasChange}
              onDetailDeviceChange={setDetailDeviceId}
              onInteractionActiveChange={setCanvasInteractionActive}
            />
          </ReactFlowProvider>
        </div>
        <div className={activeView === 'dashboard' ? 'h-full' : 'hidden'}>
          <Dashboard devices={canvasDevices} areas={areas} snapshot={snapshot} />
        </div>
      </div>
    </ThemeProvider>
  );
}

export default App;
