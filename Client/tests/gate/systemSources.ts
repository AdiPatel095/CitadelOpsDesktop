import { createHash } from 'node:crypto';
import type { Page } from '@playwright/test';
import type { Violation } from './checks';

// Attribution is evidence only. Rules and raw finding selectors stay unchanged;
// the ratchet must explicitly record each case, property and source fingerprint.
export async function annotateSystemSources(page: Page, findings: Violation[]): Promise<Violation[]> {
  const records = await page.evaluate(findings => {
    const elements = [...document.querySelectorAll<HTMLElement>('*')];
    const generated = (id: string) => /^[:_]r/i.test(id);
    function path(element: Element, stable = false): string {
      if (element.id && (!stable || !generated(element.id))) return `${element.localName}#${element.id}`;
      const parent = element.parentElement;
      const index = parent ? [...parent.children].indexOf(element) + 1 : 1;
      return `${parent ? `${path(parent, stable)} > ` : ''}${element.localName}:nth-child(${index})`;
    }
    const byPath = new Map(elements.map(element => [path(element), element]));
    const label = (element: HTMLElement) => `${element.tagName.toLowerCase()}${element.id ? `#${element.id}` : ''} ${element.getAttribute('aria-label') ?? element.textContent?.trim().replace(/\s+/g, ' ').slice(0, 100) ?? ''}`;
    const visible = (element: HTMLElement) => {
      const rect = element.getBoundingClientRect(), style = getComputedStyle(element);
      return rect.width > 0 && rect.height > 0 && style.visibility !== 'hidden' && style.display !== 'none' && !element.closest('[inert]');
    };
    return findings.map(finding => {
      if (!['R2', 'smallTargets', 'clippedControls', 'axe:color-contrast'].includes(finding.rule)) return {};
      const pseudo = finding.element.endsWith('::before') ? '::before' : finding.element.endsWith('::after') ? '::after' : '';
      let element = byPath.get(pseudo ? finding.element.slice(0, -pseudo.length) : finding.element);
      if (!element && ['smallTargets', 'clippedControls'].includes(finding.rule)) {
        element = elements.find(node => {
          if (!visible(node) || label(node) !== finding.element) return false;
          if (finding.rule === 'clippedControls') return node.scrollWidth > node.clientWidth + 1;
          const rect = node.getBoundingClientRect();
          return finding.detail.startsWith(`${rect.width.toFixed(1)} × ${rect.height.toFixed(1)};`);
        });
      }
      if (!element && finding.rule === 'axe:color-contrast') {
        try { element = document.querySelector<HTMLElement>(finding.element) ?? undefined; } catch { /* retain an unresolved finding */ }
      }
      if (!element) return {};
      const classes = (node: Element) => (node.getAttribute('class') ?? '').split(/\s+/).filter(Boolean).sort().join(' ');
      let sharedOwner: string | undefined;
      if (finding.rule === 'axe:color-contrast') {
        const chip = element.closest('.m3-chip-success, .m3-chip-warning');
        if (chip && /foreground color: #(47752f|8a5a0c)\b/i.test(finding.detail)) sharedOwner = 'CIT-74 chip contrast PRs D#197/F#152';
      } else if (element.closest('#workspace-navigation, .liquid-header, [data-settings-run-strip], [data-stop-control], section[aria-labelledby^="readiness-"], [data-setup-checklist]') || (!element.closest('[role="dialog"]') && element.closest('[data-view="automation"]'))) {
        sharedOwner = 'CIT-74 area (b): shared navigation/automation';
      } else if (element.closest('[role="dialog"]')?.querySelector('[data-start-confirm]')) {
        sharedOwner = 'CIT-74 area (b): shared start confirmation';
      } else {
        const frame = element.closest('.m3-page-header, .ui-card__header, .scheduler-modal-title, .liquid-modal-title, .ui-segments');
        if (frame && (finding.rule === 'R2' || frame.matches('.ui-segments'))) {
          let explicitAccent = false;
          for (let node: Element | null = element; node; node = node.parentElement) {
            if (classes(node).split(' ').some(name => /^(text|bg|border|ring)-primary(?:\/|$)/.test(name) || name === 'm3-chip-primary')) explicitAccent = true;
            if (node === frame) break;
          }
          if (!explicitAccent) sharedOwner = 'CIT-74 shared UI follow-up (coordinator): card/dialog/segmented control';
        }
      }
      const chain = [];
      for (let node: Element | null = element; node; node = node.parentElement) {
        chain.push([node.localName, generated(node.id) ? '' : node.id, classes(node), node.getAttribute('role'), node.getAttribute('data-variant'), node.getAttribute('data-size')]);
      }
      return { baselineElement: `${path(element, true)}${pseudo}`, sharedOwner, sourceClasses: chain.slice(0, 4).map(node => String(node[2])), material: JSON.stringify(chain) };
    });
  }, findings);
  return findings.map((finding, index) => {
    const { material, ...record } = records[index];
    return material ? { ...finding, ...record, sourceFingerprint: createHash('sha256').update(material).digest('hex') } : finding;
  });
}
