import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { createServer } from 'vite';
import { readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';
import { assertDisabledNeutral, reportAccentUsage } from '../visual/rules';

const accents = ['--accent', '--accent-hover', '--accent-pressed', '--accent-container', '--text-on-accent-container'];
const statuses = ['success', 'warning', 'danger', 'info', 'neutral'].flatMap((tone) => [`--status-${tone}`, `--status-${tone}-bg`, `--status-${tone}-border`]);
const hooks = ['data-variant="primary"', 'aria-current="page"', 'role="tab" aria-selected="true"', 'data-current-selection="true"', 'data-brand-mark'];

async function content(page: Page, theme: string, markup: string, css = '') {
  const tokens = [...accents, '--control-on', ...statuses].map((name, index) => `${name}: rgb(${100 + index}, ${40 + index}, ${30 + index});`).join('\n');
  await page.setContent(`<html data-theme="${theme}"><head><style>
    :root { ${tokens} }
    body { color: rgb(10, 10, 10); }
    button { color: inherit; background: transparent; border: 0; }
    ${css}
  </style></head><body>${markup}</body></html>`);
}

async function report(page: Page, caseName: string, theme: string) {
  await reportAccentUsage(page, caseName);
  const width = page.viewportSize()!.width;
  return JSON.parse(await readFile(join('test-results', 'accent-report', `${caseName}-${width}-${theme}.json`), 'utf8')) as {
    width: number; theme: string; offenders: { element: string; pseudo: string; property: string; colour: string; tokens: string[] }[];
  };
}

test('coral off switches and disabled status buttons list offenders', async ({ page }, testInfo) => {
  await content(page, testInfo.project.name, '<button id="off" role="switch" aria-checked="false">Off</button><button id="disabled" disabled>Disabled</button>', '#off { background: var(--accent); } #disabled { color: var(--status-success); }');
  await expect(assertDisabledNeutral(page)).rejects.toThrow(/button#off: background-color.*--accent/);
  await expect(assertDisabledNeutral(page)).rejects.toThrow(/button#disabled: color.*--status-success/);
});

test('every accent, control-on and chromatic status token is forbidden on disabled descendants', async ({ page }, testInfo) => {
  const tokens = [...accents, '--control-on', ...statuses.filter((token) => !token.startsWith('--status-neutral'))];
  await content(page, testInfo.project.name, `<section aria-disabled="true">${tokens.map((token, index) => `<span id="token-${index}" style="background-color: var(${token})">Child</span>`).join('')}</section>`, ':root { --status-custom-bg: rgb(230, 40, 70); }');
  let message = '';
  try { await assertDisabledNeutral(page); } catch (error) { message = String(error); }
  for (const [index, token] of tokens.entries()) {
    expect(message).toContain(`span#token-${index}: background-color`);
    expect(message).toContain(token);
  }
});

test('all checked properties catch forbidden colours including multiple shadows', async ({ page }, testInfo) => {
  const properties = ['color', 'background-color', 'border-top-color', 'border-right-color', 'border-bottom-color', 'border-left-color', 'outline-color', 'box-shadow'];
  await content(page, testInfo.project.name, properties.map((property, index) => `<div aria-disabled="true" id="property-${index}" style="border: 1px solid rgb(10,10,10); ${property}: ${property === 'box-shadow' ? '0 0 1px rgb(10,10,10), 0 0 2px var(--accent)' : 'var(--accent)'}">Control</div>`).join(''));
  let message = '';
  try { await assertDisabledNeutral(page); } catch (error) { message = String(error); }
  for (const [index, property] of properties.entries()) expect(message).toContain(`div#property-${index}: ${property}`);
});

test('neutral controls pass and zero-width accent borders are ignored', async ({ page }, testInfo) => {
  await content(page, testInfo.project.name, '<button disabled>Disabled</button><div aria-disabled="true"><span>Child</span></div><button role="switch" aria-checked="false">Off</button>', 'button { border: 0 solid var(--accent); }');
  await assertDisabledNeutral(page);
});

test('each C6 hook permits accents on itself and descendants, while all five unhooked accents report', async ({ page }, testInfo) => {
  await content(page, testInfo.project.name, `${hooks.map((hook, index) => `<div id="hook-${index}" ${hook} style="color: var(--accent)"><span>Accent child</span></div>`).join('')}${accents.map((token, index) => `<div id="unhooked-${index}" style="background-color: var(${token})">Accent</div>`).join('')}`);
  const result = await report(page, 'hooks', testInfo.project.name);
  expect(result.width).toBe(page.viewportSize()!.width);
  expect(result.theme).toBe(testInfo.project.name);
  expect(result.offenders.some((entry) => entry.element.includes('hook-'))).toBe(false);
  for (const [index, token] of accents.entries()) {
    expect(result.offenders).toContainEqual(expect.objectContaining({ element: `div#unhooked-${index}`, property: 'background-color', tokens: [token] }));
  }
  // C6 never exempts disabled controls from R3.
  await page.locator('#hook-0').evaluate((element) => element.setAttribute('aria-disabled', 'true'));
  await expect(assertDisabledNeutral(page)).rejects.toThrow(/div#hook-0: color/);
});

test('inactive hooks do not permit accents', async ({ page }, testInfo) => {
  await content(page, testInfo.project.name, '<div id="wrong" data-variant="secondary" aria-current="false" role="tab" aria-selected="false" data-current-selection="false" style="color: var(--accent)">Accent</div>');
  expect((await report(page, 'inactive-hooks', testInfo.project.name)).offenders).toContainEqual(expect.objectContaining({ element: 'div#wrong', property: 'color' }));
});

test('before, after and SVG fill/stroke are scanned for R3 and R2', async ({ page }, testInfo) => {
  await content(page, testInfo.project.name, '<div aria-disabled="true"><span id="pseudo">Control</span><svg><path id="icon" d="M0 0h10" /></svg></div>', '#pseudo::before { content: "Before"; background: var(--accent); } #pseudo::after { content: "After"; box-shadow: 0 0 2px var(--accent-hover); } #icon { fill: var(--accent-container); stroke: var(--accent-pressed); }');
  let message = '';
  try { await assertDisabledNeutral(page); } catch (error) { message = String(error); }
  expect(message).toContain('span#pseudo::before: background-color');
  expect(message).toContain('span#pseudo::after: box-shadow');
  expect(message).toContain('path#icon: fill');
  expect(message).toContain('path#icon: stroke');
  const result = await report(page, 'pseudo-svg', testInfo.project.name);
  for (const [element, pseudo, property] of [['span#pseudo', '::before', 'background-color'], ['span#pseudo', '::after', 'box-shadow'], ['path#icon', '', 'fill'], ['path#icon', '', 'stroke']]) {
    expect(result.offenders).toContainEqual(expect.objectContaining({ element, pseudo, property }));
  }
});

test('locally overridden tokens resolve through the colour probe', async ({ page }, testInfo) => {
  await content(page, testInfo.project.name, '<div id="local" aria-disabled="true" style="--accent: #ff1234; background: var(--accent)">Local</div>');
  await expect(assertDisabledNeutral(page)).rejects.toThrow(/div#local: background-color = rgb\(255, 18, 52\)/);
  expect((await report(page, 'local-token', testInfo.project.name)).offenders).toContainEqual(expect.objectContaining({ element: 'div#local', colour: 'rgb(255, 18, 52)' }));
});

test('accent reporting never fails when the page is unavailable', async ({ page }) => {
  await page.close();
  await reportAccentUsage(page, 'closed-page');
});

test('neutral status colours are allowed on disabled and off controls', async ({ page }, testInfo) => {
  await content(page, testInfo.project.name, '<button disabled style="color: var(--status-neutral); background: var(--status-neutral-bg); border: 1px solid var(--status-neutral-border)">Disabled</button><button role="switch" aria-checked="false" style="background: var(--status-neutral-bg)">Off</button>');
  await assertDisabledNeutral(page);
});

const root = fileURLToPath(new URL('../../', import.meta.url));
const source = existsSync(join(root, 'src/commandCenter')) ? 'src/commandCenter' : 'src';
let vite: Awaited<ReturnType<typeof createServer>>;
let renderSwitch: (checked: boolean, disabled?: boolean) => string;
let renderDelta: (value: number, text: string) => string;
test.beforeAll(async () => {
  vite = await createServer({ root, configFile: false, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true, hmr: false } });
  const { Switch } = await vite.ssrLoadModule(`/${source}/components/ui/Switch.tsx`);
  const { DeltaValue } = await vite.ssrLoadModule(`/${source}/components/ui/DeltaValue.tsx`);
  renderSwitch = (checked, disabled = false) => renderToStaticMarkup(createElement(Switch, { checked, disabled, onChange() {}, ariaLabel: `${disabled ? 'Disabled' : 'Enabled'} ${checked ? 'on' : 'off'}` }));
  renderDelta = (value, text) => renderToStaticMarkup(createElement(DeltaValue, { value }, text));
});
test.afterAll(async () => vite?.close());

test('real Switch tokens, compact target, RTL and keyboard focus preserve the contract', async ({ page }, testInfo) => {
  const tokens = await readFile(join(root, source, 'styles/tokens.css'), 'utf8');
  const css = await readFile(join(root, source, 'components/ui/switch.css'), 'utf8');
  await page.setContent(`<style>${tokens} ${css}</style>${renderSwitch(false)}${renderSwitch(true)}${renderSwitch(false, true)}${renderSwitch(true, true)}`);
  await page.locator('html').evaluate((element, theme) => element.setAttribute('data-theme', theme), testInfo.project.name);
  await assertDisabledNeutral(page);
  const off = page.getByRole('switch', { name: 'Enabled off' });
  const on = page.getByRole('switch', { name: 'Enabled on' });
  await expect(on.locator('svg')).toHaveAttribute('width', '12');
  await expect(off.locator('svg')).toHaveCount(0);
  await page.keyboard.press('Tab');
  await expect(off).toBeFocused();
  expect(await off.locator('.ui-switch__track').evaluate(element => getComputedStyle(element).outlineWidth)).toBe('2px');
  for (const dir of ['ltr', 'rtl']) {
    await page.locator('html').evaluate((element, direction) => element.dir = direction, dir);
    const positions = await page.locator('.ui-switch').evaluateAll(elements => elements.slice(0, 2).map(element => {
      const thumb = element.querySelector('.ui-switch__thumb')!.getBoundingClientRect();
      return thumb.x - element.getBoundingClientRect().x;
    }));
    expect(dir === 'ltr' ? positions[1] > positions[0] : positions[1] < positions[0]).toBe(true);
  }
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await off.evaluate(element => ({ width: element.getBoundingClientRect().width, height: element.getBoundingClientRect().height }))).toEqual({ width: 44, height: 44 });
  await assertDisabledNeutral(page);
});

test('Delta isolates Arabic gain and loss signs with semantic colours', async ({ page }, testInfo) => {
  const tokens = await readFile(join(root, source, 'styles/tokens.css'), 'utf8');
  const css = await readFile(join(root, source, 'components/ui/delta.css'), 'utf8');
  const arabic = new Intl.NumberFormat('ar').format(1620);
  await page.setContent(`<style>${tokens} ${css}</style><div dir="rtl">${renderDelta(1620, arabic)}${renderDelta(-1620, arabic)}${renderDelta(0, '0')}</div>`);
  await page.locator('html').evaluate((element, theme) => element.setAttribute('data-theme', theme), testInfo.project.name);
  await expect(page.locator('[data-delta="gain"]')).toHaveText(`+${arabic}`);
  await expect(page.locator('[data-delta="loss"]')).toHaveText(`−${arabic}`);
  await expect(page.locator('[data-delta="zero"]')).toHaveText('0');
  expect(await page.locator('bdi').evaluateAll(elements => elements.every(element => getComputedStyle(element).unicodeBidi === 'isolate'))).toBe(true);
  for (const [tone, token] of [['gain', '--status-success'], ['loss', '--status-danger'], ['zero', '--text-primary']]) {
    expect(await page.locator(`[data-delta="${tone}"]`).evaluate((element, token) => {
      const probe = document.createElement('span'); probe.style.color = `var(${token})`; element.after(probe);
      const matches = getComputedStyle(element).color === getComputedStyle(probe).color; probe.remove(); return matches;
    }, token)).toBe(true);
  }
});
test('R11 rejects duplicate primaries, including disabled actions, in every region type', async ({ page }) => {
  const { assertOnePrimaryPerRegion } = await import('../visual/rules');
  for (const region of ['data-region="card"', 'data-region="toolbar"', 'data-region="page-header"', 'role="dialog"']) {
    await page.setContent(`<section ${region}><button data-variant="primary">A</button><button data-variant="primary" disabled>B</button></section>`);
    await expect(assertOnePrimaryPerRegion(page)).rejects.toThrow(/R11/);
  }
});
test('R11 assigns nested actions to the nearest region and ignores hidden controls', async ({ page }) => {
  const { assertOnePrimaryPerRegion } = await import('../visual/rules');
  await page.setContent('<section data-region="card"><button data-variant="primary">A</button><div data-region="toolbar"><button data-variant="primary">B</button></div><button hidden data-variant="primary">Hidden</button></section>');
  await assertOnePrimaryPerRegion(page);
});
