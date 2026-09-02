import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({
  root: clientRoot,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
});
const wallet = await vite.ssrLoadModule('/src/playerTracker/components/PlayerTrackerWallet.ts');

after(async () => {
  await vite.close();
});

test('My Stats keeps historically nonzero resources and currencies selectable after live balances reach zero', () => {
  const ids = wallet.collectWalletMetricIDs(
    { 1: 5_000, 12: 0 },
    { 9: 0, 22: 4 },
    [
      { currencies: { 'resource:12': 800, 'currency:9': 7, 'resource:1': 4_000 } },
      { currencies: { 'resource:37': 0, 'currency:30': 0 } },
    ],
  );

  assert.deepEqual(ids.resourceIDs, ['1', '12']);
  assert.deepEqual(ids.currencyIDs, ['9', '22']);
});

test('My Stats wallet inventory ignores aliases and malformed retained keys', () => {
  const ids = wallet.collectWalletMetricIDs({}, {}, [{
    currencies: {
      coins: 10,
      rubies: 2,
      'resource:0': 3,
      'resource:not-an-id': 4,
      'currency:17': Number.NaN,
      'currency:18': 5,
    },
  }]);

  assert.deepEqual(ids.resourceIDs, []);
  assert.deepEqual(ids.currencyIDs, ['18']);
});

test('My Stats restores coin and ruby balances from canonical retained resource entries', () => {
  const oldSample = {
    coins: 0,
    rubies: 0,
    currencies: { 'resource:1': 5_000, 'resource:2': 44 },
  };
  assert.equal(wallet.retainedWalletBalance(oldSample, 'coins'), 5_000);
  assert.equal(wallet.retainedWalletBalance(oldSample, 'rubies'), 44);
});
