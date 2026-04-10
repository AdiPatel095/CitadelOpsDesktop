import type { CastleBuildingRow } from '../dashboard/models/PlayerCastleInfo.ts';
import {
  WODS_DRAGON_BREATH_FORGE,
  WODS_DRAGON_HOARD,
  WODS_REFINERY,
  WODS_TOOLSMITH,
} from '../dashboard/castleQueueVisibility';

/**
 * Empire **spl** LID per production strip. 0/1 are confirmed from captures; 2–5 match the dashboard
 * server poll order—verify in game_websocket.log with each building’s queue open and adjust if needed.
 */
export const SLOT_LID_RECRUITMENT = 0;
export const SLOT_LID_TOOL_WORKSHOP = 1;
export const SLOT_LID_REFINERY = 2;
export const SLOT_LID_TOOLSMITH = 3;
export const SLOT_LID_DRAGON_HOARD = 4;
export const SLOT_LID_DRAGON_BREATH_FORGE = 5;

/** Parsed from Empire **spl** / **bup**.spl for the focused castle (WID = unit, tool, or recipe type). */
export interface BarracksProductionSlot {
  wid: number;
  tua: number;
  rct?: number;
  ict?: number;
  /** Batch / manual order id from **P** / **PS** (distinguishes parallel refinery manuals). */
  pid?: number;
  spid?: number;
}

export interface BarracksProductionQueue {
  lid: number;
  active?: BarracksProductionSlot;
  queued: BarracksProductionSlot[];
  tct?: number;
}

/** One UI row in the recruitment strip: currently training vs queued batch. */
export type BarracksQueueRowKind = 'active' | 'queued';

export interface BarracksQueueRow {
  kind: BarracksQueueRowKind;
  wid: number;
  tua: number;
  pid?: number;
  spid?: number;
}

/** Layout: recruit/tool = 1 active + 5 queued; refinery / toolsmith / dragon buildings = 2 active + 4 queued (manual crafting). */
export interface SlotStripLayout {
  activeSlots: 1 | 2;
  queueSlots: 4 | 5;
}

function rowFromSlot(kind: BarracksQueueRowKind, s: BarracksProductionSlot): BarracksQueueRow {
  return {
    kind,
    wid: s.wid,
    tua: s.tua,
    pid: s.pid,
    spid: s.spid,
  };
}

/**
 * Build UI rows for one **spl** strip: either one PS “active” plus queued **P** rows, or (manual buildings)
 * first `activeSlots` merged PS+**P** entries shown as active, then queue.
 */
export function productionQueueRows(
  bp: BarracksProductionQueue | null | undefined,
  layout: SlotStripLayout
): (BarracksQueueRow | null)[] {
  const total = layout.activeSlots + layout.queueSlots;
  if (layout.activeSlots === 1) {
    const rows: (BarracksQueueRow | null)[] = [];
    if (bp?.active && bp.active.wid > 0) {
      rows.push(rowFromSlot('active', bp.active));
    }
    for (const q of bp?.queued ?? []) {
      if (q.wid > 0 && rows.length < total) {
        rows.push(rowFromSlot('queued', q));
      }
    }
    while (rows.length < total) {
      rows.push(null);
    }
    return rows.slice(0, total);
  }
  const items: BarracksProductionSlot[] = [];
  if (bp?.active && bp.active.wid > 0) {
    items.push(bp.active);
  }
  for (const q of bp?.queued ?? []) {
    if (q.wid > 0) {
      items.push(q);
    }
  }
  const rows: (BarracksQueueRow | null)[] = [];
  for (let i = 0; i < total; i++) {
    const it = items[i];
    if (!it) {
      rows.push(null);
    } else {
      const kind: BarracksQueueRowKind = i < layout.activeSlots ? 'active' : 'queued';
      rows.push(rowFromSlot(kind, it));
    }
  }
  return rows;
}

export type SlotProductionByLid = Record<string, BarracksProductionQueue>;

/** **crin** / **crst** slot bundle (server adds `labels` from craftingRecipes.json). */
export interface CraftingSlotBundle {
  crid: number[];
  labels?: string[];
  bv?: number[];
}

/** One crafting building on the focused castle (matches **CBI**). */
export interface CraftingBuildingSnapshot {
  kid: number;
  aid: number;
  oid: number;
  wid: number;
  cqid: number;
  ps: CraftingSlotBundle;
  qs: CraftingSlotBundle;
}

export type CraftingManualStripId =
  | 'refinery'
  | 'toolsmith'
  | 'dragon-hoard'
  | 'dragon-breath-forge';

function wodsForCraftingStrip(id: CraftingManualStripId): ReadonlySet<number> {
  switch (id) {
    case 'refinery':
      return WODS_REFINERY;
    case 'toolsmith':
      return WODS_TOOLSMITH;
    case 'dragon-hoard':
      return WODS_DRAGON_HOARD;
    case 'dragon-breath-forge':
      return WODS_DRAGON_BREATH_FORGE;
  }
}

/**
 * Crafting snapshot for a manual building strip, matched by **WID** (Empire buildings.json) on the CBI row.
 */
export function craftingSnapshotForStrip(
  cf: CastleFocusState | null | undefined,
  stripId: CraftingManualStripId
): CraftingBuildingSnapshot | undefined {
  const wods = wodsForCraftingStrip(stripId);
  const list = cf?.craftingQueues;
  if (!list?.length) return undefined;
  return list.find((s) => wods.has(Math.trunc(Number(s.wid))));
}

/** One cell in the manual crafting strip (merged PS then QS, same layout idea as **spl** 2+4). */
export interface CraftingStripRow {
  kind: 'active' | 'queued';
  crid: number;
  label: string;
  qty: number;
}

/** Build UI rows for crin/crst: non-zero CRIDs from ps then qs; first `activeSlots` = active. */
export function craftingStripRowsMerged(
  snap: CraftingBuildingSnapshot | undefined,
  layout: SlotStripLayout
): (CraftingStripRow | null)[] {
  const total = layout.activeSlots + layout.queueSlots;
  const out: (CraftingStripRow | null)[] = [];
  if (!snap) {
    for (let i = 0; i < total; i++) out.push(null);
    return out;
  }
  type Item = { crid: number; label: string; qty: number };
  const items: Item[] = [];
  const push = (b: CraftingSlotBundle) => {
    for (let i = 0; i < b.crid.length; i++) {
      const c = Math.trunc(Number(b.crid[i])) || 0;
      if (c <= 0) continue;
      const label =
        typeof b.labels?.[i] === 'string' && b.labels[i].trim() ? b.labels[i].trim() : `CRID ${c}`;
      const qty = b.bv != null && b.bv[i] != null ? Number(b.bv[i]) : 0;
      items.push({ crid: c, label, qty: Number.isFinite(qty) ? qty : 0 });
    }
  };
  push(snap.ps);
  push(snap.qs);
  for (let i = 0; i < total; i++) {
    const it = items[i];
    if (!it) {
      out.push(null);
      continue;
    }
    out.push({
      kind: i < layout.activeSlots ? 'active' : 'queued',
      crid: it.crid,
      label: it.label,
      qty: it.qty,
    });
  }
  return out;
}

function parseCraftingSlotBundle(raw: unknown): CraftingSlotBundle {
  if (!raw || typeof raw !== 'object') {
    return { crid: [] };
  }
  const o = raw as Record<string, unknown>;
  const cr: unknown[] = Array.isArray(o.crid) ? o.crid : [];
  const crid = cr.map((x) => Math.trunc(Number(x)) || 0);
  let labels: string[] | undefined;
  if (Array.isArray(o.labels)) {
    labels = o.labels.map((x) => (typeof x === 'string' ? x : String(x)));
    if (labels.length === 0) labels = undefined;
  }
  let bv: number[] | undefined;
  if (Array.isArray(o.bv)) {
    bv = o.bv.map((x) => Number(x) || 0);
    if (bv.length === 0) bv = undefined;
  }
  return { crid, labels, bv };
}

function parseCraftingQueues(raw: unknown): CraftingBuildingSnapshot[] | undefined {
  if (!Array.isArray(raw)) return undefined;
  const out: CraftingBuildingSnapshot[] = [];
  for (const e of raw) {
    if (!e || typeof e !== 'object') continue;
    const o = e as Record<string, unknown>;
    const wid = Math.trunc(Number(o.wid)) || 0;
    if (wid <= 0) continue;
    out.push({
      kid: Math.trunc(Number(o.kid)) || 0,
      aid: Math.trunc(Number(o.aid)) || 0,
      oid: Math.trunc(Number(o.oid)) || 0,
      wid,
      cqid: Math.trunc(Number(o.cqid)) || 0,
      ps: parseCraftingSlotBundle(o.ps),
      qs: parseCraftingSlotBundle(o.qs),
    });
  }
  return out.length > 0 ? out : undefined;
}

function parseSlotProductionByLid(raw: unknown): SlotProductionByLid | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const o = raw as Record<string, unknown>;
  const out: SlotProductionByLid = {};
  for (const [k, v] of Object.entries(o)) {
    if (!/^\d+$/.test(k)) continue;
    const q = parseBarracksProduction(v);
    if (q) out[k] = q;
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

/** Queue for one **spl** LID on the focused castle. */
export function slotProductionForLid(
  cf: CastleFocusState | null | undefined,
  lid: number
): BarracksProductionQueue | undefined {
  if (!cf?.slotProductionByLid) return undefined;
  return cf.slotProductionByLid[String(lid)];
}

/** One of your castles from GCL (sent on every `castleFocus` websocket message). */
export interface PlayerCastleOption {
  aid: number;
  kingdomID: number;
  name: string;
  mapX: number;
  mapY: number;
}

/** In-game castle currently in view (server GameState.CastleFocus); JAA gca BG/BD rows for the focused castle. */
export interface CastleFocusState {
  aid: number;
  kingdomID: number;
  mapPX?: number;
  mapPY?: number;
  castleName?: string;
  /**
   * Pickup-eligible decorations aggregated by WID using EmpireItems buildings.json on the server
   * (e.g. "3x Rose Arch", "Banner").
   */
  decorationSummary?: string[];
  catalogVersion?: string;
  bgRows?: CastleBuildingRow[];
  bdRows?: CastleBuildingRow[];
  /** All account castles + map coords (GCL); global, not tied to Units/resource views. */
  playerCastles?: PlayerCastleOption[];
  /**
   * Per-**spl**-LID production strips for the focused castle (e.g. "0" recruit, "1" tool workshop).
   * Populated from server `slotProductionByLid`.
   */
  slotProductionByLid?: SlotProductionByLid | null;
  /**
   * Sovereign crafting (crin, crst): per-building WID with ps/qs CRID arrays
   * (enriched with `labels` on the server).
   */
  craftingQueues?: CraftingBuildingSnapshot[] | null;
}

function slotFromParsed(e: Record<string, unknown>): BarracksProductionSlot {
  const pid = e.pid != null ? Number(e.pid) : undefined;
  const spid = e.spid != null ? Number(e.spid) : undefined;
  return {
    wid: Number(e.wid) || 0,
    tua: Number(e.tua) || 0,
    rct: e.rct != null ? Number(e.rct) : undefined,
    ict: e.ict != null ? Number(e.ict) : undefined,
    ...(pid != null && pid > 0 ? { pid } : {}),
    ...(spid != null && spid > 0 ? { spid } : {}),
  };
}

function parseBarracksProduction(raw: unknown): BarracksProductionQueue | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const o = raw as Record<string, unknown>;
  const lid = Number(o.lid) || 0;
  const tct = o.tct != null ? Number(o.tct) : undefined;
  let active: BarracksProductionSlot | undefined;
  if (o.active != null && typeof o.active === 'object') {
    const a = o.active as Record<string, unknown>;
    const s = slotFromParsed(a);
    if (s.wid > 0) {
      active = s;
    }
  }
  let queued: BarracksProductionSlot[] = [];
  if (Array.isArray(o.queued)) {
    queued = o.queued
      .filter((e): e is Record<string, unknown> => e != null && typeof e === 'object')
      .map((e) => slotFromParsed(e))
      .filter((s) => s.wid > 0);
  }
  return { lid, active, queued, tct };
}

/** Normalize websocket JSON (numbers may be floats) into CastleFocusState. */
export function parseCastleFocusPayload(raw: unknown): CastleFocusState | null {
  if (!raw || typeof raw !== 'object') return null;
  const p = raw as Record<string, unknown>;
  let playerCastles: PlayerCastleOption[] | undefined;
  if (Array.isArray(p.playerCastles)) {
    const opts = p.playerCastles
      .filter((e): e is Record<string, unknown> => e != null && typeof e === 'object')
      .map((e) => {
        const aid = Number(e.aid) || 0;
        return {
          aid,
          kingdomID: Number(e.kingdomID) || 0,
          name: typeof e.name === 'string' && e.name.trim() ? e.name.trim() : `Castle ${aid}`,
          mapX: Number(e.mapX) || 0,
          mapY: Number(e.mapY) || 0,
        };
      })
      .filter((c) => c.aid > 0);
    if (opts.length > 0) playerCastles = opts;
  }
  const slotRaw =
    (p as Record<string, unknown>).slotProductionByLid ??
    (p as Record<string, unknown>).slotProductionByLID;
  let slotProductionByLid = parseSlotProductionByLid(slotRaw);
  const craftingQueues = parseCraftingQueues(p.craftingQueues);
  const legacyBarracks = parseBarracksProduction(p.barracksProduction);
  if (legacyBarracks && !slotProductionByLid?.['0']) {
    slotProductionByLid = { ...(slotProductionByLid ?? {}), '0': legacyBarracks };
  }
  return {
    ...(p as unknown as CastleFocusState),
    aid: Number(p.aid) || 0,
    kingdomID: Number(p.kingdomID) || 0,
    mapPX: p.mapPX != null ? Number(p.mapPX) : undefined,
    mapPY: p.mapPY != null ? Number(p.mapPY) : undefined,
    castleName: typeof p.castleName === 'string' ? p.castleName : undefined,
    decorationSummary: Array.isArray(p.decorationSummary)
      ? (p.decorationSummary as unknown[]).map((s) => String(s))
      : undefined,
    catalogVersion: typeof p.catalogVersion === 'string' ? p.catalogVersion : undefined,
    bgRows: Array.isArray(p.bgRows) ? (p.bgRows as CastleBuildingRow[]) : undefined,
    bdRows: Array.isArray(p.bdRows) ? (p.bdRows as CastleBuildingRow[]) : undefined,
    playerCastles,
    slotProductionByLid,
    craftingQueues,
  };
}

/** BG + BD rows from focus (BuildingParser → GameState via JaaCastleFocus). */
export function mergedCastleFocusRows(cf: CastleFocusState | null): CastleBuildingRow[] {
  if (!cf) return [];
  return [...(cf.bgRows ?? []), ...(cf.bdRows ?? [])];
}

/** Single-line fallback for aria-label (native title is unreliable in desktop webviews). */
export function castleFocusDecorationsTooltip(
  cf: CastleFocusState | null | undefined,
  getDecoration?: (id: number) => { name: string } | undefined
): string {
  const { lines, heading } = castleFocusDecorationTooltipContent(cf, getDecoration);
  if (heading) {
    return `${heading}: ${lines.join(', ')}`;
  }
  return lines[0] ?? '';
}

export type CastleFocusDecorationTooltipContent = {
  heading: string;
  lines: string[];
};

/** One decoration aggregate from BG/BD rows, resolved via public `decorations/index.json` (MetadataContext). */
export type PublicDecorationTooltipRow = {
  wid: number;
  count: number;
  name: string;
};

/**
 * Rows whose `buildingID` exists in the public decorations index (`/game-data/decorations/index.json`).
 */
export function decorationTooltipRowsFromPublicIndex(
  cf: CastleFocusState | null | undefined,
  getDecoration: (id: number) => { name: string } | undefined
): PublicDecorationTooltipRow[] {
  if (!cf) return [];
  const rows = mergedCastleFocusRows(cf);
  const countByWid = new Map<number, number>();
  for (const r of rows) {
    const meta = getDecoration(r.buildingID);
    const nm = meta?.name?.trim();
    if (!nm) continue;
    countByWid.set(r.buildingID, (countByWid.get(r.buildingID) ?? 0) + 1);
  }
  if (countByWid.size === 0) return [];
  const pairs: PublicDecorationTooltipRow[] = [...countByWid.entries()].map(([wid, count]) => {
    const meta = getDecoration(wid)!;
    return {
      wid,
      count,
      name: meta.name.trim(),
    };
  });
  pairs.sort((a, b) => {
    const an = a.name.toLowerCase();
    const bn = b.name.toLowerCase();
    if (an !== bn) return an.localeCompare(bn, undefined, { sensitivity: 'base' });
    return a.wid - b.wid;
  });
  return pairs;
}

/** Flat "Nx Name" strings for callers that only need text. */
export function decorationSummaryLinesFromPublicIndex(
  cf: CastleFocusState | null | undefined,
  getDecoration: (id: number) => { name: string } | undefined
): string[] {
  return decorationTooltipRowsFromPublicIndex(cf, getDecoration).map((r) => `${r.count}x ${r.name}`);
}

/**
 * Content for the hover panel: prefer client-resolved names from public decoration index, then server
 * `decorationSummary`, else JAA row fallback.
 */
export function castleFocusDecorationTooltipContent(
  cf: CastleFocusState | null | undefined,
  getDecoration?: (id: number) => { name: string } | undefined
): CastleFocusDecorationTooltipContent {
  if (!cf || !cf.aid || cf.aid <= 0) {
    return { heading: '', lines: ['Focus a castle in the game (JAA) to see decorations here.'] };
  }
  if (getDecoration) {
    const fromIndex = decorationSummaryLinesFromPublicIndex(cf, getDecoration);
    if (fromIndex.length > 0) {
      return { heading: 'Decorations', lines: fromIndex };
    }
  }
  const summary = (cf.decorationSummary ?? []).map((s) => s.trim()).filter(Boolean);
  if (summary.length > 0) {
    return { heading: 'Decorations', lines: summary };
  }
  const fallback = aggregateNamedRowsForTooltip(cf);
  if (fallback.length > 0) {
    return { heading: 'Buildings in view (game names)', lines: fallback };
  }
  return {
    heading: '',
    lines: [
      'No pickup-eligible decorations in snapshot. Switch castle from the header menu or focus in-game (JAA).',
    ],
  };
}

/** Group BG/BD rows by non-empty display name when server summary is missing (names often "Unknown"). */
function aggregateNamedRowsForTooltip(cf: CastleFocusState): string[] {
  const rows = mergedCastleFocusRows(cf);
  const byName = new Map<string, number>();
  for (const r of rows) {
    const n = r.name?.trim();
    if (!n || n.toLowerCase() === 'unknown') continue;
    byName.set(n, (byName.get(n) ?? 0) + 1);
  }
  const names = [...byName.keys()].sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
  return names.map((name) => {
    const c = byName.get(name) ?? 1;
    return `${c}x ${name}`;
  });
}

export function castleFocusDisplayName(cf: CastleFocusState | null | undefined): string {
  if (!cf || !cf.aid || cf.aid <= 0) return '—';
  const n = cf.castleName?.trim();
  return n || 'Focused castle';
}
