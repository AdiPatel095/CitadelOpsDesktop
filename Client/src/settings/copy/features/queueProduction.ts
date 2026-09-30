import type { MessageKey } from '../../../i18n/messages';
import type { QueueProductionCastleSettings, QueueProductionItem } from '../../QueueProductionClientState';
import type { QueueProductionClientSettingsV1 } from '../../QueueProductionClientState';
import type { CastleCopyContext, CastleCopyDescriptor, CastleCopyField } from '../castleCopy';
import { toolName, troopName } from './unitChecks';

const message = (key: MessageKey): MessageKey => key;

/** Recruit Troops and Auto Tool keep their plan in the same `mode/castles` shape (`QueueProductionClientState`). */
export type QueueCopyDraft = QueueProductionClientSettingsV1;

const defaultRecord = (): QueueProductionCastleSettings => ({ enabled: false, items: [], cursor: 0 });

const normalizedItems = (value: unknown): QueueProductionItem[] => (Array.isArray(value) ? value as QueueProductionItem[] : []);
const sameItems = (left: unknown, right: unknown) => JSON.stringify(normalizedItems(left).map((item) => [item.id, item.amount ?? 0, item.minId ?? 0, item.maxId ?? 0]))
  === JSON.stringify(normalizedItems(right).map((item) => [item.id, item.amount ?? 0, item.minId ?? 0, item.maxId ?? 0]));

function itemName(kind: 'recruit' | 'tool', context: CastleCopyContext, id: number): string {
  return kind === 'recruit' ? troopName(context, id) : toolName(context, id);
}

function createQueueFields(kind: 'recruit' | 'tool'): CastleCopyField<QueueProductionCastleSettings>[] {
  return [
    {
      id: 'items', labelKey: message(kind === 'recruit' ? 'castleCopy.field.recruitItems' : 'castleCopy.field.toolItems'),
      noteKey: message('castleCopy.note.rotationRestarts'),
      read: (record) => record.items,
      write: (record, value, destination) => {
        const items = normalizedItems(value).map((item) => ({ ...item }));
        // The rotation restarts when the list changes; it is progress, never setup, so it is never copied.
        return { ...record, items, kingdomId: destination.kingdomId, cursor: sameItems(record.items, items) ? record.cursor : 0 };
      },
      equal: sameItems,
      describe: (value, context) => {
        const items = normalizedItems(value);
        return {
          messageKey: message('castleCopy.value.items'),
          params: { count: items.length, list: items.map((item) => `${itemName(kind, context, item.id)}${item.amount ? ` ×${item.amount.toLocaleString()}` : ''}`).join(', ') },
        };
      },
      entryId: (entry) => (entry as QueueProductionItem).id,
    },
    {
      id: 'enabled', labelKey: message('castleCopy.field.includedQueue'), consequential: true,
      read: (record) => record.enabled,
      write: (record, value, destination) => ({ ...record, enabled: value === true, kingdomId: destination.kingdomId }),
      describe: (value) => ({ messageKey: message('castleCopy.value.included'), params: { on: value === true ? 'on' : 'off' } }),
    },
  ];
}

function createQueueDescriptor(kind: 'recruit' | 'tool'): CastleCopyDescriptor<QueueCopyDraft, QueueProductionCastleSettings> {
  return {
    featureId: kind === 'recruit' ? 'autoRecruit' : 'autoTool',
    sectionId: 'plan',
    primaryFieldId: 'items',
    fields: createQueueFields(kind),
    notCopiedKey: message('castleCopy.notCopied.queue'),
    enabledIncludeKey: message('castleCopy.include.queue'),
    defaultRecord,
    recordFor: (draft, key) => draft.castles[key],
    isConfigured: (record) => record.enabled || record.items.length > 0,
    validate: (source, destination, context) => {
      if (context.usesScheduledItems?.(destination)) {
        return [{ id: 'scheduled', state: 'incompatible', messageKey: message('castleCopy.reason.scheduled'), params: { castle: destination.name } }];
      }
      if (source.items.length === 0) return [];
      const allowed = context.allowedItemIds?.(destination);
      if (!allowed) {
        return [{ id: 'catalog', state: 'unknown', messageKey: message('castleCopy.reason.catalogLoading'), params: { castle: destination.name } }];
      }
      return source.items.filter((item) => !allowed.includes(item.id)).map((item) => ({
        id: `not-offered:${item.id}`, state: 'incompatible' as const, messageKey: message('castleCopy.reason.notOffered'),
        params: { item: itemName(kind, context, item.id), castle: destination.name }, fieldId: 'items', dropEntryIds: [item.id],
      }));
    },
    apply: (draft, key, next) => ({ ...draft, castles: { ...draft.castles, [key]: next } }),
  };
}

export const recruitCopyDescriptor = createQueueDescriptor('recruit');
export const toolCopyDescriptor = createQueueDescriptor('tool');
