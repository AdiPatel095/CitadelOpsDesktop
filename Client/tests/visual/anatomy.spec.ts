import { test } from '@playwright/test';
import { openView, prepare, settle } from './harness';
import { verifyCardAnatomy } from './cardAnatomy';

// The desktop fixture has no World Intel card data; the portal covers that view.
const views = [
  { name: 'castle', label: 'Castle', view: 'castle' },
  { name: 'automation', label: 'Automation', view: 'automation' },
  { name: 'feature-stats', label: 'Feature Stats', view: 'events' },
];
for (const view of views) {
  for (const theme of ['dark', 'light'] as const) {
    test(`CIT-67 ${view.name} anatomy ${theme}`, async ({ page }) => {
      const verifyNetwork = await prepare(page, theme);
      if (view.name !== 'castle') await openView(page, view.label, view.view);
      await settle(page);
      await verifyCardAnatomy(page, view.name === 'automation');
      verifyNetwork();
    });
  }
}
