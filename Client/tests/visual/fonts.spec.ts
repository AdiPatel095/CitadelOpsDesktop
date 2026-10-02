import { test, expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { readFileSync, existsSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { prepare } from './harness';

const client = fileURLToPath(new URL('../../', import.meta.url));
const hosted = existsSync(resolve(client, 'src/commandCenter/styles/fonts.css'));
const repo = hosted ? client : resolve(client, '..');
const manifest = JSON.parse(readFileSync(resolve(client, 'public/fonts/manrope/fonts-manifest.json'), 'utf8'));
const css = readFileSync(resolve(client, hosted ? 'src/commandCenter/styles/fonts.css' : 'src/styles/fonts.css'), 'utf8');
const origin = `http://127.0.0.1:${(hosted ? 18461 : 41736) + Number(process.env.CIT_VISUAL_PORT_OFFSET ?? 0)}`;
const specimens = {
  latin: 'The kingdom 0123456789',
  'latin-ext': 'șțıİąėįųūěščřžďťňľĺŕőű',
  cyrillic: 'РусскийБългарски',
  'cyrillic-ext': 'ӨөҺһ',
  greek: 'Ελληνικάάέήίόύώ',
};

test.describe('Manrope delivery', () => {
  test.skip(!manifest.source.google_fonts_commit, 'Font provenance incomplete. Waiting for Oscar to pin google/fonts commit.');
  test.beforeAll(() => {
    // Once pinned, partial metadata/missing assets FAIL rather than silently skipping.
    execFileSync('python3', [resolve(repo, 'scripts/fonts/build-manrope'), '--check']);
  });

  for (const locale of ['en', 'ru']) {
    test(`${locale} landing requests only the required Manrope subsets`, async ({ page }) => {
      test.skip(!hosted, 'The public landing page belongs to the portal.');
      const verifyNetwork = await prepare(page, 'dark');
      const requested = new Set<string>();
      const loaded = new Set<string>();
      page.on('request', (request) => {
        const url = new URL(request.url());
        if (url.pathname.endsWith('.woff2')) requested.add(url.pathname);
      });
      page.on('response', (response) => {
        const url = new URL(response.url());
        if (url.pathname.endsWith('.woff2') && response.ok()) loaded.add(url.pathname);
      });
      await page.addInitScript((locale) => localStorage.setItem('citadelops.viewer-locale', locale), locale);
      await page.goto(origin);
      await expect(page.locator('html')).toHaveAttribute('lang', locale);
      await expect(page.locator('.portal-language select').first()).toHaveValue(locale);
      await expect(page.locator('.cop-landing__activity-updated').first()).toBeVisible();
      await page.waitForLoadState('networkidle');
      await page.evaluate(() => document.fonts.ready);
      if (locale === 'en') {
        expect([...requested]).toEqual(['/fonts/manrope/manrope-latin.woff2']);
        expect([...loaded]).toEqual(['/fonts/manrope/manrope-latin.woff2']);
      } else {
        expect(requested).toContain('/fonts/manrope/manrope-cyrillic.woff2');
        expect(loaded).toContain('/fonts/manrope/manrope-cyrillic.woff2');
        expect(await page.evaluate(() => Array.from(document.fonts).some((face) =>
          face.family === 'Manrope' && face.unicodeRange.includes('U+400-45F') && face.status === 'loaded'
        ))).toBe(true);
      }
      verifyNetwork();
    });
  }

  test('application stacks follow language in both themes', async ({ page }) => {
    const verifyNetwork = await prepare(page, 'dark');
    if (hosted) await page.goto(origin);
    await page.waitForLoadState('networkidle');
    expect(await page.evaluate(() => getComputedStyle(document.body).fontSynthesis)).toBe('none');
    for (const theme of ['dark', 'light']) {
      const families = await page.evaluate((theme) => {
        document.documentElement.dataset.theme = theme;
        const samples = ['en', 'ar', 'ja', 'ko', 'zh-CN', 'zh-TW'].map((lang) => {
          const element = document.createElement('span');
          element.lang = lang;
          element.style.fontFamily = 'var(--font-sans)';
          element.textContent = 'CitadelOps 0123456789';
          document.body.append(element);
          const family = getComputedStyle(element).fontFamily;
          element.remove();
          return family;
        });
        return { body: getComputedStyle(document.body).fontFamily, samples };
      }, theme);
      expect(families.body).toMatch(/^Manrope,/);
      for (const family of families.samples) expect(family).toMatch(/^Manrope,/);
      for (const [index, system] of ['Manrope Fallback', 'SF Arabic', 'Hiragino Sans', 'Apple SD Gothic Neo', 'PingFang SC', 'PingFang TC'].entries()) {
        expect(families.samples[index]).toContain(system);
      }
    }
    await page.evaluate(() => document.fonts.ready);
    expect(await page.evaluate(() => document.fonts.check('16px Manrope'))).toBe(true);
    verifyNetwork();
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
      for (const weight of [400, 500, 600, 700]) {
        for (const [script, text] of Object.entries(samples)) {
          const element = document.createElement('span');
          element.id = `${script}-${weight}`;
          element.textContent = text;
          element.style.font = `${weight} 16px Manrope`;
          document.body.append(element);
          await document.fonts.load(`${weight} 16px Manrope`, text);
        }
        for (const digit of '0123456789') {
          const element = document.createElement('span');
          element.dataset.weight = String(weight);
          element.textContent = digit;
          element.style.cssText = `display:inline-block;font:${weight} 64px Manrope;font-variant-numeric:tabular-nums`;
          document.body.append(element);
        }
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
    for (const weight of [400, 500, 600, 700]) {
      const widths = await page.locator(`[data-weight="${weight}"]`).evaluateAll((elements) => elements.map((element) => element.getBoundingClientRect().width));
      expect(widths).toHaveLength(10);
      expect(Math.max(...widths) - Math.min(...widths)).toBeLessThan(0.01);
      for (const script of Object.keys(specimens)) {
        const { nodeId } = await session.send('DOM.querySelector', { nodeId: root.nodeId, selector: `#${script}-${weight}` });
        const { fonts } = await session.send('CSS.getPlatformFontsForNode', { nodeId });
        expect(fonts.length).toBeGreaterThan(0);
        expect(fonts.every((font) => font.isCustomFont && font.familyName.includes('Manrope'))).toBe(true);
      }
    }
    await session.detach();
  });
});
