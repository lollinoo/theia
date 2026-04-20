import { useState, useCallback, useEffect } from 'react';
import { ReactFlowProvider } from '@xyflow/react';
import Canvas from './components/Canvas';
import NavigationPill from './components/NavigationPill';
import { Watermark } from './components/Watermark';
import { Dashboard } from './components/Dashboard';
import AreaHub from './components/AreaHub';
import { ThemeProvider } from './contexts/ThemeContext';
import { useWebSocket } from './hooks/useWebSocket';
import { fetchAreas } from './api/client';
import type { Area, Device, Link } from './types/api';

export type ActiveView = 'hub' | 'canvas' | 'dashboard';

function App() {
  const [activeView, setActiveView] = useState<ActiveView>('canvas');
  const [selectedAreaId, setSelectedAreaId] = useState<string | null>(null);
  const [detailDeviceId, setDetailDeviceId] = useState<string | null>(null);
  const [canvasDevices, setCanvasDevices] = useState<Device[]>([]);
  const [canvasLinks, setCanvasLinks] = useState<Link[]>([]);
  const [areas, setAreas] = useState<Area[]>([]);

  const { snapshot, alerts, reconnecting, prometheusStatus } = useWebSocket('/api/v1/ws', detailDeviceId);

  // Fetch areas on mount
  useEffect(() => {
    fetchAreas()
      .then(setAreas)
      .catch(() => {
        // Area fetch failure is non-fatal; Hub will show empty state
      });
  }, []);

  // Re-fetch areas when switching to hub view (pick up changes from settings)
  useEffect(() => {
    if (activeView === 'hub') {
      fetchAreas()
        .then(setAreas)
        .catch(() => {});
    }
  }, [activeView]);

  const handleCanvasDevicesChange = useCallback((devices: Device[]) => {
    setCanvasDevices(devices);
  }, []);

  const handleCanvasLinksChange = useCallback((links: Link[]) => {
    setCanvasLinks(links);
  }, []);

  const handleViewChange = useCallback((view: ActiveView) => {
    setActiveView(view);
  }, []);

  const handleAreaSelect = useCallback((areaId: string | null) => {
    setSelectedAreaId(areaId);
    setActiveView('canvas');
  }, []);

  const handleAreasChange = useCallback(() => {
    fetchAreas()
      .then(setAreas)
      .catch(() => {});
  }, []);

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
      <Watermark activeView={activeView} selectedAreaId={selectedAreaId} areas={areas} />
      {/* All views stay mounted; inactive ones hidden via CSS */}
      <div className={activeView === 'hub' ? 'h-full overflow-y-auto' : 'hidden'}>
        <AreaHub
          devices={canvasDevices}
          areas={areas}
          links={canvasLinks}
          snapshot={snapshot}
          onAreaSelect={handleAreaSelect}
          onOpenSettings={() => {
            setActiveView('canvas');
          }}
        />
      </div>
      <div className={activeView === 'canvas' ? 'h-full' : 'hidden'}>
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
