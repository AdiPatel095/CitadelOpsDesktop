import { readFileSync } from 'node:fs';
import { test, expect, type Page } from '@playwright/test';
import type { Violation } from './checks';
import { openCase, writeReport } from './harness';
import { gateCases, type GateCase } from './views';

// Serialized into the browser for both unfocused and focused snapshots. Compare
// descendants too: Switch intentionally puts its outline on its track.
function focusSnapshot(mode: 'baseline' | 'active') {
  let active = document.activeElement as HTMLElement;
  while (active.shadowRoot?.activeElement) active = active.shadowRoot.activeElement as HTMLElement;
  const roots = mode === 'active' ? [active] : [...document.querySelectorAll<HTMLElement>('[data-gate-keyboard]')];
  return roots.map(root => {
    const nodes: HTMLElement[] = [root];
    function collect(element: HTMLElement) {
      for (const child of element.children) {
        if (child instanceof HTMLElement) { nodes.push(child); collect(child); }
      }
      if (element.shadowRoot) for (const child of element.shadowRoot.children) {
        if (child instanceof HTMLElement) { nodes.push(child); collect(child); }
      }
    }
    collect(root);
    return { id: root.getAttribute('data-gate-keyboard'), focusVisible: nodes.some(node => node.matches(':focus-visible')),
      styles: nodes.map(node => {
        const style = getComputedStyle(node); const rect = node.getBoundingClientRect();
        const visible = rect.width > 0 && rect.height > 0 && style.visibility !== 'hidden'
          && style.display !== 'none' && node.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true });
        const outline = [style.outlineStyle, style.outlineWidth, style.outlineColor, style.outlineOffset].join(' ');
        return { visible, outline, hasOutline: style.outlineStyle !== 'none' && parseFloat(style.outlineWidth) > 0
          && !['transparent', 'rgba(0, 0, 0, 0)'].includes(style.outlineColor),
          shadow: style.boxShadow, background: style.backgroundColor, border: style.borderColor };
      }) };
  });
}

async function keyboardWalk(page: Page): Promise<Violation[]> {
  const candidates = await page.evaluate(() => {
    const interactive = 'button, a[href], input:not([type="hidden"]), select, textarea, summary, [role="button"], [role="tab"], [role="switch"], [role="checkbox"], [role="combobox"]';
    const modal = [...document.querySelectorAll<HTMLElement>('[role="dialog"][aria-modal="true"]')].find(element => element.getClientRects().length > 0);
    const scope = modal ?? document;
    const elements: HTMLElement[] = [];
    function collect(root: Document | ShadowRoot | HTMLElement) {
      for (const element of root.querySelectorAll<HTMLElement>('*')) {
        if (element.matches(`${interactive}, [tabindex]`)) elements.push(element);
        if (element.shadowRoot) collect(element.shadowRoot);
      }
    }
    collect(scope);
    return elements.filter(element => {
      const rect = element.getBoundingClientRect();
      return rect.width > 0 && rect.height > 0 && getComputedStyle(element).visibility !== 'hidden'
        && !element.matches(':disabled') && !element.closest('[inert]')
        && (element.tabIndex >= 0 || (element.matches(interactive) && !element.closest('[role="tablist"], [role="radiogroup"], [role="menu"], [role="listbox"]')));
    }).map((element, index) => {
      const id = String(index);
      element.setAttribute('data-gate-keyboard', id);
      return { id, label: `${element.tagName.toLowerCase()} ${element.getAttribute('aria-label') ?? element.textContent?.trim().replace(/\s+/g, ' ').slice(0, 100) ?? ''}` };
    });
  });
  const violations: Violation[] = [];
  const reached = new Set<string>();
  // Leave the pointer-selected navigation control before starting the Tab walk.
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
  const baseline = await page.evaluate(focusSnapshot, 'baseline');
  const limit = candidates.length * 2 + 10;
  let first: string | undefined;
  for (let step = 0; step < limit; step++) {
    await page.keyboard.press('Tab');
    const [active] = await page.evaluate(focusSnapshot, 'active');
    if (active.id === null) continue;
    if (first === active.id) break;
    first ??= active.id;
    reached.add(active.id);
    const before = candidates.find(candidate => candidate.id === active.id)!;
    const previous = baseline.find(snapshot => snapshot.id === active.id)!;
    const indicator = active.styles.some((style, index) => {
      if (!style.visible) return false;
      const old = previous.styles[index];
      // Static decorative descendant outlines must not make a ringless control pass.
      if (style.hasOutline && (index === 0 || !old?.visible || style.outline !== old.outline)) return true;
      return old?.visible && (style.shadow !== old.shadow || style.background !== old.background || style.border !== old.border);
    });
    if (!active.focusVisible || !indicator) {
      violations.push({ rule: 'focusVisible', element: before.label, detail: 'Tab focus has no detected visible outline, shadow, background or border indicator' });
    }
  }
  // Composite widgets use arrow keys after their one Tab stop.
  for (const group of await page.locator('[role="tablist"], [role="radiogroup"]').all()) {
    if (!await group.isVisible()) continue;
    const controls = group.locator('[role="tab"], [role="radio"]');
    const count = await controls.count();
    const start = group.locator('[tabindex="0"]').first();
    if (!await start.count()) continue;
    await start.focus();
    const seen = new Set<number>();
    for (let step = 0; step < count + 1; step++) {
      const index = await controls.evaluateAll(elements => elements.indexOf(document.activeElement as HTMLElement));
      if (index >= 0) seen.add(index);
      await page.keyboard.press('ArrowRight');
    }
    if (seen.size < count) violations.push({ rule: 'keyboardReachable', element: await group.getAttribute('aria-label') ?? 'composite', detail: `Arrow navigation reached ${seen.size}/${count} controls` });
  }
  for (const candidate of candidates) {
    if (!reached.has(candidate.id)) violations.push({ rule: 'keyboardReachable', element: candidate.label, detail: 'Visible enabled control was not reached in the Tab cycle' });
  }
  return violations;
}

async function settingsFocus(page: Page): Promise<Violation[]> {
  const violations: Violation[] = [];
  const opener = page.getByRole('button', { name: 'Open Auto Towers settings', exact: true });
  await opener.focus();
  await page.keyboard.press('Enter');
  const dialog = page.getByRole('dialog');
  await dialog.waitFor({ state: 'visible' });
  const inside = () => dialog.evaluate(element => element.contains(document.activeElement));
  if (!(await inside())) violations.push({ rule: 'dialogFocus', element: 'Auto Towers', detail: 'Opening the settings dialog did not move focus inside it' });
  const count = await dialog.locator('button, a[href], input:not([type="hidden"]), select, textarea, [tabindex]').count();
  for (const key of ['Tab', 'Shift+Tab']) {
    for (let step = 0; step <= count; step++) {
      await page.keyboard.press(key);
      if (!(await inside())) {
        violations.push({ rule: 'dialogFocusTrap', element: 'Auto Towers', detail: `${key} escaped the settings dialog` });
        break;
      }
    }
  }
  await page.keyboard.press('Escape');
  try {
    await dialog.waitFor({ state: 'hidden', timeout: 2000 });
  } catch {
    violations.push({ rule: 'dialogEscape', element: 'Auto Towers', detail: 'Escape did not close the settings dialog' });
  }
  if (!(await opener.evaluate(element => element === document.activeElement))) {
    violations.push({ rule: 'dialogReturnFocus', element: 'Auto Towers', detail: 'Focus did not return to the settings opener' });
  }
  return violations;
}

async function currentDialogFocus(page: Page, entry: GateCase): Promise<Violation[]> {
  const violations: Violation[] = [];
  const dialog = page.getByRole('dialog').first();
  const opener = page.locator('[data-gate-opener="true"]');
  const inside = () => dialog.evaluate(element => element.contains(document.activeElement));
  if (!await inside()) violations.push({ rule: 'dialogFocus', element: entry.name, detail: 'Dialog did not take focus' });
  const count = await dialog.locator('button, a[href], input:not([type="hidden"]), select, textarea, [tabindex], md-filled-button, md-text-button').count();
  for (const key of ['Tab', 'Shift+Tab']) {
    for (let step = 0; step <= count; step++) {
      await page.keyboard.press(key);
      if (!await inside()) {
        violations.push({ rule: 'dialogFocusTrap', element: entry.name, detail: `${key} escaped the dialog` });
        break;
      }
    }
  }
  await page.keyboard.press('Escape');
  try { await dialog.waitFor({ state: 'hidden', timeout: 2000 }); }
  catch { violations.push({ rule: 'dialogEscape', element: entry.name, detail: 'Escape did not close the dialog' }); }
  if (!await opener.evaluate(element => element === document.activeElement || element.contains(document.activeElement))) violations.push({ rule: 'dialogReturnFocus', element: entry.name, detail: 'Focus did not return to the opener' });
  return violations;
}

for (const locale of ['en', 'ar'] as const) {
for (const entry of gateCases) {
  test(`keyboard ${entry.name} ${locale} dark`, async ({ page }, testInfo) => {
    test.skip(!['1440', '390'].includes(testInfo.project.name), 'Keyboard coverage is 1440 and 390');
    const { verifyNetwork, unavailable } = await openCase(page, entry, { theme: 'dark', locale });
    const violations = [...unavailable];
    if (!unavailable.length) {
      await page.waitForLoadState('networkidle');
      await page.evaluate(() => document.fonts.ready);
      violations.push(...await keyboardWalk(page));
      if (entry.settings || entry.dialog) violations.push(...await currentDialogFocus(page, entry));
    }
    verifyNetwork();
    await writeReport(testInfo, 'keyboard', entry, violations, { theme: 'dark', locale });
  });
}

}

test('settings modal keyboard focus dark', async ({ page }, testInfo) => {
  test.skip(!['1440', '390'].includes(testInfo.project.name), 'Keyboard coverage is 1440 and 390');
  const entry: GateCase = { name: 'settings-auto-towers-focus', variant: 'app', path: '/dashboard', label: 'Automation', view: 'automation', ready: '[data-view="automation"]' };
  const { verifyNetwork, unavailable } = await openCase(page, entry);
  const violations = [...unavailable];
  if (!unavailable.length) violations.push(...await settingsFocus(page));
  verifyNetwork();
  await writeReport(testInfo, 'keyboard', entry, violations);
});


test('focus-rule fixture: Switch child focus indicator passes', async ({ page }) => {
  const switchCss = readFileSync(new URL('../../' + (process.cwd().endsWith('/Client') ? 'src' : 'src/commandCenter') + '/components/ui/switch.css', import.meta.url), 'utf8');
  await page.setContent(`<style>:root { --focus-ring: #2463eb; --border-strong: #555; --surface-control: #eee; } ${switchCss}</style>
    <button type="button" role="switch" aria-checked="false" aria-label="Fixture Switch" class="ui-switch">
      <span aria-hidden="true" class="ui-switch__track"></span><span aria-hidden="true" class="ui-switch__thumb"></span>
    </button>`);
  expect(await keyboardWalk(page)).toEqual([]);
});

test('focus-rule fixture: control with no indicator fails', async ({ page }) => {
  await page.setContent(`<style>button, button:focus-visible { outline: none; box-shadow: none; background: white; border: 0; }
    span { display: inline-block; outline: 2px solid blue; }</style>
    <button aria-label="No focus indicator"><span>Static decoration</span></button>`);
  expect(await keyboardWalk(page)).toEqual([{
    rule: 'focusVisible', element: 'button No focus indicator',
    detail: 'Tab focus has no detected visible outline, shadow, background or border indicator',
  }]);
});
