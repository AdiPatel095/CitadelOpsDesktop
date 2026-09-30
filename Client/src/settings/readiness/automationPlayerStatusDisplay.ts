import type { AutomationStateV2 } from '../../api/Contracts';
import type { AutomationEnabledControl } from '../AutomationEnabled';
import { describeMessage } from '../../i18n/messages';
import { formatMessage } from '../../i18n/formatMessage';
import { featureStateLaneIds, type AutomationStateContext } from './runtimeState';
import { automationPlayerStatus } from './playerStatus';
import { mostSeverePlayerStatus } from '../../components/playerStatusDisplay';

/** Read-only card adapter. Runtime lane order comes from CIT-20, not UI labels. */
export function describeFeaturePlayerStatus(input: {
  featureId: string;
  states: Record<string, AutomationStateV2 | undefined>;
  control: AutomationEnabledControl;
  context: AutomationStateContext;
  desktopLocked?: boolean;
  buildLaneActive?: boolean;
}) {
  const { featureId, states, control, context, desktopLocked, buildLaneActive } = input;
  const lanes = featureStateLaneIds(featureId).map(id => {
    const active = id !== 'autoBeriWorldBuild' || buildLaneActive !== false;
    return { id, active, value: automationPlayerStatus({ featureId, runtime: states[id], enabled: active ? control : { configured: false, enabled: false }, context, desktopLocked }) };
  });
  const builder = states.autoStormBuild;
  const missing = builder?.metrics?.stormMissingDecorations;
  if (featureId === 'autoStorm' && control.enabled && builder && typeof missing === 'number' && Number.isFinite(missing) && missing > 0) {
    const descriptor = describeMessage('automation.missingDecorations', { count: missing });
    const detail = formatMessage(descriptor, 'en', {}).text;
    lanes.push({ id: 'builder-missing-decorations', active: true, value: automationPlayerStatus({ featureId, runtime: { ...builder, status: 'warning', detail, detailDescriptor: { ...descriptor, fallbackText: detail } }, laneId: 'builder-missing-decorations', enabled: control, context, desktopLocked }) });
  }
  const fallback = automationPlayerStatus({ featureId, enabled: control, context, desktopLocked });
  return { lanes, overall: mostSeverePlayerStatus(lanes.filter(lane => lane.active).map(lane => lane.value), fallback) };
}
