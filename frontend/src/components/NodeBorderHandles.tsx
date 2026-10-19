import { Handle, Position } from '@xyflow/react';
import { type CSSProperties, type JSX, memo } from 'react';

interface BorderHandleDescriptor {
  id: 'top' | 'right' | 'bottom' | 'left';
  position: Position;
  enabledStyle: CSSProperties;
  disabledStyle: CSSProperties;
  disabledClassName: string;
}

const transparentHandleStyle: CSSProperties = {
  background: 'transparent',
  borderWidth: 0,
  borderRadius: 0,
  zIndex: 30,
};

function createBorderHandleDescriptor(
  id: BorderHandleDescriptor['id'],
  position: Position,
  dimensions: CSSProperties,
): BorderHandleDescriptor {
  return {
    id,
    position,
    enabledStyle: {
      ...transparentHandleStyle,
      ...dimensions,
      pointerEvents: 'auto',
    },
    disabledStyle: {
      ...transparentHandleStyle,
      ...dimensions,
      pointerEvents: 'none',
    },
    disabledClassName: `react-flow__handle react-flow__handle-${position} nodrag source`,
  };
}

const borderHandleDescriptors: readonly BorderHandleDescriptor[] = [
  createBorderHandleDescriptor('top', Position.Top, { width: '100%', height: 12 }),
  createBorderHandleDescriptor('right', Position.Right, { width: 12, height: '100%' }),
  createBorderHandleDescriptor('bottom', Position.Bottom, { width: '100%', height: 12 }),
  createBorderHandleDescriptor('left', Position.Left, { width: 12, height: '100%' }),
];

export interface NodeBorderHandlesProps {
  isConnectable?: boolean;
  editMode?: boolean;
}

/**
 * Renders four continuous, invisible connection hit zones around a node border.
 *
 * When non-connectable (view mode and ghost nodes), renders lightweight static DOM
 * anchor elements with the exact `.source`, `data-handleid`, and `data-handlepos`
 * contract expected by `@xyflow/system`'s `getHandleBounds` so edges resolve on
 * initial load without registering Zustand store subscriptions per handle.
 */
export const NodeBorderHandles = memo(function NodeBorderHandles({
  isConnectable,
  editMode,
}: NodeBorderHandlesProps): JSX.Element {
  const isEnabled = editMode ?? isConnectable ?? false;

  if (!isEnabled) {
    return (
      <>
        {borderHandleDescriptors.map(({ id, position, disabledClassName, disabledStyle }) => (
          <div
            key={id}
            data-testid={`border-handle-${id}`}
            data-handleid={id}
            data-handlepos={position}
            data-handle-id={id}
            data-handle-type="source"
            data-handle-position={position}
            data-is-connectable="false"
            className={disabledClassName}
            style={disabledStyle}
          />
        ))}
      </>
    );
  }

  return (
    <>
      {borderHandleDescriptors.map(({ id, position, enabledStyle }) => (
        <Handle
          key={id}
          id={id}
          type="source"
          position={position}
          isConnectable={true}
          style={enabledStyle}
        />
      ))}
    </>
  );
});
