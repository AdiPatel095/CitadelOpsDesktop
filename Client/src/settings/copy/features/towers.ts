import type { MessageKey } from '../../../i18n/messages';
import { defaultAutoTowerCastleSettings, type AutoTowerCastleSettings } from '../../AutoTowerClientState';
import type { CastleCopyDescriptor, CastleCopyField } from '../castleCopy';
import { troopEntryReasons, troopName } from './unitChecks';

const message = (key: MessageKey): MessageKey => key;

export type TowersCopyDraft = Record<string, AutoTowerCastleSettings>;

const fields: CastleCopyField<AutoTowerCastleSettings>[] = [
  {
    id: 'unitId', labelKey: message('castleCopy.field.towerTroop'),
    read: (record) => record.unitId,
    write: (record, value) => ({ ...record, unitId: Number(value) || 0 }),
    describe: (value, context) => (Number(value) > 0
      ? { messageKey: message('castleCopy.value.unit'), params: { unit: troopName(context, Number(value)) } }
      : { messageKey: message('castleCopy.value.none') }),
  },
  {
    id: 'radius', labelKey: message('castleCopy.field.radius'),
    read: (record) => record.radius,
    write: (record, value) => ({ ...record, radius: Number(value) || record.radius }),
    describe: (value) => ({ messageKey: message('castleCopy.value.radius'), params: { count: Number(value) || 0 } }),
  },
  {
    id: 'maidenOnly', labelKey: message('castleCopy.field.maidenOnly'),
    read: (record) => record.maidenOnly,
    write: (record, value) => ({ ...record, maidenOnly: value === true }),
    describe: (value) => ({ messageKey: message('castleCopy.value.onOff'), params: { on: value === true ? 'on' : 'off' } }),
  },
  {
    id: 'enabled', labelKey: message('castleCopy.field.included'), consequential: true,
    read: (record) => record.enabled,
    write: (record, value) => ({ ...record, enabled: value === true }),
    describe: (value) => ({ messageKey: message('castleCopy.value.included'), params: { on: value === true ? 'on' : 'off' } }),
  },
];

/**
 * Auto Towers per-castle setup (`automation.autoTowers` castles[id]). Copies the tower troop, radius and the
 * maiden-supported preference. Whether the castle takes part (`enabled`) is consequential and only copied by an
 * explicit include. Check timing, map scan, Advisor, travel and the daily limit are account-wide.
 */
export const towersCopyDescriptor: CastleCopyDescriptor<TowersCopyDraft, AutoTowerCastleSettings> = {
  featureId: 'autoTowers',
  sectionId: 'castles',
  primaryFieldId: 'unitId',
  fields,
  notCopiedKey: message('castleCopy.notCopied.towers'),
  enabledIncludeKey: message('castleCopy.include.feature'),
  defaultRecord: () => defaultAutoTowerCastleSettings(),
  recordFor: (draft, key) => draft[key],
  isConfigured: (record) => {
    const defaults = defaultAutoTowerCastleSettings();
    return record.enabled || record.unitId > 0 || record.radius !== defaults.radius || record.maidenOnly;
  },
  validate: (source, destination, context) => (
    source.unitId > 0 ? troopEntryReasons('unitId', [{ id: source.unitId, amount: 1 }], destination, context).map((reason) => (
      // One tower troop: a troop the game does not know makes the whole destination incompatible, not a partial copy.
      reason.dropEntryIds ? { ...reason, dropEntryIds: undefined, fieldId: undefined } : reason
    )) : []
  ),
  apply: (draft, key, next) => ({ ...draft, [key]: next }),
};
