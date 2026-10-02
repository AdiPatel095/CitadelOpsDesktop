import { defineConfig } from '@playwright/test';
import visual from './playwright.config';

// Inherit each repository's sealed visual server, port offset and worker limit.
export default defineConfig({
  ...visual,
  testDir: './tests/gate',
  testMatch: ['layout.spec.ts', 'keyboard.spec.ts', 'a11y.spec.ts'],
  outputDir: 'test-results/gate-artifacts',
  updateSnapshots: 'none',
  use: { ...visual.use, screenshot: 'off' },
  reporter: [['list']],
  projects: [390, 768, 1024, 1440, 1920].map(width => ({
    name: String(width),
    use: {
      viewport: { width, height: width === 390 ? 844 : width === 768 ? 1024 : width === 1024 ? 768 : 900 },
      hasTouch: width <= 768,
      isMobile: width <= 768,
    },
  })),
});
