import type { CastleStateV2, GameStateV2, MapObservationV2 } from '../../api/Contracts';

export interface RiftMapCoords {
  castleAid: number;
  kingdomID: number;
  centerX: number;
  centerY: number;
  riftKingdomID: number;
  found: boolean;
  rift: MapObservationV2 | null;
  deltaX: number;
  deltaY: number;
  distance: number;
}

export const GAA_NODE_RIFT_TYPE = 43;

export function riftMapCoordsFromState(
  state: GameStateV2 | null,
  castle: CastleStateV2 | null,
): RiftMapCoords | null {
  if (!state) return null;
  const located = Object.entries(state.map)
    .flatMap(([kingdomID, nodes]) => Object.values(nodes).map((node) => ({ kingdomID: Number(kingdomID), node })))
    .find(({ node }) => node.typeId === GAA_NODE_RIFT_TYPE);
  const centerX = castle?.x ?? 0;
  const centerY = castle?.y ?? 0;
  const deltaX = located ? located.node.x - centerX : 0;
  const deltaY = located ? located.node.y - centerY : 0;
  return {
    castleAid: castle?.id ?? 0,
    kingdomID: castle?.kingdomId ?? 0,
    centerX,
    centerY,
    riftKingdomID: located?.kingdomID ?? 0,
    found: located != null,
    rift: located?.node ?? null,
    deltaX,
    deltaY,
    distance: located && (centerX !== 0 || centerY !== 0)
      ? Math.max(Math.abs(deltaX), Math.abs(deltaY))
      : 0,
  };
}

function formatSigned(value: number): string {
  return value >= 0 ? `+${value}` : String(value);
}

export function formatRiftDelta(deltaX: number, deltaY: number): string {
  return `${formatSigned(deltaX)}, ${formatSigned(deltaY)}`;
}
