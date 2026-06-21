import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

const battleReportRoutes = new Set([
  '/api/battle-reports',
  '/api/battleReports',
  '/Data/BattleReports.jsonl',
  '/BattleReports.jsonl',
])

function localBattleReportsPlugin(): Plugin {
  return {
    name: 'citadel-local-battle-reports',
    configureServer(server) {
      let announcedSource = ''

      server.middlewares.use((req, res, next) => {
        const url = new URL(req.url ?? '/', 'http://localhost')
        if (!battleReportRoutes.has(url.pathname)) {
          next()
          return
        }

        const source = resolveBattleReportsFile()
        if (!source) {
          res.statusCode = 404
          res.setHeader('Content-Type', 'application/json; charset=utf-8')
          res.end(JSON.stringify({ error: 'No local BattleReports.jsonl archive found.' }))
          return
        }

        try {
          const reports = readLocalParsedBattleReports(source)
          const announcement = `${source}:${reports.length}`
          if (announcedSource !== announcement) {
            announcedSource = announcement
            server.config.logger.info(`Serving ${reports.length} parsed local battle reports from ${source}`)
          }

          res.statusCode = 200
          res.setHeader('Content-Type', 'application/json; charset=utf-8')
          res.setHeader('Cache-Control', 'no-store')
          res.end(JSON.stringify({ reports }))
        } catch (error) {
          next(error)
        }
      })
    },
  }
}

function resolveBattleReportsFile(): string | null {
  const candidates = battleReportCandidates()
  return candidates.find((candidate) => fs.existsSync(candidate)) ?? null
}

function battleReportCandidates(): string[] {
  const explicitFile = process.env.CITADEL_BATTLE_REPORTS_FILE
  const explicitDataDir = process.env.CITADEL_DATA_DIR
  const downloadsDir = path.join(os.homedir(), 'Downloads')

  return [
    explicitFile,
    explicitDataDir ? path.join(explicitDataDir, 'BattleReports.jsonl') : '',
    path.join(process.cwd(), 'public', 'Data', 'BattleReports.jsonl'),
    path.join(process.cwd(), 'Data', 'BattleReports.jsonl'),
    path.resolve(process.cwd(), '..', 'Data', 'BattleReports.jsonl'),
    path.join(downloadsDir, 'Adolphus_Murtry', 'Data', 'BattleReports.jsonl'),
    path.join(downloadsDir, 'Adolphus_Murtry', 'CitadelOpsDesktop', 'Data', 'BattleReports.jsonl'),
    path.join(downloadsDir, 'Amos_Burton', 'Data', 'BattleReports.jsonl'),
  ].filter(Boolean)
}

function readLocalParsedBattleReports(source: string): LocalParsedReport[] {
  const text = fs.readFileSync(source, 'utf8')
  return text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .flatMap((line) => {
      try {
        const capture = JSON.parse(line) as JsonRecord
        const parsed = parseLocalBattleReportCapture(capture)
        return reportHasBothPlayers(parsed) ? [parsed] : []
      } catch {
        return []
      }
    })
}

type JsonRecord = Record<string, unknown>

interface LocalCombatant {
  playerID?: number
  name?: string
  playerName?: string
  allianceID?: number
  alliance?: string
  allianceName?: string
  role?: string
}

interface LocalMetrics {
  attackerSent: number
  attackerLost: number
  attackersKilled: number
  defenderStationed: number
  defenderLost: number
  defendersKilled: number
  attackTradeRatio?: number
  defenseTradeRatio?: number
}

interface LocalBattleItemDetail {
  side: string
  phase: string
  lane?: string
  wodID: number
  amount: number
  lost?: number
  used?: number
}

interface LocalWaveLane {
  lane: string
  result: string
  attackerLost: number
  defenderLost: number
  attackerStart: number
  defenderStart: number
  attackerUnitDetails: LocalBattleItemDetail[]
  defenderUnitDetails: LocalBattleItemDetail[]
  attackerToolDetails: LocalBattleItemDetail[]
  defenderToolDetails: LocalBattleItemDetail[]
}

interface LocalWave {
  index: number
  wave: number
  lanes: LocalWaveLane[]
}

interface LocalEffect {
  code: string
  label: string
  name: string
  value: number
  formattedValue: string
  displayText: string
  category: string
  sortOrder: number
  side: string
}

interface LocalParsedReport {
  id: string
  reportID: string
  mid: number
  lid: number
  battleKey: string
  kingdomID?: number
  targetName: string
  castleName: string
  battleType: string
  occurredAt: string
  dateMs: number
  result: string
  role: string
  attacker?: LocalCombatant
  defender?: LocalCombatant
  metrics: LocalMetrics
  effects: LocalEffect[]
  commanderEffects: LocalEffect[]
  castellanEffects: LocalEffect[]
  topUnits: LocalBattleItemDetail[]
  supportTools: LocalBattleItemDetail[]
  waves: LocalWave[]
}

function parseLocalBattleReportCapture(capture: JsonRecord): LocalParsedReport {
  const mid = numericValue(capture.mid)
  const lid = numericValue(capture.lid) || mid
  const capturedAt = numericValue(capture.capturedAtUnixMillis)
  const battleKey = stringValue(capture.battleKey)
  const bls = recordValue(capture.bls)
  const blm = recordValue(capture.blm)
  const bld = recordValue(capture.bld)
  const report: LocalParsedReport = {
    id: captureID(mid, lid),
    reportID: captureID(mid, lid),
    mid,
    lid,
    battleKey,
    targetName: cleanBattleLocation(battleKey) || 'Unknown castle',
    castleName: cleanBattleLocation(battleKey) || 'Unknown castle',
    battleType: 'Castle battle',
    occurredAt: capturedAt > 0 ? new Date(capturedAt).toISOString() : '',
    dateMs: capturedAt,
    result: 'Unknown',
    role: 'Unknown',
    metrics: emptyMetrics(),
    effects: [],
    commanderEffects: [],
    castellanEffects: [],
    topUnits: [],
    supportTools: [],
    waves: [],
  }

  applyLocalBattleMeta(report, bls)
  applyLocalBattleItemSummaries(report, bld)
  applyLocalBattleWaves(report, bld)
  applyLocalBattleWaves(report, blm)
  report.topUnits = aggregateBattleItems(report.topUnits)
  report.supportTools = aggregateBattleItems(report.supportTools)
  report.result = inferBattleResult(report.metrics)
  return report
}

function applyLocalBattleMeta(report: LocalParsedReport, bls: JsonRecord | null) {
  if (!bls) {
    return
  }

  const lid = numericValue(bls.LID)
  if (lid > 0) {
    report.lid = lid
    report.id = captureID(report.mid, lid)
    report.reportID = report.id
  }

  const ai = recordValue(bls.AI)
  if (ai) {
    const targetName = cleanBattleLocation(stringValue(ai.N))
    if (targetName) {
      report.targetName = targetName
      report.castleName = targetName
    }
    const kingdomID = numericValue(ai.K)
    if (Number.isFinite(kingdomID)) {
      report.kingdomID = kingdomID
    }
  }

  const players = playerInfoByOID(bls)
  const roles = pbiRoles(bls)
  roles.forEach((role, oid) => {
    const combatant = combatantFromPlayerInfo(oid, role, players.get(oid))
    if (!combatant) {
      return
    }
    if (role === 'attacker') {
      report.attacker = combatant
    } else if (role === 'defender') {
      report.defender = combatant
    }
  })

  report.metrics = metricsFromPBI(bls)
  report.commanderEffects = effectsFromLeader(bls.AL, 'commander')
  report.castellanEffects = effectsFromLeader(bls.DB, 'castellan')
  report.effects = [...report.commanderEffects, ...report.castellanEffects]
}

function playerInfoByOID(bls: JsonRecord): Map<number, JsonRecord> {
  const players = new Map<number, JsonRecord>()
  arrayValue(bls.PI).forEach((raw) => {
    const info = recordValue(raw)
    const oid = numericValue(info?.OID)
    if (info && oid > 0) {
      players.set(oid, info)
    }
  })
  return players
}

function pbiRoles(bls: JsonRecord): Map<number, string> {
  const roles = new Map<number, string>()
  arrayValue(bls.PBI).forEach((raw) => {
    const row = arrayValue(raw)
    const oid = numericValue(row[0])
    const side = numericValue(row[1])
    if (!oid) {
      return
    }
    if (side === 0) {
      roles.set(oid, 'attacker')
    } else if (side === 1) {
      roles.set(oid, 'defender')
    }
  })
  return roles
}

function metricsFromPBI(bls: JsonRecord): LocalMetrics {
  const metrics = emptyMetrics()
  arrayValue(bls.PBI).forEach((raw) => {
    const row = arrayValue(raw)
    const side = numericValue(row[1])
    const started = Math.abs(numericValue(row[2]))
    const lost = Math.abs(numericValue(row[3]))
    if (side === 0) {
      metrics.attackerSent += started
      metrics.attackerLost += lost
      metrics.attackersKilled += lost
    } else if (side === 1) {
      metrics.defenderStationed += started
      metrics.defenderLost += lost
      metrics.defendersKilled += lost
    }
  })
  if (metrics.attackerLost > 0) {
    metrics.attackTradeRatio = round(metrics.defenderLost / metrics.attackerLost, 2)
  }
  if (metrics.defenderLost > 0) {
    metrics.defenseTradeRatio = round(metrics.attackerLost / metrics.defenderLost, 2)
  }
  return metrics
}

function emptyMetrics(): LocalMetrics {
  return {
    attackerSent: 0,
    attackerLost: 0,
    attackersKilled: 0,
    defenderStationed: 0,
    defenderLost: 0,
    defendersKilled: 0,
  }
}

function combatantFromPlayerInfo(oid: number, role: string, info?: JsonRecord): LocalCombatant | null {
  const name = stringValue(info?.N)
  if (!name) {
    return null
  }
  const alliance = stringValue(info?.AN)
  const allianceID = numericValue(info?.AID)
  return {
    playerID: oid,
    name,
    playerName: name,
    allianceID,
    alliance,
    allianceName: alliance,
    role,
  }
}

function effectsFromLeader(value: unknown, side: string): LocalEffect[] {
  const leader = recordValue(value)
  if (!leader) {
    return []
  }

  const grouped = new Map<string, { effect: LocalEffect; meta: LocalEffectMeta }>()
  arrayValue(leader.AE).forEach((raw) => {
    const row = arrayValue(raw)
    const id = numericValue(row[0])
    const values = battleEffectValuesFromValue(row[1])
    if (!id || values.length === 0) {
      return
    }

    const value = battleEffectValue(id, values)
    if (value === 0) {
      return
    }

    const meta = battleEffectMeta(id)
    const key = `${meta.label}|${meta.unit}|${meta.category}|${meta.template}`
    const existing = grouped.get(key)
    if (existing) {
      existing.effect.code = `${existing.effect.code},${id}`
      existing.effect.value += value
      if (meta.order < existing.effect.sortOrder) {
        existing.effect.sortOrder = meta.order
      }
      return
    }

    grouped.set(key, {
      meta,
      effect: {
        code: String(id),
        label: meta.label,
        name: meta.label,
        value,
        formattedValue: '',
        displayText: '',
        category: meta.category,
        sortOrder: meta.order,
        side,
      },
    })
  })

  return Array.from(grouped.values())
    .map(({ effect, meta }) => {
      const value = round(effect.value, 1)
      return {
        ...effect,
        value,
        formattedValue: formatBattleEffectValue(value, meta),
        displayText: formatBattleEffectText(value, meta),
      }
    })
    .sort((a, b) => a.sortOrder - b.sortOrder || a.label.localeCompare(b.label))
}

function applyLocalBattleItemSummaries(report: LocalParsedReport, data: JsonRecord | null) {
  if (!data) {
    return
  }
  arrayValue(data.Y).forEach((raw) => {
    const row = arrayValue(raw)
    const oid = numericValue(row[0])
    const side = sideRoleFromOID(report, oid)
    if (side) {
      report.topUnits.push(...battleItemRows(row.slice(1), side, 'courtyard', '', 'unit'))
    }
  })
  arrayValue(data.S).forEach((raw) => {
    const row = arrayValue(raw)
    const oid = numericValue(row[0])
    const side = sideRoleFromOID(report, oid)
    if (side) {
      report.supportTools.push(...battleItemRows(row.slice(1), side, 'support', '', 'tool'))
    }
  })
}

function applyLocalBattleWaves(report: LocalParsedReport, data: JsonRecord | null) {
  if (!data || report.waves.length > 0) {
    return
  }
  arrayValue(data.W).forEach((rawWave, waveIndex) => {
    const waveRows = arrayValue(rawWave)
    const wave: LocalWave = { index: waveIndex, wave: waveIndex + 1, lanes: [] }
    for (let laneIndex = 0; laneIndex < 3; laneIndex += 1) {
      const lane = emptyLane(laneName(laneIndex))
      waveRows.forEach((sideRaw) => {
        const sideRow = arrayValue(sideRaw)
        const side = sideRoleFromOID(report, numericValue(sideRow[0]))
        if (!side) {
          return
        }
        const laneSummary = sideRow[laneIndex + 1]
        let started = 0
        let lost = 0
        let units: LocalBattleItemDetail[] = []
        let tools: LocalBattleItemDetail[] = []
        const totals = laneTotals(laneSummary)
        started = totals.started
        lost = totals.lost
        const details = laneItemDetails(laneSummary, side, lane.lane)
        units = details.units
        tools = details.tools
        if (units.length > 0) {
          const itemTotals = battleItemTotals(units)
          started = itemTotals.started
          lost = itemTotals.lost
          report.topUnits.push(...units)
        }
        if (tools.length > 0) {
          report.supportTools.push(...tools)
        }
        if (side === 'attacker') {
          lane.attackerStart += started
          lane.attackerLost += lost
          lane.attackerUnitDetails.push(...units)
          lane.attackerToolDetails.push(...tools)
        } else if (side === 'defender') {
          lane.defenderStart += started
          lane.defenderLost += lost
          lane.defenderUnitDetails.push(...units)
          lane.defenderToolDetails.push(...tools)
        }
      })
      lane.result = inferLaneResult(lane)
      wave.lanes.push(lane)
    }
    report.waves.push(wave)
  })
}

function emptyLane(lane: string): LocalWaveLane {
  return {
    lane,
    result: 'HELD',
    attackerLost: 0,
    defenderLost: 0,
    attackerStart: 0,
    defenderStart: 0,
    attackerUnitDetails: [],
    defenderUnitDetails: [],
    attackerToolDetails: [],
    defenderToolDetails: [],
  }
}

function laneTotals(value: unknown): { started: number; lost: number } {
  const row = arrayValue(value)
  if (row.length < 3 || !isNumberLike(row[0])) {
    return { started: 0, lost: 0 }
  }
  const started = Math.abs(numericValue(row[0]))
  const delta = numericValue(row[2])
  return { started, lost: delta < 0 ? Math.abs(delta) : 0 }
}

function laneItemDetails(value: unknown, side: string, lane: string): { units: LocalBattleItemDetail[]; tools: LocalBattleItemDetail[] } {
  const groups = arrayValue(value)
  if (groups.length === 0 || isNumberLike(groups[0])) {
    return { units: [], tools: [] }
  }
  const units = battleItemRows(arrayValue(groups[0]), side, 'wall', lane, 'unit')
  const tools = groups.slice(1).flatMap((group) => battleItemRows(arrayValue(group), side, 'wall', lane, 'tool'))
  return { units, tools }
}

function battleItemRows(
  rows: unknown[],
  side: string,
  phase: string,
  lane: string,
  kind: 'unit' | 'tool'
): LocalBattleItemDetail[] {
  return rows.flatMap((raw): LocalBattleItemDetail[] => {
    const row = arrayValue(raw)
    const wodID = numericValue(row[0])
    if (wodID <= 0) {
      return []
    }
    const amount = Math.abs(numericValue(row[1]))
    const delta = numericValue(row[2])
    const usedOrLost = delta < 0 ? Math.abs(delta) : 0
    const item: LocalBattleItemDetail = { side, phase, lane, wodID, amount }
    if (kind === 'tool') {
      item.used = usedOrLost
      return amount || usedOrLost ? [item] : []
    }
    item.lost = usedOrLost
    return amount || usedOrLost ? [item] : []
  })
}

function battleItemTotals(items: LocalBattleItemDetail[]): { started: number; lost: number } {
  return items.reduce(
    (totals, item) => ({
      started: totals.started + item.amount,
      lost: totals.lost + (item.lost ?? 0),
    }),
    { started: 0, lost: 0 }
  )
}

function aggregateBattleItems(items: LocalBattleItemDetail[]): LocalBattleItemDetail[] {
  const byKey = new Map<string, LocalBattleItemDetail>()
  items.forEach((item) => {
    const key = `${item.side}|${item.phase}|${item.wodID}`
    const existing = byKey.get(key)
    if (!existing) {
      byKey.set(key, { ...item, lane: '' })
      return
    }
    existing.amount += item.amount
    existing.lost = (existing.lost ?? 0) + (item.lost ?? 0)
    existing.used = (existing.used ?? 0) + (item.used ?? 0)
  })
  return Array.from(byKey.values()).sort((a, b) => {
    const left = (a.lost ?? 0) + (a.used ?? 0)
    const right = (b.lost ?? 0) + (b.used ?? 0)
    return right - left || b.amount - a.amount
  })
}

function sideRoleFromOID(report: LocalParsedReport, oid: number): string {
  if (report.attacker?.playerID === oid) {
    return 'attacker'
  }
  if (report.defender?.playerID === oid || oid < 0) {
    return 'defender'
  }
  return ''
}

function reportHasBothPlayers(report: LocalParsedReport): boolean {
  return Boolean(report.attacker?.playerID && report.defender?.playerID)
}

function inferBattleResult(metrics: LocalMetrics): string {
  if (metrics.attackerSent > 0 && metrics.attackerLost >= metrics.attackerSent) {
    return 'Defeat'
  }
  return 'Victory'
}

function inferLaneResult(lane: LocalWaveLane): string {
  if (lane.defenderStart > 0 && lane.defenderLost < lane.defenderStart) {
    return 'HELD'
  }
  if (lane.defenderStart > 0 && lane.defenderLost >= lane.defenderStart) {
    return 'BREACHED'
  }
  if (lane.attackerStart > 0 && lane.attackerLost < lane.attackerStart) {
    return 'BREACHED'
  }
  return 'HELD'
}

function captureID(mid: number, lid: number): string {
  return `${mid}-${lid || mid}`
}

function cleanBattleLocation(value: string): string {
  const trimmed = value.trim()
  if (!trimmed) {
    return ''
  }
  const parts = trimmed.split('+')
  const last = parts[parts.length - 1]?.trim()
  return last && !/^-?\d+$/.test(last) ? last : trimmed
}

function laneName(index: number): string {
  return ['left', 'middle', 'right'][index] ?? `lane-${index + 1}`
}

interface LocalEffectMeta {
  label: string
  template: string
  unit: 'percent' | 'number' | 'flag'
  category: string
  order: number
  negative?: boolean
}

function battleEffectMeta(id: number): LocalEffectMeta {
  return battleEffectMetadata[id] ?? {
    label: `Effect ${id}`,
    template: `Effect ${id}`,
    unit: 'percent',
    category: 'Other effects',
    order: 900,
  }
}

function battleEffect(
  label: string,
  template: string,
  unit: LocalEffectMeta['unit'],
  category: string,
  order: number
): LocalEffectMeta {
  return { label, template, unit, category, order }
}

function battleEffectNegative(
  label: string,
  template: string,
  unit: LocalEffectMeta['unit'],
  category: string,
  order: number
): LocalEffectMeta {
  return { label, template, unit, category, order, negative: true }
}

function battleEffectFlag(label: string, template: string, category: string, order: number): LocalEffectMeta {
  return { label, template, unit: 'flag', category, order }
}

const battleEffectMetadata: Record<number, LocalEffectMeta> = {
  61: battleEffect('Melee unit strength', 'melee unit strength when attacking', 'percent', 'Unit effects', 10),
  411: battleEffect('Melee units attack strength', 'melee units attack strength', 'percent', 'Unit effects', 10),
  613: battleEffect('Melee unit strength', 'melee unit strength when attacking', 'percent', 'Unit effects', 10),
  62: battleEffect('Ranged unit strength', 'ranged unit strength when attacking', 'percent', 'Unit effects', 11),
  412: battleEffect('Ranged units attack strength', 'ranged units attack strength', 'percent', 'Unit effects', 11),
  614: battleEffect('Ranged unit strength', 'ranged unit strength when attacking', 'percent', 'Unit effects', 11),
  410: battleEffect('Courtyard attack combat strength', 'courtyard attack combat strength', 'percent', 'Attack effects', 12),
  504: battleEffect('Courtyard attack strength', 'combat strength when attacking enemy courtyards', 'percent', 'Attack effects', 12),
  386: battleEffect('Courtyard attack strength', 'combat strength when attacking enemy courtyards', 'percent', 'Attack effects', 12),
  423: battleEffect('Front combat strength', 'combat strength on the front when attacking', 'percent', 'Attack effects', 13),
  424: battleEffect('Flank combat strength', 'combat strength on the flanks when attacking', 'percent', 'Attack effects', 14),
  503: battleEffect('Front unit limit', 'unit limit on the front', 'percent', 'Attack effects', 20),
  369: battleEffect('Front unit limit', 'unit limit on the front', 'percent', 'Attack effects', 20),
  66: battleEffect('Flank unit limit', 'unit limit on the flanks', 'percent', 'Attack effects', 21),
  368: battleEffect('Flank unit limit', 'flank unit limit when attacking', 'percent', 'Attack effects', 21),
  700: battleEffect('Final assault capacity', 'to troop capacity for final assault', 'number', 'Courtyard effects', 22),
  701: battleEffect('Final assault capacity', 'to troop capacity for final assault', 'percent', 'Courtyard effects', 23),
  512: battleEffect('Courtyard support strength', 'courtyard support unit strength', 'percent', 'Courtyard effects', 24),
  426: battleEffect('Army travel speed', 'army travel speed', 'percent', 'Pre-battle effects', 30),
  53: battleEffect('Army travel speed', 'army travel speed', 'percent', 'Pre-battle effects', 30),
  97: battleEffect('Travel speed', 'Military, espionage and trade travel speed', 'percent', 'Pre-battle effects', 30),
  19: battleEffect('Attack travel speed', 'Attack travel speed', 'percent', 'Pre-battle effects', 31),
  55: battleEffect('Later army detection', 'later army detection', 'percent', 'Pre-battle effects', 32),
  111: battleEffect('Loot capacity', 'loot capacity', 'percent', 'Post-battle effects', 40),
  431: battleEffect('Resources plundered', 'resources plundered when looting', 'percent', 'Post-battle effects', 41),
  54: battleEffect('Resources plundered', 'resources plundered when looting', 'percent', 'Post-battle effects', 41),
  51: battleEffect('Glory earned', 'glory points earned when attacking', 'percent', 'Post-battle effects', 42),
  100: battleEffect('Glory bonus', 'Glory bonus', 'percent', 'Post-battle effects', 42),
  45: battleEffect('Glory earned', 'glory points earned when attacking', 'percent', 'Post-battle effects', 42),
  22: battleEffect('Glory earned', 'glory points earned when attacking', 'percent', 'Post-battle effects', 42),
  52: battleEffect('Honor earned', 'honor points earned in battle', 'percent', 'Post-battle effects', 43),
  82: battleEffect('XP earned', 'XP earned in battle', 'percent', 'Post-battle effects', 44),
  112: battleEffect('XP earned', 'XP earned in battle', 'percent', 'Post-battle effects', 44),
  43: battleEffect('Coin loot', 'Coins looted from NPC targets', 'percent', 'Post-battle effects', 45),
  60: battleEffect('Equipment find', 'chance of finding better equipment', 'percent', 'Post-battle effects', 46),
  48: battleEffect('Attack strength bonus', 'combat strength when attacking', 'percent', 'Attack effects', 49),
  20: battleEffect('Alliance attack strength', 'Combat strength bonus for attacks', 'percent', 'Attack effects', 50),
  25: battleEffect('Event target attack strength', 'Combat strength bonus against Foreign and Bloodcrow castles', 'percent', 'Attack effects', 51),

  339: battleEffect('Melee defense', 'combat strength for melee units when defending', 'percent', 'Unit effects', 110),
  10: battleEffect('Melee defense', 'combat strength for melee units when defending', 'percent', 'Unit effects', 110),
  340: battleEffect('Ranged defense', 'combat strength for ranged units when defending', 'percent', 'Unit effects', 111),
  11: battleEffect('Ranged defense', 'combat strength for ranged units when defending', 'percent', 'Unit effects', 111),
  370: battleEffect('Courtyard defense', 'combat strength when defending the courtyard', 'percent', 'Courtyard effects', 112),
  501: battleEffect('Courtyard defense', 'combat strength when defending the courtyard', 'percent', 'Courtyard effects', 112),
  509: battleEffect('Front defense', 'combat strength on the front when defending', 'percent', 'Defense unit effects', 113),
  510: battleEffect('Flank defense', 'combat strength on the flanks when defending', 'percent', 'Defense unit effects', 114),
  420: battleEffect('Wall unit limit', 'unit limit on the wall', 'number', 'Defense unit effects', 120),
  387: battleEffect('Wall unit limit', 'to troop capacity on wall defense', 'percent', 'Defense unit effects', 120),
  702: battleEffect('Courtyard defense capacity', 'to troop capacity in courtyard defense', 'number', 'Courtyard effects', 121),
  371: battleEffect('Courtyard defense capacity', 'to troop capacity in courtyard defense', 'number', 'Courtyard effects', 121),
  705: battleEffect('Courtyard defense capacity', 'to troop capacity in courtyard defense', 'percent', 'Courtyard effects', 121),
  706: battleEffect('Alliance support capacity', 'to alliance support troop capacity', 'number', 'Courtyard effects', 122),
  385: battleEffect('Alliance support capacity', 'to alliance support troop capacity', 'number', 'Courtyard effects', 122),
  427: battleEffect('Surviving soldiers', 'more surviving soldiers after defense', 'percent', 'Post-battle effects', 123),
  428: battleEffect('Sight radius', 'Sight Radius', 'percent', 'Pre-battle effects', 124),
  4: battleEffect('Earlier attack warning', 'earlier attack warning', 'percent', 'Pre-battle effects', 125),
  5: battleEffectNegative('Fire damage suffered', 'fire damage suffered when defending', 'percent', 'Post-battle effects', 126),
  429: battleEffectNegative('Fire damage suffered', 'fire damage suffered', 'percent', 'Post-battle effects', 126),
  94: battleEffect('Militia in the castle', 'militia in the castle', 'number', 'Pre-battle effects', 127),
  115: battleEffectFlag('Militia replacement', 'Replaces the armed citizen with Militia', 'Pre-battle effects', 128),
  1: battleEffect('Glory defense bonus', 'glory points earned when defending', 'percent', 'Post-battle effects', 129),
  21: battleEffect('Alliance defense strength', 'Combat strength bonus for defense', 'percent', 'Defense unit effects', 130),
  27: battleEffect('Khan defense strength', 'Combat strength bonus against Khan attacks', 'percent', 'Defense unit effects', 131),
}

function formatBattleEffectValue(value: number, meta: LocalEffectMeta): string {
  if (meta.unit === 'flag') {
    return ''
  }
  const normalized = meta.negative ? -Math.abs(value) : value
  const prefix = normalized > 0 ? '+' : ''
  if (meta.unit === 'number') {
    return `${prefix}${Math.round(normalized)}`
  }
  return `${prefix}${round(normalized, 1).toFixed(1)}%`
}

function formatBattleEffectText(value: number, meta: LocalEffectMeta): string {
  if (meta.unit === 'flag') {
    return meta.template
  }
  return `${formatBattleEffectValue(value, meta)} ${meta.template}`.trim()
}

function battleEffectValue(id: number, values: number[]): number {
  if (values.length === 0) {
    return 0
  }
  if (values.length > 1 && values.length % 2 === 0 && values[0] > 100) {
    const total = values.reduce((sum, value, index) => index % 2 === 1 ? sum + value : sum, 0)
    if (total !== 0) {
      return total
    }
  }
  return values[0]
}

function battleEffectValuesFromValue(value: unknown): number[] {
  const record = recordValue(value)
  if (record) {
    return numericArrayValue(record.value)
  }
  return numericArrayValue(value)
}

function numericArrayValue(value: unknown): number[] {
  return arrayValue(value).map((entry) => numericValue(entry))
}

function numericValue(value: unknown): number {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value
  }
  if (typeof value === 'string' && value.trim()) {
    const parsed = Number(value)
    return Number.isFinite(parsed) ? parsed : 0
  }
  return 0
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function recordValue(value: unknown): JsonRecord | null {
  return typeof value === 'object' && value !== null && !Array.isArray(value) ? value as JsonRecord : null
}

function arrayValue(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

function isNumberLike(value: unknown): boolean {
  return typeof value === 'number' || (typeof value === 'string' && value.trim() !== '' && Number.isFinite(Number(value)))
}

function round(value: number, decimals: number): number {
  const factor = 10 ** decimals
  return Math.round(value * factor) / factor
}

// https://vite.dev/config/
export default defineConfig({
  plugins: [localBattleReportsPlugin(), react(), tailwindcss()],
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/ws': {
        target: 'ws://localhost:8080',
        ws: true,
        changeOrigin: true,
      },
    }
  }
})
