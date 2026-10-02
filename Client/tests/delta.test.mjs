import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const root = fileURLToPath(new URL('..', import.meta.url));
const source = existsSync(`${root}/src/commandCenter`) ? '/src/commandCenter' : '/src';
const vite = await createServer({ root, configFile: false, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true, hmr: false } });
const { deltaTone, withDeltaSign } = await vite.ssrLoadModule(`${source}/components/ui/delta.ts`);
after(() => vite.close());

test('positive deltas have gain tone and a plus sign', () => {
  assert.equal(deltaTone(1620), 'gain');
  assert.equal(withDeltaSign('1,620%', 1620), '+1,620%');
});

test('negative deltas have loss tone and a U+2212 minus sign', () => {
  assert.equal(deltaTone(-2), 'loss');
  const formatted = withDeltaSign('2%', -2);
  assert.equal(formatted, '−2%');
  assert.equal(formatted.codePointAt(0), 0x2212);
});

test('zero, negative zero and NaN have zero tone and no sign', () => {
  for (const value of [0, -0, NaN]) {
    assert.equal(deltaTone(value), 'zero');
    assert.equal(withDeltaSign('0', value), '0');
  }
});

test('Arabic formatting is preserved with the sign prepended', () => {
  const formatted = new Intl.NumberFormat('ar-EG').format(1620);
  assert.equal(formatted, '١٬٦٢٠');
  assert.equal(withDeltaSign(formatted, 1620), `+${formatted}`);
  assert.equal(withDeltaSign(formatted, -1620), `−${formatted}`);
  assert.equal(withDeltaSign(formatted, 0), formatted);
});
