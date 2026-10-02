import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests/visual-rules',
  testMatch: 'rules.fixture.spec.ts',
  workers: 1,
  retries: 0,
  reporter: 'list',
  outputDir: 'test-results/rules-fixture',
  use: { browserName: 'chromium', headless: true },
  projects: [
    { name: 'light', use: { viewport: { width: 1440, height: 900 }, colorScheme: 'light' } },
    { name: 'dark', use: { viewport: { width: 1024, height: 768 }, colorScheme: 'dark' } },
  ],
});
