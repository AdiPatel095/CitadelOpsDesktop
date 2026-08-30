import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { CalendarDays, Clock, Plus, Search, Trash2, Wand2 } from 'lucide-react';
import { Badge, Button, Input, Select, SettingsModal, Switch } from '../../components/ui';
import { showTroopPicker } from '../../components/TroopPickerModal';
import { showToolPicker } from '../../components/ToolPickerModal';
import UnitImage from '../../components/UnitImage';
import ToolImage from '../../components/ToolImage';
import { useMetadata } from '../../context/MetadataContext';
import { highestAvailableUnitIDInFamily, unitUpgradeFamily } from '../UnitUpgradeFamily';
import {
  DAY_MINUTES,
  MIN_SLOT_MINUTES,
  SCHEDULE_STEP_MINUTES,
  WEEK_DAYS,
  clampSlotEnd,
  clampSlotStart,
  createScheduleSlot,
  formatDuration,
  formatMinuteOfDay,
  normalizeWeeklySchedule,
  parseMinuteOfDay,
  scheduleSummary,
  snapMinute,
  type WeeklySchedule,
  type WeeklyScheduleSlot,
} from '../SchedulerTypes';

const HOUR_HEIGHT = 18;
const GRID_HEIGHT = HOUR_HEIGHT * 24;
const HOUR_MARKERS = Array.from({ length: 23 }, (_, hour) => hour + 1);
const SLOT_EDGE_RESIZE_PX = 10;
const SLOT_OPTION_ICON_MIN_HEIGHT_RATIO = 4 / 3;
const SLOT_OPTION_HORIZONTAL_INSET = 12;
const SLOT_OPTION_ICON_SCALE = 0.85;
const SLOT_OPTION_ICON_MIN_SIZE = 14;
const SLOT_OPTION_ICON_MAX_SIZE = 168;
const CALENDAR_GRID_TEMPLATE = '4.75rem repeat(7, minmax(8rem, 1fr))';

type DragMode = 'move' | 'resize-start' | 'resize-end' | 'create' | 'copy-prev-day' | 'copy-next-day';

interface DragState {
  slotId?: string;
  mode: DragMode;
  originX: number;
  originY: number;
  originalStart?: number;
  originalEnd?: number;
  originalDay?: number;
  originalOptions?: Record<string, unknown>;
  columnWidth?: number;
  columnLeft?: number;
  day?: number;
  originMinute?: number;
  hasMoved: boolean;
}

interface SlotFormState {
  id?: string;
  day: number;
  startTime: string;
  endTime: string;
  options: Record<string, string>;
  error: string;
}

export interface ScheduleSlotOptionField {
  id: string;
  label: string;
  type: 'number' | 'text';
  picker?: 'troop' | 'tool';
  placeholder?: string;
  required?: boolean;
  integer?: boolean;
  min?: number;
  max?: number;
  allowedUnitIds?: number[];
  allowedToolIds?: number[];
  hidden?: boolean;
  unitRange?: {
    minOptionId: string;
    maxOptionId: string;
  };
}

export interface ScheduleSlotOptionsConfig {
  enabledLabel: string;
  formTitle: string;
  fields: ScheduleSlotOptionField[];
}

interface WeeklySchedulerProps {
  value: WeeklySchedule;
  onChange: (schedule: WeeklySchedule) => void;
  slotOptionsConfig?: ScheduleSlotOptionsConfig;
  className?: string;
}

function slotTop(slot: WeeklyScheduleSlot): number {
  return (slot.startMinute / 60) * HOUR_HEIGHT;
}

function slotHeight(slot: WeeklyScheduleSlot): number {
  return Math.max(16, ((slot.endMinute - slot.startMinute) / 60) * HOUR_HEIGHT);
}

function pointerDeltaMinutes(originY: number, clientY: number): number {
  const rawDelta = ((clientY - originY) / HOUR_HEIGHT) * 60;
  return Math.round(rawDelta / SCHEDULE_STEP_MINUTES) * SCHEDULE_STEP_MINUTES;
}

function pointerModeForSlot(event: React.PointerEvent, slot: WeeklyScheduleSlot): DragMode {
  const rect = event.currentTarget.getBoundingClientRect();
  const y = event.clientY - rect.top;
  const edgeSize = Math.min(SLOT_EDGE_RESIZE_PX, Math.max(6, rect.height / 3));
  if (y <= edgeSize) return 'resize-start';
  if (y >= rect.height - edgeSize) return 'resize-end';
  return 'move';
}

function minuteFromLocalY(localY: number): number {
  return snapMinute((localY / HOUR_HEIGHT) * 60);
}

function slotFromDragCreate(day: number, originMinute: number, currentMinute: number, id?: string): WeeklyScheduleSlot {
  const origin = clampSlotStart(originMinute);
  const current = snapMinute(currentMinute);
  let startMinute = Math.min(origin, current);
  let endMinute = Math.max(origin, current);

  if (endMinute - startMinute < MIN_SLOT_MINUTES) {
    if (current < origin) {
      startMinute = Math.max(0, origin - MIN_SLOT_MINUTES);
      endMinute = origin;
    } else {
      startMinute = origin;
      endMinute = Math.min(DAY_MINUTES, origin + MIN_SLOT_MINUTES);
    }
  }

  startMinute = clampSlotStart(startMinute);
  endMinute = clampSlotEnd(endMinute);
  if (endMinute - startMinute < MIN_SLOT_MINUTES) {
    startMinute = Math.max(0, endMinute - MIN_SLOT_MINUTES);
  }

  return {
    id: id ?? createScheduleSlot(day, startMinute, endMinute).id,
    day,
    startMinute,
    endMinute,
  };
}

function copyTargetDaysForDrag(drag: DragState, clientX: number): number[] {
  if (
    drag.originalDay == null ||
    drag.originalStart == null ||
    drag.originalEnd == null ||
    !drag.columnWidth ||
    drag.columnLeft == null
  ) {
    return [];
  }

  const columnRight = drag.columnLeft + drag.columnWidth;
  const directionalDelta =
    drag.mode === 'copy-next-day'
      ? Math.ceil(Math.max(0, clientX - columnRight) / drag.columnWidth)
      : -Math.ceil(Math.max(0, drag.columnLeft - clientX) / drag.columnWidth);
  if (directionalDelta === 0) return [];

  const targetDay = Math.max(0, Math.min(6, drag.originalDay + directionalDelta));
  if (targetDay === drag.originalDay) return [];

  const step = targetDay > drag.originalDay ? 1 : -1;
  const days: number[] = [];
  for (let day = drag.originalDay + step; step > 0 ? day <= targetDay : day >= targetDay; day += step) {
    days.push(day);
  }
  return days;
}

function slotOptionsToFormOptions(slot: WeeklyScheduleSlot, config?: ScheduleSlotOptionsConfig): Record<string, string> {
  if (!config) return {};
  const options: Record<string, string> = {};
  config.fields.forEach((field) => {
    const value = slot.options?.[field.id];
    if (value != null) {
      options[field.id] = String(value);
    }
  });
  return options;
}

function parseSlotFormOptions(
  formOptions: Record<string, string>,
  config: ScheduleSlotOptionsConfig,
): { options: Record<string, unknown>; error?: string } {
  const options: Record<string, unknown> = {};

  for (const field of config.fields) {
    const rawValue = (formOptions[field.id] ?? '').trim();
    if (!rawValue) {
      if (field.required) {
        return { options: {}, error: `${field.label} is required.` };
      }
      continue;
    }

    if (field.type === 'number') {
      const value = Number(rawValue);
      if (!Number.isFinite(value) || (field.integer && !Number.isInteger(value))) {
        return { options: {}, error: `${field.label} must be a valid number.` };
      }
      if (field.min != null && value < field.min) {
        return { options: {}, error: `${field.label} must be at least ${field.min}.` };
      }
      if (field.max != null && value > field.max) {
        return { options: {}, error: `${field.label} must be at most ${field.max}.` };
      }
      options[field.id] = value;
      continue;
    }

    options[field.id] = rawValue;
  }

  return { options };
}

function slotOptionsSummary(slot: WeeklyScheduleSlot, config?: ScheduleSlotOptionsConfig): string {
  if (!config) return '';
  const parts = config.fields
    .filter((field) => !field.hidden)
    .map((field) => {
      const value = slot.options?.[field.id];
      return value == null || value === '' ? '' : `${field.label} ${value}`;
    })
    .filter(Boolean);
  return parts.join(' · ');
}

function troopOptionField(config?: ScheduleSlotOptionsConfig): ScheduleSlotOptionField | undefined {
  return config?.fields.find((field) => field.picker === 'troop');
}

function toolOptionField(config?: ScheduleSlotOptionsConfig): ScheduleSlotOptionField | undefined {
  return config?.fields.find((field) => field.picker === 'tool');
}

function troopIDFromSlot(slot: WeeklyScheduleSlot, config?: ScheduleSlotOptionsConfig): number | null {
  const field = troopOptionField(config);
  if (!field) return null;
  const value = Number(slot.options?.[field.id]);
  return Number.isFinite(value) && value > 0 ? Math.trunc(value) : null;
}

function toolIDFromSlot(slot: WeeklyScheduleSlot, config?: ScheduleSlotOptionsConfig): number | null {
  const field = toolOptionField(config);
  if (!field) return null;
  const value = Number(slot.options?.[field.id]);
  return Number.isFinite(value) && value > 0 ? Math.trunc(value) : null;
}

export const WeeklyScheduler: React.FC<WeeklySchedulerProps> = ({
  value,
  onChange,
  slotOptionsConfig,
  className = '',
}) => {
  const { getTool, getTroop, troops } = useMetadata();
  const schedule = useMemo(() => normalizeWeeklySchedule(value), [value]);
  const slotOptionsEnabled = !!slotOptionsConfig && !!schedule.slotOptionsEnabled;
  const [editingSlot, setEditingSlot] = useState<SlotFormState | null>(null);
  const [draggingSlot, setDraggingSlot] = useState<string | null>(null);
  const [copyPreviewSlots, setCopyPreviewSlots] = useState<WeeklyScheduleSlot[]>([]);
  const [slotLaneWidth, setSlotLaneWidth] = useState(128);
  const gridShellRef = useRef<HTMLDivElement | null>(null);
  const dragRef = useRef<DragState | null>(null);
  const suppressNextClickRef = useRef(false);

  const dayOptions = useMemo(
    () => WEEK_DAYS.map((day) => ({ value: String(day.value), label: day.label })),
    [],
  );

  const slotsByDay = useMemo(() => {
    const grouped = new Map<number, WeeklyScheduleSlot[]>();
    WEEK_DAYS.forEach((day) => grouped.set(day.value, []));
    schedule.slots.forEach((slot) => {
      grouped.get(slot.day)?.push(slot);
    });
    return grouped;
  }, [schedule.slots]);

  const activeDayCount = useMemo(
    () => Array.from(slotsByDay.values()).filter((slots) => slots.length > 0).length,
    [slotsByDay],
  );

  const weeklyDuration = useMemo(
    () => schedule.slots.reduce((sum, slot) => sum + slot.endMinute - slot.startMinute, 0),
    [schedule.slots],
  );

  const commitSchedule = useCallback(
    (next: WeeklySchedule) => {
      onChange(normalizeWeeklySchedule(next));
    },
    [onChange],
  );

  const updateSlot = useCallback(
    (slotId: string, patch: Partial<WeeklyScheduleSlot>) => {
      commitSchedule({
        ...schedule,
        slots: schedule.slots.map((slot) =>
          slot.id === slotId ? { ...slot, ...patch } : slot,
        ),
      });
    },
    [commitSchedule, schedule],
  );

  const upsertSlot = useCallback(
    (nextSlot: WeeklyScheduleSlot) => {
      commitSchedule({
        ...schedule,
        slots: [
          ...schedule.slots.filter((slot) => slot.id !== nextSlot.id),
          nextSlot,
        ],
      });
    },
    [commitSchedule, schedule],
  );

  const buildSideCopySlots = useCallback(
    (drag: DragState, clientX: number) => {
      if (drag.originalStart == null || drag.originalEnd == null) return [];

      return copyTargetDaysForDrag(drag, clientX)
        .filter((day) => !schedule.slots.some((slot) =>
          slot.day === day &&
          slot.startMinute === drag.originalStart &&
          slot.endMinute === drag.originalEnd
        ))
        .map((day) => ({
          id: `copy-preview-${drag.slotId ?? 'slot'}-${day}-${drag.originalStart}-${drag.originalEnd}`,
          day,
          startMinute: drag.originalStart,
          endMinute: drag.originalEnd,
          ...(slotOptionsEnabled && drag.originalOptions ? { options: { ...drag.originalOptions } } : {}),
        }));
    },
    [schedule.slots, slotOptionsEnabled],
  );

  const copySlotToAdjacentDays = useCallback(
    (drag: DragState, clientX: number) => {
      const copies = buildSideCopySlots(drag, clientX)
        .map((slot) => createScheduleSlot(slot.day, slot.startMinute, slot.endMinute, slot.options));
      if (copies.length === 0) return;

      commitSchedule({
        ...schedule,
        slots: [...schedule.slots, ...copies],
      });
    },
    [buildSideCopySlots, commitSchedule, schedule],
  );

  useEffect(() => {
    const element = gridShellRef.current;
    if (!element || typeof ResizeObserver === 'undefined') return;

    const updateSlotLaneWidth = () => {
      const firstDayColumn = element.querySelector<HTMLElement>('[data-schedule-day]');
      const nextWidth = firstDayColumn?.getBoundingClientRect().width ?? 0;
      if (nextWidth > 0) {
        setSlotLaneWidth(nextWidth);
      }
    };

    updateSlotLaneWidth();

    const observer = new ResizeObserver(updateSlotLaneWidth);
    observer.observe(element);

    const firstDayColumn = element.querySelector<HTMLElement>('[data-schedule-day]');
    if (firstDayColumn) {
      observer.observe(firstDayColumn);
    }

    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    const applyDragUpdate = (drag: DragState, clientX: number, clientY: number, final = false) => {
      const delta = pointerDeltaMinutes(drag.originY, clientY);

      if (drag.mode === 'copy-prev-day' || drag.mode === 'copy-next-day') {
        setCopyPreviewSlots(drag.hasMoved ? buildSideCopySlots(drag, clientX) : []);
        if (final && drag.hasMoved) {
          copySlotToAdjacentDays(drag, clientX);
        }
        return;
      }

      if (drag.mode === 'create') {
        if (!drag.hasMoved || drag.day == null || drag.originMinute == null) return;
        document.body.style.userSelect = 'none';
        const currentMinute = drag.originMinute + delta;
        const nextSlot = slotFromDragCreate(
          drag.day,
          drag.originMinute,
          currentMinute,
          drag.slotId,
        );
        drag.slotId = nextSlot.id;
        upsertSlot(nextSlot);
        setDraggingSlot(nextSlot.id);
        return;
      }

      if (!drag.slotId || drag.originalStart == null || drag.originalEnd == null) return;

      if (drag.mode === 'move') {
        const duration = drag.originalEnd - drag.originalStart;
        const startMinute = clampSlotStart(
          Math.min(DAY_MINUTES - duration, Math.max(0, drag.originalStart + delta)),
        );
        updateSlot(drag.slotId, {
          startMinute,
          endMinute: startMinute + duration,
        });
        return;
      }

      if (drag.mode === 'resize-start') {
        updateSlot(drag.slotId, {
          startMinute: clampSlotStart(
            Math.min(drag.originalEnd - MIN_SLOT_MINUTES, drag.originalStart + delta),
          ),
        });
        return;
      }

      updateSlot(drag.slotId, {
        endMinute: clampSlotEnd(
          Math.max(drag.originalStart + MIN_SLOT_MINUTES, drag.originalEnd + delta),
        ),
      });
    };

    const handlePointerMove = (event: PointerEvent) => {
      const drag = dragRef.current;
      if (!drag) return;

      event.preventDefault();
      if (Math.abs(event.clientY - drag.originY) > 2 || Math.abs(event.clientX - drag.originX) > 2) {
        drag.hasMoved = true;
      }
      applyDragUpdate(drag, event.clientX, event.clientY);
    };

    const handlePointerUp = (event: PointerEvent) => {
      const drag = dragRef.current;
      if (drag) {
        if (Math.abs(event.clientY - drag.originY) > 2 || Math.abs(event.clientX - drag.originX) > 2) {
          drag.hasMoved = true;
        }
        applyDragUpdate(drag, event.clientX, event.clientY, true);
      }
      dragRef.current = null;
      setDraggingSlot(null);
      setCopyPreviewSlots([]);
      document.body.style.userSelect = '';
      if (drag?.hasMoved) {
        suppressNextClickRef.current = true;
        window.setTimeout(() => {
          suppressNextClickRef.current = false;
        }, 0);
      }
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', handlePointerUp);
    return () => {
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', handlePointerUp);
      document.body.style.userSelect = '';
    };
  }, [buildSideCopySlots, copySlotToAdjacentDays, updateSlot, upsertSlot]);

  const beginDrag = (
    event: React.PointerEvent,
    slot: WeeklyScheduleSlot,
    mode: DragMode,
  ) => {
    if (event.button !== 0) return;
    event.preventDefault();
    event.stopPropagation();
    document.body.style.userSelect = 'none';
    setCopyPreviewSlots([]);
    const dayColumn = event.currentTarget.closest('[data-schedule-day]')?.getBoundingClientRect();
    dragRef.current = {
      slotId: slot.id,
      mode,
      originX: event.clientX,
      originY: event.clientY,
      originalStart: slot.startMinute,
      originalEnd: slot.endMinute,
      originalDay: slot.day,
      originalOptions: slot.options ? { ...slot.options } : undefined,
      columnWidth: dayColumn?.width,
      columnLeft: dayColumn?.left,
      hasMoved: false,
    };
    setDraggingSlot(slot.id);
  };

  const beginSlotDrag = (event: React.PointerEvent<HTMLDivElement>, slot: WeeklyScheduleSlot) => {
    beginDrag(event, slot, pointerModeForSlot(event, slot));
  };

  const beginCreateDrag = (event: React.PointerEvent<HTMLDivElement>, day: number) => {
    if (event.button !== 0) return;
    const rect = event.currentTarget.getBoundingClientRect();
    const localY = event.clientY - rect.top;
    const originMinute = minuteFromLocalY(localY);
    const slotAtPoint = [...schedule.slots]
      .reverse()
      .find((slot) =>
        slot.day === day &&
        originMinute >= slot.startMinute &&
        originMinute <= slot.endMinute
      );

    if (slotAtPoint) {
      const slotStartY = (slotAtPoint.startMinute / 60) * HOUR_HEIGHT;
      const slotEndY = (slotAtPoint.endMinute / 60) * HOUR_HEIGHT;
      const mode =
        localY <= slotStartY + SLOT_EDGE_RESIZE_PX
          ? 'resize-start'
          : localY >= slotEndY - SLOT_EDGE_RESIZE_PX
            ? 'resize-end'
            : 'move';
      beginDrag(event, slotAtPoint, mode);
      return;
    }

    dragRef.current = {
      mode: 'create',
      originX: event.clientX,
      originY: event.clientY,
      day,
      originMinute,
      hasMoved: false,
    };
  };

  const openCreateSlot = (day: number, minute: number) => {
    const startMinute = clampSlotStart(minute);
    const endMinute = Math.min(DAY_MINUTES, Math.max(startMinute + 60, startMinute + MIN_SLOT_MINUTES));
    setEditingSlot({
      day,
      startTime: formatMinuteOfDay(startMinute),
      endTime: formatMinuteOfDay(endMinute),
      options: {},
      error: '',
    });
  };

  const openAddSlot = () => {
    const now = new Date();
    const startMinute = clampSlotStart(snapMinute(now.getHours() * 60 + now.getMinutes()));
    openCreateSlot(now.getDay(), startMinute);
  };

  const openEditSlot = (slot: WeeklyScheduleSlot) => {
    if (suppressNextClickRef.current) return;
    setEditingSlot({
      id: slot.id,
      day: slot.day,
      startTime: formatMinuteOfDay(slot.startMinute),
      endTime: formatMinuteOfDay(slot.endMinute),
      options: slotOptionsToFormOptions(slot, slotOptionsConfig),
      error: '',
    });
  };

  const handleDayClick = (event: React.MouseEvent<HTMLDivElement>, day: number) => {
    if (suppressNextClickRef.current) return;
    const rect = event.currentTarget.getBoundingClientRect();
    const minute = minuteFromLocalY(event.clientY - rect.top);
    const slotAtPoint = [...schedule.slots]
      .reverse()
      .find((slot) =>
        slot.day === day &&
        minute >= slot.startMinute &&
        minute <= slot.endMinute
      );
    if (slotAtPoint) {
      openEditSlot(slotAtPoint);
      return;
    }
    openCreateSlot(day, minute);
  };

  const saveSlotForm = () => {
    if (!editingSlot) return;
    const startMinute = parseMinuteOfDay(editingSlot.startTime);
    const endMinute = parseMinuteOfDay(editingSlot.endTime);
    if (startMinute == null || endMinute == null) {
      setEditingSlot({ ...editingSlot, error: 'Use HH:MM time, including 24:00 for day end.' });
      return;
    }
    if (endMinute - startMinute < MIN_SLOT_MINUTES) {
      setEditingSlot({ ...editingSlot, error: 'End time must be at least 15 minutes after start.' });
      return;
    }

    const formOptions = { ...editingSlot.options };
    const unitField = troopOptionField(slotOptionsConfig);
    if (slotOptionsEnabled && unitField?.unitRange) {
      const family = unitUpgradeFamily(Number(formOptions[unitField.id]), troops);
      if (family) {
        formOptions[unitField.unitRange.minOptionId] = String(family.minId);
        formOptions[unitField.unitRange.maxOptionId] = String(family.maxId);
      }
    }

    const parsedOptions =
      slotOptionsEnabled && slotOptionsConfig
        ? parseSlotFormOptions(formOptions, slotOptionsConfig)
        : { options: {} };
    if (parsedOptions.error) {
      setEditingSlot({ ...editingSlot, error: parsedOptions.error });
      return;
    }

    const nextSlot: WeeklyScheduleSlot = {
      id: editingSlot.id ?? createScheduleSlot(editingSlot.day, startMinute, endMinute).id,
      day: editingSlot.day,
      startMinute,
      endMinute,
    };
    if (slotOptionsEnabled && Object.keys(parsedOptions.options).length > 0) {
      nextSlot.options = parsedOptions.options;
    }

    commitSchedule({
      ...schedule,
      slots: editingSlot.id
        ? schedule.slots.map((slot) => (slot.id === editingSlot.id ? nextSlot : slot))
        : [...schedule.slots, nextSlot],
    });
    setEditingSlot(null);
  };

  const deleteEditingSlot = () => {
    if (!editingSlot?.id) return;
    commitSchedule({
      ...schedule,
      slots: schedule.slots.filter((slot) => slot.id !== editingSlot.id),
    });
    setEditingSlot(null);
  };

  const setAllWeek = () => {
    commitSchedule({
      ...schedule,
      enabled: true,
      slots: WEEK_DAYS.map((day) => createScheduleSlot(day.value, 0, DAY_MINUTES)),
    });
  };

  const clearSlots = () => {
    commitSchedule({
      ...schedule,
      slots: [],
    });
  };

  const setSlotOptionsEnabled = (enabled: boolean) => {
    commitSchedule({
      ...schedule,
      slotOptionsEnabled: enabled,
      slots: enabled
        ? schedule.slots
        : schedule.slots.map((slot) => ({
            id: slot.id,
            day: slot.day,
            startMinute: slot.startMinute,
            endMinute: slot.endMinute,
      })),
    });
  };

  const selectTroopForSlotOption = async (field: ScheduleSlotOptionField) => {
    if (!editingSlot) return;
    const currentValue = Number(editingSlot.options[field.id]);
    const preselectedID = Number.isFinite(currentValue) && currentValue > 0
      ? highestAvailableUnitIDInFamily(currentValue, field.allowedUnitIds, troops)
      : null;
    const result = await showTroopPicker({
      mode: 'single',
      title: `Select ${field.label}`,
      preselected: preselectedID != null ? [preselectedID] : [],
      allowedUnitIds: field.allowedUnitIds,
    });
    if (typeof result !== 'number') return;

    setEditingSlot((current) => {
      if (!current) return current;
      const options = {
        ...current.options,
        [field.id]: String(result),
      };
      const family = unitUpgradeFamily(result, troops);
      if (field.unitRange && family) {
        options[field.unitRange.minOptionId] = String(family.minId);
        options[field.unitRange.maxOptionId] = String(family.maxId);
      }
      return {
        ...current,
        options,
        error: '',
      };
    });
  };

  const selectToolForSlotOption = async (field: ScheduleSlotOptionField) => {
    if (!editingSlot) return;
    const currentValue = Number(editingSlot.options[field.id]);
    const result = await showToolPicker({
      mode: 'single',
      title: `Select ${field.label}`,
      preselected: Number.isFinite(currentValue) && currentValue > 0 ? [currentValue] : [],
      allowedToolIds: field.allowedToolIds,
    });
    if (typeof result !== 'number') return;

    setEditingSlot((current) => {
      if (!current) return current;
      return {
        ...current,
        options: {
          ...current.options,
          [field.id]: String(result),
        },
        error: '',
      };
    });
  };

  const renderSlotOptionBadge = (slot: WeeklyScheduleSlot, visualHeight: number) => {
    if (!slotOptionsEnabled || !slotOptionsConfig) return null;
    const troopID = troopIDFromSlot(slot, slotOptionsConfig);
    if (troopID != null) {
	  const troop = getTroop(troopID);
	  const unitName = troop?.name || `Unit ${troopID}`;
	  const rawLevel = Number(troop?.level);
	  const level = Number.isFinite(rawLevel) && rawLevel > 0 ? rawLevel : undefined;
      const label = level ? `${unitName} L${level}` : unitName;
      const family = unitUpgradeFamily(troopID, troops);
      const familyLabel = family && family.minId !== family.maxId
        ? `Auto IDs ${family.minId}-${family.maxId}`
        : `Auto ID ${troopID}`;
      const slotUsableWidth = Math.max(0, slotLaneWidth - SLOT_OPTION_HORIZONTAL_INSET);
      const shouldShowIcon = visualHeight / Math.max(1, slotUsableWidth) > SLOT_OPTION_ICON_MIN_HEIGHT_RATIO;
      const iconSize = Math.round(Math.min(
        SLOT_OPTION_ICON_MAX_SIZE,
        Math.max(
          SLOT_OPTION_ICON_MIN_SIZE,
          slotUsableWidth * SLOT_OPTION_ICON_SCALE,
        ),
      ));

      if (shouldShowIcon) {
        return (
          <span
            className="schedule-slot-option-card"
            title={`${label} · ${familyLabel}`}
          >
            <span className="schedule-slot-option-image-stage">
              <UnitImage
                unitId={troopID}
                size={iconSize}
                showLevel={true}
                className="schedule-slot-option-icon !bg-transparent drop-shadow-md"
              />
            </span>
            <span className="schedule-slot-unit-name">{unitName}</span>
          </span>
        );
      }

      return (
        <span
          className="schedule-slot-option-badge"
          title={`${label} · ${familyLabel}`}
        >
          {level && <span className="schedule-slot-level-badge">L{level}</span>}
          <span className="schedule-slot-unit-name">{unitName}</span>
        </span>
      );
    }

    const toolID = toolIDFromSlot(slot, slotOptionsConfig);
    if (toolID != null) {
      const toolName = getTool(toolID)?.name || `Tool ${toolID}`;
      const slotUsableWidth = Math.max(0, slotLaneWidth - SLOT_OPTION_HORIZONTAL_INSET);
      const shouldShowIcon = visualHeight / Math.max(1, slotUsableWidth) > SLOT_OPTION_ICON_MIN_HEIGHT_RATIO;
      const iconSize = Math.round(Math.min(
        SLOT_OPTION_ICON_MAX_SIZE,
        Math.max(
          SLOT_OPTION_ICON_MIN_SIZE,
          slotUsableWidth * SLOT_OPTION_ICON_SCALE,
        ),
      ));

      if (shouldShowIcon) {
        return (
          <span
            className="schedule-slot-option-card"
            title={toolName}
          >
            <span className="schedule-slot-option-image-stage">
              <ToolImage
                toolId={toolID}
                size={iconSize}
                showLevel={true}
                className="schedule-slot-option-icon !bg-transparent drop-shadow-md"
              />
            </span>
            <span className="schedule-slot-unit-name">{toolName}</span>
          </span>
        );
      }

      return (
        <span
          className="schedule-slot-option-badge"
          title={toolName}
        >
          <span className="schedule-slot-unit-name">{toolName}</span>
        </span>
      );
    }

    const optionSummary = slotOptionsSummary(slot, slotOptionsConfig);
    if (!optionSummary) return null;
    return (
      <span className="schedule-slot-option-badge schedule-slot-option-badge-text">
        {optionSummary}
      </span>
    );
  };

  return (
    <div className={`weekly-scheduler ${className}`}>
      <div className="schedule-planner-shell">
        <div className="schedule-planner-hero">
          <span className="schedule-toolbar-icon">
            <CalendarDays className="h-4 w-4" />
          </span>
          <div className="schedule-toolbar-title">
            <span className="schedule-kicker">Schedule Control</span>
            <div className="schedule-title-row">
              <h3>Weekly Schedule</h3>
              <Badge variant={schedule.enabled ? 'success' : 'secondary'}>{scheduleSummary(schedule)}</Badge>
            </div>
          </div>

          <div className="schedule-hero-metrics">
            <div className="schedule-hero-metric">
              <span>Slots</span>
              <strong>{schedule.slots.length}</strong>
            </div>
            <div className="schedule-hero-metric">
              <span>Days</span>
              <strong>{activeDayCount}/7</strong>
            </div>
            <div className="schedule-hero-metric">
              <span>Time</span>
              <strong>{formatDuration(weeklyDuration)}</strong>
            </div>
          </div>
        </div>

        <div className="schedule-command-bar">
          <div className="schedule-command-toggles">
            <div className="schedule-control-chip">
              <span>Use Schedule</span>
              <Switch
                size="sm"
                checked={schedule.enabled}
                onChange={(enabled) => commitSchedule({ ...schedule, enabled })}
              />
            </div>
            {slotOptionsConfig && (
              <div className="schedule-control-chip">
                <span>{slotOptionsConfig.enabledLabel}</span>
                <Switch
                  size="sm"
                  checked={slotOptionsEnabled}
                  onChange={setSlotOptionsEnabled}
                />
              </div>
            )}
          </div>

          <div className="schedule-command-actions">
            <Button variant="outline" size="sm" onClick={openAddSlot} leftIcon={<Plus className="h-4 w-4" />}>
              Add Slot
            </Button>
            <Button variant="ghost" size="sm" onClick={setAllWeek} leftIcon={<Wand2 className="h-4 w-4" />}>
              All Week
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={clearSlots}
              disabled={schedule.slots.length === 0}
              leftIcon={<Trash2 className="h-4 w-4" />}
            >
              Clear
            </Button>
          </div>
        </div>
      </div>

      <div ref={gridShellRef} className="schedule-grid-shell custom-scrollbar">
        <div className="schedule-grid-min">
          <div
            className="schedule-grid-header"
            style={{ gridTemplateColumns: CALENDAR_GRID_TEMPLATE }}
          >
            <div className="schedule-time-heading">
              <Clock className="h-3.5 w-3.5" />
              <span>24h</span>
            </div>
            {WEEK_DAYS.map((day) => {
              const daySlotCount = slotsByDay.get(day.value)?.length ?? 0;
              return (
                <div
                  key={day.value}
                  className={`schedule-day-heading${daySlotCount > 0 ? ' schedule-day-heading-active' : ''}`}
                >
                  <div className="schedule-day-copy">
                    <span className="schedule-day-short">{day.short}</span>
                    <span className="schedule-day-name">{day.label}</span>
                  </div>
                  <span className="schedule-day-count">
                    {daySlotCount}
                  </span>
                </div>
              );
            })}
          </div>

          <div
            className="schedule-grid-body"
            style={{ gridTemplateColumns: CALENDAR_GRID_TEMPLATE }}
          >
            <div className="schedule-time-column" style={{ height: GRID_HEIGHT }}>
              {HOUR_MARKERS.map((hour) => (
                <div
                  key={hour}
                  className="schedule-time-marker"
                  style={{ top: hour * HOUR_HEIGHT }}
                >
                  {formatMinuteOfDay(hour * 60)}
                </div>
              ))}
            </div>

            {WEEK_DAYS.map((day) => (
              <div
                key={day.value}
                data-schedule-day
                className="schedule-day-column"
                style={{ height: GRID_HEIGHT }}
                onPointerDown={(event) => beginCreateDrag(event, day.value)}
                onClick={(event) => handleDayClick(event, day.value)}
              >
                {HOUR_MARKERS.map((hour) => (
                  <div
                    key={hour}
                    className="schedule-hour-line"
                    style={{ top: hour * HOUR_HEIGHT }}
                  />
                ))}

                {copyPreviewSlots
                  .filter((slot) => slot.day === day.value)
                  .map((slot) => {
                    const visualHeight = slotHeight(slot);
                    const showDuration = visualHeight >= 28;
                    return (
                      <div
                        key={slot.id}
                        data-schedule-copy-preview
                        className="schedule-slot schedule-slot-preview"
                        style={{
                          top: slotTop(slot),
                          height: visualHeight,
                        }}
                        aria-hidden="true"
                      >
                        <div className="schedule-slot-content">
                          <span className="schedule-slot-time">
                            <Clock className="h-3 w-3 shrink-0" />
                            {formatMinuteOfDay(slot.startMinute)}-{formatMinuteOfDay(slot.endMinute)}
                          </span>
                          {showDuration && (
                            <span className="schedule-slot-duration">{formatDuration(slot.endMinute - slot.startMinute)}</span>
                          )}
                          {renderSlotOptionBadge(slot, visualHeight)}
                        </div>
                      </div>
                    );
                  })}

                {(slotsByDay.get(day.value) ?? []).map((slot) => {
                  const isDragging = draggingSlot === slot.id;
                  const visualHeight = slotHeight(slot);
                  const showDuration = visualHeight >= 28;
                  return (
                    <div
                      key={slot.id}
                      data-schedule-slot
                      role="button"
                      tabIndex={0}
                      onPointerDown={(event) => beginSlotDrag(event, slot)}
                      onClick={(event) => {
                        event.stopPropagation();
                        openEditSlot(slot);
                      }}
                      onKeyDown={(event) => {
                        if (event.key === 'Enter' || event.key === ' ') {
                          event.preventDefault();
                          openEditSlot(slot);
                        }
                      }}
                      className={`schedule-slot ${
                        isDragging ? 'schedule-slot-dragging' : 'schedule-slot-idle'
                      }`}
                      style={{
                        top: slotTop(slot),
                        height: visualHeight,
                      }}
                      title={`${WEEK_DAYS[slot.day].label} ${formatMinuteOfDay(slot.startMinute)}-${formatMinuteOfDay(slot.endMinute)} (${formatDuration(slot.endMinute - slot.startMinute)})`}
                    >
                      <button
                        type="button"
                        aria-label="Copy this slot to previous day"
                        className="schedule-slot-copy-handle schedule-slot-copy-handle-left"
                        onPointerDown={(event) => beginDrag(event, slot, 'copy-prev-day')}
                      />
                      <button
                        type="button"
                        aria-label="Copy this slot to next day"
                        className="schedule-slot-copy-handle schedule-slot-copy-handle-right"
                        onPointerDown={(event) => beginDrag(event, slot, 'copy-next-day')}
                      />
                      <button
                        type="button"
                        aria-label="Resize start time"
                        className="schedule-slot-resize-handle schedule-slot-resize-start"
                        onPointerDown={(event) => beginDrag(event, slot, 'resize-start')}
                      />
                      <div className="schedule-slot-content">
                        <span className="schedule-slot-time">
                          <Clock className="h-3 w-3 shrink-0" />
                          {formatMinuteOfDay(slot.startMinute)}-{formatMinuteOfDay(slot.endMinute)}
                        </span>
                        {showDuration && (
                          <span className="schedule-slot-duration">{formatDuration(slot.endMinute - slot.startMinute)}</span>
                        )}
                        {renderSlotOptionBadge(slot, visualHeight)}
                      </div>
                      <button
                        type="button"
                        aria-label="Resize end time"
                        className="schedule-slot-resize-handle schedule-slot-resize-end"
                        onPointerDown={(event) => beginDrag(event, slot, 'resize-end')}
                      />
                    </div>
                  );
                })}
              </div>
            ))}
          </div>
        </div>
      </div>

      <SettingsModal
        isOpen={editingSlot != null}
        onClose={() => setEditingSlot(null)}
        maxWidth="md"
        title={editingSlot?.id ? 'Edit Schedule Slot' : 'Add Schedule Slot'}
        icon={<Clock className="h-4 w-4" />}
        onSave={saveSlotForm}
        saveLabel="Save Slot"
        footerLeading={editingSlot?.id ? (
          <Button variant="danger" onClick={deleteEditingSlot} leftIcon={<Trash2 className="h-4 w-4" />}>
            Delete
          </Button>
        ) : undefined}
      >
        {editingSlot && (
          <div className="schedule-slot-form">
            <div className="schedule-field">
              <label>
                Day
              </label>
              <Select
                value={String(editingSlot.day)}
                options={dayOptions}
                onChange={(day) => setEditingSlot({ ...editingSlot, day: Number(day), error: '' })}
              />
            </div>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="schedule-field">
                <label>
                  Start Time
                </label>
                <Input
                  value={editingSlot.startTime}
                  onChange={(event) => setEditingSlot({ ...editingSlot, startTime: event.target.value, error: '' })}
                  placeholder="09:00"
                  className="font-mono"
                />
              </div>
              <div className="schedule-field">
                <label>
                  End Time
                </label>
                <Input
                  value={editingSlot.endTime}
                  onChange={(event) => setEditingSlot({ ...editingSlot, endTime: event.target.value, error: '' })}
                  placeholder="17:00"
                  className="font-mono"
                />
              </div>
            </div>

            {slotOptionsEnabled && slotOptionsConfig && (
              <div className="schedule-option-panel">
                <div className="ui-kicker schedule-option-panel-title">
                  {slotOptionsConfig.formTitle}
                </div>
                <div className="schedule-option-fields">
                  {slotOptionsConfig.fields.filter((field) => !field.hidden).map((field) => (
                    <div
                      key={field.id}
                      className={`schedule-field${field.picker ? ' schedule-picker-field' : ''}`}
                    >
                      <label>
                        {field.label}
                      </label>
                      {field.picker === 'troop' ? (
                        <div className="schedule-troop-picker-row">
                          {Number(editingSlot.options[field.id]) > 0 ? (
                            <UnitImage
                              unitId={Number(editingSlot.options[field.id])}
                              size={48}
                              showLevel={true}
                              className="!bg-transparent drop-shadow-md"
                            />
                          ) : (
                            <div className="schedule-empty-unit">
                              <Search className="h-4 w-4" />
                            </div>
                          )}
                          <div className="schedule-troop-picker-copy">
                            <div className="schedule-troop-picker-name">
                              {Number(editingSlot.options[field.id]) > 0
								? getTroop(Number(editingSlot.options[field.id]))?.name || `Unit ${editingSlot.options[field.id]}`
                                : 'No unit selected'}
                            </div>
                            <div className="schedule-troop-picker-id">
                              {Number(editingSlot.options[field.id]) > 0
                                ? (() => {
                                    const unitID = Number(editingSlot.options[field.id]);
                                    const family = unitUpgradeFamily(unitID, troops);
                                    const range = family && family.minId !== family.maxId
                                      ? `${family.minId}-${family.maxId}`
                                      : String(unitID);
                                    return `ID ${unitID} · auto ${range}`;
                                  })()
                                : field.placeholder}
                            </div>
                          </div>
                          <Button
                            variant="outline"
                            size="sm"
                            className="schedule-troop-picker-button"
                            onClick={() => selectTroopForSlotOption(field)}
                            leftIcon={<Search className="h-4 w-4" />}
                          >
                            Choose
                          </Button>
                        </div>
                      ) : field.picker === 'tool' ? (
                        <div className="schedule-troop-picker-row">
                          {Number(editingSlot.options[field.id]) > 0 ? (
                            <ToolImage
                              toolId={Number(editingSlot.options[field.id])}
                              size={48}
                              showLevel={true}
                              className="!bg-transparent drop-shadow-md"
                            />
                          ) : (
                            <div className="schedule-empty-unit">
                              <Search className="h-4 w-4" />
                            </div>
                          )}
                          <div className="schedule-troop-picker-copy">
                            <div className="schedule-troop-picker-name">
                              {Number(editingSlot.options[field.id]) > 0
                                ? getTool(Number(editingSlot.options[field.id]))?.name || `Tool ${editingSlot.options[field.id]}`
                                : 'No tool selected'}
                            </div>
                            <div className="schedule-troop-picker-id">
                              {Number(editingSlot.options[field.id]) > 0 ? editingSlot.options[field.id] : field.placeholder}
                            </div>
                          </div>
                          <Button
                            variant="outline"
                            size="sm"
                            className="schedule-troop-picker-button"
                            onClick={() => selectToolForSlotOption(field)}
                            leftIcon={<Search className="h-4 w-4" />}
                          >
                            Choose
                          </Button>
                        </div>
                      ) : (
                        <Input
                          type={field.type}
                          value={editingSlot.options[field.id] ?? ''}
                          min={field.min}
                          max={field.max}
                          step={field.integer ? 1 : undefined}
                          onChange={(event) => setEditingSlot({
                            ...editingSlot,
                            options: {
                              ...editingSlot.options,
                              [field.id]: event.target.value,
                            },
                            error: '',
                          })}
                          placeholder={field.placeholder}
                        />
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}

            {editingSlot.error && (
              <div className="schedule-form-error">
                {editingSlot.error}
              </div>
            )}
          </div>
        )}
      </SettingsModal>
    </div>
  );
};
