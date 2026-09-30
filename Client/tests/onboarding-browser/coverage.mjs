/**
 * The coverage matrix (CIT-22): which scenario exercises which module in which column. The table in
 * Docs/OnboardingCoverage.md is derived from the scenario files by this module, and `tests/onboarding-coverage.test.mjs`
 * fails when the document and the scenarios disagree. Regenerate with `UPDATE_COVERAGE=1 npm test -- --test-name-pattern=coverage`.
 */
export const FEATURE_MODULES = [
  ['autoNomad', 'Auto Nomad / Samurai'], ['autoInvasion', 'Auto Invasion'], ['autoBeriWorld', 'Auto Beri World'], ['autoKhan', 'Auto Khan'],
  ['autoStorm', 'Auto Storm'], ['autoTowers', 'Auto Towers'], ['autoFortress', 'Auto Fortress'], ['autoFoodBalance', 'Auto Food Balance'],
  ['autoStation', 'Auto Station'], ['autoBird', 'Auto Bird'], ['autoRecruit', 'Recruit Troops'], ['autoTool', 'Auto Tool'],
  ['autoHospital', 'Auto Hospital'], ['autoTCI', 'Auto TCI'], ['autoSceatRes', 'Auto Sceat Resources'], ['autoBooster', 'Auto Booster'],
  ['autoBuyer', 'Auto Buyer'], ['autoAdvisor', 'Auto Advisor'], ['autoEquipmentCleanup', 'Auto Equipment Cleanup'],
];
export const SHARED_MODULES = [
  ['presets-views', 'Attack and Defense Presets views'], ['movement-assignments', 'Movement commander assignments'],
  ['account-center-hosted', 'Account Center (hosted)'], ['settings-connection-desktop', 'Settings connection (desktop)'],
];

export const COLUMNS = [
  ['inline', 'Inline setup (CIT-15/16)'], ['readiness', 'Readiness and repair (CIT-18)'], ['disclosure', 'Disclosure (CIT-17)'],
  ['phases', 'Before you start, phases, first result, Stop (CIT-20)'], ['copy', 'Copy (CIT-21)'], ['goal', 'Goal, checklist, recovery, repair (CIT-19)'],
];

const EVENT = new Set(['autoNomad', 'autoInvasion', 'autoBeriWorld', 'autoKhan', 'autoStorm']);
const COPY = new Set(['autoTowers', 'autoStation', 'autoBird', 'autoRecruit', 'autoTool']);

/** Whether a module supports a column; otherwise the reason it does not apply. */
export function support(moduleId, column) {
  if (FEATURE_MODULES.some(([id]) => id === moduleId)) {
    if (column === 'inline') return EVENT.has(moduleId) ? { yes: true } : { yes: false, why: 'not preset-dependent: settings are edited directly' };
    if (column === 'copy') {
      if (COPY.has(moduleId)) return { yes: true };
      return { yes: false, why: moduleId === 'autoFoodBalance' || moduleId === 'autoHospital' ? 'account-wide settings only; no per-castle copy' : 'no per-castle setup to copy' };
    }
    return { yes: true };
  }
  const shared = {
    'presets-views': { inline: true }, 'movement-assignments': { readiness: true },
    'account-center-hosted': { goal: true }, 'settings-connection-desktop': { readiness: true, goal: true },
  }[moduleId] ?? {};
  return shared[column] ? { yes: true } : { yes: false, why: 'not part of this surface' };
}

/** Platform notes per module (declared once, checked against the scenario files). */
export const PLATFORM_NOTES = {
  autoBird: 'both; hosted gates Bird presets and castle controls behind `birdRuntimePresets` / `birdCastleControls` (2.4.0-beta.2)',
  autoTowers: 'both; hosted gates the Baron Advisor description behind `towerAdvisor`',
  autoAdvisor: 'both; hosted gates behind `towerAdvisor`',
  'account-center-hosted': 'hosted only',
  'settings-connection-desktop': 'desktop only (Start Bot, background login); hosted uses the connection repair panel',
};

export function scenariosFor(scenarios, moduleId, column) {
  return scenarios.filter((scenario) => scenario.modules.includes(moduleId) && scenario.columns.includes(column)).map((scenario) => scenario.id).sort();
}

export function deriveRows(scenarios, disclosureCounts) {
  const rows = [];
  for (const [id, label] of [...FEATURE_MODULES, ...SHARED_MODULES]) {
    const cells = COLUMNS.map(([column]) => {
      const supported = support(id, column);
      if (!supported.yes) return `n/a: ${supported.why}`;
      const list = scenariosFor(scenarios, id, column);
      return list.length > 0 ? list.map((entry) => `\`${entry}\``).join(', ') : 'MISSING';
    });
    const platforms = new Set(scenarios.filter((scenario) => scenario.modules.includes(id)).flatMap((scenario) => scenario.platforms));
    const note = PLATFORM_NOTES[id] ?? (platforms.size === 2 ? 'both' : [...platforms].join(', ') || 'both');
    rows.push({ id, label, cells, disclosure: disclosureCounts[id], note });
  }
  return rows;
}

export function renderTable(scenarios, disclosureCounts) {
  const header = `| Module | ${COLUMNS.map(([, title]) => title).join(' | ')} | Disclosure rows | Platform notes | Evidence |`;
  const rule = `|${Array(COLUMNS.length + 4).fill('---').join('|')}|`;
  const lines = deriveRows(scenarios, disclosureCounts).map((row) => (
    `| \`${row.id}\` ${row.label} | ${row.cells.join(' | ')} | ${row.disclosure ?? 'n/a: not a settings editor'} | ${row.note} | pending: \`QA/Evidence/CIT-22/\` (Sophie) |`
  ));
  return [header, rule, ...lines].join('\n');
}
