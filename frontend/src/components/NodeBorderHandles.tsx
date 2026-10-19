import { Handle, Position } from '@xyflow/react';
import type { CSSProperties, JSX } from 'react';

interface BorderHandleDescriptor {
  id: 'top' | 'right' | 'bottom' | 'left';
  position: Position;
  style: CSSProperties;
}

const borderHandleDescriptors: readonly BorderHandleDescriptor[] = [
  { id: 'top', position: Position.Top, style: { width: '100%', height: 12 } },
  { id: 'right', position: Position.Right, style: { width: 12, height: '100%' } },
  { id: 'bottom', position: Position.Bottom, style: { width: '100%', height: 12 } },
  { id: 'left', position: Position.Left, style: { width: 12, height: '100%' } },
];

const transparentHandleStyle: CSSProperties = {
  background: 'transparent',
  borderWidth: 0,
  borderRadius: 0,
  zIndex: 30,
};

export interface NodeBorderHandlesProps {
  isConnectable?: boolean;
  editMode?: boolean;
}

/** Renders four continuous, invisible connection hit zones around a node border. */
export function NodeBorderHandles({
  isConnectable,
  editMode,
}: NodeBorderHandlesProps): JSX.Element | null {
  const isEnabled = editMode ?? isConnectable ?? false;
  if (!isEnabled) {
    return null;
  }

  return (
    <>
      {borderHandleDescriptors.map(({ id, position, style }) => (
        <Handle
          key={id}
          id={id}
          type="source"
          position={position}
          isConnectable={isEnabled}
          style={{
            ...transparentHandleStyle,
            ...style,
            pointerEvents: 'auto',
          }}
        />
      ))}
    </>
  );
}
