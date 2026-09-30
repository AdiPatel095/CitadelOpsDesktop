import { defineConfig } from '@playwright/test';

const host = process.env.VISUAL_HOST === '1';
if (host && process.env.CI) throw new Error('Host snapshots are local comparisons only.');
if (!host && (!process.env.CI && process.env.PLAYWRIGHT_CONTAINER !== '1')) {
  throw new Error('Use test:visual:host locally; canonical snapshots require CI or PLAYWRIGHT_CONTAINER=1.');
}
if (!host && (process.platform !== 'linux' || process.arch !== 'x64')) {
  throw new Error('Canonical snapshots require the pinned linux/amd64 Playwright container.');
}

export default defineConfig({
  testDir: './tests/visual',
  testMatch: 'snapshots.spec.ts',
  timeout: 60_000,
  retries: 0,
  workers: 1,
  fullyParallel: false,
  updateSnapshots: 'missing',
  snapshotPathTemplate: host
    ? '{testDir}/.host/__screenshots__/{arg}{ext}'
    : '{testDir}/__screenshots__/{arg}{ext}',
  outputDir: process.env.VISUAL_OUTPUT_DIR ?? (host ? 'tests/visual/.host/test-results' : 'test-results'),
  reporter: [['list'], ['html', {
    outputFolder: process.env.VISUAL_REPORT_DIR ?? (host ? 'tests/visual/.host/report' : 'playwright-report'),
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
