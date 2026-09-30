import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests/visual',
  testMatch: 'snapshots.spec.ts',
  timeout: 60_000,
  retries: 0,
  workers: 1,
  fullyParallel: false,
  updateSnapshots: 'missing',
  snapshotPathTemplate: '{testDir}/__screenshots__/{arg}{ext}',
  outputDir: 'test-results',
  reporter: [['list'], ['html', {
    outputFolder: 'playwright-report',
    open: 'never',
  }]],
  expect: { toHaveScreenshot: { threshold: 0.2, maxDiffPixels: 0, animations: 'disabled', caret: 'hide' } },
  use: {
    baseURL: 'http://127.0.0.1:41736',
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
  ],
  webServer: [{
    command: 'node scripts/visual/serve.mjs',
    url: 'http://127.0.0.1:41736',
    reuseExistingServer: false,
    timeout: 180_000,
  }],
});
