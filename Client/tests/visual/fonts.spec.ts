import { test, expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { readFileSync, existsSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const client = fileURLToPath(new URL('../../', import.meta.url));
const hosted = existsSync(resolve(client, 'src/commandCenter/styles/fonts.css'));
const repo = hosted ? client : resolve(client, '..');
const manifest = JSON.parse(readFileSync(resolve(client, 'public/fonts/manrope/fonts-manifest.json'), 'utf8'));
const css = readFileSync(resolve(client, hosted ? 'src/commandCenter/styles/fonts.css' : 'src/styles/fonts.css'), 'utf8');
const origin = hosted ? 'http://127.0.0.1:18461' : 'http://127.0.0.1:41736';
const specimens = {
  latin: 'The kingdom 0123456789',
  'latin-ext': 'șțıİąėįųūěščřžďťňľĺŕőű',
  cyrillic: 'РусскийБългарски',
  'cyrillic-ext': 'ӨөҺһ',
  greek: 'Ελληνικάάέήίόύώ',
};

test.describe('Manrope delivery — provenance-gated scaffold', () => {
  test.skip(!manifest.source.google_fonts_commit, 'Font provenance incomplete. Waiting for Oscar to pin google/fonts commit.');
  test.beforeAll(() => {
    // Once pinned, partial metadata/missing assets FAIL rather than silently skipping.
    execFileSync('python3', [resolve(repo, 'scripts/fonts/build-manrope'), '--check']);
  });

  test('every script uses Manrope from self with no fallback glyphs', async ({ page, context }) => {
    const external: string[] = [];
    const requested = new Set<string>();
    await context.route('**/*', async (route) => {
      const url = new URL(route.request().url());
      if (url.origin !== origin) {
        external.push(url.href);
        return route.abort('blockedbyclient');
      }
      if (url.pathname.endsWith('.woff2')) requested.add(url.pathname);
      if (url.pathname === '/__manrope_specimen') {
        return route.fulfill({ contentType: 'text/html', body: `<html lang="en"><head><style>${css}</style></head><body></body></html>` });
      }
      return route.continue();
    });
    await page.goto(`${origin}/__manrope_specimen`);
    await page.evaluate(async (samples) => {
      for (const [script, text] of Object.entries(samples)) {
        const element = document.createElement('span');
        element.id = script;
        element.textContent = text;
        element.style.font = '400 16px Manrope';
        document.body.append(element);
        await document.fonts.load('400 16px Manrope', text);
      }
      await document.fonts.ready;
    }, specimens);
    expect(await page.evaluate(() => document.fonts.check('16px Manrope'))).toBe(true);
    expect(await page.evaluate(() => Array.from(document.fonts).filter((face) => face.family === 'Manrope').map((face) => face.status))).toEqual(Array(5).fill('loaded'));
    expect([...requested].sort()).toEqual(Object.keys(specimens).map((script) => `/fonts/manrope/manrope-${script}.woff2`).sort());
    expect(external).toEqual([]);
    const session = await context.newCDPSession(page);
    await session.send('DOM.enable');
    await session.send('CSS.enable');
    const { root } = await session.send('DOM.getDocument');
    for (const script of Object.keys(specimens)) {
      const { nodeId } = await session.send('DOM.querySelector', { nodeId: root.nodeId, selector: `#${script}` });
      const { fonts } = await session.send('CSS.getPlatformFontsForNode', { nodeId });
      expect(fonts.length).toBeGreaterThan(0);
      expect(fonts.every((font) => font.isCustomFont && font.familyName.includes('Manrope'))).toBe(true);
    }
    await session.detach();
  });
  // After Oscar's gate: extend to all four weights, tnum and locale cases;
  // record Lighthouse medians and baseline diffs before font-stack integration.
});
