import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import type { Page } from '@playwright/test';

const accentTokens = ['--accent', '--accent-hover', '--accent-pressed', '--accent-container', '--text-on-accent-container'];
const disabledSelector = ':disabled, [aria-disabled="true"], [role="switch"][aria-checked="false"]';
const accentHooks = '[data-variant="primary"], [aria-current="page"], [role="tab"][aria-selected="true"], [data-current-selection="true"], [data-brand-mark]';

async function scanColours(page: Page, disabled: boolean) {
  return page.evaluate(({ accentTokens, disabledSelector, accentHooks, disabled }) => {
    const probe = document.createElement('span');
    probe.style.setProperty('display', 'none', 'important');
    document.documentElement.append(probe);
    const offenders: { element: string; pseudo: string; property: string; colour: string; tokens: string[] }[] = [];

    function describe(element: Element): string {
      if (element.id) return `${element.localName}#${element.id}`;
      const parent = element.parentElement;
      const index = parent ? Array.from(parent.children).indexOf(element) + 1 : 1;
      return `${parent ? `${describe(parent)} > ` : ''}${element.localName}:nth-child(${index})`;
    }

    function resolveColour(value: string): string | undefined {
      probe.style.removeProperty('color');
      if (!value.trim() || !CSS.supports('color', value)) return undefined;
      probe.style.setProperty('color', value, 'important');
      return getComputedStyle(probe).color;
    }

    try {
      const elements = new Set<Element>();
      if (disabled) {
        for (const control of document.querySelectorAll(disabledSelector)) {
          elements.add(control);
          for (const child of control.querySelectorAll('*')) elements.add(child);
        }
      } else {
        for (const element of document.querySelectorAll('*')) {
          if (element !== probe && !element.closest(accentHooks)) elements.add(element);
        }
      }

      for (const element of elements) {
        for (const pseudo of ['', '::before', '::after']) {
          const style = getComputedStyle(element, pseudo || null);
          if (pseudo && (style.content === 'none' || style.content === 'normal')) continue;
          const names = [...accentTokens];
          if (disabled) {
            names.push('--control-on', ...Array.from(style).filter((name) => name.startsWith('--status-')));
          }
          const tokenColours = new Map<string, string[]>();
          for (const name of names) {
            const colour = resolveColour(style.getPropertyValue(name));
            if (colour) tokenColours.set(colour, [...(tokenColours.get(colour) ?? []), name]);
          }

          const properties = ['color', 'background-color', 'outline-color'];
          for (const side of ['top', 'right', 'bottom', 'left']) {
            if (parseFloat(style.getPropertyValue(`border-${side}-width`)) > 0) properties.push(`border-${side}-color`);
          }
          if (element.namespaceURI === 'http://www.w3.org/2000/svg') properties.push('fill', 'stroke');
          const colours = properties.map((property) => ({ property, colour: resolveColour(style.getPropertyValue(property)) }));
          // Computed shadows serialize their colours separately from offsets and radii.
          const shadowColours = style.boxShadow.match(/(?:rgba?|hsla?|hwb|lab|lch|oklab|oklch|color)\([^()]*\)|#[\da-f]+/gi) ?? [];
          for (const value of shadowColours) colours.push({ property: 'box-shadow', colour: resolveColour(value) });
          for (const { property, colour } of colours) {
            const tokens = colour && tokenColours.get(colour);
            if (tokens && colour) offenders.push({ element: describe(element), pseudo, property, colour, tokens });
          }
        }
      }
      return {
        width: window.innerWidth,
        theme: document.documentElement.getAttribute('data-theme') ?? document.body.getAttribute('data-theme') ?? (document.documentElement.classList.contains('dark') ? 'dark' : 'light'),
        offenders,
      };
    } finally {
      probe.remove();
    }
  }, { accentTokens, disabledSelector, accentHooks, disabled });
}

export async function assertDisabledNeutral(page: Page): Promise<void> {
  const { offenders } = await scanColours(page, true);
  if (offenders.length) {
    throw new Error(`Disabled/off controls use semantic or accent colours:\n${offenders.map(({ element, pseudo, property, colour, tokens }) => `${element}${pseudo}: ${property} = ${colour} (${tokens.join(', ')})`).join('\n')}`);
  }
}

export async function reportAccentUsage(page: Page, caseName: string): Promise<void> {
  // R2 is report-only, including when the page or output directory is unavailable.
  try {
    const report = await scanColours(page, false);
    const safeName = (value: string) => value.replace(/[^a-zA-Z0-9_-]/g, '_');
    const directory = join('test-results', 'accent-report');
    await mkdir(directory, { recursive: true });
    await writeFile(join(directory, `${safeName(caseName)}-${report.width}-${safeName(report.theme)}.json`), `${JSON.stringify(report, null, 2)}\n`);
  } catch {
    // Reporting must never turn an otherwise passing snapshot into a failure.
  }
}
