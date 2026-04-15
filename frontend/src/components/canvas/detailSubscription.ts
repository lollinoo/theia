export function getCanvasDetailDeviceId(
  panelContent: { type: string; data?: unknown } | null,
): string | null {
  if (panelContent === null) {
    return null;
  }

  if (panelContent.type === 'deviceConfig') {
    const data = panelContent.data as { device?: { id?: string } } | undefined;
    return data?.device?.id ?? null;
  }

  if (panelContent.type === 'interfaceStats') {
    const data = panelContent.data as { device?: { id?: string }; link?: unknown } | undefined;
    if (data?.device && !data.link) {
      return data.device.id ?? null;
    }
  }

  return null;
}
