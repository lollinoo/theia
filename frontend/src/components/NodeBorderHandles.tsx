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

/** Renders four continuous, invisible connection hit zones around a node border. */
export function NodeBorderHandles({ isConnectable }: { isConnectable: boolean }): JSX.Element {
  return (
    <>
      {borderHandleDescriptors.map(({ id, position, style }) => (
        <Handle
          key={id}
          id={id}
          type="source"
          position={position}
          isConnectable={isConnectable}
          style={{
            ...transparentHandleStyle,
            ...style,
            pointerEvents: isConnectable ? 'auto' : 'none',
          }}
        />
      ))}
    </>
  );
}
