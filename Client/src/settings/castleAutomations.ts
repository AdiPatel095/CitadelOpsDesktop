import { castleSettingsKey, castleSettingsEntry } from './stormRole';
import { parseAutoAdvisorClientState } from './AutoAdvisorClientState';
import { parseAutoBeriWorldSettings } from './AutoBeriWorldClientState';
import { parseAutoBirdClientState } from './AutoBirdClientState';
import { parseAutoBuyerClientState } from './AutoBuyerClientState';
import { parseAutoInvasionClientState } from './AutoInvasionClientState';
import { parseAutoKhanClientState } from './AutoKhanClientState';
import { parseAutoNomadClientState } from './AutoNomadClientState';
import { normalizeAutoSceatResSettings } from './AutoSceatResClientState';
import { parseAutoStormClientState } from './AutoStormClientState';
import { parseAutoStationClientState } from './AutoStationClientState';
import { normalizeAutoToolSettings } from './AutoToolClientState';
import { parseAutoTowerClientState } from './AutoTowerClientState';
import { normalizeRecruitTroopsSettings } from './RecruitTroopsClientState';
import { birdCopyDescriptor } from './copy/features/bird';
import { stationCopyDescriptor } from './copy/features/station';
import type { SettingsFeatureId } from './disclosure/placement';
import { GOAL_SAVED_SECTION } from './onboarding/goals';

const FEATURE_ORDER: readonly SettingsFeatureId[] = [
  'autoTowers', 'autoRecruit', 'autoTool', 'autoSceatRes', 'autoStation', 'autoBird',
  'autoKhan', 'autoNomad', 'autoInvasion', 'autoAdvisor', 'autoBeriWorld', 'autoBuyer', 'autoStorm',
];

/** Saved castle references only: this does not infer per-castle runtime activity or feature status. */
export function automationsActingOnCastle(
  sections: Readonly<Record<string, unknown>>,
  castleId: string | number,
  kingdomId = 0,
): SettingsFeatureId[] {
  const id = Number(castleId);
  if (!Number.isSafeInteger(id) || id <= 0) return [];
  const key = String(id);
  const castle = { id, kingdomId };
  return FEATURE_ORDER.filter((feature) => {
    const section = sections[GOAL_SAVED_SECTION[feature]];
    if (section === null || typeof section !== 'object' || Array.isArray(section)) return false;
    switch (feature) {
      case 'autoTowers': return castleSettingsEntry(parseAutoTowerClientState(section).castles, castle)?.enabled === true;
      case 'autoRecruit': return (normalizeRecruitTroopsSettings(section).castles[key]?.items.length ?? 0) > 0;
      case 'autoTool': return (normalizeAutoToolSettings(section).castles[key]?.items.length ?? 0) > 0;
      case 'autoSceatRes':
        return Object.values(normalizeAutoSceatResSettings(section).castles[key]?.buildings ?? {})
          .some((building) => building.steps.length > 0);
      case 'autoStation': {
        const saved = parseAutoStationClientState(section);
        const row = castleSettingsEntry(saved.settings, castle, saved.stormLegacyKey);
        return row !== undefined && stationCopyDescriptor.isConfigured(row);
      }
      case 'autoBird': {
        const saved = parseAutoBirdClientState(section);
        const active = saved.presets.presets.find((preset) => preset.id === saved.activePresetId);
        const row = castleSettingsEntry(active?.settings ?? saved.ignoreSettings.settings, castle, saved.stormLegacyKey);
        return Array.isArray(row) && birdCopyDescriptor.isConfigured(row);
      }
      case 'autoKhan': return parseAutoKhanClientState(section).sourceCastleId === id;
      case 'autoNomad': return parseAutoNomadClientState(section).sourceCastleId === id;
      case 'autoInvasion': return parseAutoInvasionClientState(section).sourceCastleId === id;
      case 'autoAdvisor': return parseAutoAdvisorClientState(section).sourceCastleId === id;
      case 'autoBeriWorld': return parseAutoBeriWorldSettings(section).sourceCastleId === id;
      case 'autoBuyer': {
        const saved = parseAutoBuyerClientState(section);
        return saved.sourceCastleId === id || saved.feast.sourceCastleId === id;
      }
      case 'autoStorm': {
        const saved = parseAutoStormClientState(section);
        return castleSettingsKey(castle) === 'storm' && (saved.unlock.enabled || saved.decorationPresetId !== '');
      }
      default: return false;
    }
  });
}
