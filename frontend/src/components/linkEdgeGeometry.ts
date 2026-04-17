export interface EdgePathModel {
  edgePath: string;
  labelX: number;
  labelY: number;
}

interface SelfLoopPathOptions {
  sourceX: number;
  sourceY: number;
  targetX: number;
  targetY: number;
  parallelIndex?: number;
}

export function buildSelfLoopPathModel({
  sourceX,
  sourceY,
  targetX,
  targetY,
  parallelIndex,
}: SelfLoopPathOptions): EdgePathModel {
  const lane = parallelIndex ?? 0;
  const horizontalSpan = Math.max(Math.abs(sourceX - targetX), 56);
  const verticalSpan = Math.abs(sourceY - targetY);
  const direction = sourceX >= targetX ? 1 : -1;
  const shoulder = Math.max(46, Math.round(horizontalSpan * 0.28)) + lane * 14;
  const crownSpread = Math.max(28, Math.round(horizontalSpan * 0.14)) + lane * 8;
  const rise = Math.max(88 + lane * 22, verticalSpan + 64);
  const apexY = Math.min(sourceY, targetY) - rise;
  const centerX = (sourceX + targetX) / 2;
  const sourceShoulderX = sourceX + direction * shoulder;
  const targetShoulderX = targetX - direction * shoulder;
  const sourceCrownX = centerX + direction * crownSpread;
  const targetCrownX = centerX - direction * crownSpread;
  const controlY = apexY + Math.max(28, Math.round(rise * 0.58));

  return {
    edgePath: [
      `M ${sourceX},${sourceY}`,
      `C ${sourceShoulderX},${sourceY} ${sourceCrownX},${controlY} ${centerX},${apexY}`,
      `C ${targetCrownX},${controlY} ${targetShoulderX},${targetY} ${targetX},${targetY}`,
    ].join(' '),
    labelX: centerX,
    labelY: apexY - 6,
  };
}
