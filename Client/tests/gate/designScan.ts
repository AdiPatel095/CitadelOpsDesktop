import type { Page } from '@playwright/test';
import { scanColours, assertOnePrimaryPerRegion } from '../visual/rules';
import { typographyFindings } from '../visual/typographyScan';
import type { Violation } from './checks';

export async function designRules(page: Page, locale: string): Promise<Violation[]> {
  const violations: Violation[] = [];
  for (const disabled of [false, true]) {
    const { offenders } = await scanColours(page, disabled);
    for (const item of offenders) violations.push({ rule: disabled ? 'R3' : 'R2', element: `${item.element}${item.pseudo}`, detail: `${item.property}: ${item.colour} (${item.tokens.join(', ')})` });
  }
  for (const detail of await typographyFindings(page)) violations.push({ rule: detail.includes(': size ') ? 'R4' : detail.includes(': weight ') ? 'R5' : 'R13', element: detail.split(': ')[0], detail });
  try { await assertOnePrimaryPerRegion(page); } catch (error) { violations.push({ rule: 'R11', element: 'page', detail: String(error) }); }
  violations.push(...await page.evaluate(scanDesignDOM, locale));
  return violations;
}

/** Self-contained DOM scan shared with browser rule fixtures. */
export function scanDesignDOM(locale: string): Violation[] {
    const findings: { rule: string; element: string; detail: string }[] = [];
    const statuses = ['running', 'waiting', 'done', 'paused', 'blocked', 'needs-attention', 'off', 'unknown'];
    const visible = (element: Element) => element.getClientRects().length > 0 && getComputedStyle(element).visibility !== 'hidden';
    const label = (element: Element) => `${element.tagName}.${element.getAttribute('class') ?? ''}: ${element.textContent?.trim().slice(0, 80)}`;
    for (const element of document.querySelectorAll('[data-player-status]')) {
      if (visible(element) && !statuses.includes(element.getAttribute('data-player-status')!)) findings.push({ rule: 'R12', element: label(element), detail: 'Status is outside the M1 vocabulary' });
    }
    for (const element of document.querySelectorAll<HTMLElement>('.ui-tabs')) {
      if (visible(element) && element.scrollWidth > element.clientWidth + 1) findings.push({ rule: 'R15', element: label(element), detail: 'Tab navigation overflows its container' });
    }
    if (matchMedia('(prefers-reduced-motion: reduce)').matches) {
      for (const animation of document.getAnimations()) {
        const effect = animation.effect;
        if (effect instanceof KeyframeEffect && Number(effect.getComputedTiming().activeDuration) > 1 && effect.getKeyframes().some(frame => frame.transform && frame.transform !== 'none')) findings.push({ rule: 'R16', element: effect.target ? label(effect.target) : 'animation', detail: 'Transform animation remains active with reduced motion' });
      }
    }
    if (locale === 'ar') {
      for (const element of document.querySelectorAll<HTMLElement>('body *')) {
        if (!visible(element) || element.matches('script, style, noscript, svg, svg *')) continue;
        const text = [...element.childNodes].filter(node => node.nodeType === Node.TEXT_NODE).map(node => node.textContent).join('').trim();
        const code = element.matches('code, [data-code], [data-id]') || /^#[0-9]+$/.test(text);
        const isolated = element.closest('bdi, [dir]:not(html):not(body)');
        if (code && !isolated) findings.push({ rule: 'bidiIdentity', element: label(element), detail: 'ID or code has no isolated direction' });
        if (code && !/mono/i.test(getComputedStyle(element).fontFamily)) findings.push({ rule: 'bidiMono', element: label(element), detail: 'ID or code does not use a mono font' });
        if (!text || !/[A-Za-z]/.test(text) || /[\u0600-\u06ff]/.test(text)) continue;
        // CIT-103 uses lang=en + dir=ltr. Explicit auto direction and CSS/BDI
        // isolation are equally valid. Do not count the document's RTL root.
        let direction: string | null = null;
        let language: string | null = null;
        let englishIsolated = false;
        for (let ancestor: HTMLElement | null = element; ancestor && ancestor !== document.documentElement; ancestor = ancestor.parentElement) {
          direction ??= ancestor.getAttribute('dir')?.toLowerCase() ?? null;
          language ??= ancestor.getAttribute('lang')?.toLowerCase() ?? null;
          const bidi = getComputedStyle(ancestor).unicodeBidi;
          if (ancestor.tagName === 'BDI' || ancestor.getAttribute('dir') === 'auto' || ['isolate', 'plaintext'].includes(bidi)) englishIsolated = true;
        }
        englishIsolated ||= direction === 'ltr' && /^en(?:-|$)/.test(language ?? '');
        const fallback = /^en(?:-|$)/.test(language ?? '');
        if (fallback && !englishIsolated) findings.push({ rule: 'bidiFallback', element: label(element), detail: 'English fallback has no isolated direction' });
        if (!englishIsolated) findings.push({ rule: 'bidiIsolation', element: label(element), detail: 'English fallback or game identity has no isolated direction' });
      }
    }
    return findings;
}
