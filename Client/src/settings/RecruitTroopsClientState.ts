import {
  createQueueProductionClientState,
  type QueueProductionCastleSettings,
  type QueueProductionClientSettingsV1,
  type QueueProductionItem,
  type QueueProductionMode,
} from './QueueProductionClientState';

export type RecruitTroopsMode = QueueProductionMode;
export type RecruitTroopsItem = QueueProductionItem;
export type RecruitTroopsCastleSettings = QueueProductionCastleSettings;
export type RecruitTroopsClientSettingsV1 = QueueProductionClientSettingsV1;
export const RECRUIT_CHECK_INTERVAL_SEC_PER_MIN = 60;

const recruitTroopsState = createQueueProductionClientState({
  configurationSection: 'automation.recruitTroops',
  schedulePrefix: 'autoRecruit',
  supportsGloryTitleFallback: true,
});

export const DEFAULT_RECRUIT_CHECK_INTERVAL_SEC = recruitTroopsState.defaultCheckIntervalSec;
export const MIN_RECRUIT_CHECK_INTERVAL_SEC = recruitTroopsState.minCheckIntervalSec;
export const MAX_RECRUIT_CHECK_INTERVAL_SEC = recruitTroopsState.maxCheckIntervalSec;
export const MIN_RECRUIT_CHECK_INTERVAL_MIN = recruitTroopsState.minCheckIntervalMin;
export const MAX_RECRUIT_CHECK_INTERVAL_MIN = recruitTroopsState.maxCheckIntervalMin;
export const DEFAULT_RECRUIT_CHECK_INTERVAL_MIN = recruitTroopsState.defaultCheckIntervalMin;
export const recruitCastleScheduleID = recruitTroopsState.castleScheduleID;
export const clampRecruitCheckIntervalSec = recruitTroopsState.clampCheckIntervalSec;
export const recruitCheckIntervalSecToMinutes = recruitTroopsState.checkIntervalSecToMinutes;
export const recruitCheckIntervalMinutesToSec = recruitTroopsState.checkIntervalMinutesToSec;
export const defaultRecruitTroopsSettings = recruitTroopsState.defaultSettings;
export const normalizeRecruitTroopsSettings = recruitTroopsState.normalizeSettings;
export const persistRecruitTroopsSettings = recruitTroopsState.persistSettings;
