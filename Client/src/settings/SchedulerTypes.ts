export const DAY_MINUTES = 24 * 60;
export const SCHEDULE_STEP_MINUTES = 15;
export const MIN_SLOT_MINUTES = 15;

export const WEEK_DAYS = [
  { value: 0, short: 'Sun', label: 'Sunday' },
  { value: 1, short: 'Mon', label: 'Monday' },
  { value: 2, short: 'Tue', label: 'Tuesday' },
  { value: 3, short: 'Wed', label: 'Wednesday' },
  { value: 4, short: 'Thu', label: 'Thursday' },
  { value: 5, short: 'Fri', label: 'Friday' },
  { value: 6, short: 'Sat', label: 'Saturday' },
] as const;

export interface WeeklyScheduleSlot {
  id: string;
  day: number;
  startMinute: number;
  endMinute: number;
  options?: Record<string, unknown>;
}

export interface WeeklySchedule {
  enabled: boolean;
  timeZone?: string;
  slotOptionsEnabled?: boolean;
  slots: WeeklyScheduleSlot[];
}

export type FeatureSchedules = Record<string, WeeklySchedule>;

export function createEmptyWeeklySchedule(): WeeklySchedule {
  return {
    enabled: false,
    timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    slots: [],
  };
}

export function createScheduleSlot(
  day: number,
  startMinute: number,
  endMinute: number,
  options?: Record<string, unknown>,
): WeeklyScheduleSlot {
  const slot: WeeklyScheduleSlot = {
    id: crypto.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(16).slice(2)}`,
    day,
    startMinute,
    endMinute,
  };
  const normalizedOptions = normalizeSlotOptions(options);
  if (normalizedOptions) {
    slot.options = normalizedOptions;
  }
  return slot;
}

export function clampMinute(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.min(DAY_MINUTES, Math.max(0, Math.round(value)));
}

export function snapMinute(value: number): number {
  return clampMinute(Math.round(value / SCHEDULE_STEP_MINUTES) * SCHEDULE_STEP_MINUTES);
}

export function clampSlotStart(value: number): number {
  return Math.min(DAY_MINUTES - MIN_SLOT_MINUTES, Math.max(0, snapMinute(value)));
}

export function clampSlotEnd(value: number): number {
  return Math.max(MIN_SLOT_MINUTES, Math.min(DAY_MINUTES, snapMinute(value)));
}

function normalizeSlot(raw: unknown): WeeklyScheduleSlot | null {
  if (!raw || typeof raw !== 'object') return null;
  const slot = raw as Partial<WeeklyScheduleSlot>;
  const day = Number(slot.day);
  const startMinute = clampSlotStart(Number(slot.startMinute));
  const endMinute = clampSlotEnd(Number(slot.endMinute));
  if (!Number.isFinite(day) || endMinute - startMinute < MIN_SLOT_MINUTES) return null;
  const normalized: WeeklyScheduleSlot = {
    id: typeof slot.id === 'string' && slot.id ? slot.id : createScheduleSlot(day, startMinute, endMinute).id,
    day: Math.min(6, Math.max(0, Math.floor(day))),
    startMinute,
    endMinute,
  };
  const options = normalizeSlotOptions(slot.options);
  if (options) {
    normalized.options = options;
  }
  return normalized;
}

function normalizeSlotOptions(raw: unknown): Record<string, unknown> | undefined {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return undefined;

  const options: Record<string, unknown> = {};
  Object.entries(raw as Record<string, unknown>).forEach(([key, value]) => {
    if (!key) return;
    if (
      value == null ||
      typeof value === 'string' ||
      typeof value === 'number' ||
      typeof value === 'boolean'
    ) {
      options[key] = value;
    }
  });

  return Object.keys(options).length > 0 ? options : undefined;
}

export function normalizeWeeklySchedule(raw: unknown): WeeklySchedule {
  if (!raw || typeof raw !== 'object') return createEmptyWeeklySchedule();
  const payload = raw as Partial<WeeklySchedule>;
  const slots = Array.isArray(payload.slots)
    ? payload.slots.map(normalizeSlot).filter((slot): slot is WeeklyScheduleSlot => slot != null)
    : [];

  slots.sort((a, b) => {
    if (a.day !== b.day) return a.day - b.day;
    if (a.startMinute !== b.startMinute) return a.startMinute - b.startMinute;
    return a.endMinute - b.endMinute;
  });

  return {
    enabled: !!payload.enabled,
    timeZone:
      typeof payload.timeZone === 'string' && payload.timeZone
        ? payload.timeZone
        : Intl.DateTimeFormat().resolvedOptions().timeZone,
    slotOptionsEnabled: !!payload.slotOptionsEnabled,
    slots,
  };
}

export function normalizeFeatureSchedules(raw: unknown): FeatureSchedules {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return {};
  const schedules: FeatureSchedules = {};
  Object.entries(raw as Record<string, unknown>).forEach(([featureID, schedule]) => {
    if (!featureID) return;
    schedules[featureID] = normalizeWeeklySchedule(schedule);
  });
  return schedules;
}

export function parseMinuteOfDay(value: string): number | null {
  const trimmed = value.trim();
  if (trimmed === '24:00') return DAY_MINUTES;
  const match = /^([01]\d|2[0-3]):([0-5]\d)$/.exec(trimmed);
  if (!match) return null;
  return Number(match[1]) * 60 + Number(match[2]);
}

export function formatMinuteOfDay(minute: number): string {
  const clamped = clampMinute(minute);
  if (clamped >= DAY_MINUTES) return '24:00';
  const hours = Math.floor(clamped / 60);
  const mins = clamped % 60;
  return `${hours.toString().padStart(2, '0')}:${mins.toString().padStart(2, '0')}`;
}

export function formatDuration(totalMinutes: number): string {
  const minutes = Math.max(0, Math.round(totalMinutes));
  const hours = Math.floor(minutes / 60);
  const mins = minutes % 60;
  if (hours === 0) return `${mins}m`;
  if (mins === 0) return `${hours}h`;
  return `${hours}h ${mins}m`;
}

export function scheduleSummary(schedule: WeeklySchedule): string {
  const slotCount = schedule.slots.length;
  const totalMinutes = schedule.slots.reduce((sum, slot) => sum + slot.endMinute - slot.startMinute, 0);
  if (!schedule.enabled) return 'Schedule off';
  if (slotCount === 0) return 'No active slots';
  return `${slotCount} slot${slotCount === 1 ? '' : 's'} - ${formatDuration(totalMinutes)} weekly`;
}

export function scheduleAllowsAt(schedule: WeeklySchedule, now = new Date()): boolean {
  if (!schedule.enabled) return true;
  if (schedule.slots.length === 0) return false;

  const timeZone = schedule.timeZone || Intl.DateTimeFormat().resolvedOptions().timeZone;
  const local = scheduleLocalTime(now, timeZone);
  return schedule.slots.some((slot) => (
    slot.day === local.day && local.minute >= slot.startMinute && local.minute < slot.endMinute
  ));
}

function scheduleLocalTime(now: Date, timeZone: string): { day: number; minute: number } {
  try {
    const parts = new Intl.DateTimeFormat('en-US', {
      timeZone,
      weekday: 'short',
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23',
    }).formatToParts(now);
    const values = Object.fromEntries(parts.map((part) => [part.type, part.value]));
    const day = WEEK_DAYS.findIndex((item) => item.short === values.weekday);
    const hour = Number(values.hour);
    const minute = Number(values.minute);
    if (day >= 0 && Number.isFinite(hour) && Number.isFinite(minute)) {
      return { day, minute: hour * 60 + minute };
    }
  } catch {
    // Fall back to the browser's local time when an invalid time zone is stored.
  }
  return { day: now.getDay(), minute: now.getHours() * 60 + now.getMinutes() };
}
