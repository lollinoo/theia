/**
 * Renders link edge UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
import {
  BaseEdge,
  type Edge,
  type EdgeProps,
  getBezierPath,
  useInternalNode,
  useReactFlow,
} from '@xyflow/react';
import {
  memo,
  type MouseEvent as ReactMouseEvent,
  type PointerEvent as ReactPointerEvent,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import type { LinkRoute, LinkWaypoint } from '../types/api';
import type { DeviceNode } from './DeviceCard';
import { buildEditableLinkPath, nearestRouteInsertion } from './editableLinkGeometry';
import { deviceNodeBorderRadius, nodeRect } from './floatingEdgeGeometry';
import { LinkRouteControls } from './LinkRouteControls';
import { buildSelfLoopPathModel } from './linkEdgeGeometry';
import { registerLinkLabel, unregisterLinkLabel } from './linkLabelRegistry';
import {
  insertRouteWaypoint,
  LINK_ROUTE_DRAG_THRESHOLD_PX,
  moveRouteWaypoint,
  nudgeRouteWaypoint,
  removeRouteWaypoint,
} from './linkRouteEditing';
import {
  type LinkEdgeData,
  type LinkRouteEditToken,
  resolveEdgeTone,
  resolveLinkBadgePresentation,
} from './linkSemantics';

/** Describes the link edge type contract used by the UI component boundary. */
export type LinkEdgeType = Edge<LinkEdgeData>;

const LINK_ROUTE_KEYBOARD_COMMIT_DELAY_MS = 180;

type RoutePointerTarget = SVGPathElement | HTMLButtonElement;

interface RoutePointerGesture {
  kind: 'path' | 'waypoint';
  pointerId: number;
  originClient: LinkWaypoint;
  baseRoute: LinkRoute | null;
  waypointIndex: number | null;
  dragging: boolean;
  captureTarget: RoutePointerTarget | null;
  editToken: LinkRouteEditToken;
}

function routeInsertion(
  route: LinkRoute | null,
  segments: Parameters<typeof nearestRouteInsertion>[0],
  point: LinkWaypoint,
) {
  return route === null || route.waypoints.length === 0
    ? { insertIndex: 0, point }
    : nearestRouteInsertion(segments, point);
}

function LinkEdgeInner({
  id,
  source,
  target,
  sourceX,
  sourceY,
  sourcePosition,
  targetX,
  targetY,
  targetPosition,
  selected,
  data,
}: EdgeProps<LinkEdgeType>) {
  const sourceNode = useInternalNode<DeviceNode>(source);
  const targetNode = useInternalNode<DeviceNode>(target);
  const { screenToFlowPosition } = useReactFlow();
  const [hovered, setHovered] = useState(false);
  const [draftRoute, setDraftRoute] = useState<LinkRoute | null>(null);
  const [draftIsAutomatic, setDraftIsAutomatic] = useState(false);
  const [selectedWaypointIndex, setSelectedWaypointIndex] = useState<number | null>(null);
  const pointerGestureRef = useRef<RoutePointerGesture | null>(null);
  const draftRouteRef = useRef<LinkRoute | null>(null);
  const latestPointerPointRef = useRef<LinkWaypoint | null>(null);
  const animationFrameRef = useRef<number | null>(null);
  const keyboardCommitTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const edgeHitTargetRef = useRef<SVGPathElement | null>(null);
  const suppressNextClickRef = useRef(false);
  const draftBadgePresentationRef = useRef<ReturnType<typeof resolveLinkBadgePresentation>>(null);
  const hasFrozenBadgePresentationRef = useRef(false);
  const routeEditToken = data?.routeEditToken;
  const previousRouteEditTokenRef = useRef(routeEditToken);
  const activeRoute = draftRoute ?? data?.route ?? null;
  const renderedRoute = draftIsAutomatic ? null : activeRoute;
  const routeEditable =
    data?.routeEditable === true &&
    routeEditToken !== undefined &&
    typeof data.onRouteCommit === 'function';
  const canEditRoute = selected === true && routeEditable;
  const interactionMode = data?.interactionMode ?? 'idle';
  const isInteractive = interactionMode === 'interactive';
  const isActive = selected || hovered;
  const isConnected = data?.emphasis === 'connected';
  const isMuted = data?.emphasis === 'muted';
  const index = data?.parallelIndex || 0;
  const laneOrientation = source <= target ? 1 : -1;
  const isSelfLoop =
    source === target || data?.link?.source_device_id === data?.link?.target_device_id;
  const sourceBounds = nodeRect(sourceNode);
  const targetBounds = nodeRect(targetNode);
  const sourceBoundsX = sourceBounds?.x ?? null;
  const sourceBoundsY = sourceBounds?.y ?? null;
  const sourceBoundsWidth = sourceBounds?.width ?? null;
  const sourceBoundsHeight = sourceBounds?.height ?? null;
  const targetBoundsX = targetBounds?.x ?? null;
  const targetBoundsY = targetBounds?.y ?? null;
  const targetBoundsWidth = targetBounds?.width ?? null;
  const targetBoundsHeight = targetBounds?.height ?? null;
  const sourceRadius = deviceNodeBorderRadius(sourceNode);
  const targetRadius = deviceNodeBorderRadius(targetNode);
  const sourceRect = useMemo(
    () =>
      sourceBoundsX === null ||
      sourceBoundsY === null ||
      sourceBoundsWidth === null ||
      sourceBoundsHeight === null
        ? null
        : {
            x: sourceBoundsX,
            y: sourceBoundsY,
            width: sourceBoundsWidth,
            height: sourceBoundsHeight,
          },
    [sourceBoundsHeight, sourceBoundsWidth, sourceBoundsX, sourceBoundsY],
  );
  const targetRect = useMemo(
    () =>
      targetBoundsX === null ||
      targetBoundsY === null ||
      targetBoundsWidth === null ||
      targetBoundsHeight === null
        ? null
        : {
            x: targetBoundsX,
            y: targetBoundsY,
            width: targetBoundsWidth,
            height: targetBoundsHeight,
          },
    [targetBoundsHeight, targetBoundsWidth, targetBoundsX, targetBoundsY],
  );
  const isAutomaticRoute = renderedRoute === null || renderedRoute.waypoints.length === 0;
  const automaticPath = useMemo(() => {
    const [edgePath, labelX, labelY] = getBezierPath({
      sourceX,
      sourceY,
      sourcePosition,
      targetX,
      targetY,
      targetPosition,
    });
    return { edgePath, labelX, labelY, segments: [], waypoints: [] };
  }, [sourcePosition, sourceX, sourceY, targetPosition, targetX, targetY]);
  const manualPath = useMemo(() => {
    if (renderedRoute === null || renderedRoute.waypoints.length === 0) {
      return null;
    }
    return buildEditableLinkPath({
      sourceRect,
      targetRect,
      fallbackSource: { x: sourceX, y: sourceY },
      fallbackTarget: { x: targetX, y: targetY },
      route: renderedRoute,
      parallelIndex: index,
      laneOrientation,
      sourceRadius,
      targetRadius,
    });
  }, [
    index,
    laneOrientation,
    renderedRoute,
    sourceRect,
    sourceRadius,
    sourceX,
    sourceY,
    targetRect,
    targetRadius,
    targetX,
    targetY,
  ]);
  const selfLoopPath = useMemo(
    () =>
      buildSelfLoopPathModel({
        sourceX,
        sourceY,
        targetX,
        targetY,
        parallelIndex: index,
      }),
    [index, sourceX, sourceY, targetX, targetY],
  );
  const routePath = isAutomaticRoute
    ? isSelfLoop
      ? { ...automaticPath, ...selfLoopPath }
      : automaticPath
    : manualPath!;
  const { edgePath, labelX, labelY } = routePath;

  const sign = index % 2 === 0 ? 1 : -1;
  const magnitude = Math.ceil(index / 2) * 20;
  const labelOffsetY = sign * magnitude;
  const tone = resolveEdgeTone(data);
  const haloColor =
    isConnected &&
    data?.areaColor &&
    tone.semanticState !== 'warning' &&
    tone.semanticState !== 'critical'
      ? data.areaColor
      : tone.haloColor;
  const isOperationalAlert = tone.semanticState === 'warning' || tone.semanticState === 'critical';
  const strokeOpacity = isMuted
    ? 0.22
    : isConnected
      ? 0.98
      : isOperationalAlert
        ? 0.96
        : isActive
          ? 0.94
          : 0.72;
  const strokeWidth = isActive || isConnected ? tone.width + 0.7 : tone.width;
  const labelYOffset = labelY + labelOffsetY;
  const badgePresentation = useMemo(
    () =>
      resolveLinkBadgePresentation({
        data,
        zoom: 1,
        path: edgePath,
        fallbackX: labelX,
        fallbackY: labelYOffset,
        edgeTone: tone,
        parallelIndex: data?.parallelIndex,
        isActive,
        isConnected,
        isMuted,
      }),
    [data, edgePath, isActive, isConnected, isMuted, labelX, labelYOffset, tone],
  );
  const registeredBadgePresentation =
    (draftRoute !== null || draftIsAutomatic) && hasFrozenBadgePresentationRef.current
      ? draftBadgePresentationRef.current
      : badgePresentation;

  const freezeBadgePresentation = () => {
    if (hasFrozenBadgePresentationRef.current) {
      return;
    }
    draftBadgePresentationRef.current = badgePresentation;
    hasFrozenBadgePresentationRef.current = true;
  };

  const clearFrozenBadgePresentation = () => {
    draftBadgePresentationRef.current = null;
    hasFrozenBadgePresentationRef.current = false;
  };

  const clearAnimationFrame = () => {
    if (animationFrameRef.current !== null) {
      window.cancelAnimationFrame(animationFrameRef.current);
      animationFrameRef.current = null;
    }
  };

  const clearKeyboardCommitTimer = () => {
    if (keyboardCommitTimerRef.current !== null) {
      window.clearTimeout(keyboardCommitTimerRef.current);
      keyboardCommitTimerRef.current = null;
    }
  };

  const pointerFlowPosition = (clientX: number, clientY: number): LinkWaypoint =>
    screenToFlowPosition({ x: clientX, y: clientY }, { snapToGrid: false });

  const applyLatestPointerPoint = (): LinkRoute | null => {
    const gesture = pointerGestureRef.current;
    const point = latestPointerPointRef.current;
    const currentRoute = draftRouteRef.current;
    if (!gesture?.dragging || gesture.waypointIndex === null || point === null || !currentRoute) {
      return currentRoute;
    }

    latestPointerPointRef.current = null;
    const nextRoute = moveRouteWaypoint(currentRoute, gesture.waypointIndex, point);
    draftRouteRef.current = nextRoute;
    setDraftIsAutomatic(false);
    setDraftRoute(nextRoute);
    return nextRoute;
  };

  const queuePointerPoint = (point: LinkWaypoint) => {
    latestPointerPointRef.current = point;
    if (animationFrameRef.current !== null) {
      return;
    }

    animationFrameRef.current = window.requestAnimationFrame(() => {
      animationFrameRef.current = null;
      applyLatestPointerPoint();
    });
  };

  const flushPointerPoint = (): LinkRoute | null => {
    clearAnimationFrame();
    return applyLatestPointerPoint();
  };

  const capturePointer = (target: RoutePointerTarget, pointerId: number) => {
    target.setPointerCapture(pointerId);
  };

  const releasePointer = (gesture: RoutePointerGesture) => {
    const target = gesture.captureTarget;
    if (!target) {
      return;
    }
    if (
      typeof target.hasPointerCapture === 'function' &&
      !target.hasPointerCapture(gesture.pointerId)
    ) {
      return;
    }
    target.releasePointerCapture(gesture.pointerId);
  };

  const abandonPointerGesture = (event: ReactPointerEvent<RoutePointerTarget>) => {
    const gesture = pointerGestureRef.current;
    if (!gesture || gesture.pointerId !== event.pointerId) {
      return;
    }

    pointerGestureRef.current = null;
    clearAnimationFrame();
    latestPointerPointRef.current = null;
    suppressNextClickRef.current = false;
    if (gesture.dragging) {
      event.preventDefault();
      event.stopPropagation();
      draftRouteRef.current = null;
      clearFrozenBadgePresentation();
      setDraftRoute(null);
      setDraftIsAutomatic(false);
      if (gesture.kind === 'path') {
        setSelectedWaypointIndex(null);
      }
    }
    releasePointer(gesture);
  };

  const handleEdgePointerDown = (event: ReactPointerEvent<SVGPathElement>) => {
    suppressNextClickRef.current = false;
    if (!canEditRoute || event.button !== 0 || pointerGestureRef.current !== null) {
      return;
    }

    pointerGestureRef.current = {
      kind: 'path',
      pointerId: event.pointerId,
      originClient: { x: event.clientX, y: event.clientY },
      baseRoute: renderedRoute,
      waypointIndex: null,
      dragging: false,
      captureTarget: null,
      editToken: routeEditToken!,
    };
  };

  const handleEdgePointerMove = (event: ReactPointerEvent<SVGPathElement>) => {
    const gesture = pointerGestureRef.current;
    if (!gesture || gesture.pointerId !== event.pointerId) {
      return;
    }

    if (!gesture.dragging) {
      const clientDistance = Math.hypot(
        event.clientX - gesture.originClient.x,
        event.clientY - gesture.originClient.y,
      );
      if (clientDistance < LINK_ROUTE_DRAG_THRESHOLD_PX) {
        return;
      }

      const originPoint = pointerFlowPosition(gesture.originClient.x, gesture.originClient.y);
      const currentPoint = pointerFlowPosition(event.clientX, event.clientY);
      const insertion = routeInsertion(gesture.baseRoute, routePath.segments, originPoint);
      const insertedRoute = insertRouteWaypoint(
        gesture.baseRoute,
        insertion.insertIndex,
        insertion.point,
      );
      if (insertedRoute === gesture.baseRoute) {
        pointerGestureRef.current = null;
        return;
      }

      clearKeyboardCommitTimer();
      const nextRoute = moveRouteWaypoint(insertedRoute, insertion.insertIndex, currentPoint);
      freezeBadgePresentation();
      gesture.dragging = true;
      gesture.waypointIndex = insertion.insertIndex;
      gesture.captureTarget = event.currentTarget;
      draftRouteRef.current = nextRoute;
      latestPointerPointRef.current = null;
      suppressNextClickRef.current = true;
      setSelectedWaypointIndex(insertion.insertIndex);
      setDraftIsAutomatic(false);
      setDraftRoute(nextRoute);
      capturePointer(event.currentTarget, event.pointerId);
      event.preventDefault();
      event.stopPropagation();
      return;
    }

    event.preventDefault();
    event.stopPropagation();
    queuePointerPoint(pointerFlowPosition(event.clientX, event.clientY));
  };

  const handleEdgePointerLeave = (event: ReactPointerEvent<SVGPathElement>) => {
    abandonPointerGesture(event);
  };

  const waypointPointForClientPosition = (
    gesture: RoutePointerGesture,
    clientX: number,
    clientY: number,
  ): LinkWaypoint => {
    const waypoint =
      gesture.waypointIndex === null
        ? undefined
        : gesture.baseRoute?.waypoints[gesture.waypointIndex];
    const currentPoint = pointerFlowPosition(clientX, clientY);
    if (!waypoint) {
      return currentPoint;
    }
    const originPoint = pointerFlowPosition(gesture.originClient.x, gesture.originClient.y);
    return {
      x: waypoint.x + currentPoint.x - originPoint.x,
      y: waypoint.y + currentPoint.y - originPoint.y,
    };
  };

  const finishPointerGesture = (
    event: ReactPointerEvent<RoutePointerTarget>,
    includeEventPoint: boolean,
  ) => {
    const gesture = pointerGestureRef.current;
    if (!gesture || gesture.pointerId !== event.pointerId) {
      return;
    }

    if (!gesture.dragging) {
      pointerGestureRef.current = null;
      latestPointerPointRef.current = null;
      releasePointer(gesture);
      return;
    }

    event.preventDefault();
    event.stopPropagation();
    if (includeEventPoint) {
      latestPointerPointRef.current =
        gesture.kind === 'waypoint'
          ? waypointPointForClientPosition(gesture, event.clientX, event.clientY)
          : pointerFlowPosition(event.clientX, event.clientY);
    }
    const nextRoute = flushPointerPoint();
    if (nextRoute) {
      data?.onRouteCommit?.(id, nextRoute, gesture.editToken);
    }
    pointerGestureRef.current = null;
    latestPointerPointRef.current = null;
    draftRouteRef.current = null;
    clearFrozenBadgePresentation();
    setDraftRoute(null);
    setDraftIsAutomatic(false);
    releasePointer(gesture);
  };

  const handleWaypointPointerDown = (
    event: ReactPointerEvent<HTMLButtonElement>,
    waypointIndex: number,
  ) => {
    if (
      !canEditRoute ||
      !renderedRoute ||
      event.button !== 0 ||
      pointerGestureRef.current !== null
    ) {
      return;
    }

    latestPointerPointRef.current = null;
    pointerGestureRef.current = {
      kind: 'waypoint',
      pointerId: event.pointerId,
      originClient: { x: event.clientX, y: event.clientY },
      baseRoute: renderedRoute,
      waypointIndex,
      dragging: false,
      captureTarget: event.currentTarget,
      editToken: routeEditToken!,
    };
    setSelectedWaypointIndex(waypointIndex);
    capturePointer(event.currentTarget, event.pointerId);
    event.preventDefault();
  };

  const handleWaypointPointerMove = (
    event: ReactPointerEvent<HTMLButtonElement>,
    waypointIndex: number,
  ) => {
    const gesture = pointerGestureRef.current;
    if (
      !gesture ||
      gesture.pointerId !== event.pointerId ||
      gesture.waypointIndex !== waypointIndex
    ) {
      return;
    }
    if (!gesture.dragging) {
      const clientDistance = Math.hypot(
        event.clientX - gesture.originClient.x,
        event.clientY - gesture.originClient.y,
      );
      if (clientDistance < LINK_ROUTE_DRAG_THRESHOLD_PX) {
        return;
      }
      clearKeyboardCommitTimer();
      clearAnimationFrame();
      freezeBadgePresentation();
      draftRouteRef.current = gesture.baseRoute;
      gesture.dragging = true;
    }
    event.preventDefault();
    queuePointerPoint(waypointPointForClientPosition(gesture, event.clientX, event.clientY));
  };

  const handleEdgeDoubleClick = (event: ReactMouseEvent<SVGPathElement>) => {
    if (!canEditRoute) {
      return;
    }

    event.preventDefault();
    event.stopPropagation();
    const pointerPoint = pointerFlowPosition(event.clientX, event.clientY);
    const insertion = routeInsertion(renderedRoute, routePath.segments, pointerPoint);
    const nextRoute = insertRouteWaypoint(renderedRoute, insertion.insertIndex, insertion.point);
    if (nextRoute === renderedRoute) {
      return;
    }
    clearKeyboardCommitTimer();
    setSelectedWaypointIndex(insertion.insertIndex);
    data?.onRouteCommit?.(id, nextRoute, routeEditToken!);
    draftRouteRef.current = null;
    clearFrozenBadgePresentation();
    setDraftRoute(null);
    setDraftIsAutomatic(false);
  };

  const scheduleKeyboardCommit = (nextRoute: LinkRoute | null, editToken: LinkRouteEditToken) => {
    clearKeyboardCommitTimer();
    keyboardCommitTimerRef.current = window.setTimeout(() => {
      keyboardCommitTimerRef.current = null;
      data?.onRouteCommit?.(id, nextRoute, editToken);
      draftRouteRef.current = null;
      clearFrozenBadgePresentation();
      setDraftRoute(null);
      setDraftIsAutomatic(false);
    }, LINK_ROUTE_KEYBOARD_COMMIT_DELAY_MS);
  };

  const handleWaypointNudge = (waypointIndex: number, dx: number, dy: number) => {
    if (routeEditToken === undefined) {
      return;
    }
    const currentRoute = draftRouteRef.current ?? renderedRoute;
    if (!currentRoute) {
      return;
    }
    const nextRoute = nudgeRouteWaypoint(currentRoute, waypointIndex, dx, dy);
    freezeBadgePresentation();
    draftRouteRef.current = nextRoute;
    setSelectedWaypointIndex(waypointIndex);
    setDraftIsAutomatic(false);
    setDraftRoute(nextRoute);
    scheduleKeyboardCommit(nextRoute, routeEditToken);
  };

  const handleWaypointRemove = (waypointIndex: number) => {
    if (routeEditToken === undefined) {
      return;
    }
    const currentRoute = draftRouteRef.current ?? renderedRoute;
    if (!currentRoute) {
      return;
    }
    const nextRoute = removeRouteWaypoint(currentRoute, waypointIndex);
    freezeBadgePresentation();
    draftRouteRef.current = nextRoute;
    if (nextRoute) {
      setSelectedWaypointIndex(Math.min(waypointIndex, nextRoute.waypoints.length - 1));
      setDraftIsAutomatic(false);
      setDraftRoute(nextRoute);
    } else {
      setSelectedWaypointIndex(null);
      setDraftRoute(null);
      setDraftIsAutomatic(true);
      edgeHitTargetRef.current?.focus({ preventScroll: true });
    }
    scheduleKeyboardCommit(nextRoute, routeEditToken);
  };

  useLayoutEffect(() => {
    if (previousRouteEditTokenRef.current === routeEditToken) {
      return;
    }
    previousRouteEditTokenRef.current = routeEditToken;
    const gesture = pointerGestureRef.current;
    pointerGestureRef.current = null;
    if (gesture !== null) {
      releasePointer(gesture);
    }
    clearAnimationFrame();
    clearKeyboardCommitTimer();
    latestPointerPointRef.current = null;
    draftRouteRef.current = null;
    suppressNextClickRef.current = false;
    clearFrozenBadgePresentation();
    setDraftRoute(null);
    setDraftIsAutomatic(false);
    setSelectedWaypointIndex(null);
  }, [routeEditToken]);

  useLayoutEffect(() => {
    if (registeredBadgePresentation === null || registeredBadgePresentation.items.length === 0) {
      unregisterLinkLabel(id);
      return;
    }

    registerLinkLabel({
      edgeId: id,
      interactive: isInteractive,
      presentation: registeredBadgePresentation,
    });
  }, [id, isInteractive, registeredBadgePresentation]);

  useEffect(
    () => () => {
      unregisterLinkLabel(id);
    },
    [id],
  );

  useEffect(
    () => () => {
      if (animationFrameRef.current !== null) {
        window.cancelAnimationFrame(animationFrameRef.current);
      }
      if (keyboardCommitTimerRef.current !== null) {
        window.clearTimeout(keyboardCommitTimerRef.current);
      }
    },
    [],
  );

  return (
    <>
      {(isActive || isConnected) && (
        <BaseEdge
          id={`${id}-halo`}
          path={edgePath}
          style={{
            stroke: haloColor,
            strokeOpacity: isConnected ? 0.22 : 0.18,
            strokeWidth: strokeWidth + 4,
            transition: isInteractive
              ? 'none'
              : 'stroke-width 120ms ease, stroke-opacity 120ms ease',
          }}
        />
      )}

      <BaseEdge
        id={id}
        path={edgePath}
        style={{
          stroke: tone.color,
          strokeOpacity,
          strokeWidth,
          strokeDasharray: isMuted ? '10 12' : undefined,
          transition: isInteractive
            ? 'none'
            : 'stroke-width 120ms ease, stroke-opacity 120ms ease, stroke 120ms ease',
        }}
      />

      {/* biome-ignore lint/a11y/noStaticElementInteractions: This transparent SVG path is a pointer-only edge hit target, not a keyboard command. */}
      <path
        ref={edgeHitTargetRef}
        d={edgePath}
        fill="none"
        stroke="transparent"
        strokeWidth={18}
        tabIndex={-1}
        aria-label={`Link ${id}`}
        className="cursor-pointer"
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
        onClick={(event) => {
          if (!suppressNextClickRef.current) {
            return;
          }
          suppressNextClickRef.current = false;
          event.preventDefault();
          event.stopPropagation();
        }}
        onDoubleClick={handleEdgeDoubleClick}
        onPointerDown={handleEdgePointerDown}
        onPointerMove={handleEdgePointerMove}
        onPointerLeave={handleEdgePointerLeave}
        onPointerUp={(event) => finishPointerGesture(event, true)}
        onPointerCancel={abandonPointerGesture}
        onLostPointerCapture={abandonPointerGesture}
        onContextMenu={(event) => {
          if (!data?.onContextMenu) {
            return;
          }

          event.preventDefault();
          event.stopPropagation();
          data.onContextMenu(event, id);
        }}
      />

      <LinkRouteControls
        edgeId={id}
        route={renderedRoute}
        selected={selected === true}
        editable={routeEditable}
        selectedWaypointIndex={selectedWaypointIndex}
        onWaypointFocus={setSelectedWaypointIndex}
        onWaypointPointerDown={handleWaypointPointerDown}
        onWaypointPointerMove={handleWaypointPointerMove}
        onWaypointPointerUp={(event) => finishPointerGesture(event, true)}
        onWaypointPointerCancel={(event) => abandonPointerGesture(event)}
        onWaypointLostPointerCapture={(event) => abandonPointerGesture(event)}
        onWaypointNudge={handleWaypointNudge}
        onWaypointRemove={handleWaypointRemove}
      />
    </>
  );
}

const LinkEdge = memo(LinkEdgeInner, (prev, next) => {
  return (
    prev.id === next.id &&
    prev.selected === next.selected &&
    prev.data?.inertVirtualLink === next.data?.inertVirtualLink &&
    prev.data?.utilization === next.data?.utilization &&
    prev.data?.alertStatus === next.data?.alertStatus &&
    prev.data?.bandwidthLabel === next.data?.bandwidthLabel &&
    prev.data?.speedLabel === next.data?.speedLabel &&
    prev.data?.throughputLabel === next.data?.throughputLabel &&
    prev.data?.negotiationTitle === next.data?.negotiationTitle &&
    prev.data?.autonegTitle === next.data?.autonegTitle &&
    prev.data?.speedMismatch === next.data?.speedMismatch &&
    prev.data?.negotiationState === next.data?.negotiationState &&
    prev.data?.sourceIfStatus === next.data?.sourceIfStatus &&
    prev.data?.targetIfStatus === next.data?.targetIfStatus &&
    prev.data?.sourceDeviceStatus === next.data?.sourceDeviceStatus &&
    prev.data?.targetDeviceStatus === next.data?.targetDeviceStatus &&
    prev.data?.sourceDeviceAlertStatus === next.data?.sourceDeviceAlertStatus &&
    prev.data?.targetDeviceAlertStatus === next.data?.targetDeviceAlertStatus &&
    prev.data?.sourceDeviceHealth === next.data?.sourceDeviceHealth &&
    prev.data?.targetDeviceHealth === next.data?.targetDeviceHealth &&
    prev.data?.sourceDevicePrimaryHealth === next.data?.sourceDevicePrimaryHealth &&
    prev.data?.targetDevicePrimaryHealth === next.data?.targetDevicePrimaryHealth &&
    prev.data?.sourceDeviceReachability === next.data?.sourceDeviceReachability &&
    prev.data?.targetDeviceReachability === next.data?.targetDeviceReachability &&
    prev.data?.sourceDeviceNetworkReachable === next.data?.sourceDeviceNetworkReachable &&
    prev.data?.targetDeviceNetworkReachable === next.data?.targetDeviceNetworkReachable &&
    prev.data?.sourceDeviceSnmpReachable === next.data?.sourceDeviceSnmpReachable &&
    prev.data?.targetDeviceSnmpReachable === next.data?.targetDeviceSnmpReachable &&
    prev.data?.interactionMode === next.data?.interactionMode &&
    prev.data?.areaColor === next.data?.areaColor &&
    prev.data?.emphasis === next.data?.emphasis &&
    prev.data?.parallelIndex === next.data?.parallelIndex &&
    prev.data?.route === next.data?.route &&
    prev.data?.routeEditable === next.data?.routeEditable &&
    prev.data?.routeEditToken === next.data?.routeEditToken &&
    prev.data?.onRouteCommit === next.data?.onRouteCommit &&
    prev.source === next.source &&
    prev.target === next.target &&
    prev.sourceX === next.sourceX &&
    prev.sourceY === next.sourceY &&
    prev.targetX === next.targetX &&
    prev.targetY === next.targetY &&
    prev.sourcePosition === next.sourcePosition &&
    prev.targetPosition === next.targetPosition
  );
});

export default LinkEdge;
