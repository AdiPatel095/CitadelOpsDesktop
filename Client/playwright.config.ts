import { defineConfig } from '@playwright/test';

const port = 41736 + Number(process.env.CIT_VISUAL_PORT_OFFSET ?? 0);

export default defineConfig({
  testDir: './tests/visual',
  testMatch: ['automation-cards.spec.ts', 'snapshots.spec.ts', 'copy.spec.ts', 'views.spec.ts', 'fonts.spec.ts', 'anatomy.spec.ts', 'tabs.spec.ts', 'states.spec.ts', 'castle-order.spec.ts', 'typography.spec.ts'],
  timeout: 60_000,
  retries: 0,
  workers: 1,
  fullyParallel: false,
  updateSnapshots: 'none',
  snapshotPathTemplate: '{testDir}/__screenshots__/{arg}{ext}',
  outputDir: 'test-results',
  reporter: [['list'], ['html', {
    outputFolder: 'playwright-report',
    open: 'never',
  }]],
  expect: { timeout: 10_000, toHaveScreenshot: { threshold: 0.2, maxDiffPixels: 0, animations: 'disabled', caret: 'hide' } },
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    browserName: 'chromium',
    deviceScaleFactor: 1,
    reducedMotion: 'reduce',
    locale: 'en-US',
    timezoneId: 'UTC',
    serviceWorkers: 'block',
    trace: 'retain-on-failure',
  },
  projects: [
    { name: '1440', use: { viewport: { width: 1440, height: 900 } } },
    { name: '1024', use: { viewport: { width: 1024, height: 768 } } },
    { name: '390', testMatch: ['copy.spec.ts', 'views.spec.ts', 'anatomy.spec.ts', 'tabs.spec.ts', 'states.spec.ts', 'castle-order.spec.ts', 'typography.spec.ts'], use: { viewport: { width: 390, height: 844 } } },
  ],
  webServer: [{
    command: `node scripts/visual/serve.mjs ${port}`,
    url: `http://127.0.0.1:${port}`,
    reuseExistingServer: false,
    timeout: 600_000,
  }],
});
