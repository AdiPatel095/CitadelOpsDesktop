import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const root = fileURLToPath(new URL('..', import.meta.url));
const source = existsSync(`${root}/src/commandCenter`) ? '/src/commandCenter' : '/src';
const vite = await createServer({ root, configFile: false, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true, hmr: false } });
const { formatDurationEnd } = await vite.ssrLoadModule(`${source}/i18n/automationDuration.ts`);
after(() => vite.close());

const timestamp = Date.UTC(2026, 8, 29, 13, 0);
for (const locale of ['en', 'de', 'ar']) {
  test(`end time uses ${locale} conventions without seconds`, () => {
    const formatted = formatDurationEnd(timestamp, locale, 'UTC');
    assert.equal(formatDurationEnd(new Date(timestamp), locale, 'UTC'), formatted);
    if (locale === 'en') {
      assert.match(formatted, /1:00\s?PM/);
      assert.ok(formatted.includes('2026'));
    } else if (locale === 'de') {
      assert.ok(formatted.includes('13:00'));
      assert.ok(formatted.includes('2026'));
      assert.doesNotMatch(formatted, /AM|PM/);
    } else {
      assert.match(formatted, /[\u0600-\u06FF\u0660-\u0669]/);
    }
    assert.doesNotMatch(formatted, /\d:\d{2}:\d{2}/);
  });
}

test('both dialog end times use the app locale through the shared formatter', () => {
  const dialog = readFileSync(`${root}${source}/settings/components/AutomationDurationModal.tsx`, 'utf8');
  assert.doesNotMatch(dialog, /toLocaleString\s*\(/);
  assert.equal((dialog.match(/formatDurationEnd\s*\(/g) ?? []).length, 2);
  assert.ok(dialog.includes('formatDurationEnd(turnsOffAt, locale)'));
  assert.ok(dialog.includes('formatDurationEnd(currentUntil, locale)'));
});
