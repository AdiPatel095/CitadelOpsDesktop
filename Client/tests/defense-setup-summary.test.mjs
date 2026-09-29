import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const { describeMessage, messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');
const { formatMessage } = await vite.ssrLoadModule('/src/i18n/formatMessage.ts');

after(async () => {
  await vite.close();
});

const summary = (tools, types) => formatMessage(describeMessage('defenseSetup.summary', { tools, types }), 'en', {}).text;

test('the defense summary pluralizes tools and tool types (Maya, CIT-16 nit)', () => {
  assert.equal(summary(1, 1), '1 tool · 1 tool type');
  assert.equal(summary(2, 1), '2 tools · 1 tool type');
  assert.equal(summary(0, 0), '0 tools · 0 tool types');
  assert.equal(summary(1500, 3), '1,500 tools · 3 tool types');
});

test('tool, troop and tool-type counts in the defense field and Khan/Storm plan copy use ICU plurals', () => {
  const format = (key, params) => formatMessage(describeMessage(key, params), 'en', {}).text;
  for (const key of ['defenseSetup.summary', 'eventAttackSetup.summary', 'eventAttackReadiness.composition']) {
    for (const count of ['tools', 'troops', 'types', 'waves']) {
      if (messages[key].includes(`{${count},`)) assert.match(messages[key], new RegExp(`\\{${count}, plural,`), `${key}: ${count}`);
    }
    assert.doesNotMatch(messages[key], /runtime/i, key);
  }
  assert.equal(format('eventAttackSetup.summary', { waves: 1, troops: 1, tools: 1 }), '1 wave · 1 troop · 1 tool');
  assert.equal(format('eventAttackReadiness.composition', { waves: 2, troops: 4000, tools: 0 }), '2 waves, 4,000 troops and 0 tools.');
});
