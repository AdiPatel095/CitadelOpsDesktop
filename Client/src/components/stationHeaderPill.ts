export interface StationHeaderPillInput {
  enabled: boolean;
  status: string;
  threatCount: number;
  nextImpact: number;
  now: number;
  stationName: string;
  blockedLabel: string;
}

export interface StationHeaderPill {
  tone: 'off' | 'on' | 'warning' | 'error';
  text: string;
}

function formatStationImpact(msLeft: number): string {
  if (msLeft <= 0) return 'now';
  const seconds = Math.ceil(msLeft / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainder = seconds % 60;
  return remainder > 0 ? `${minutes}m ${remainder}s` : `${minutes}m`;
}

/** Keep the desktop and portal Station warning presentation identical. */
export function stationHeaderPill(input: StationHeaderPillInput): StationHeaderPill {
  const { enabled, status, threatCount, nextImpact, now, stationName, blockedLabel } = input;
  if (!enabled) return { tone: 'off', text: 'Auto Station off' };
  const impact = nextImpact > 0 ? formatStationImpact(nextImpact - now) : '';
  switch (status) {
    case 'blocked':
      if (threatCount <= 0 && nextImpact <= 0) {
        return { tone: 'warning', text: `${stationName} · ${blockedLabel}` };
      }
      // A blocked castle retains the incoming warning instead of looking healthy.
      return { tone: 'warning', text: `${threatCount} incoming · ${impact || 'checking'}` };
    case 'threat':
      return { tone: 'warning', text: `${threatCount} incoming · ${impact || 'checking'}` };
    case 'evacuating':
      return { tone: 'warning', text: 'Auto Station evacuating…' };
    case 'protected':
      return {
        tone: 'on',
        text: threatCount > 0 ? `${threatCount} incoming protected` : 'Troops protected',
      };
    case 'recalling':
      return { tone: 'on', text: 'Auto Station recalling…' };
    case 'waiting':
      return { tone: 'warning', text: 'Auto Station waiting' };
    case 'error':
      return { tone: 'error', text: 'Auto Station error' };
    default:
      return { tone: 'on', text: 'Auto Station armed' };
  }
}
