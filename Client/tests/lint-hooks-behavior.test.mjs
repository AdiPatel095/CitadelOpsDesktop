import assert from 'node:assert/strict';
import { after, before, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { chromium } from '@playwright/test';
import react from '@vitejs/plugin-react';
import { createServer } from 'vite';

let vite;
let browser;
let origin;
before(async () => {
  vite = await createServer({
    configFile: false, root: fileURLToPath(new URL('..', import.meta.url)),
    cacheDir: '.visual/lint-hooks-vite-cache', logLevel: 'silent',
    plugins: [react(), {
      name: 'lint-hooks-test-page',
      configureServer(server) {
        server.middlewares.use('/__lint_hooks', async (_request, response) => {
          const html = await server.transformIndexHtml('/__lint_hooks', `<!doctype html><html><body>
            <div id="root"></div><script type="module">
            import * as React from 'react'; import { createRoot } from 'react-dom/client';
            window.React = React; window.createRoot = createRoot;
            </script></body></html>`);
          response.setHeader('Content-Type', 'text/html'); response.end(html);
        });
      },
    }], server: { host: '127.0.0.1', port: 0 },
  });
  await vite.listen(); origin = vite.resolvedUrls.local[0];
  browser = await chromium.launch({ headless: true });
});
after(async () => { await browser?.close(); await vite?.close(); });

async function withPage(run) {
  const page = await browser.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  // No API transport or live account: refuse every nonlocal request.
  await page.route('**/*', route => new URL(route.request().url()).origin === new URL(origin).origin
    ? route.continue() : route.abort());
  try {
    await page.clock.install({ time: new Date('2026-09-29T12:00:00Z') });
    await page.goto(`${origin}__lint_hooks`);
    await page.waitForFunction(() => window.React && window.createRoot);
    await page.clock.pauseAt(new Date('2026-09-29T12:01:00Z'));
    await run(page); assert.deepEqual(errors, []);
  } finally { await page.close(); }
}

for (const refreshMs of [1000, 3000]) {
  test(`clock refreshes at ${refreshMs}ms, changes cadence and cleans up on unmount`, () => withPage(async page => {
    await page.evaluate(async refreshMs => {
      const { createElement, StrictMode } = window.React;
      const { useNow } = await import('/src/useNow.ts');
      function Clock({ cadence }) {
        const now = useNow(cadence);
        return createElement('output', { id: 'clock', 'data-cadence': cadence }, String(now));
      }
      const root = window.createRoot(document.getElementById('root'));
      window.renderClock = cadence => root.render(createElement(StrictMode, null, createElement(Clock, { cadence })));
      window.unmountClock = () => root.unmount();
      window.renderClock(refreshMs);
    }, refreshMs);
    await page.locator('#clock').waitFor();
    const start = Number(await page.locator('#clock').textContent());
    await page.clock.runFor(refreshMs - 1);
    assert.equal(Number(await page.locator('#clock').textContent()), start);
    await page.clock.runFor(1);
    await page.waitForFunction(start => Number(document.getElementById('clock').textContent) > start, start);
    assert.equal(Number(await page.locator('#clock').textContent()), start + refreshMs);
    await page.evaluate(() => window.renderClock(5000));
    await page.waitForFunction(() => document.getElementById('clock').dataset.cadence === '5000');
    await page.clock.runFor(refreshMs);
    assert.equal(Number(await page.locator('#clock').textContent()), start + refreshMs);
    await page.clock.runFor(5000 - refreshMs);
    await page.waitForFunction(expected => Number(document.getElementById('clock').textContent) === expected, start + refreshMs + 5000);
    // Record any surviving interval callback, even if React ignores a post-unmount setState.
    await page.evaluate(() => {
      window.unmountClock();
      const original = Date.now;
      window.clockReads = 0;
      Date.now = () => { window.clockReads++; return original(); };
    });
    await page.clock.runFor(10_000);
    assert.equal(await page.evaluate(() => window.clockReads), 0);
  }));
}

test('copy replay reads the reset draft after reload and preserves unrelated latest settings', () => withPage(async page => {
  await page.evaluate(async () => {
    const { createElement, useEffect, useState, StrictMode } = window.React;
    const { useCastleCopyReplayState, useCastleCopyReplayRun } = await import('/src/settings/copy/useCastleCopyReplay.tsx');
    const core = await import('/src/settings/copy/castleCopy.ts');
    const { makeReplay } = await import('/src/settings/copy/castleCopyReplay.ts');
    const { stationCopyDescriptor: descriptor } = await import('/src/settings/copy/features/station.ts');
    const { castleCandidates } = await import('/src/settings/copy/candidates.ts');
    const castles = [1, 2, 3].map(id => ({ id, name: `C${id}`, kingdomId: 0, slotType: id === 1 ? 1 : 4,
      units: { stationed: { 2: 500 } }, unitsObservedAt: '0001-01-01T00:00:00Z', resources: {}, buildings: {} }));
    const state = { castles: Object.fromEntries(castles.map(castle => [castle.id, castle])) };
    const context = { state, troops: { 2: { id: 2, name: 'Crossbowmen' } }, tools: {}, metadataReady: true,
      observation: { connected: true, session: { generation: 5, baselineGeneration: 5, changedAt: '2026-09-29T09:00:00Z' } },
      candidates: castleCandidates(castles, state) };
    const original = { 1: [{ id: 2, amount: 100 }] };
    const preview = core.previewCastleCopy(descriptor, original, '1', ['2'], context);
    const input = core.defaultCopyInput(descriptor, preview);
    const record = makeReplay('1', input, preview, core.buildCopySelection(preview, input));
    function Editor() {
      const copy = useCastleCopyReplayState();
      const [draft, setDraft] = useState(original);
      const [reloaded, setReloaded] = useState(false);
      const [open, setOpen] = useState(true);
      useEffect(() => {
        if (reloaded) setDraft({ ...original, 3: [{ id: 2, amount: 7 }] });
      }, [reloaded]);
      useCastleCopyReplayRun(copy, { descriptor, draft, context, featureLabel: 'Auto Station', applyDraft: setDraft, isOpen: open });
      return createElement('div', null,
        createElement('button', { id: 'reload', onClick: () => { copy.setReplay(record); setReloaded(true); copy.setPhase('loaded'); } }, 'Reload'),
        createElement('button', { id: 'close', onClick: () => setOpen(false) }, 'Close'),
        createElement('output', { id: 'draft' }, JSON.stringify(draft)),
        createElement('output', { id: 'status' }, String(copy.status)),
        createElement('output', { id: 'record' }, String(copy.replay !== null)));
    }
    window.createRoot(document.getElementById('root')).render(createElement(StrictMode, null, createElement(Editor)));
  });
  await page.locator('#reload').click();
  await page.waitForFunction(() => document.getElementById('status').textContent === 'true');
  assert.deepEqual(JSON.parse(await page.locator('#draft').textContent()), {
    1: [{ id: 2, amount: 100 }], 2: [{ id: 2, amount: 100 }], 3: [{ id: 2, amount: 7 }],
  });
  await page.locator('#close').click();
  await page.waitForFunction(() => document.getElementById('record').textContent === 'false');
  assert.equal(await page.locator('#status').textContent(), 'false');
}));
