import assert from 'node:assert/strict';
import { test, after } from 'node:test';
import { existsSync, readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
const root = fileURLToPath(new URL('..', import.meta.url));
const prefix = existsSync(`${root}/src/commandCenter`) ? '/src/commandCenter' : '/src';
const vite = await createServer({ root, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
const { StatusBadge } = await vite.ssrLoadModule(`${prefix}/components/ui/StatusBadge.tsx`);
const { RoleTag, accountPlayerRole } = await vite.ssrLoadModule(`${prefix}/components/ui/RoleTag.tsx`);
const { ConnectionStatus } = await vite.ssrLoadModule(`${prefix}/components/ConnectionStatus.tsx`);
const { mostSeverePlayerStatus, headerPlayerStatus } = await vite.ssrLoadModule(`${prefix}/components/playerStatusDisplay.ts`);
const { describeMessage } = await vite.ssrLoadModule(`${prefix}/i18n/messages.ts`);
const { describeFeaturePlayerStatus } = await vite.ssrLoadModule(`${prefix}/settings/readiness/automationPlayerStatusDisplay.ts`);
const shell = prefix.includes('commandCenter') ? await vite.ssrLoadModule('/src/components/AccountPlayerStatus.tsx') : undefined;
after(() => vite.close());
const render = (component, props) => renderToStaticMarkup(React.createElement(component, props));
const reason = describeMessage('playerStatus.connected');
const glyphs = { running: 'circle-play', waiting: 'clock', done: 'circle-check', paused: 'circle-pause', blocked: 'circle-minus', 'needs-attention': 'triangle-alert', off: 'power', unknown: 'circle-question-mark' };
for (const [status, glyph] of Object.entries(glyphs)) test(`badge ${status} has the prescribed glyph, localized word and reason`, () => {
  const html = render(StatusBadge, { status, reason });
  assert.match(html, new RegExp(`lucide-${glyph}`));
  assert.match(html, new RegExp(`data-player-status="${status}"`));
  assert.match(html, /player-status-word/); assert.match(html, /player-status-card-reason/); assert.match(html, /Connected/);
  assert.doesNotMatch(html, /animate-|<button|aria-hidden="true"[^>]*player-status-word/);
});
test('card reasons wrap in full; inline truncation requires a full-reason reveal target', () => {
  const detail = { key: '', fallback: 'A long {raw} game reason', fallbackText: 'A long {raw} game reason' };
  assert.match(render(StatusBadge, { status: 'blocked', reason: detail, inline: true }), /player-status-card-reason/);
  const html = render(StatusBadge, { status: 'blocked', reason: detail, inline: true, canRevealReason: true });
  assert.match(html, /aria-label="Blocked · A long \{raw\} game reason"/); assert.match(html, /role="tooltip"/); assert.match(html, /tabindex="0"/);
});
for (const role of ['owner', 'admin', 'editor']) test(`role tag ${role} is neutral and has no status glyph`, () => {
  const html = render(RoleTag, { role });
  assert.match(html, new RegExp(`data-player-role="${role}"`));
  assert.match(html, new RegExp(role[0].toUpperCase() + role.slice(1))); assert.doesNotMatch(html, /<svg|data-status-role/);
});
test('ownership takes precedence over admin, and edit maps to Editor', () => {
  assert.equal(accountPlayerRole({ isOwner: true, accessLevel: 'admin' }), 'owner');
  assert.equal(accountPlayerRole({ isOwner: false, accessLevel: 'admin' }), 'admin');
  assert.equal(accountPlayerRole({ isOwner: false, accessLevel: 'edit' }), 'editor');
});
test('aggregate uses danger, warning, info, success, neutral priority with stable ties', () => {
  const item = (status) => ({ status, reason });
  for (const [values, expected] of [[['off', 'running'], 'running'], [['running', 'waiting'], 'waiting'], [['waiting', 'blocked'], 'blocked'], [['paused', 'needs-attention'], 'needs-attention'], [['blocked', 'paused'], 'blocked']]) {
    assert.equal(mostSeverePlayerStatus(values.map(item), item('unknown')).status, expected);
  }
  assert.equal(mostSeverePlayerStatus([], item('unknown')).status, 'unknown');
});
test('shared header helper retains checkpoint account status and full reason is one native disclosure away', () => {
  const value = headerPlayerStatus({ surface: 'hosted', status: 'disconnected', loggedIn: false, now: 0, checkpoint: true, checkpointObservedAt: '2026-09-30T12:00:00Z', accountStatus: { status: 'blocked', reason } });
  assert.equal(value.status, 'blocked'); assert.equal(value.reason.key, 'playerStatus.checkpointSuffix');
  const html = render(ConnectionStatus, { value });
  assert.match(html, /<details/); assert.match(html, /<summary/); assert.match(html, /player-status-inline-reason/); assert.match(html, /player-status-card-reason/);
});
test('CIT-60 CSS uses semantic roles, distinct unknown/role outlines, and no animated glyphs or card clamping', () => {
  const css = readFileSync(`${root}${prefix}/components/ui/StatusBadge.css`, 'utf8');
  for (const role of ['success', 'info', 'warning', 'danger', 'neutral']) assert.match(css, new RegExp(`var\\(--status-${role}\\)`));
  assert.match(css, /data-player-status="unknown"[^}]*background: transparent; border: 1px dashed var\(--border-default\)/);
  assert.match(css, /player-role-tag[^}]*border-radius: var\(--radius-sm\)/);
  assert.doesNotMatch(css, /#[0-9a-f]{3,8}\b|rgba?\(|animation:|line-clamp/);
  assert.doesNotMatch(css.match(/\.player-status-card-reason[^}]+}/)[0], /overflow: hidden|text-overflow/);
});
const now = Date.parse('2026-09-30T12:00:00Z');
const context = { now, connected: true, connectionSince: '2026-09-30T11:00:00Z' };
const runtime = (status, detail = status) => ({ status, detail, updatedAt: '2026-09-30T12:00:00Z' });
const feature = (featureId, states, extra = {}) => describeFeaturePlayerStatus({ featureId, states, control: { configured: true, enabled: true }, context, ...extra });
test('card lane adapter keeps CIT-20 order and the reason for each lane', () => {
  const value = feature('autoStorm', { autoStorm: runtime('running', 'Attacking'), autoStormShop: runtime('waiting', 'Waiting for Aquamarine'), autoStormBuild: runtime('blocked', 'No building slot') });
  assert.deepEqual(value.lanes.map(lane => lane.id), ['autoStorm', 'autoStormShop', 'autoStormBuild']);
  assert.deepEqual(value.lanes.map(lane => lane.value.status), ['running', 'waiting', 'blocked']);
  assert.equal(value.overall.reason.fallbackText, 'No building slot');
});
test('disabled Berimond build lane cannot set the aggregate severity', () => {
  const states = { autoBeriWorld: runtime('running'), autoBeriWorldAttack: runtime('running'), autoBeriWorldTools: runtime('running'), autoBeriWorldBuild: runtime('failed') };
  assert.equal(feature('autoBeriWorld', states, { buildLaneActive: false }).overall.status, 'running');
  assert.equal(feature('autoBeriWorld', states, { buildLaneActive: true }).overall.status, 'needs-attention');
});
test('real missing-decoration evidence adds a Blocked lane with the full existing reason', () => {
  const states = { autoStorm: runtime('running'), autoStormShop: runtime('running'), autoStormBuild: { ...runtime('running'), metrics: { stormMissingDecorations: 2 } } };
  const value = feature('autoStorm', states);
  assert.equal(value.overall.status, 'blocked');
  assert.equal(value.lanes.at(-1).id, 'builder-missing-decorations');
  assert.equal(value.overall.reason.key, 'automation.missingDecorations');
  assert.match(render(StatusBadge, value.overall), /2 target decorations/);
  assert.equal(feature('autoStorm', states, { control: { configured: false, enabled: false } }).overall.status, 'off');
});
test('stale or non-finite decoration evidence cannot invent a healthy or Blocked claim', () => {
  const states = { autoStormBuild: { ...runtime('running'), updatedAt: '2026-09-30T10:00:00Z', metrics: { stormMissingDecorations: 2 } } };
  assert.equal(feature('autoStorm', states).lanes.at(-1).value.status, 'unknown');
  for (const count of [NaN, Infinity, 0, -1]) assert.equal(feature('autoStorm', { autoStormBuild: { ...runtime('running'), metrics: { stormMissingDecorations: count } } }).lanes.length, 3);
});
test('saved automation badges downgrade green and retain warning/danger with the saved-data reason', () => {
  for (const [raw, expected] of [['running', 'unknown'], ['complete', 'unknown'], ['blocked', 'blocked'], ['failed', 'needs-attention']]) {
    const value = feature('autoRecruit', { autoRecruit: runtime(raw, 'Game reason') }, { context: { ...context, connected: false, presence: { mode: 'checkpoint', checkpointObservedAt: '2026-09-30T12:00:00Z' } } }).overall;
    assert.equal(value.status, expected);
    assert.match(render(StatusBadge, value), /Saved data from|saved data from/);
    if (expected !== 'unknown') assert.match(render(StatusBadge, value), /Game reason/);
  }
});
test('the portal shell can pass its formatted messages without losing language metadata', () => {
  const html = render(StatusBadge, { status: 'waiting', reason, labelMessage: { text: 'انتظار', translated: true, resolvedLocale: 'ar' }, reasonMessage: { text: 'تفاصيل كاملة', translated: true, resolvedLocale: 'ar' } });
  assert.match(html, /lang="ar" dir="rtl"/); assert.match(html, /انتظار · تفاصيل كاملة/);
  assert.match(render(RoleTag, { role: 'editor', labelMessage: { text: 'محرر', translated: true, resolvedLocale: 'ar' } }), /lang="ar" dir="rtl"/);
});

if (shell) test('portal shell adapters render the shared status and role with source fallback', () => {
  assert.match(render(shell.AccountStatusBadge, { status: 'blocked', reason }), /Blocked · Connected/);
  assert.match(render(shell.AccountRoleTag, { role: 'owner' }), /Owner/);
});
