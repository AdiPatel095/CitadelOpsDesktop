import { expect, type Page } from '@playwright/test';

export async function verifyCardAnatomy(page: Page, automation: boolean) {
  const anatomy = await page.evaluate(() => {
    const tokenColor = (name: string) => {
      const probe = document.createElement('span');
      probe.style.color = `var(${name})`;
      document.body.append(probe);
      const color = getComputedStyle(probe).color;
      probe.remove();
      return color;
    };
    const cards = [...document.querySelectorAll<HTMLElement>('.ui-card')].map(element => {
      const style = getComputedStyle(element);
      return {
        background: style.backgroundColor, border: style.borderTopColor, width: style.borderTopWidth,
        current: element.dataset.currentSelection === 'true',
        padding: style.paddingTop, startEnd: style.borderStartEndRadius,
      };
    });
    return {
      canvas: tokenColor('--surface-canvas'), card: tokenColor('--surface-card'), inset: tokenColor('--surface-inset'),
      border: tokenColor('--border-default'), accent: tokenColor('--accent'),
      page: getComputedStyle(document.body).backgroundColor, cards,
      headers: [...document.querySelectorAll('.ui-card__header')].map(element => {
        const style = getComputedStyle(element);
        return { background: style.backgroundColor, shadow: style.boxShadow };
      }),
      panels: [...document.querySelectorAll('.ui-panel, .ui-metric-tile')].map(element => getComputedStyle(element).backgroundColor),
      sectionTitles: [...document.querySelectorAll('.ui-section-header__title')].map(element => {
        const style = getComputedStyle(element);
        return { font: style.fontFamily, size: style.fontSize, weight: style.fontWeight, color: style.color };
      }),
      nestedCards: document.querySelectorAll('.ui-card .ui-card').length,
      overflow: document.documentElement.scrollWidth - window.innerWidth,
    };
  });
  expect(anatomy.page, 'page uses the canvas surface').toBe(anatomy.canvas);
  expect(anatomy.cards.length, 'view contains shared cards').toBeGreaterThan(0);
  for (const card of anatomy.cards) {
    expect(card.background).toBe(anatomy.card);
    expect(card.width).toBe(card.current ? '2px' : '1px');
    expect(card.border).toBe(card.current ? anatomy.accent : anatomy.border);
    expect(card.startEnd).toBe('12px');
    const width = page.viewportSize()!.width;
    expect(card.padding).toBe(`${(width < 768 ? 16 : width < 1280 ? 20 : 24) - (card.current ? 1 : 0)}px`);
  }
  for (const header of anatomy.headers) {
    expect(header.background).toBe('rgba(0, 0, 0, 0)');
    expect(header.shadow).toBe('none');
  }
  for (const panel of anatomy.panels) expect(panel).toBe(anatomy.inset);
  expect(anatomy.overflow, 'no horizontal page scroll').toBeLessThanOrEqual(1);
  if (automation) {
    expect(anatomy.sectionTitles.length).toBeGreaterThan(1);
    for (const title of anatomy.sectionTitles) {
      expect(title).toEqual(anatomy.sectionTitles[0]);
      expect(title.size).toBe('20px');
      expect(title.weight).toBe('600');
    }
  }
}
