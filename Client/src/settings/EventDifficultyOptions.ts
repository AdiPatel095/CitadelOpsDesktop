import { useEffect, useMemo, useRef, useState } from 'react';
import { useCitadelAPI } from '../api/useCitadelAPI';
import type { CatalogResponse } from '../api/Contracts';

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

type CatalogLoader = <T extends Record<string, unknown>>(name: string) => Promise<CatalogResponse<T>>;

interface EventDifficultyRows {
  difficulties: EventDifficultyRow[];
  types: EventDifficultyTypeRow[];
  achievements: AchievementRow[];
}

const CATALOG_CACHE_MS = 5 * 60_000;
const rowCache = new Map<string, { at: number; rows: Promise<EventDifficultyRows> }>();

/**
 * The three official catalogs the difficulty options are built from, cached per catalog version (CIT-20): the
 * Automation row, the Start check and the editors read the same rows, so they cannot disagree about which
 * difficulties are unlocked. A failed load is not cached.
 */
export function loadEventDifficultyRows(getCatalog: CatalogLoader, catalogVersion: string, now: number = Date.now()): Promise<EventDifficultyRows> {
  const cached = rowCache.get(catalogVersion);
  if (cached && now - cached.at < CATALOG_CACHE_MS) return cached.rows;
  const rows = Promise.all([
    getCatalog<EventDifficultyRow>('eventAutoScalingDifficulties'),
    getCatalog<EventDifficultyTypeRow>('eventAutoScalingDifficultyTypes'),
    getCatalog<AchievementRow>('achievements'),
  ]).then(([difficulties, types, achievements]) => ({ difficulties: difficulties.items, types: types.items, achievements: achievements.items }));
  rowCache.set(catalogVersion, { at: now, rows });
  rows.catch(() => { if (rowCache.get(catalogVersion)?.rows === rows) rowCache.delete(catalogVersion); });
  return rows;
}

export async function loadEventDifficultyOptions(
  getCatalog: CatalogLoader,
  catalogVersion: string,
  eventIDs: readonly number[],
  completedAchievements: Record<string, boolean>,
): Promise<Record<string, EventDifficultyOption[]>> {
  const rows = await loadEventDifficultyRows(getCatalog, catalogVersion);
  return buildEventDifficultyOptions(rows.difficulties, rows.types, rows.achievements, eventIDs, completedAchievements);
}

export function resetEventDifficultyCacheForTests(): void {
  rowCache.clear();
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
  const [rows, setRows] = useState<EventDifficultyRows | null>(null);
  const [loadedKey, setLoadedKey] = useState('');
  const [error, setError] = useState('');

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    const key = `${catalogVersion}:${eventKey}`;
    setError('');
    void loadEventDifficultyRows((name) => getCatalogRef.current(name), catalogVersion).then((loaded) => {
      if (cancelled) return;
      setRows(loaded);
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
