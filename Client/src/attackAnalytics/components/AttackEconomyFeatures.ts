export const attackEconomyFeatureDefinitions = [
  { id: 'autoInvasion', label: 'Auto Invasion', description: 'Foreign Lord and Bloodcrow castles', color: '#f97316' },
  { id: 'autoFortress', label: 'Auto Fortress', description: 'Outer-kingdom fortresses', color: '#0ea5e9' },
  { id: 'autoTowers', label: 'Auto Towers', description: 'Robber-baron and kingdom towers', color: '#f59e0b' },
  { id: 'autoStorm', label: 'Auto Storm', description: 'Storm forts and resource islands', color: '#38bdf8' },
  { id: 'autoNomad', label: 'Auto Nomad', description: 'Nomad and Samurai camps', color: '#ef4444' },
  { id: 'autoAdvisor', label: 'Auto Advisor', description: 'Advisor-selected event targets', color: '#14b8a6' },
  { id: 'autoKhan', label: 'Auto Khan', description: 'Khan camp attacks', color: '#eab308' },
  { id: 'autoBeriWorld', label: 'Auto Beri', description: 'Berimond towers', color: '#a855f7' },
  { id: 'riftMaiden', label: 'Rift Maiden', description: 'Rift Maiden waves', color: '#ec4899' },
  { id: 'riftReplay', label: 'Rift Replay', description: 'Replayed Rift attacks', color: '#8b5cf6' },
] as const;

export type AttackEconomyFeatureID = typeof attackEconomyFeatureDefinitions[number]['id'];
