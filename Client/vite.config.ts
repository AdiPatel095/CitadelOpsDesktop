import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

const projectDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')

const battleReportRoutes = new Set([
  '/api/battle-reports',
  '/api/battleReports',
  '/api/reports/battle',
  '/Data/BattleReports.jsonl',
  '/BattleReports.jsonl',
])

const cloudBattleReportRoutes = new Set([
  '/api/battle-reports/cloud',
  '/api/battleReports/cloud',
])

function localBattleReportsPlugin(): Plugin {
  return {
    name: 'citadel-local-battle-reports',
    configureServer(server) {
      let announcedSource = ''

      server.middlewares.use((req, res, next) => {
        const url = new URL(req.url ?? '/', 'http://localhost')
        if (cloudBattleReportRoutes.has(url.pathname)) {
          fetchCloudBattleReports()
            .then(({ body, status, contentType }) => {
              res.statusCode = status
              res.setHeader('Content-Type', contentType)
              res.setHeader('Cache-Control', 'no-store')
              res.end(body)
            })
            .catch(() => {
              res.statusCode = 404
              res.setHeader('Content-Type', 'application/json; charset=utf-8')
              res.end(JSON.stringify({ error: 'No cloud Battle Reports endpoint available.' }))
            })
          return
        }

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

async function fetchCloudBattleReports(): Promise<{ body: string; status: number; contentType: string }> {
  const response = await fetch(cloudBattleReportsURL(), {
    headers: cloudBattleReportHeaders(),
  })
  const body = await response.text()
  if (response.ok) {
    const reports = parseCloudBattleReports(body)
    return {
      body: JSON.stringify({ reports }),
      status: 200,
      contentType: 'application/json; charset=utf-8',
    }
  }
  return {
    body,
    status: response.status,
    contentType: response.headers.get('content-type') || 'application/json; charset=utf-8',
  }
}

function parseCloudBattleReports(text: string): LocalParsedReport[] {
  const trimmed = text.trim()
  if (!trimmed) {
    return []
  }
  try {
    return parsedCloudReportsFromUnknown(JSON.parse(trimmed))
  } catch {
    return []
  }
}

function parsedCloudReportsFromUnknown(value: unknown): LocalParsedReport[] {
  if (Array.isArray(value)) {
    return value.flatMap(parsedCloudReportsFromUnknown)
  }
  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (!trimmed) {
      return []
    }
    try {
      return parsedCloudReportsFromUnknown(JSON.parse(trimmed))
    } catch {
      return []
    }
  }
  const record = recordValue(value)
  if (!record) {
    return []
  }
  const nestedReports = Array.isArray(record.reports)
    ? record.reports
    : Array.isArray(record.data)
      ? record.data
      : Array.isArray(record.items)
        ? record.items
        : null
  if (nestedReports) {
    return nestedReports.flatMap(parsedCloudReportsFromUnknown)
  }
  if (typeof record.payload === 'string') {
    return parsedCloudReportsFromUnknown(record.payload)
  }
  const parsed = recordValue(record.parsed) ?? recordValue(record.parsedReport) ?? recordValue(record.report)
  if (parsed && reportHasBothPlayers(parsed as LocalParsedReport)) {
    return [parsed as LocalParsedReport]
  }
  const report = parseLocalBattleReportCapture(record)
  return reportHasBothPlayers(report) ? [report] : []
}

function cloudBattleReportsURL(): string {
  const explicitFetchURL = process.env.BATTLE_REPORTS_FETCH_URL
  const explicitUploadURL = process.env.BATTLE_REPORTS_UPLOAD_URL
  const base = process.env.CLOUD_BACKEND_URL || 'https://citadelops.app/api'
  return explicitFetchURL || explicitUploadURL || `${base.replace(/\/+$/, '')}/reports/battle`
}

function cloudBattleReportHeaders(): Record<string, string> {
  const headers: Record<string, string> = {}
  const reportKey = process.env.REPORT_UPLOAD_KEY || process.env.CITADEL_REPORT_UPLOAD_KEY
  if (reportKey) {
    headers['X-Citadel-Report-Key'] = reportKey
  }
  return headers
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
    path.join(projectDir, 'BattleReports.jsonl'),
    path.join(process.cwd(), 'public', 'Data', 'BattleReports.jsonl'),
    path.join(process.cwd(), 'Data', 'BattleReports.jsonl'),
    path.join(projectDir, 'Data', 'BattleReports.jsonl'),
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
  targetX?: number
  targetY?: number
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
  report.result = inferBattleResult(report)
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
    if (isNumberLike(ai.X) && isNumberLike(ai.Y)) {
      report.targetX = numericValue(ai.X)
      report.targetY = numericValue(ai.Y)
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
  const combatMode = reportBattleEffectCombatMode(report)
  report.commanderEffects = effectsFromLeader(bls.AL, 'commander', combatMode)
  report.castellanEffects = effectsFromLeader(bls.DB, 'castellan', combatMode)
  report.effects = [...report.commanderEffects, ...report.castellanEffects]
}

function reportBattleEffectCombatMode(report: LocalParsedReport): BattleEffectCombatMode {
  return report.attacker?.playerID && report.defender?.playerID ? 'pvp' : 'pve'
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

function effectsFromLeader(value: unknown, side: string, combatMode: BattleEffectCombatMode): LocalEffect[] {
  const leader = recordValue(value)
  if (!leader) {
    return []
  }

  const grouped = new Map<string, {
    effect: LocalEffect
    meta: LocalEffectMeta
    sources: Map<string, { value: number; cap?: number }>
  }>()
  leaderEffectRows(leader, combatMode).forEach((row) => {
    const meta = battleEffectMeta(row.id, side, row.source, row.mapped)
    if (meta.skip || (meta.unknown && row.source !== 'active')) {
      return
    }

    const value = battleEffectValue(row.id, row.values) * (meta.scale ?? 1)
    if (value === 0) {
      return
    }

    const key = `${meta.label}|${meta.unit}|${meta.category}|${meta.template}`
    const capKey = row.cap ? `${row.id}:${row.capKey}` : `${row.id}:${row.source}:${row.rawID}:${row.index}`
    const existing = grouped.get(key)
    if (existing) {
      existing.effect.code = `${existing.effect.code},${row.rawID}`
      const source = existing.sources.get(capKey)
      if (source) {
        source.value += value
      } else {
        existing.sources.set(capKey, { value, cap: row.cap })
      }
      if (meta.order < existing.effect.sortOrder) {
        existing.effect.sortOrder = meta.order
      }
      return
    }

    grouped.set(key, {
      meta,
      effect: {
        code: String(row.rawID),
        label: meta.label,
        name: meta.label,
        value: 0,
        formattedValue: '',
        displayText: '',
        category: meta.category,
        sortOrder: meta.order,
        side,
      },
      sources: new Map([[capKey, { value, cap: row.cap }]]),
    })
  })

  return Array.from(grouped.values())
    .map(({ effect, meta, sources }) => {
      const value = round(Array.from(sources.values()).reduce((total, source) => {
        const capped = source.cap ? Math.min(Math.abs(source.value), source.cap) * Math.sign(source.value) : source.value
        return total + capped
      }, 0), 1)
      return {
        ...effect,
        value,
        formattedValue: formatBattleEffectValue(value, meta),
        displayText: formatBattleEffectText(value, meta),
      }
    })
    .sort((a, b) => a.sortOrder - b.sortOrder || a.label.localeCompare(b.label))
}

type BattleEffectCombatMode = 'pvp' | 'pve'
type BattleEffectSource = 'active' | 'equipment' | 'gem' | 'set'

interface LocalLeaderEffectRow {
  id: number
  rawID: number
  values: number[]
  source: BattleEffectSource
  mapped: boolean
  index: number
  cap?: number
  capKey: string
}

interface LocalBattleEffectDefinition {
  effectID: number
  capID: number
  name: string
  effectTypeID: number
  cap?: number
  isPvp: boolean
  isPve: boolean
}

interface LocalBattleEffectData {
  equipmentEffectIDs: Map<number, number>
  effectDefinitions: Map<number, LocalBattleEffectDefinition>
  relicEffectIDs: Map<number, number>
  equipmentEffects: Map<number, LocalCatalogEffect[]>
  gemEffects: Map<number, LocalCatalogEffect[]>
  equipmentSetEffects: Map<number, LocalEquipmentSetEffect[]>
}

interface LocalCatalogEffect {
  id: number
  values: number[]
}

interface LocalEquipmentSetEffect {
  needed: number
  effects: LocalCatalogEffect[]
}

let cachedBattleEffectData: LocalBattleEffectData | null = null

function leaderEffectRows(leader: JsonRecord, combatMode: BattleEffectCombatMode): LocalLeaderEffectRow[] {
  const rows: LocalLeaderEffectRow[] = []
  let index = 0

  arrayValue(leader.AE).forEach((raw) => {
    const parsed = effectRowFromArray(arrayValue(raw), 'active', combatMode, index)
    index += 1
    if (parsed) {
      rows.push(parsed)
    }
  })

  const setCounts = new Map<number, number>()
  arrayValue(leader.EQ).forEach((raw) => {
    const item = arrayValue(raw)
    const setID = numericValue(item[7])
    if (setID > 0) {
      setCounts.set(setID, (setCounts.get(setID) ?? 0) + 1)
    }

    const equipmentID = numericValue(item[6])
    const catalogEffects = battleEffectData().equipmentEffects.get(equipmentID) ?? []
    arrayValue(item[5]).forEach((effectRaw, effectIndex) => {
      const effectRow = arrayValue(effectRaw)
      const catalogEffect = catalogEffects[effectIndex]
      const rawID = catalogEffect?.id ?? numericValue(effectRow[0])
      const values = catalogEffect ? valuesFromCatalogEffect(catalogEffect, effectRow) : battleEffectValuesFromRow(effectRow)
      const parsed = normalizedLeaderEffectRow(rawID, values, 'equipment', combatMode, index, true)
      index += 1
      if (parsed) {
        rows.push(parsed)
      }
    })

    const catalogGemID = numericValue(item[10])
    battleEffectData().gemEffects.get(catalogGemID)?.forEach((effect) => {
      const parsed = normalizedLeaderEffectRow(effect.id, effect.values, 'gem', combatMode, index)
      index += 1
      if (parsed) {
        rows.push(parsed)
      }
    })

    arrayValue(arrayValue(arrayValue(item[12])[3])[4]).forEach((effectRaw) => {
      const effectRow = arrayValue(effectRaw)
      const parsed = normalizedLeaderEffectRow(
        numericValue(effectRow[0]),
        battleEffectValuesFromRow(effectRow),
        'gem',
        combatMode,
        index
      )
      index += 1
      if (parsed) {
        rows.push(parsed)
      }
    })
  })

  setCounts.forEach((count, setID) => {
    battleEffectSetEffects(setID, count).forEach((effect) => {
      const parsed = normalizedLeaderEffectRow(effect.id, effect.values, 'set', combatMode, index)
      index += 1
      if (parsed) {
        rows.push(parsed)
      }
    })
  })

  return rows
}

function effectRowFromArray(
  row: unknown[],
  source: BattleEffectSource,
  combatMode: BattleEffectCombatMode,
  index: number
): LocalLeaderEffectRow | null {
  return normalizedLeaderEffectRow(numericValue(row[0]), battleEffectValuesFromRow(row), source, combatMode, index)
}

function normalizedLeaderEffectRow(
  rawID: number,
  values: number[],
  source: BattleEffectSource,
  combatMode: BattleEffectCombatMode,
  index: number,
  mapCatalogIDs = true
): LocalLeaderEffectRow | null {
  if (!rawID || values.length === 0) {
    return null
  }

  const data = battleEffectData()
  let id = rawID
  let mapped = false
  if (source !== 'active' && mapCatalogIDs) {
    id = data.relicEffectIDs.get(rawID) ?? data.equipmentEffectIDs.get(rawID) ?? rawID
    mapped = id !== rawID
  }

  let definition = data.effectDefinitions.get(id)
  if (source !== 'active' && !mapCatalogIDs) {
    definition = undefined
  }
  if (definition?.isPvp && combatMode !== 'pvp') {
    return null
  }
  if (definition?.isPve && combatMode !== 'pve') {
    return null
  }

  return {
    id,
    rawID,
    values,
    source,
    mapped,
    index,
    cap: battleEffectCap(id, definition, source),
    capKey: definition ? `${definition.effectID}:${definition.capID}` : `${id}`,
  }
}

function battleEffectValuesFromRow(row: unknown[]): number[] {
  if (Array.isArray(row[2])) {
    return numericArrayValue(row[2])
  }
  return battleEffectValuesFromValue(row[1])
}

function valuesFromCatalogEffect(effect: LocalCatalogEffect, row: unknown[]): number[] {
  const rawValues = battleEffectValuesFromRow(row)
  if (rawValues.length > 0) {
    return rawValues
  }
  return effect.values
}

function battleEffectCap(
  id: number,
  definition: LocalBattleEffectDefinition | undefined,
  source: BattleEffectSource
): number | undefined {
  if (source === 'active' && !battleEffectActiveCaps.has(id)) {
    return undefined
  }
  if (id in battleEffectCapOverrides) {
    return battleEffectCapOverrides[id]
  }
  return definition?.cap
}

function battleEffectData(): LocalBattleEffectData {
  if (cachedBattleEffectData) {
    return cachedBattleEffectData
  }

  const caps = new Map<number, number>()
  readDataArray('effect_caps/items.json').forEach((entry) => {
    const capID = numericValue(entry.capID)
    const max = numericValue(entry.maxTotalBonus)
    if (capID > 0 && max > 0) {
      caps.set(capID, max)
    }
  })

  const effectDefinitions = new Map<number, LocalBattleEffectDefinition>()
  readDataArray('effects/items.json').forEach((entry) => {
    const effectID = numericValue(entry.effectID)
    if (!effectID) {
      return
    }
    const name = stringValue(entry.name)
    const capID = numericValue(entry.capID)
    effectDefinitions.set(effectID, {
      effectID,
      capID,
      name,
      effectTypeID: numericValue(entry.effectTypeID),
      cap: caps.get(capID),
      isPvp: stringValue(entry.isPvPFight) === '1' || /pvp/i.test(name),
      isPve: stringValue(entry.isPvEFight) === '1' || /pve/i.test(name),
    })
  })

  const equipmentEffectIDs = new Map<number, number>()
  readDataArray('equipment_effects/items.json').forEach((entry) => {
    const equipmentEffectID = numericValue(entry.equipmentEffectID)
    const effectID = numericValue(entry.effectID)
    if (equipmentEffectID > 0 && effectID > 0) {
      equipmentEffectIDs.set(equipmentEffectID, effectID)
    }
  })

  const relicEffectIDs = new Map<number, number>()
  readDataArray('relic_effects/items.json').forEach((entry) => {
    const relicEffectID = numericValue(entry.id)
    const effectID = numericValue(entry.effectID)
    if (relicEffectID > 0 && effectID > 0) {
      relicEffectIDs.set(relicEffectID, effectID)
    }
  })

  const equipmentEffects = new Map<number, LocalCatalogEffect[]>()
  readDataArray('equipments/items.json').forEach((entry) => {
    const equipmentID = numericValue(entry.equipmentID)
    const effects = catalogEffectsFromString(stringValue(entry.effects))
    if (equipmentID > 0 && effects.length > 0) {
      equipmentEffects.set(equipmentID, effects)
    }
  })

  const gemEffects = new Map<number, LocalCatalogEffect[]>()
  readDataArray('gems/items.json').forEach((entry) => {
    const gemID = numericValue(entry.gemID)
    const effects = catalogEffectsFromString(stringValue(entry.effects))
    if (gemID > 0 && effects.length > 0) {
      gemEffects.set(gemID, effects)
    }
  })

  const equipmentSetEffects = new Map<number, LocalEquipmentSetEffect[]>()
  readDataArray('equipment_sets/items.json').forEach((entry) => {
    const setID = numericValue(entry.setID)
    const needed = numericValue(entry.neededItems)
    const effects = catalogEffectsFromString(stringValue(entry.effects))
    if (setID > 0 && needed > 0 && effects.length > 0) {
      const existing = equipmentSetEffects.get(setID) ?? []
      existing.push({ needed, effects })
      equipmentSetEffects.set(setID, existing)
    }
  })

  cachedBattleEffectData = {
    equipmentEffectIDs,
    effectDefinitions,
    relicEffectIDs,
    equipmentEffects,
    gemEffects,
    equipmentSetEffects,
  }
  return cachedBattleEffectData
}

function battleEffectSetEffects(setID: number, equippedCount: number): LocalCatalogEffect[] {
  if (setID <= 0 || equippedCount <= 0) {
    return []
  }
  return (battleEffectData().equipmentSetEffects.get(setID) ?? []).flatMap((entry) =>
    entry.needed <= equippedCount ? entry.effects : []
  )
}

function catalogEffectsFromString(value: string): LocalCatalogEffect[] {
  if (!value) {
    return []
  }
  return value.split(',').flatMap((effect) => {
    const [rawID, rawValues] = effect.split('&')
    const id = numericValue(rawID)
    const values = stringValue(rawValues)
      .split('#')
      .flatMap((part) => part.split('+'))
      .map((part) => numericValue(part))
      .filter((part) => part !== 0)
    return id > 0 && values.length > 0 ? [{ id, values }] : []
  })
}

function readDataArray(relativePath: string): JsonRecord[] {
  try {
    const text = fs.readFileSync(path.join(projectDir, 'Server', 'Data', relativePath), 'utf8')
    const parsed = JSON.parse(text)
    return Array.isArray(parsed) ? parsed.flatMap((entry) => {
      const record = recordValue(entry)
      return record ? [record] : []
    }) : []
  } catch {
    return []
  }
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

function inferBattleResult(report: LocalParsedReport): string {
  const waveResult = inferBattleResultFromWaves(report.waves)
  if (waveResult) {
    return waveResult
  }
  return inferBattleResultFromMetrics(report.metrics)
}

function inferBattleResultFromWaves(waves: LocalWave[]): string {
  let hasAttackLane = false
  for (const wave of waves) {
    for (const lane of wave.lanes ?? []) {
      if ((lane.attackerStart ?? 0) <= 0 && (lane.attackerLost ?? 0) <= 0) {
        continue
      }
      hasAttackLane = true
      if (inferLaneResult(lane) === 'BREACHED') {
        return 'Victory'
      }
    }
  }
  return hasAttackLane ? 'Defeat' : ''
}

function inferBattleResultFromMetrics(metrics: LocalMetrics): string {
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
  scale?: number
  skip?: boolean
  unknown?: boolean
}

function battleEffectMeta(id: number, side: string, source: BattleEffectSource, mapped: boolean): LocalEffectMeta {
  if (source !== 'active' && mapped) {
    const mappedMeta = battleEffectMetadata[id] ?? battleOfficialEffectMeta(id)
    if (mappedMeta) {
      return mappedMeta
    }
  }
  if (side === 'commander' && source !== 'active' && id in battleCommanderLiveEffectMetadata) {
    return battleCommanderLiveEffectMetadata[id]
  }
  return battleEffectMetadata[id] ?? battleOfficialEffectMeta(id) ?? {
    label: `Effect ${id}`,
    template: `Effect ${id}`,
    unit: 'percent',
    category: 'Other effects',
    order: 900,
    unknown: true,
  }
}

function battleOfficialEffectMeta(id: number): LocalEffectMeta | null {
  const definition = battleEffectData().effectDefinitions.get(id)
  if (!definition) {
    return null
  }
  const name = definition.name.toLowerCase()
  if (name.includes('offensivemeleebonus')) {
    return battleEffect('Combat strength for melee units', 'combat strength for melee units', 'percent', 'Unit effects', 10)
  }
  if (name.includes('offensiverangebonus')) {
    return battleEffect('Combat strength for ranged units', 'combat strength for ranged units', 'percent', 'Unit effects', 11)
  }
  if (name.includes('attackboostyard')) {
    return battleEffect('Combat strength when attacking enemy courtyard', 'combat strength when attacking enemy courtyard', 'percent', 'Unit effects', 12)
  }
  if (name.includes('attackunitamountfront')) {
    return battleEffect('Front unit limit', 'unit limit on the front', 'percent', 'Attack effects', 20)
  }
  if (name.includes('attackunitamountflank')) {
    return battleEffect('Flank unit limit', 'unit limit on the flanks', 'percent', 'Attack effects', 21)
  }
  if (battleOfficialEffectIsAttackSupportUnits(definition, name)) {
    return battleEffect('Attack support units', 'attack support units', 'number', 'Attack effects', 24)
  }
  if (battleOfficialEffectIsUnitSpecificBonus(definition, name)) {
    return battleEffect('Unit-specific attack strength', 'attack strength for matching units', 'number', 'Unit effects', 25)
  }
  if (name.includes('attackbonus')) {
    return battleEffect('Unit combat strength when attacking', 'unit combat strength when attacking', 'percent', 'Attack effects', 15)
  }
  if (name.includes('wallreduction')) {
    return battleEffectNegative('Wall protection', 'wall protection', 'percent', 'Defense structure effects', 16)
  }
  if (name.includes('gatereduction')) {
    return battleEffectNegative('Gate protection', 'gate protection', 'percent', 'Defense structure effects', 17)
  }
  if (name.includes('moatreduction')) {
    return battleEffectNegative('Moat protection', 'moat protection', 'percent', 'Defense structure effects', 18)
  }
  if (name.includes('melee') && name.includes('defense')) {
    return battleEffect('Combat strength for defensive melee units', 'combat strength bonus for defensive melee units', 'percent', 'Unit effects', 110)
  }
  if (name.includes('range') && name.includes('defense')) {
    return battleEffect('Combat strength for defensive ranged units', 'combat strength bonus for defensive ranged units', 'percent', 'Unit effects', 111)
  }
  if (name.includes('defenseboostyard')) {
    return battleEffect('Combat strength when defending the courtyard', 'combat strength when defending the courtyard', 'percent', 'Unit effects', 112)
  }
  if (name.includes('defenseunitamountwall')) {
    return battleEffect('Wall unit limit', 'to troop capacity on wall defense', 'percent', 'Defense unit effects', 120)
  }
  if (battleOfficialEffectIsDefenseSupportUnits(definition, name)) {
    return battleEffect('Defense support units', 'defense support units', 'number', 'Defense unit effects', 122)
  }
  if (name.includes('wallbonus')) {
    return battleEffect('Wall protection', 'wall protection', 'percent', 'Defense structure effects', 115)
  }
  if (name.includes('gatebonus')) {
    return battleEffect('Gate protection', 'gate protection', 'percent', 'Defense structure effects', 116)
  }
  if (name.includes('moatbonus')) {
    return battleEffect('Moat protection', 'moat protection', 'percent', 'Defense structure effects', 117)
  }
  if (name.includes('supporttroopcapacity')) {
    return battleEffect('Alliance support capacity', 'to alliance support troop capacity', 'number', 'Courtyard effects', 122)
  }
  if (name.includes('troopcapacity') || name.includes('defensecapacity')) {
    return battleEffect('Courtyard defense capacity', 'to troop capacity in courtyard defense', 'number', 'Courtyard effects', 121)
  }
  if (name.includes('additionalwaves')) {
    return battleEffect('Additional waves', 'additional wave(s)', 'number', 'Pre-battle effects', 29)
  }
  if (name.includes('returntravelboost')) {
    return battleEffect('Army return travel speed', 'army return travel speed', 'percent', 'Post-battle effects', 33)
  }
  if (name.includes('speedbonus') || name.includes('travelspeed')) {
    return battleEffect('Army travel speed', 'army travel speed', 'percent', 'Pre-battle effects', 30)
  }
  if (name.includes('stealthbonus')) {
    return battleEffect('Later army detection', 'later army detection', 'percent', 'Pre-battle effects', 32)
  }
  if (name.includes('perceptionbonus')) {
    return battleEffect('Earlier attack warning', 'earlier attack warning', 'percent', 'Pre-battle effects', 125)
  }
  if (name.includes('lootbonus')) {
    return battleEffect('Loot capacity', 'loot capacity', 'percent', 'Post-battle effects', 40)
  }
  if (name.includes('resourcesplundered')) {
    return battleEffect('Resources plundered', 'resources plundered when looting', 'percent', 'Post-battle effects', 41)
  }
  if (name.includes('honor')) {
    return battleEffect('Honor earned', 'honor points earned in battle', 'percent', 'Post-battle effects', 43)
  }
  if (name.includes('xp')) {
    return battleEffect('XP earned', 'XP earned in battle', 'percent', 'Post-battle effects', 44)
  }
  if (name.includes('fire')) {
    return battleEffectNegative('Fire damage suffered', 'fire damage suffered when defending', 'percent', 'Defense structure effects', 126)
  }
  if (name.includes('resourcelost') || name.includes('lootreduction')) {
    return battleEffectNegative('Resources lost', 'resources lost after being looted', 'percent', 'Post-battle effects', 126)
  }
  if (name.includes('cooldownreduction')) {
    return battleEffect('Cooldown reduction', 'cooldown reduction', 'percent', 'Post-battle effects', 45)
  }
  if (battleOfficialEffectIsEconomy(name)) {
    const label = humanizeBattleEffectName(definition.name)
    return label ? battleEffect(label, label.toLowerCase(), 'percent', 'Economy effects', 800) : null
  }
  const label = humanizeBattleEffectName(definition.name)
  return label
    ? battleEffect(label, label.toLowerCase(), 'percent', battleOfficialEffectCategory(name), 850)
    : null
}

function battleOfficialEffectCategory(name: string): string {
  if (/speed|stealth|warning|sight/.test(name)) {
    return 'Pre-battle effects'
  }
  if (/loot|honor|glory|fame|xp/.test(name)) {
    return 'Post-battle effects'
  }
  if (battleOfficialEffectIsEconomy(name)) {
    return 'Economy effects'
  }
  if (/wall|gate|moat|fire/.test(name)) {
    return 'Defense structure effects'
  }
  if (/capacity|yard|courtyard/.test(name)) {
    return 'Courtyard effects'
  }
  if (name.includes('attack')) {
    return 'Attack effects'
  }
  if (name.includes('defense')) {
    return 'Defense unit effects'
  }
  return 'Other effects'
}

function battleOfficialEffectIsEconomy(name: string): boolean {
  return /production|resource|research|collector|loyalty|publicorder/.test(name)
}

function battleOfficialEffectIsAttackSupportUnits(definition: LocalBattleEffectDefinition, name: string): boolean {
  return definition.effectTypeID === 51 || name.includes('attacksupportunits')
}

function battleOfficialEffectIsDefenseSupportUnits(definition: LocalBattleEffectDefinition, name: string): boolean {
  return definition.effectTypeID === 47 || name.includes('defensesupportunits')
}

function battleOfficialEffectIsUnitSpecificBonus(definition: LocalBattleEffectDefinition, name: string): boolean {
  return definition.effectTypeID === 148 || name.includes('attackbonusunit')
}

function humanizeBattleEffectName(name: string): string {
  const trimmed = name.trim().replace(/^relic/, '')
  if (!trimmed) {
    return ''
  }
  return trimmed
    .replace(/[_-]+/g, ' ')
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .split(/\s+/)
    .filter(Boolean)
    .map((word) => {
      const lower = word.toLowerCase()
      if (lower === 'pvp') {
        return 'PvP'
      }
      if (lower === 'pve') {
        return 'PvE'
      }
      return `${lower.slice(0, 1).toUpperCase()}${lower.slice(1)}`
    })
    .join(' ')
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

const battleEffectActiveCaps = new Set([5, 61, 62, 368, 369, 386, 410, 411, 412, 423, 424])

const battleEffectCapOverrides: Partial<Record<number, number>> = {
  473: 150,
}

const battleCommanderLiveEffectMetadata: Record<number, LocalEffectMeta> = {
  3: battleEffectNegative('Wall protection', 'wall protection', 'percent', 'Defense structure effects', 16),
  103: battleEffectNegative('Wall protection', 'wall protection', 'percent', 'Defense structure effects', 16),
  110: battleEffectNegative('Wall protection', 'wall protection', 'percent', 'Defense structure effects', 16),
  4: battleEffectNegative('Gate protection', 'gate protection', 'percent', 'Defense structure effects', 17),
  104: battleEffectNegative('Gate protection', 'gate protection', 'percent', 'Defense structure effects', 17),
  111: battleEffectNegative('Gate protection', 'gate protection', 'percent', 'Defense structure effects', 17),
  5: battleEffectNegative('Moat protection', 'moat protection', 'percent', 'Defense structure effects', 18),
  105: battleEffectNegative('Moat protection', 'moat protection', 'percent', 'Defense structure effects', 18),
  112: battleEffectNegative('Moat protection', 'moat protection', 'percent', 'Defense structure effects', 18),

  209: battleEffectNegative('Wall protection', 'wall protection against NPC targets', 'percent', 'Defense structure effects', 16),
  501: battleEffectNegative('Wall protection', 'wall protection against NPC targets', 'percent', 'Defense structure effects', 16),
  507: battleEffectNegative('Wall protection', 'wall protection against NPC targets', 'percent', 'Defense structure effects', 16),
  210: battleEffectNegative('Gate protection', 'gate protection against NPC targets', 'percent', 'Defense structure effects', 17),
  502: battleEffectNegative('Gate protection', 'gate protection against NPC targets', 'percent', 'Defense structure effects', 17),
  508: battleEffectNegative('Gate protection', 'gate protection against NPC targets', 'percent', 'Defense structure effects', 17),
  216: battleEffectNegative('Moat protection', 'moat protection against NPC targets', 'percent', 'Defense structure effects', 18),
  514: battleEffectNegative('Moat protection', 'moat protection against NPC targets', 'percent', 'Defense structure effects', 18),

  309: battleEffectNegative('Wall protection', 'wall protection of Castle Lords', 'percent', 'Defense structure effects', 16),
  808: battleEffectNegative('Wall protection', 'wall protection of Castle Lords', 'percent', 'Defense structure effects', 16),
  310: battleEffectNegative('Gate protection', 'gate protection of Castle Lords', 'percent', 'Defense structure effects', 17),
  809: battleEffectNegative('Gate protection', 'gate protection of Castle Lords', 'percent', 'Defense structure effects', 17),
  317: battleEffectNegative('Moat protection', 'moat protection of Castle Lords', 'percent', 'Defense structure effects', 18),
  807: battleEffectNegative('Moat protection', 'moat protection of Castle Lords', 'percent', 'Defense structure effects', 18),
}

const battleEffectMetadata: Record<number, LocalEffectMeta> = {
  61: battleEffect('Combat strength for melee units', 'combat strength for melee units', 'percent', 'Unit effects', 10),
  411: battleEffect('Combat strength for melee units', 'combat strength for melee units', 'percent', 'Unit effects', 10),
  613: battleEffect('Combat strength for melee units', 'combat strength for melee units', 'percent', 'Unit effects', 10),
  467: battleEffect('Combat strength for melee units', 'combat strength for melee units', 'percent', 'Unit effects', 10),
  62: battleEffect('Combat strength for ranged units', 'combat strength for ranged units', 'percent', 'Unit effects', 11),
  412: battleEffect('Combat strength for ranged units', 'combat strength for ranged units', 'percent', 'Unit effects', 11),
  614: battleEffect('Combat strength for ranged units', 'combat strength for ranged units', 'percent', 'Unit effects', 11),
  468: battleEffect('Combat strength for ranged units', 'combat strength for ranged units', 'percent', 'Unit effects', 11),
  410: battleEffect('Combat strength when attacking enemy courtyard', 'combat strength when attacking enemy courtyard', 'percent', 'Unit effects', 12),
  504: battleEffect('Combat strength when attacking enemy courtyard', 'combat strength when attacking enemy courtyard', 'percent', 'Unit effects', 12),
  386: battleEffect('Combat strength when attacking enemy courtyard', 'combat strength when attacking enemy courtyard', 'percent', 'Unit effects', 12),
  475: battleEffect('Combat strength when attacking enemy courtyard', 'combat strength when attacking enemy courtyard', 'percent', 'Unit effects', 12),
  423: battleEffect('Combat strength for the front', 'combat strength for the front when attacking', 'percent', 'Unit effects', 13),
  478: battleEffect('Combat strength for the front', 'combat strength for the front when attacking', 'percent', 'Unit effects', 13),
  424: battleEffect('Combat strength for the flanks', 'combat strength for the flanks when attacking', 'percent', 'Unit effects', 14),
  479: battleEffect('Combat strength for the flanks', 'combat strength for the flanks when attacking', 'percent', 'Unit effects', 14),
  477: battleEffect('Unit combat strength when attacking', 'unit combat strength when attacking', 'percent', 'Attack effects', 15),
  469: battleEffectNegative('Wall protection', 'wall protection of Castle Lords', 'percent', 'Defense structure effects', 16),
  470: battleEffectNegative('Gate protection', 'gate protection of Castle Lords', 'percent', 'Defense structure effects', 17),
  471: battleEffectNegative('Moat protection', 'moat protection of Castle Lords', 'percent', 'Defense structure effects', 18),
  503: battleEffect('Front unit limit', 'unit limit on the front', 'percent', 'Attack effects', 20),
  369: battleEffect('Front unit limit', 'unit limit on the front', 'percent', 'Attack effects', 20),
  476: battleEffect('Front unit limit', 'unit limit on the front', 'percent', 'Attack effects', 20),
  66: battleEffect('Flank unit limit', 'unit limit on the flanks', 'percent', 'Attack effects', 21),
  368: battleEffect('Flank unit limit', 'unit limit on the flanks', 'percent', 'Attack effects', 21),
  474: battleEffect('Flank unit limit', 'unit limit on the flanks', 'percent', 'Attack effects', 21),
  700: battleEffect('Final assault capacity', 'to troop capacity for final assault', 'number', 'Courtyard effects', 22),
  701: battleEffect('Final assault capacity', 'to troop capacity for final assault', 'percent', 'Courtyard effects', 23),
  511: battleEffect('Attack support units', 'attack support units', 'number', 'Attack effects', 24),
  512: battleEffect('Attack support units', 'attack support units', 'number', 'Attack effects', 24),
  29: battleEffect('Additional waves', 'additional wave(s)', 'number', 'Pre-battle effects', 29),
  484: battleEffect('Additional waves', 'additional wave(s)', 'number', 'Pre-battle effects', 29),
  426: battleEffect('Army travel speed', 'army travel speed', 'percent', 'Pre-battle effects', 30),
  53: battleEffect('Army travel speed', 'army travel speed', 'percent', 'Pre-battle effects', 30),
  472: battleEffect('Army travel speed', 'army travel speed', 'percent', 'Pre-battle effects', 30),
  97: battleEffect('Travel speed', 'Military, espionage and trade travel speed', 'percent', 'Pre-battle effects', 30),
  19: battleEffect('Attack travel speed', 'Attack travel speed', 'percent', 'Pre-battle effects', 31),
  55: battleEffect('Later army detection', 'later army detection', 'percent', 'Pre-battle effects', 32),
  481: battleEffect('Later army detection', 'later army detection', 'percent', 'Pre-battle effects', 32),
  482: battleEffect('Army return travel speed', 'army return travel speed against Castle Lords', 'percent', 'Post-battle effects', 33),
  111: battleEffect('Loot capacity', 'loot capacity', 'percent', 'Post-battle effects', 40),
  431: battleEffect('Resources plundered', 'resources plundered when looting', 'percent', 'Post-battle effects', 41),
  54: battleEffect('Resources plundered', 'resources plundered when looting', 'percent', 'Post-battle effects', 41),
  473: battleEffect('Resources plundered', 'resources plundered when looting', 'percent', 'Post-battle effects', 41),
  51: battleEffect('Glory earned', 'glory points earned when attacking', 'percent', 'Post-battle effects', 42),
  100: battleEffect('Glory bonus', 'Glory bonus', 'percent', 'Post-battle effects', 42),
  45: battleEffect('Glory earned', 'glory points earned when attacking', 'percent', 'Post-battle effects', 42),
  22: battleEffect('Glory earned', 'glory points earned when attacking', 'percent', 'Post-battle effects', 42),
  52: battleEffect('Honor earned', 'honor points earned in battle', 'percent', 'Post-battle effects', 43),
  82: battleEffect('XP earned', 'XP earned in battle', 'percent', 'Post-battle effects', 44),
  112: battleEffect('XP earned', 'XP earned in battle', 'percent', 'Post-battle effects', 44),
  43: battleEffect('Coin loot', 'Coins looted from NPC targets', 'percent', 'Post-battle effects', 45),
  60: battleEffect('Equipment find', 'chance of finding better equipment', 'percent', 'Post-battle effects', 46),
  48: battleEffect('Unit combat strength when attacking', 'unit combat strength when attacking', 'percent', 'Attack effects', 15),
  20: battleEffect('Unit combat strength when attacking', 'unit combat strength when attacking', 'percent', 'Attack effects', 15),
  25: battleEffect('Event target attack strength', 'Combat strength bonus against Foreign and Bloodcrow castles', 'percent', 'Attack effects', 51),

  339: battleEffect('Combat strength for defensive melee units', 'combat strength bonus for defensive melee units', 'percent', 'Unit effects', 110),
  10: battleEffect('Combat strength for defensive melee units', 'combat strength bonus for defensive melee units', 'percent', 'Unit effects', 110),
  12105: battleEffect('Combat strength for defensive melee units', 'combat strength bonus for defensive melee units', 'percent', 'Unit effects', 110),
  12203: battleEffect('Combat strength for defensive melee units', 'combat strength bonus for defensive melee units', 'percent', 'Unit effects', 110),
  12303: battleEffect('Combat strength for defensive melee units', 'combat strength bonus for defensive melee units', 'percent', 'Unit effects', 110),
  12507: battleEffect('Combat strength for defensive melee units', 'combat strength bonus for defensive melee units', 'percent', 'Unit effects', 110),
  340: battleEffect('Combat strength for defensive ranged units', 'combat strength bonus for defensive ranged units', 'percent', 'Unit effects', 111),
  11: battleEffect('Combat strength for defensive ranged units', 'combat strength bonus for defensive ranged units', 'percent', 'Unit effects', 111),
  12106: battleEffect('Combat strength for defensive ranged units', 'combat strength bonus for defensive ranged units', 'percent', 'Unit effects', 111),
  12204: battleEffect('Combat strength for defensive ranged units', 'combat strength bonus for defensive ranged units', 'percent', 'Unit effects', 111),
  12304: battleEffect('Combat strength for defensive ranged units', 'combat strength bonus for defensive ranged units', 'percent', 'Unit effects', 111),
  12508: battleEffect('Combat strength for defensive ranged units', 'combat strength bonus for defensive ranged units', 'percent', 'Unit effects', 111),
  370: battleEffect('Combat strength when defending the courtyard', 'combat strength when defending the courtyard', 'percent', 'Unit effects', 112),
  501: battleEffect('Combat strength when defending the courtyard', 'combat strength when defending the courtyard', 'percent', 'Unit effects', 112),
  12108: battleEffect('Combat strength when defending the courtyard', 'combat strength when defending the courtyard', 'percent', 'Unit effects', 112),
  12206: battleEffect('Combat strength when defending the courtyard', 'combat strength when defending the courtyard', 'percent', 'Unit effects', 112),
  12306: battleEffect('Combat strength when defending the courtyard', 'combat strength when defending the courtyard', 'percent', 'Unit effects', 112),
  12510: battleEffect('Combat strength when defending the courtyard', 'combat strength when defending the courtyard', 'percent', 'Unit effects', 112),
  12109: battleEffect('Combat strength for defense units', 'combat strength for defense units', 'percent', 'Unit effects', 113),
  12501: battleEffect('Combat strength for defense units', 'combat strength for defense units', 'percent', 'Unit effects', 113),
  509: battleEffect('Front defense', 'combat strength on the front when defending', 'percent', 'Defense unit effects', 113),
  510: battleEffect('Flank defense', 'combat strength on the flanks when defending', 'percent', 'Defense unit effects', 114),
  12111: battleEffect('Flank defense', 'combat strength for defense units of the flanks', 'percent', 'Defense unit effects', 114),
  12102: battleEffect('Wall protection', 'wall protection', 'percent', 'Defense structure effects', 115),
  12103: battleEffect('Gate protection', 'gate protection', 'percent', 'Defense structure effects', 116),
  12104: battleEffect('Moat protection', 'moat protection', 'percent', 'Defense structure effects', 117),
  420: battleEffect('Wall unit limit', 'unit limit on the wall', 'number', 'Defense unit effects', 120),
  387: battleEffect('Wall unit limit', 'to troop capacity on wall defense', 'percent', 'Defense unit effects', 120),
  12107: battleEffect('Wall unit limit', 'to troop capacity on wall defense', 'percent', 'Defense unit effects', 120),
  702: battleEffect('Courtyard defense capacity', 'to troop capacity in courtyard defense', 'number', 'Courtyard effects', 121),
  371: battleEffect('Courtyard defense capacity', 'to troop capacity in courtyard defense', 'number', 'Courtyard effects', 121),
  705: battleEffect('Courtyard defense capacity', 'to troop capacity in courtyard defense', 'percent', 'Courtyard effects', 121),
  706: battleEffect('Alliance support capacity', 'to alliance support troop capacity', 'number', 'Courtyard effects', 122),
  385: battleEffect('Alliance support capacity', 'to alliance support troop capacity', 'number', 'Courtyard effects', 122),
  507: battleEffect('Defense support units', 'defense support units', 'number', 'Defense unit effects', 122),
  508: battleEffect('Defense support units', 'defense support units', 'number', 'Defense unit effects', 122),
  12112: battleEffect('Protector support', 'Level 10 Protector of the north in courtyard defense', 'number', 'Defense unit effects', 122),
  427: battleEffect('Surviving soldiers', 'more surviving soldiers after defense', 'percent', 'Post-battle effects', 123),
  428: battleEffect('Sight radius', 'Sight Radius', 'percent', 'Pre-battle effects', 124),
  4: battleEffect('Earlier attack warning', 'earlier attack warning', 'percent', 'Pre-battle effects', 125),
  12503: battleEffect('Earlier attack warning', 'earlier attack warning when defending against Castle Lords', 'percent', 'Pre-battle effects', 125),
  5: battleEffectNegative('Fire damage suffered', 'fire damage suffered when defending', 'percent', 'Defense structure effects', 126),
  429: battleEffectNegative('Fire damage suffered', 'fire damage suffered when defending', 'percent', 'Defense structure effects', 126),
  12309: battleEffectNegative('Fire damage suffered', 'fire damage suffered when defending', 'percent', 'Defense structure effects', 126),
  12511: battleEffectNegative('Fire damage suffered', 'fire damage suffered when defending', 'percent', 'Defense structure effects', 126),
  12101: battleEffectNegative('Resources lost', 'resources lost after being looted', 'percent', 'Post-battle effects', 126),
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
  if (battleEffectUsesFirstPairedValue(id) && values.length > 1) {
    return values[1]
  }
  if (values.length > 1 && values.length % 2 === 0 && values[0] > 100) {
    const total = values.reduce((sum, value, index) => index % 2 === 1 ? sum + value : sum, 0)
    if (total !== 0) {
      return total
    }
  }
  return values[0]
}

function battleEffectUsesFirstPairedValue(id: number): boolean {
  const definition = battleEffectData().effectDefinitions.get(id)
  if (!definition) {
    return false
  }
  return definition.effectTypeID === 148 || definition.name.toLowerCase().includes('attackbonusunit')
}

function battleEffectValuesFromValue(value: unknown): number[] {
  const record = recordValue(value)
  if (record) {
    return numericArrayValue(record.value)
  }
  if (isNumberLike(value)) {
    return [numericValue(value)]
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
