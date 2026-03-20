import { useState, useCallback } from 'react';
import { ReactFlowProvider } from 'reactflow';
import Canvas from './components/Canvas';
import { NavBar, type ActiveView } from './components/NavBar';
import { Dashboard } from './components/Dashboard';
import type { Device } from './types/api';

function App() {
  const [activeView, setActiveView] = useState<ActiveView>('canvas');
  const [canvasDevices, setCanvasDevices] = useState<Device[]>([]);

  const handleCanvasDevicesChange = useCallback((devices: Device[]) => {
    setCanvasDevices(devices);
  }, []);

  return (
    <div className="h-screen w-screen overflow-hidden bg-bg-canvas text-text-primary">
      <NavBar activeView={activeView} onViewChange={setActiveView} />
      {/* Both views stay mounted; inactive one is hidden via CSS */}
      <div className={activeView === 'canvas' ? 'h-full' : 'hidden'}>
        <ReactFlowProvider>
          <Canvas onDevicesChange={handleCanvasDevicesChange} />
        </ReactFlowProvider>
      </div>
      <div className={activeView === 'dashboard' ? 'h-full' : 'hidden'}>
        <Dashboard devices={canvasDevices} />
      </div>
    </div>
  );
}

export default App;
