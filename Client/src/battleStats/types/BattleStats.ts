export type BattleRole = 'attacker' | 'defender' | 'unknown';

export type BattleResult = 'Victory' | 'Defeat' | 'Unknown';

export interface BattleCombatant {
  playerID?: string | number;
  playerId?: string | number;
  dummy?: boolean;
  name?: string;
  playerName?: string;
  alliance?: string;
  allianceName?: string;
  allianceTag?: string;
  castleName?: string;
  role?: BattleRole | string;
}

export interface BattleMetrics {
  attackerSent?: number;
  attackSent?: number;
  attackerLost?: number;
  attackLost?: number;
  attackersKilled?: number;
  defenderStationed?: number;
  defenderLost?: number;
  defenseLost?: number;
  defendersKilled?: number;
  wallLosses?: number;
  courtyardLosses?: number;
  attackTradeRatio?: number;
  defenseTradeRatio?: number;
}

export interface BattleEffect {
  code?: string;
  label?: string;
  name?: string;
  value?: number;
  formattedValue?: string;
  displayText?: string;
  category?: string;
  sortOrder?: number;
  side?: 'commander' | 'castellan' | string;
}

export interface BattleItemDetail {
  side?: 'attacker' | 'defender' | string;
  phase?: 'wall' | 'courtyard' | 'support' | string;
  lane?: 'left' | 'middle' | 'right' | string;
  wodID?: number;
  wodId?: number;
  id?: number;
  amount?: number;
  lost?: number;
  used?: number;
}

export interface BattleWaveLane {
  lane?: 'left' | 'middle' | 'right' | string;
  result?: 'HELD' | 'BREACHED' | string;
  attackerLost?: number;
  defenderLost?: number;
  attackerStart?: number;
  defenderStart?: number;
  attackerUnitDetails?: BattleItemDetail[];
  defenderUnitDetails?: BattleItemDetail[];
  attackerToolDetails?: BattleItemDetail[];
  defenderToolDetails?: BattleItemDetail[];
}

export interface BattleWave {
  index?: number;
  wave?: number;
  result?: string;
  lanes?: BattleWaveLane[];
}

export interface ParsedReport {
  id?: string;
  reportID?: string;
  battleKey?: string;
  mid?: string | number;
  lid?: string | number;
  kingdomID?: number;
  kingdomId?: number;
  targetX?: number;
  targetY?: number;
  targetName?: string;
  castleName?: string;
  battleType?: string;
  occurredAt?: string;
  timestamp?: string;
  receivedAt?: string;
  dateUnix?: number;
  dateMs?: number;
  result?: BattleResult | string;
  outcome?: BattleResult | string;
  role?: BattleRole | string;
  attacker?: BattleCombatant;
  defender?: BattleCombatant;
  metrics?: BattleMetrics;
  effects?: BattleEffect[];
  commanderEffects?: BattleEffect[];
  castellanEffects?: BattleEffect[];
  topUnits?: BattleItemDetail[];
  supportTools?: BattleItemDetail[];
  waves?: BattleWave[];
}

export interface BattleStatsSummary {
  reports: number;
  victories: number;
  defeats: number;
  attackLost: number;
  defenseLost: number;
  attackersKilled: number;
  defendersKilled: number;
}
