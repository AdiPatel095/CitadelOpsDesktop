import {
  createQueueProductionClientState,
  type QueueProductionCastleSettings,
  type QueueProductionClientSettingsV1,
  type QueueProductionItem,
  type QueueProductionMode,
} from './QueueProductionClientState';

export type AutoToolMode = QueueProductionMode;
export type AutoToolItem = QueueProductionItem;
export type AutoToolCastleSettings = QueueProductionCastleSettings;
export type AutoToolClientSettingsV1 = QueueProductionClientSettingsV1;
export const AUTO_TOOL_CHECK_INTERVAL_SEC_PER_MIN = 60;

const autoToolState = createQueueProductionClientState({
  configurationSection: 'automation.autoTool',
  schedulePrefix: 'autoTool',
});

export const DEFAULT_AUTO_TOOL_CHECK_INTERVAL_SEC = autoToolState.defaultCheckIntervalSec;
export const MIN_AUTO_TOOL_CHECK_INTERVAL_SEC = autoToolState.minCheckIntervalSec;
export const MAX_AUTO_TOOL_CHECK_INTERVAL_SEC = autoToolState.maxCheckIntervalSec;
export const MIN_AUTO_TOOL_CHECK_INTERVAL_MIN = autoToolState.minCheckIntervalMin;
export const MAX_AUTO_TOOL_CHECK_INTERVAL_MIN = autoToolState.maxCheckIntervalMin;
export const DEFAULT_AUTO_TOOL_CHECK_INTERVAL_MIN = autoToolState.defaultCheckIntervalMin;
export const autoToolCastleScheduleID = autoToolState.castleScheduleID;
export const clampAutoToolCheckIntervalSec = autoToolState.clampCheckIntervalSec;
export const autoToolCheckIntervalSecToMinutes = autoToolState.checkIntervalSecToMinutes;
export const autoToolCheckIntervalMinutesToSec = autoToolState.checkIntervalMinutesToSec;
export const defaultAutoToolSettings = autoToolState.defaultSettings;
export const normalizeAutoToolSettings = autoToolState.normalizeSettings;
export const persistAutoToolSettings = autoToolState.persistSettings;
