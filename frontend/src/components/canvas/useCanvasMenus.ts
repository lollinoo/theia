import { useMemo, useState } from 'react';
import type { ReactFlowInstance } from '@xyflow/react';

import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import type { Device, Link } from '../../types/api';

interface UseCanvasMenusParams {
  reactFlow: ReactFlowInstance<DeviceNode, LinkEdgeType>;
}

interface UseCanvasMenusReturn {
  deviceMenu: { deviceId: string; x: number; y: number } | null;
  setDeviceMenu: React.Dispatch<React.SetStateAction<{ deviceId: string; x: number; y: number } | null>>;
  edgeMenu: { edgeID: string; x: number; y: number } | null;
  setEdgeMenu: React.Dispatch<React.SetStateAction<{ edgeID: string; x: number; y: number } | null>>;
  panelContent: { type: string; data?: unknown } | null;
  setPanelContent: React.Dispatch<React.SetStateAction<{ type: string; data?: unknown } | null>>;
  showShortcuts: boolean;
  setShowShortcuts: React.Dispatch<React.SetStateAction<boolean>>;
  showSearch: boolean;
  setShowSearch: React.Dispatch<React.SetStateAction<boolean>>;
  editMode: boolean;
  setEditMode: React.Dispatch<React.SetStateAction<boolean>>;
  shortcuts: Record<string, { key: string; ctrl?: boolean; description: string; handler: () => void }>;
  getPanelTitle: () => string;
}

export function useCanvasMenus({ reactFlow }: UseCanvasMenusParams): UseCanvasMenusReturn {
  const [deviceMenu, setDeviceMenu] = useState<{ deviceId: string; x: number; y: number } | null>(null);
  const [edgeMenu, setEdgeMenu] = useState<{ edgeID: string; x: number; y: number } | null>(null);
  const [panelContent, setPanelContent] = useState<{ type: string; data?: unknown } | null>(null);
  const [showShortcuts, setShowShortcuts] = useState(false);
  const [showSearch, setShowSearch] = useState(false);
  const [editMode, setEditMode] = useState(false);

  const shortcuts = useMemo(() => ({
    search: {
      key: 'k',
      ctrl: true,
      description: 'Search devices',
      handler: () => setShowSearch((s) => !s),
    },
    addDevice: {
      key: 'a',
      description: 'Add device',
      handler: () => setPanelContent({ type: 'addDevice' }),
    },
    createLink: {
      key: 'l',
      description: 'Create link',
      handler: () => setPanelContent({ type: 'create-link' }),
    },
    editMode: {
      key: 'e',
      description: 'Toggle edit mode',
      handler: () => setEditMode((m) => !m),
    },
    settings: {
      key: ',',
      ctrl: true,
      description: 'Settings',
      handler: () => setPanelContent({ type: 'settings' }),
    },
    zoomIn: {
      key: '+',
      description: 'Zoom in',
      handler: () => { void reactFlow.zoomIn({ duration: 200 }); },
    },
    zoomOut: {
      key: '-',
      description: 'Zoom out',
      handler: () => { void reactFlow.zoomOut({ duration: 200 }); },
    },
    zoomFit: {
      key: '0',
      description: 'Fit view',
      handler: () => { void reactFlow.fitView({ padding: 0.18, duration: 280 }); },
    },
    help: {
      key: '?',
      description: 'Shortcuts help',
      handler: () => setShowShortcuts((s) => !s),
    },
    escape: {
      key: 'escape',
      description: 'Close panels',
      handler: () => {
        if (deviceMenu) setDeviceMenu(null);
        else if (edgeMenu) setEdgeMenu(null);
        else if (panelContent) setPanelContent(null);
        else if (showSearch) setShowSearch(false);
        else if (showShortcuts) setShowShortcuts(false);
      },
    },
  }), [reactFlow, deviceMenu, edgeMenu, panelContent, showSearch, showShortcuts]);

  function getPanelTitle(): string {
    if (!panelContent) return '';
    if (panelContent.type === 'alerts') return 'Alerts';
    if (panelContent.type === 'settings') return 'Settings';
    if (panelContent.type === 'addDevice') return 'Add Device';
    if (panelContent.type === 'create-link') return 'Create Link';
    if (panelContent.type === 'link-details') return 'Link Details';
    if (panelContent.type === 'deviceConfig') {
      const data = panelContent.data as { device?: Device } | undefined;
      if (data?.device) {
        const d = data.device;
        return d.tags?.display_name || d.sys_name || d.hostname || 'Configure Device';
      }
      return 'Configure Device';
    }
    if (panelContent.type === 'bulkEdit') {
      const data = panelContent.data as { deviceIds?: string[] } | undefined;
      const count = data?.deviceIds?.length ?? 0;
      return `Bulk Edit (${count})`;
    }
    if (panelContent.type === 'interfaceStats') {
      const data = panelContent.data as { link?: Link; sourceDevice?: Device; targetDevice?: Device; device?: Device } | undefined;
      if (data?.link && data.sourceDevice && data.targetDevice) {
        const srcName = data.sourceDevice.tags?.display_name || data.sourceDevice.sys_name || data.sourceDevice.hostname;
        const dstName = data.targetDevice.tags?.display_name || data.targetDevice.sys_name || data.targetDevice.hostname;
        return `${srcName} -- ${dstName}`;
      }
      if (data?.device) {
        return data.device.tags?.display_name || data.device.sys_name || data.device.ip;
      }
      return 'Interface Stats';
    }
    return '';
  }

  return {
    deviceMenu,
    setDeviceMenu,
    edgeMenu,
    setEdgeMenu,
    panelContent,
    setPanelContent,
    showShortcuts,
    setShowShortcuts,
    showSearch,
    setShowSearch,
    editMode,
    setEditMode,
    shortcuts,
    getPanelTitle,
  };
}
