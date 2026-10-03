import React from 'react';
import { closeConnectionRepair, useConnectionRepairOpen } from '../settings/connection/repairRequest';
import { connectionRepairElement } from '../settings/connection/repairSurface';
import { ConnectionRepairDialog } from './ConnectionRepairDialog';

/**
 * Mounted once in the application shell. Shows the connection repair over whatever is open (an editor keeps its
 * draft) and closes back to it. The hosted portal registers its own surface; desktop uses the dialog.
 */
export const ConnectionRepairHost: React.FC<{ onOpenSettings: () => void }> = ({ onOpenSettings }) => {
  const open = useConnectionRepairOpen();
  if (!open) return null;
  return connectionRepairElement(ConnectionRepairDialog, { onClose: closeConnectionRepair, onOpenSettings: () => { closeConnectionRepair(); onOpenSettings(); } });
};
