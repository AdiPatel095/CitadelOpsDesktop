import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const core = await vite.ssrLoadModule('/src/settings/copy/castleCopy.ts');
const { castleCandidates } = await vite.ssrLoadModule('/src/settings/copy/candidates.ts');
const { towersCopyDescriptor } = await vite.ssrLoadModule('/src/settings/copy/features/towers.ts');
const { CastleCopyBody } = await vite.ssrLoadModule('/src/settings/components/CastleCopyDialog.tsx');

after(async () => {
  await vite.close();
});

const SESSION = { generation: 5, baselineGeneration: 5, changedAt: '2026-09-29T09:00:00Z' };
const troops = { 2: { id: 2, name: 'Crossbowmen' } };
const castle = (id, name, stationed) => ({ id, name, kingdomId: 0, slotType: 4, x: 0, y: 0, units: { stationed }, unitsObservedAt: '0001-01-01T00:00:00Z', resources: {}, buildings: {} });
const castles = [castle(1, 'Sunhold', { 2: 50 }), castle(2, 'Frostkeep', { 2: 10 }), castle(3, 'Ashvale', {}), castle(4, 'Mirefen', { 2: 5 })];
const state = { castles: Object.fromEntries(castles.map((entry) => [entry.id, entry])) };
const context = {
  state, troops, tools: {}, metadataReady: true, observation: { session: SESSION, connected: true },
  candidates: castleCandidates(castles.map((entry) => ({ id: entry.id, name: entry.name, kingdomId: 0 })), state),
};
const draft = { 1: { enabled: true, radius: 15, unitId: 2, maidenOnly: true }, 4: { enabled: false, radius: 22, unitId: 2, maidenOnly: false } };

function render(overrides = {}) {
  const preview = core.previewCastleCopy(towersCopyDescriptor, draft, '1', ['2', '3', '4'], context);
  const input = overrides.input ?? core.defaultCopyInput(towersCopyDescriptor, preview);
  return renderToStaticMarkup(createElement(CastleCopyBody, {
    descriptor: towersCopyDescriptor, context, featureLabel: 'Auto Towers', preview, input, onInput: () => undefined,
    sources: core.configuredSources(towersCopyDescriptor, draft, context), sourceKey: '1', onSource: () => undefined,
    running: false, minutes: 0, ...overrides.props,
  }));
}

test('the preview shows every destination with its state, exact reasons and from-to values', () => {
  const html = render();
  assert.match(html, /data-castle-copy-destination="2"[^>]*data-castle-copy-state="compatible"/);
  assert.match(html, /data-castle-copy-destination="3"[^>]*data-castle-copy-state="unavailable"/);
  assert.match(html, /Ashvale has 0 of 1 Crossbowmen stationed\./);
  assert.match(html, /Compatible/);
  assert.match(html, /Needs a look/);
  assert.match(html, /Crossbowmen/);
  assert.match(html, /15 tiles/);
  assert.match(html, /scope="row"/, 'row headers for the value table');
});

test('a destination with a different custom value shows "kept" with an explicit include', () => {
  const html = render();
  const block = html.slice(html.indexOf('data-castle-copy-destination="4"'));
  assert.match(block, /Kept as it is\./);
  assert.match(block, />Include</);
});

test('Maya\'s copy: the draft-only sentence and the not-copied footer appear exactly', () => {
  const html = render();
  assert.match(html, /This only changes the settings in this window\. Nothing is saved until you press Save, and saving never starts an automation\./);
  assert.match(html, /Not copied: check timing, map scan, Advisor, travel, daily limit\. These settings apply to all castles\./);
  assert.match(html, /Also include the selected castles in Auto Towers/);
  assert.match(html, /Castles you do not include stay as they are\. This never starts the automation\./);
});

test('nothing is ticked that needs an explicit include; the consequential include is off by default', () => {
  const html = render();
  const unavailable = html.slice(html.indexOf('data-castle-copy-destination="3"'), html.indexOf('data-castle-copy-destination="4"'));
  assert.doesNotMatch(unavailable.slice(0, unavailable.indexOf('</label>')), /checked=""/, 'a destination that needs an explicit include starts unticked');
  assert.match(html, /<input type="checkbox" class="h-4 w-4"\/><span[^>]*>Also include the selected castles in Auto Towers/, 'the include for enabled is unchecked by default');
});

test('an incompatible destination is disabled; a source without setup says so and offers nothing to apply', () => {
  const unknownTroop = { ...draft, 1: { ...draft[1], unitId: 999 } };
  const preview = core.previewCastleCopy(towersCopyDescriptor, unknownTroop, '1', ['2'], context);
  const html = renderToStaticMarkup(createElement(CastleCopyBody, {
    descriptor: towersCopyDescriptor, context, featureLabel: 'Auto Towers', preview, input: core.defaultCopyInput(towersCopyDescriptor, preview), onInput: () => undefined,
    sources: [], sourceKey: '1', onSource: () => undefined, running: false, minutes: 0,
  }));
  assert.match(html, /data-castle-copy-state="incompatible"[\s\S]*disabled=""/);
  assert.match(html, /The game does not know troop #999\./);
  const empty = core.previewCastleCopy(towersCopyDescriptor, { 1: { enabled: false, radius: 10, unitId: 0, maidenOnly: false } }, '1', ['2'], context);
  const emptyHtml = renderToStaticMarkup(createElement(CastleCopyBody, {
    descriptor: towersCopyDescriptor, context, featureLabel: 'Auto Towers', preview: empty, input: core.defaultCopyInput(towersCopyDescriptor, empty), onInput: () => undefined,
    sources: [], sourceKey: '1', onSource: () => undefined, running: false, minutes: 0,
  }));
  assert.match(emptyHtml, /Sunhold has no setup to copy yet\./);
});

test('a running feature states when saved settings are used, with the time only when the game reported one', () => {
  assert.match(render({ props: { running: true, minutes: 0 } }), /Auto Towers is running\. Saved settings are used at its next check\./);
  assert.match(render({ props: { running: true, minutes: 12 } }), /Saved settings are used at its next check, in about/);
  assert.doesNotMatch(render(), /is running/);
});

test('cancel changes nothing and apply uses only the selection (the dialog delegates to the pure core)', async () => {
  const source = await readFile(new URL('../src/settings/components/CastleCopyDialog.tsx', import.meta.url), 'utf8');
  const apply = source.slice(source.indexOf('const apply = () =>'), source.indexOf('return (\n    <Modal'));
  assert.match(apply, /applyCastleCopy\(descriptor, draft, preview, selection\)/);
  assert.match(source, /<Button variant="ghost" onClick=\{onClose\}><LocalizedText messageKey="castleCopy.cancel" \/>/, 'Cancel only closes');
  assert.doesNotMatch(source.slice(source.indexOf('castleCopy.cancel') - 200, source.indexOf('castleCopy.cancel')), /onApply/);
  assert.match(source, /disabled=\{size === 0\}/, 'Apply needs at least one selected change');
});

test('focus returns to the Copy button after the dialog closes, and nothing but a click opens it', async () => {
  const source = await readFile(new URL('../src/settings/components/CastleCopyDialog.tsx', import.meta.url), 'utf8');
  assert.match(source, /moveFocusAfterDialog\(\(\) => \(target\.isConnected \? target : trigger\.current\), browserFrames\)/);
  assert.match(source, /returnTarget\.current = event\.currentTarget; setOpen\(true\)/);
  assert.equal([...source.matchAll(/setOpen\(true\)/g)].length, 1, 'the dialog opens from the button click only');
  assert.match(source, /role|<Modal/, 'a Modal supplies role="dialog" and aria-labelledby');
});
