export function getCanvasDetailDeviceId(
  panelContent: { type: string; data?: unknown } | null,
): string | null {
  if (panelContent === null) {
    return null;
  }

  if (panelContent.type === 'deviceConfig') {
    const data = panelContent.data as { deviceId?: string } | undefined;
    return data?.deviceId ?? null;
  }

  if (panelContent.type === 'interfaceStats') {
    const data = panelContent.data as { deviceId?: string; linkId?: string } | undefined;
    if (data?.deviceId && !data.linkId) {
      return data.deviceId;
    }
  }

  return null;
}
