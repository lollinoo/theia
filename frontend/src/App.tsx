import { ReactFlowProvider } from 'reactflow';
import Canvas from './components/Canvas';

function App() {
  return (
    <div className="h-screen w-screen overflow-hidden bg-bg-canvas text-text-primary">
      <ReactFlowProvider>
        <Canvas />
      </ReactFlowProvider>
    </div>
  );
}

export default App;
