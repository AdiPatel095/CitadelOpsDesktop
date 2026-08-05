import { useEffect, useMemo, useRef, useState } from 'react';
import { useCitadelAPI } from '../api/ApiContext';

type EventDifficultyRow = Record<string, unknown> & {
  difficultyID?: unknown;
  eventID?: unknown;
  difficultyTypeID?: unknown;
  isLocked?: unknown;
};

type EventDifficultyTypeRow = Record<string, unknown> & {
  difficultyTypeID?: unknown;
  name?: unknown;
  sortOrder?: unknown;
};

type AchievementRow = Record<string, unknown> & {
  achievementID?: unknown;
  unlocksDifficulty?: unknown;
};

export interface EventDifficultyOption {
  value: string;
  label: string;
}

interface EventDifficultyCatalogState {
  optionsByEvent: Record<string, EventDifficultyOption[]>;
  loading: boolean;
  error: string;
}

export function useEventDifficultyOptions(
  enabled: boolean,
  eventIDs: readonly number[],
  completedAchievements: Record<string, boolean>,
): EventDifficultyCatalogState {
  const { catalogs, getCatalog } = useCitadelAPI();
  const getCatalogRef = useRef(getCatalog);
  getCatalogRef.current = getCatalog;
  const eventKey = eventIDs.join(',');
  const catalogVersion = catalogs?.metadata.digestSha256 ?? catalogs?.metadata.itemVersion ?? '';
  const [rows, setRows] = useState<{
    difficulties: EventDifficultyRow[];
    types: EventDifficultyTypeRow[];
    achievements: AchievementRow[];
  } | null>(null);
  const [loadedKey, setLoadedKey] = useState('');
  const [error, setError] = useState('');

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    const key = `${catalogVersion}:${eventKey}`;
    setError('');
    void Promise.all([
      getCatalogRef.current<EventDifficultyRow>('eventAutoScalingDifficulties'),
      getCatalogRef.current<EventDifficultyTypeRow>('eventAutoScalingDifficultyTypes'),
      getCatalogRef.current<AchievementRow>('achievements'),
    ]).then(([difficulties, types, achievements]) => {
      if (cancelled) return;
      setRows({ difficulties: difficulties.items, types: types.items, achievements: achievements.items });
      setLoadedKey(key);
    }).catch((reason: unknown) => {
      if (cancelled) return;
      setRows(null);
      setLoadedKey(key);
      setError(reason instanceof Error ? reason.message : 'Official event difficulty data is unavailable.');
    });
    return () => { cancelled = true; };
  }, [catalogVersion, enabled, eventKey]);

  const optionsByEvent = useMemo(() => {
    if (!rows) return {};
    return buildEventDifficultyOptions(rows.difficulties, rows.types, rows.achievements, eventIDs, completedAchievements);
  }, [completedAchievements, eventKey, rows]);

  return {
    optionsByEvent,
    loading: enabled && loadedKey !== `${catalogVersion}:${eventKey}`,
    error,
  };
}

export function eventDifficultyName(options: EventDifficultyOption[], difficultyID: number): string {
  return options.find((option) => option.value === String(difficultyID))?.label ?? 'Unknown';
}

function buildEventDifficultyOptions(
  difficulties: EventDifficultyRow[],
  types: EventDifficultyTypeRow[],
  achievements: AchievementRow[],
  eventIDs: readonly number[],
  completedAchievements: Record<string, boolean>,
): Record<string, EventDifficultyOption[]> {
  const wantedEvents = new Set(eventIDs);
  const difficultyTypes = new Map(types.flatMap((row) => {
    const id = integer(row.difficultyTypeID);
    if (id <= 0) return [];
    return [[id, {
      label: humanize(typeof row.name === 'string' ? row.name : ''),
      order: integer(row.sortOrder) || id,
    }] as const];
  }));
  const unlockAchievementByDifficulty = new Map(achievements.flatMap((row) => {
    const difficultyID = integer(row.unlocksDifficulty);
    const achievementID = integer(row.achievementID);
    return difficultyID > 0 && achievementID > 0 ? [[difficultyID, achievementID] as const] : [];
  }));
  const grouped: Record<string, Array<EventDifficultyOption & { order: number }>> = {};
  for (const eventID of eventIDs) grouped[String(eventID)] = [];

  for (const row of difficulties) {
    const eventID = integer(row.eventID);
    const difficultyID = integer(row.difficultyID);
    if (!wantedEvents.has(eventID) || difficultyID <= 0) continue;
    const locked = integer(row.isLocked) === 1;
    const unlockAchievementID = unlockAchievementByDifficulty.get(difficultyID) ?? 0;
    if (locked && (unlockAchievementID <= 0 || !completedAchievements[String(unlockAchievementID)])) continue;
    const difficultyType = difficultyTypes.get(integer(row.difficultyTypeID));
    grouped[String(eventID)].push({
      value: String(difficultyID),
      label: difficultyType?.label || `Difficulty ${difficultyID}`,
      order: difficultyType?.order ?? difficultyID,
    });
  }

  return Object.fromEntries(Object.entries(grouped).map(([eventID, options]) => [
    eventID,
    options
      .sort((left, right) => left.order - right.order || Number(left.value) - Number(right.value))
      .map(({ value, label }) => ({ value, label })),
  ]));
}

function integer(value: unknown): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? Math.trunc(parsed) : 0;
}

function humanize(value: string): string {
  const spaced = value.trim().replace(/([a-z0-9])([A-Z])/g, '$1 $2').replace(/[_-]+/g, ' ');
  return spaced ? spaced.charAt(0).toUpperCase() + spaced.slice(1) : '';
}
