import { FrontendWebsocket } from '../Websocket';

export const AUTO_HOSPITAL_SETTINGS_STORAGE_KEY = 'autoHospitalSettings';
export const AUTO_HOSPITAL_SETTINGS_CHANGED_EVENT = 'autoHospitalSettingsChanged';
export const DEFAULT_AUTO_HOSPITAL_CHECK_INTERVAL_SEC = 300;
export const MIN_AUTO_HOSPITAL_CHECK_INTERVAL_SEC = 30;
export const MAX_AUTO_HOSPITAL_CHECK_INTERVAL_SEC = 86400;
export const AUTO_HOSPITAL_CHECK_INTERVAL_SEC_PER_MIN = 60;
export const MIN_AUTO_HOSPITAL_CHECK_INTERVAL_MIN = Math.ceil(
  MIN_AUTO_HOSPITAL_CHECK_INTERVAL_SEC / AUTO_HOSPITAL_CHECK_INTERVAL_SEC_PER_MIN,
);
export const MAX_AUTO_HOSPITAL_CHECK_INTERVAL_MIN = Math.floor(
  MAX_AUTO_HOSPITAL_CHECK_INTERVAL_SEC / AUTO_HOSPITAL_CHECK_INTERVAL_SEC_PER_MIN,
);
export const DEFAULT_AUTO_HOSPITAL_CHECK_INTERVAL_MIN = Math.round(
  DEFAULT_AUTO_HOSPITAL_CHECK_INTERVAL_SEC / AUTO_HOSPITAL_CHECK_INTERVAL_SEC_PER_MIN,
);

export interface AutoHospitalClientSettingsV1 {
  version: 1;
  checkIntervalSec: number;
}

function defaultAutoHospitalSettings(): AutoHospitalClientSettingsV1 {
  return {
    version: 1,
    checkIntervalSec: DEFAULT_AUTO_HOSPITAL_CHECK_INTERVAL_SEC,
  };
}

export function clampAutoHospitalCheckIntervalSec(value: number): number {
  if (!Number.isFinite(value)) return DEFAULT_AUTO_HOSPITAL_CHECK_INTERVAL_SEC;
  return Math.min(
    MAX_AUTO_HOSPITAL_CHECK_INTERVAL_SEC,
    Math.max(MIN_AUTO_HOSPITAL_CHECK_INTERVAL_SEC, Math.round(value)),
  );
}

export function autoHospitalCheckIntervalSecToMinutes(value: number): number {
  return Math.min(
    MAX_AUTO_HOSPITAL_CHECK_INTERVAL_MIN,
    Math.max(
      MIN_AUTO_HOSPITAL_CHECK_INTERVAL_MIN,
      Math.round(clampAutoHospitalCheckIntervalSec(value) / AUTO_HOSPITAL_CHECK_INTERVAL_SEC_PER_MIN),
    ),
  );
}

export function autoHospitalCheckIntervalMinutesToSec(value: number): number {
  if (!Number.isFinite(value)) return DEFAULT_AUTO_HOSPITAL_CHECK_INTERVAL_SEC;
  return clampAutoHospitalCheckIntervalSec(Math.round(value) * AUTO_HOSPITAL_CHECK_INTERVAL_SEC_PER_MIN);
}

export function normalizeAutoHospitalSettings(raw: unknown): AutoHospitalClientSettingsV1 {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
    return defaultAutoHospitalSettings();
  }
  const payload = raw as Record<string, unknown>;
  return {
    version: 1,
    checkIntervalSec: clampAutoHospitalCheckIntervalSec(Number(payload.checkIntervalSec)),
  };
}

export function loadAutoHospitalSettingsFromStorage(): AutoHospitalClientSettingsV1 {
  try {
    const raw = localStorage.getItem(AUTO_HOSPITAL_SETTINGS_STORAGE_KEY);
    if (!raw) return defaultAutoHospitalSettings();
    return normalizeAutoHospitalSettings(JSON.parse(raw));
  } catch {
    return defaultAutoHospitalSettings();
  }
}

export function notifyAutoHospitalSettingsChanged(settings: AutoHospitalClientSettingsV1): void {
  window.dispatchEvent(new CustomEvent(AUTO_HOSPITAL_SETTINGS_CHANGED_EVENT, { detail: settings }));
}

export function applyAutoHospitalSettingsToLocalStorage(settings: AutoHospitalClientSettingsV1): void {
  const normalized = normalizeAutoHospitalSettings(settings);
  localStorage.setItem(AUTO_HOSPITAL_SETTINGS_STORAGE_KEY, JSON.stringify(normalized));
  notifyAutoHospitalSettingsChanged(normalized);
}

export function persistAutoHospitalSettings(settings: AutoHospitalClientSettingsV1): boolean {
  const normalized = normalizeAutoHospitalSettings(settings);
  applyAutoHospitalSettingsToLocalStorage(normalized);
  return FrontendWebsocket.sendMessage({ type: 'saveAutoHospitalSettings', payload: normalized });
}
