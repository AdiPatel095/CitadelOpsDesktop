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

test('every accent, control-on and status token is forbidden on disabled descendants', async ({ page }, testInfo) => {
  const tokens = [...accents, '--control-on', ...statuses, '--status-custom-bg'];
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
