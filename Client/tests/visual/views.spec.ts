import { expect, test } from '@playwright/test';
import type { CastleStateV2 } from '../../src/api/Contracts';
import { prepare, settle, openView } from './harness';

const views = [
  { label: 'Castle', view: 'castle' },
  { label: 'Automation', view: 'automation' },
  { label: 'Feature Stats', view: 'events' },
  { label: 'Attack Presets', view: 'attack-presets' },
  { label: 'Defense Presets', view: 'defense-presets' },
  { label: 'Equipment', view: 'equipment' },
  { label: 'Commanders', view: 'movement' },
  { label: 'Battle Stats', view: 'battle-stats' },
  { label: 'My Stats', view: 'player-tracker' },
  { label: 'Alliance Targets', view: 'alliance-targets' },
  { label: 'Rift Raid', view: 'rift' },
  { label: 'Settings', view: 'settings' },
  { label: 'Patch Notes', view: 'patch-notes' },
  { label: 'Support', view: 'support' },
] as const;

// Browser messages from requests deliberately refused by the visual harness.
const allowedConsolePrefixes = [
  'Failed to load resource: net::ERR_BLOCKED_BY_CLIENT',
  'Failed to load resource: the server responded with a status of 503 (Service Unavailable)',
];

test('every command-center view renders in dark theme', async ({ page }) => {
  test.skip(page.viewportSize()?.width === 1024, 'view coverage uses 1440 and 390');
  const errors: string[] = [];
  const unhandled: string[] = [];
  page.on('console', message => {
    const text = message.text();
    if (text.includes('[mock] unhandled')) unhandled.push(text);
    if (message.type() === 'error' && !allowedConsolePrefixes.some(prefix => text.startsWith(prefix))) errors.push(text);
  });
  page.on('pageerror', error => errors.push(error.message));
  const verifyNetwork = await prepare(page, 'dark');
  for (const { label, view } of views) {
    await test.step(label, async () => {
      if ((page.viewportSize()?.width ?? 0) < 760) {
        await page.getByRole('button', { name: 'Open workspace navigation', exact: true }).click();
      }
      if (['settings', 'patch-notes', 'support'].includes(view)) {
        await page.locator('.liquid-sidebar-system-island').hover();
      }
      await page.locator('#workspace-navigation').getByRole('button', { name: label, exact: true }).click();
      await settle(page);
      await expect(page.locator(`[data-view="${view}"]`)).toBeVisible();
      await expect(page.locator('.command-center-boundary')).toHaveCount(0);
      await expect(page.getByText(/^(Something went wrong|Application error|Unexpected Application Error|The command center hit an error)$/i)).toHaveCount(0);
      expect(errors, `${label}: console and uncaught errors`).toEqual([]);
      expect(unhandled, `${label}: unhandled mock paths`).toEqual([]);
      verifyNetwork();
      const { scrollWidth, innerWidth } = await page.evaluate(() => ({
        scrollWidth: document.documentElement.scrollWidth, innerWidth: window.innerWidth,
      }));
      expect(scrollWidth, `${label}: no horizontal overflow`).toBeLessThanOrEqual(innerWidth);
    });
  }
});

// CIT-137: use only the fictional browser-side fixture, with no game/network writes.
for (const feature of ['autoTowers', 'autoBird', 'autoStation'] as const) {
  test(`Storm role repair Save and Cancel ${feature}`, async ({ page }, testInfo) => {
    const verifyNetwork = await prepare(page, 'dark');
    await openView(page, 'Automation', 'automation');
    const seed = await page.evaluate(async (feature) => {
      const fixturePath = '/main.tsx';
      const { server } = await import(/* @vite-ignore */ fixturePath);
      const reserve = [{ id: 1, amount: 37 }];
      const value = feature === 'autoTowers' ? { castles: { 999: { enabled: true, unitId: 1, radius: 10 } } }
        : feature === 'autoBird' ? { ignoreSettings: { settings: { 999: reserve } }, presets: { presets: [] }, activePresetId: null }
        : { settings: { 999: reserve } };
      await server.handle(`/api/v2/config/automation.${feature}`, 'PUT', { value });
      server.log = [];
      const storm = (Object.values(server.built.state.castles) as CastleStateV2[]).find((castle) => castle.kingdomId === 4);
      return { revision: server.configuration().revision, stormID: String(storm?.id) };
    }, feature);
    const label = { autoTowers: 'Auto Towers', autoBird: 'Auto Bird', autoStation: 'Auto Station' }[feature];
    await page.locator('[data-view="automation"]').getByRole('button', { name: `Open ${label} settings`, exact: true }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByRole('button', { name: 'Use as Storm castle settings', exact: true }).click();
    await expect(dialog.getByRole('button', { name: 'Use as Storm castle settings', exact: true })).toHaveCount(0);
    const snapshot = async () => page.evaluate(async (feature) => {
      const fixturePath = '/main.tsx';
      const { server } = await import(/* @vite-ignore */ fixturePath);
      return { revision: server.configuration().revision, saved: server.configuration().sections[`automation.${feature}`], writes: server.log.filter((entry: { kind: string }) => entry.kind === 'config').length };
    }, feature);
    expect((await snapshot()).revision).toBe(seed.revision);
    expect((await snapshot()).writes).toBe(0);
    await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
    await page.locator('[data-view="automation"]').getByRole('button', { name: `Open ${label} settings`, exact: true }).click();
    await dialog.getByRole('button', { name: 'Use as Storm castle settings', exact: true }).click();
    await dialog.getByRole('button', { name: 'Save changes', exact: true }).click();
    await expect(dialog).toHaveCount(0);
    const after = await snapshot();
    const entries = feature === 'autoTowers' ? after.saved.castles : feature === 'autoBird' ? after.saved.ignoreSettings.settings : after.saved.settings;
    expect(entries.storm).toBeDefined();
    expect(entries['999']).toBeUndefined();
    if (feature === 'autoTowers') {
      expect(entries[seed.stormID]).toBeUndefined();
      expect(after.saved.stormLegacyKey).toBeUndefined();
    } else {
      expect(entries[seed.stormID]).toEqual(entries.storm);
      expect(after.saved.stormLegacyKey).toBe(seed.stormID);
    }
    expect(after.revision).toBe(seed.revision + 1);
    expect(after.writes).toBe(1);
    // The fixture's next runtime frame replaces the owned Storm ID; the saved
    // section and configuration revision remain unchanged.
    await page.evaluate(async () => {
      const fixturePath = '/main.tsx';
      const { server } = await import(/* @vite-ignore */ fixturePath);
      const old = (Object.values(server.built.state.castles) as CastleStateV2[]).find((castle) => castle.kingdomId === 4);
      if (!old) throw new Error('Synthetic Storm castle is missing');
      server.file.runtime = [{ label: 'Synthetic next Storm event', state: { castles: { [old.id]: null, 998: { ...old, id: 998, name: 'Synthetic next Storm' } } } }, { label: 'Synthetic Storm absent', state: { castles: { 998: null } } }];
      server.advance();
    });
    await page.locator('[data-view="automation"]').getByRole('button', { name: `Open ${label} settings`, exact: true }).click();
    await expect(dialog.getByText('Storm castle', { exact: true }).first()).toBeVisible();
    await expect(dialog.getByText(/Saved castle.*not in this world/)).toHaveCount(0);
    expect((await snapshot()).revision).toBe(seed.revision + 1);
    expect((await snapshot()).writes).toBe(1);
    verifyNetwork();
    await page.screenshot({ path: testInfo.outputPath(`${feature}-storm-repair.png`) });
    await dialog.getByRole('button', { name: 'Save changes', exact: true }).click();
    await expect(dialog).toHaveCount(0);
    const moved = await snapshot();
    expect(moved.revision).toBe(seed.revision + 2);
    const movedEntries = feature === 'autoTowers' ? moved.saved.castles : feature === 'autoBird' ? moved.saved.ignoreSettings.settings : moved.saved.settings;
    expect(movedEntries[seed.stormID]).toBeUndefined();
    if (feature !== 'autoTowers') {
      expect(moved.saved.stormLegacyKey).toBe('998');
      expect(movedEntries['998']).toEqual(movedEntries.storm);
    }
    await page.evaluate(async () => {
      const fixturePath = '/main.tsx';
      const { server } = await import(/* @vite-ignore */ fixturePath);
      if (!server.advance()) throw new Error('Synthetic absent frame was not applied');
    });
    await page.locator('[data-view="automation"]').getByRole('button', { name: `Open ${label} settings`, exact: true }).click();
    await expect(dialog.getByText('Storm castle · used when you have one', { exact: true }).first()).toBeVisible();
    await expect(dialog.getByText(/Saved castle.*not in this world/)).toHaveCount(0);
    await dialog.getByRole('button', { name: 'Save changes', exact: true }).click();
    await expect(dialog).toHaveCount(0);
    const absent = await snapshot();
    const absentEntries = feature === 'autoTowers' ? absent.saved.castles : feature === 'autoBird' ? absent.saved.ignoreSettings.settings : absent.saved.settings;
    expect(absentEntries.storm).toEqual(entries.storm);
    expect(absentEntries['998']).toBeUndefined();
    expect(absent.saved.stormLegacyKey).toBeUndefined();
    verifyNetwork();
  });
}


for (const theme of ['dark', 'light'] as const) {
  test(`Storm reserve setup Fix focuses the Storm card ${theme}`, async ({ page }, testInfo) => {
    const verifyNetwork = await prepare(page, theme);
    await openView(page, 'Automation', 'automation');
    const seed = await page.evaluate(async (theme) => {
      const fixturePath = '/main.tsx';
      const { server } = await import(/* @vite-ignore */ fixturePath);
      const storm = (Object.values(server.built.state.castles) as CastleStateV2[]).find((castle) => castle.kingdomId === 4);
      if (!storm) throw new Error('Synthetic Storm castle is missing');
      await server.handle('/api/v2/config/automation.autoBird', 'PUT', { value: { ignoreSettings: { settings: { storm: theme === 'light' ? [{ id: 1, amount: 0 }] : [] } }, presets: { presets: [] }, activePresetId: null } });
      server.log = [];
      return { revision: server.configuration().revision, name: storm.name || `castle ${storm.id}` };
    }, theme);
    await page.locator('[data-view="automation"]').getByRole('button', { name: 'Open Auto Bird settings', exact: true }).click();
    const dialog = page.getByRole('dialog');
    const guardText = `Auto Bird skips ${seed.name}: no troops to keep are set for the Storm castle.`;
    const line = dialog.locator('li').filter({ hasText: guardText });
    await expect(line).toBeVisible();
    await expect(line).toContainText('Decided at Start');
    await line.getByRole('button', { name: 'Fix', exact: true }).click();
    await expect(dialog.locator('#auto-bird-castle-storm')).toBeFocused();
    await expect(dialog.locator('#auto-bird-castle-storm')).toBeInViewport();
    const after = await page.evaluate(async () => {
      const fixturePath = '/main.tsx';
      const { server } = await import(/* @vite-ignore */ fixturePath);
      return { revision: server.configuration().revision, writes: server.log.filter((entry: { kind: string }) => entry.kind === 'config').length };
    });
    expect(after).toEqual({ revision: seed.revision, writes: 0 });
    verifyNetwork();
    await page.screenshot({ path: testInfo.outputPath(`storm-reserve-fix-${theme}.png`) });
  });
}


// Sophie's D2 regression: a legacy own-ID entry must be configured for copy
// before Save normalizes it. Opening and cancelling the copy must not write.
for (const feature of ['autoBird', 'autoStation', 'autoTowers'] as const) {
  test(`current-ID legacy Storm copy ${feature}`, async ({ page }, testInfo) => {
    const verifyNetwork = await prepare(page, 'dark');
    const seed = await page.evaluate(async (feature) => {
      const fixturePath = '/main.tsx';
      const { server } = await import(/* @vite-ignore */ fixturePath);
      const storm = (Object.values(server.built.state.castles) as CastleStateV2[]).find((castle) => castle.kingdomId === 4);
      if (!storm) throw new Error('Synthetic Storm castle is missing');
      const row = [{ id: 1, amount: 37 }];
      const value = feature === 'autoTowers' ? { castles: { [storm.id]: { enabled: true, unitId: 1, radius: 10 } } }
        : feature === 'autoBird' ? { ignoreSettings: { settings: { [storm.id]: row } }, presets: { presets: [] }, activePresetId: null }
        : { settings: { [storm.id]: row } };
      await server.handle(`/api/v2/config/automation.${feature}`, 'PUT', { value });
      server.log = [];
      return server.configuration().revision;
    }, feature);
    await openView(page, 'Automation', 'automation');

    const label = { autoBird: 'Auto Bird', autoStation: 'Auto Station', autoTowers: 'Auto Towers' }[feature];
    await page.locator('[data-view="automation"]').getByRole('button', { name: `Open ${label} settings`, exact: true }).click();
    const dialog = page.getByRole('dialog');
    const heading = dialog.getByText('Storm castle', { exact: true }).first();
    await expect(heading).toBeVisible();
    const card = heading.locator('xpath=ancestor::div[.//button[contains(.,"Copy to other castles")]][1]');
    await card.getByRole('button', { name: 'Copy to other castles…', exact: true }).click();
    const copy = page.getByRole('dialog').last();
    await expect(copy.getByText(/has no setup to copy yet/)).toHaveCount(0);
    await expect(copy.locator(`[data-castle-copy="${feature}"]`)).toBeVisible();
    await expect(copy.getByRole('button', { name: 'Apply to draft', exact: true })).toBeEnabled();
    await page.screenshot({ path: testInfo.outputPath(`${feature}-legacy-copy.png`) });
    await copy.getByRole('button', { name: 'Cancel', exact: true }).click();
    await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();

    const after = await page.evaluate(async () => {
      const fixturePath = '/main.tsx';
      const { server } = await import(/* @vite-ignore */ fixturePath);
      return { revision: server.configuration().revision, writes: server.log.filter((entry: { kind: string }) => entry.kind === 'config').length };
    });
    expect(after).toEqual({ revision: seed, writes: 0 });
    verifyNetwork();
  });
}


// Sophie's D3: Remove the idle role, Save, then reopen must keep it removed.
for (const feature of ['autoBird', 'autoStation'] as const) {
  test(`idle Storm role Remove Save reopen ${feature}`, async ({ page }, testInfo) => {
    const verifyNetwork = await prepare(page, 'dark');
    await openView(page, 'Automation', 'automation');
    const before = await page.evaluate(async (feature) => {
      const fixturePath = '/main.tsx';
      const { server } = await import(/* @vite-ignore */ fixturePath);
      const missing = Object.fromEntries((Object.values(server.built.state.castles) as CastleStateV2[]).filter((castle) => castle.kingdomId === 4).map((castle) => [castle.id, null]));
      server.file.runtime = [{ label: 'Synthetic no owned Storm', state: { castles: missing } }];
      if (!server.advance()) throw new Error('Synthetic absent frame was not applied');
      const row = [{ id: 1, amount: 37 }];
      const settings = { storm: row, 999: row, 998: row };
      const value = feature === 'autoBird' ? { stormLegacyKey: '999', ignoreSettings: { settings }, presets: { version: 1, presets: [] }, activePresetId: null } : { stormLegacyKey: '999', settings };
      await server.handle(`/api/v2/config/automation.${feature}`, 'PUT', { value });
      server.log = [];
      return server.configuration();
    }, feature);
    const snapshot = () => page.evaluate(async () => {
      const fixturePath = '/main.tsx';
      const { server } = await import(/* @vite-ignore */ fixturePath);
      return server.configuration();
    });
    const label = feature === 'autoBird' ? 'Auto Bird' : 'Auto Station';
    const open = () => page.locator('[data-view="automation"]').getByRole('button', { name: `Open ${label} settings`, exact: true }).click();
    await open();
    const dialog = page.getByRole('dialog');
    const idle = dialog.getByText('Storm castle · used when you have one', { exact: true });
    await expect(idle).toBeVisible();
    const card = idle.locator('xpath=ancestor::div[.//button[normalize-space()="Remove"]][1]');
    await card.getByRole('button', { name: 'Remove', exact: true }).click();
    await expect(idle).toHaveCount(0);
    expect(await snapshot()).toEqual(before, 'Remove changes only the draft');
    await dialog.getByRole('button', { name: 'Save changes', exact: true }).click();
    await expect(dialog).toHaveCount(0);
    const after = await snapshot();
    expect(after.revision).toBe(before.revision + 1);
    const saved = after.sections[`automation.${feature}`];
    const entries = feature === 'autoBird' ? saved.ignoreSettings.settings : saved.settings;
    expect(Object.hasOwn(entries, 'storm')).toBe(false);
    expect(Object.hasOwn(entries, '999')).toBe(false);
    expect(saved.stormLegacyKey).toBeUndefined();
    expect(entries['998']).toEqual([{ id: 1, amount: 37 }]);
    await open();
    await expect(idle).toHaveCount(0);
    await expect(dialog.getByText('Storm castle', { exact: true })).toHaveCount(0);
    await dialog.getByText(/Saved castle.*998/).last().scrollIntoViewIfNeeded();
    await page.screenshot({ path: testInfo.outputPath(`${feature}-idle-storm-removed.png`) });
    await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
    expect(await snapshot()).toEqual(after);
    verifyNetwork();
  });
}
