/**
 * Defines detail subscription behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
export function getCanvasDetailDeviceId(
  panelContent: { type: string; data?: unknown } | null,
): string | null {
  if (panelContent === null) {
    return null;
  }

  if (panelContent.type === 'deviceConfig' || panelContent.type === 'deviceDetails') {
    const data = panelContent.data as { deviceId?: string } | undefined;
    return data?.deviceId ?? null;
  }

  return null;
}
