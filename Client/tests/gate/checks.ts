import type { Page } from '@playwright/test';

export type Violation = { rule: string; element: string; detail: string };

// Missing browser APIs and detached pages are report findings, never thrown checks.
async function scan(page: Page, rule: string, kind: 'overflow' | 'targets' | 'clipping'): Promise<Violation[]> {
  try {
    return await page.evaluate(({ rule, kind }) => {
      const violations: { rule: string; element: string; detail: string }[] = [];
      if (kind === 'overflow') {
        const width = Math.max(document.documentElement.scrollWidth, document.body.scrollWidth);
        if (width > innerWidth) violations.push({ rule, element: 'html', detail: `scrollWidth ${width} > innerWidth ${innerWidth}` });
        return violations;
      }
      const selector = kind === 'targets'
        ? 'md-filled-button, md-outlined-button, md-text-button, md-icon-button, md-checkbox, md-switch, md-outlined-text-field, md-filled-text-field, button, a[href], input:not([type="hidden"]), select, textarea, summary, [role="button"], [role="tab"], [role="switch"], [role="checkbox"], [role="combobox"]'
        : 'md-filled-button, md-outlined-button, md-text-button, md-icon-button, button, [role="button"], [role="tab"], nav a, nav button, #workspace-navigation button, .player-status-word, [data-player-status]';
      const visible = (element: HTMLElement) => {
        const rect = element.getBoundingClientRect();
        const style = getComputedStyle(element);
        return rect.width > 0 && rect.height > 0 && style.visibility !== 'hidden' && style.display !== 'none' && !element.closest('[inert]');
      };
      const elements = [...document.querySelectorAll<HTMLElement>(selector)].filter(visible);
      for (const element of elements) {
        const rect = element.getBoundingClientRect();
        const label = `${element.tagName.toLowerCase()}${element.id ? `#${element.id}` : ''} ${element.getAttribute('aria-label') ?? element.textContent?.trim().replace(/\s+/g, ' ').slice(0, 100) ?? ''}`;
        if (kind === 'targets') {
          const dense = element.getAttribute('data-target-exempt') === 'dense-table';
          const minimum = dense ? 24 : 44;
          if (rect.width < minimum || rect.height < minimum) {
            violations.push({ rule, element: label, detail: `${rect.width.toFixed(1)} × ${rect.height.toFixed(1)}; minimum ${minimum} × ${minimum}` });
          }
          if (dense) {
            const tooClose = elements.some(other => {
              if (other === element || other.contains(element) || element.contains(other)) return false;
              const r = other.getBoundingClientRect();
              const horizontal = Math.max(0, r.left - rect.right, rect.left - r.right);
              const vertical = Math.max(0, r.top - rect.bottom, rect.top - r.bottom);
              return Math.hypot(horizontal, vertical) < 8;
            });
            if (tooClose) violations.push({ rule, element: label, detail: 'Dense-table action has less than 8 px spacing from another target' });
          }
        } else if (element.scrollWidth > element.clientWidth + 1) {
          violations.push({ rule, element: label, detail: `scrollWidth ${element.scrollWidth} > clientWidth ${element.clientWidth} + 1` });
        }
      }
      return violations;
    }, { rule, kind });
  } catch (error) {
    return [{ rule, element: 'page', detail: `Check unavailable: ${String(error)}` }];
  }
}

export const pageOverflow = (page: Page) => scan(page, 'pageOverflow', 'overflow');
export const smallTargets = (page: Page) => scan(page, 'smallTargets', 'targets');
export const clippedControls = (page: Page) => scan(page, 'clippedControls', 'clipping');

export async function layoutShiftAfterReady(page: Page): Promise<Violation[]> {
  try {
    return await page.evaluate(async () => {
      const rule = 'layoutShiftAfterReady';
      if (!PerformanceObserver.supportedEntryTypes.includes('layout-shift')) {
        return [{ rule, element: 'page', detail: 'layout-shift API unavailable' }];
      }
      let score = 0;
      const collect = (entries: PerformanceEntry[]) => {
        for (const entry of entries) {
          const shift = entry as PerformanceEntry & { value: number; hadRecentInput: boolean };
          if (!shift.hadRecentInput) score += shift.value;
        }
      };
      const observer = new PerformanceObserver(list => collect(list.getEntries()));
      observer.observe({ type: 'layout-shift', buffered: false });
      await document.fonts.ready;
      await new Promise(resolve => setTimeout(resolve, 2000));
      collect(observer.takeRecords());
      observer.disconnect();
      return score > 0.01 ? [{ rule, element: 'page', detail: `Post-ready layout shift ${score.toFixed(5)} > 0.01` }] : [];
    });
  } catch (error) {
    return [{ rule: 'layoutShiftAfterReady', element: 'page', detail: `Check unavailable: ${String(error)}` }];
  }
}
