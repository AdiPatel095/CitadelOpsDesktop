import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({
  root: clientRoot,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
});
const draft = await vite.ssrLoadModule('/src/settings/requirements/commanderAssignmentDraft.ts');
const eligibility = await vite.ssrLoadModule('/src/settings/requirements/commanderEligibility.ts');

after(async () => {
  await vite.close();
});

const SECTION = 'automation.commanderFeatures';

class ConflictError extends Error {
  constructor() {
    super('Settings changed elsewhere');
    this.code = 'configuration_conflict';
  }
}

function fakeSession(sections, outcomes = []) {
  const writes = [];
  const session = {
    sections: structuredClone(sections),
    writes,
    async saveSection(section, value) {
      const outcome = outcomes.shift() ?? 'ok';
      writes.push({ section, value: structuredClone(value), outcome });
      if (outcome === 'conflict') throw new ConflictError();
      session.sections = { ...session.sections, [section]: structuredClone(value) };
      return { schemaVersion: 2, revision: writes.length, updatedAt: '', sections: structuredClone(session.sections) };
    },
  };
  return session;
}

const state = {
  session: { connectionGeneration: 1 },
  player: { id: 1 },
  castles: {},
  commanders: { 1: { id: 1, available: true, equipment: {}, gems: {} }, 2: { id: 2, available: true, equipment: {}, gems: {} } },
  movements: {},
  movementSnapshot: { version: 1, connectionGeneration: 1, observedAt: '2026-09-29T11:00:00Z' },
  inventory: { equipment: {} },
};
const report = (sections) => eligibility.evaluateCommanderEligibility({
  featureId: 'autoTowers',
  state,
  assignments: draft.savedCommanderAssignments(sections),
  movement: null,
  gameLoggedIn: true,
  now: Date.parse('2026-09-29T12:00:00Z'),
});

test('toggling from the default materializes an explicit list from observed commanders only for this feature', () => {
  const saved = draft.savedCommanderAssignments({ [SECTION]: { version: 2, assignments: { autoKhan: [2] }, requirements: {} } });
  const next = draft.toggleFeatureCommander(saved, 'autoTowers', 2, false, [1, 2]);
  assert.deepEqual(next.assignments, { autoKhan: [2], autoTowers: [1] });
  assert.deepEqual(draft.setFeatureCommandersAll(next, 'autoTowers', false).assignments.autoTowers, []);
  assert.equal(Object.hasOwn(draft.setFeatureCommandersAll(next, 'autoTowers', true).assignments, 'autoTowers'), false);
});

test('a successful save writes only the commander section and readiness refreshes from the saved sections', async () => {
  const session = fakeSession({ [SECTION]: { version: 2, assignments: { autoTowers: [] }, requirements: {} }, 'automation.autoTowers': { castles: {} } });
  assert.equal(report(session.sections).assignment.state, 'blocked');
  const next = draft.toggleFeatureCommander(draft.savedCommanderAssignments(session.sections), 'autoTowers', 1, true, [1, 2]);
  const result = await draft.saveCommanderAssignmentDraft(session, next);
  assert.equal(result.ok, true);
  assert.deepEqual(session.writes.map((write) => write.section), [SECTION]);
  assert.deepEqual(result.saved.assignments.autoTowers, [1]);
  assert.equal(report(session.sections).assignment.state, 'valid');
  assert.deepEqual(session.sections['automation.autoTowers'], { castles: {} }, 'the module section is untouched');
});

test('a conflict leaves the saved document unchanged and returns the error so the panel keeps its draft', async () => {
  const sections = { [SECTION]: { version: 2, assignments: { autoTowers: [] }, requirements: {} } };
  const session = fakeSession(sections, ['conflict']);
  const next = draft.toggleFeatureCommander(draft.savedCommanderAssignments(sections), 'autoTowers', 1, true, [1, 2]);
  const result = await draft.saveCommanderAssignmentDraft(session, next);
  assert.equal(result.ok, false);
  assert.ok(result.error instanceof ConflictError);
  assert.deepEqual(session.sections, sections);
  assert.deepEqual(next.assignments.autoTowers, [1], 'the caller still holds its draft for retry');
});

test('the panel writes only after an explicit confirmation and discards without writing', async () => {
  const panel = await readFile(new URL('../src/settings/components/CommanderAssignmentPanel.tsx', import.meta.url), 'utf8');
  const saveCalls = panel.match(/saveCommanderAssignmentDraft\(/g) ?? [];
  assert.equal(saveCalls.length, 1);
  const confirmSave = panel.slice(panel.indexOf('const confirmSave'), panel.indexOf('return (', panel.indexOf('const confirmSave')));
  assert.match(confirmSave, /saveCommanderAssignmentDraft\(draftSession, pending\)/);
  assert.match(panel, /onClick=\{\(\) => void confirmSave\(\)\}/, 'only the confirmation dialog button saves');
  assert.match(panel, /onClick=\{\(\) => \{ setPending\(null\); setSaveError\(''\); \}\}/, 'discard clears the panel draft only');
  assert.doesNotMatch(panel, /automation\.enabled|submitIntent|refreshMovement/);
});

test('commander status describes the switch using activity or the localized off label', async () => {
  const panel = await readFile(new URL('../src/settings/components/CommanderAssignmentPanel.tsx', import.meta.url), 'utf8');
  assert.match(panel, /commanderRowStatus\(row\)/);
  assert.match(panel, /commanderAssignment\.offForThisAutomation/);
  assert.match(panel, /const stateId = `\$\{id\}-commander-\$\{row\.commanderId\}-state`/);
  assert.match(panel, /<span id=\{stateId\}>/);
  assert.match(panel, /ariaDescribedBy=\{stateId\}/);
  const switchSource = await readFile(new URL('../src/components/ui/Switch.tsx', import.meta.url), 'utf8');
  assert.match(switchSource, /ariaDescribedBy\?: string/);
  assert.match(switchSource, /aria-describedby=\{ariaDescribedBy\}/);
});
