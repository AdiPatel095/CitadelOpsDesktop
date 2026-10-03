import React from 'react';

/**
 * The surface that shows the connection repair. Desktop uses the default dialog; the hosted portal registers its own
 * panel before the client renders (same pattern as the hosted header chrome), because hosted repair speaks about the
 * account and its hosted runtime while desktop speaks about the local game connection.
 */
export type RepairSurfaceComponent = React.ComponentType<{ onClose: () => void; onOpenSettings: () => void }>;

let registered: RepairSurfaceComponent | null = null;

export const setConnectionRepairSurface = (surface: RepairSurfaceComponent | null): void => { registered = surface; };
export const getConnectionRepairSurface = (): RepairSurfaceComponent | null => registered;

/** The registered surface, or the given default, as an element (an element factory keeps the choice out of render-time component creation). */
export function connectionRepairElement(fallback: RepairSurfaceComponent, props: React.ComponentProps<RepairSurfaceComponent>): React.ReactElement {
  return React.createElement(registered ?? fallback, props);
}
