import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { test } from 'node:test';
import ts from 'typescript';

const source = await readFile(new URL('../src/settings/AutoBuyerClientState.ts', import.meta.url), 'utf8');
const compiled = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2022 },
});
const { autoBuyerOtherGoalsValid, parseAutoBuyerClientState } = await import(
  `data:text/javascript;base64,${Buffer.from(compiled.outputText).toString('base64')}`
);
const catalog = {
  packages: [{ shopId: 'shop', packageId: 1, stock: 10, price: { premium: true, amount: 100 } }],
  specialists: [{ id: 0, baseRubyCost: 100, validatedMaximumRubyCost: 100 }],
  specialistUpkeep: { supported: true },
};
const saved = () => parseAutoBuyerClientState({
  sourceCastleId: 10,
  packages: [{ enabled: true, shopId: 'removed', packageId: 999, targetPurchasesPerReset: 1 }],
  specialists: [{ enabled: true, id: 999, minimumDays: 14, maximumRubyCostPerPurchase: 100 }],
});

test('unchanged obsolete goals do not block a feast-only edit or mutate saved goals', () => {
  const original = saved();
  const before = JSON.stringify(original);
  const draft = structuredClone(original);
  draft.feast = { ...draft.feast, enabled: true, sourceCastleId: 10 };
  assert.equal(autoBuyerOtherGoalsValid(draft, original, catalog), true);
  assert.equal(JSON.stringify(original), before);
});
test('new, changed or newly enabled invalid goals still fail validation', () => {
  const original = saved();
  for (const change of [
    (draft) => { draft.packages[0].targetPurchasesPerReset = 2; },
    (draft) => { draft.specialists[0].maximumRubyCostPerPurchase = 200; },
    (draft) => { draft.sourceCastleId = 20; },
    (draft) => { draft.minimumRubyReserve = 1; },
    (draft) => { draft.allowRubyPackages = true; },
  ]) {
    const draft = structuredClone(original);
    change(draft);
    assert.equal(autoBuyerOtherGoalsValid(draft, original, catalog), false);
  }
  const disabled = structuredClone(original);
  disabled.packages[0].enabled = false;
  assert.equal(autoBuyerOtherGoalsValid(original, disabled, catalog), false);
  assert.equal(autoBuyerOtherGoalsValid(original, parseAutoBuyerClientState({}), catalog), false);
});
test('changed valid goals enforce ruby opt-in, stock, cost and specialist duration floors', () => {
  const original = parseAutoBuyerClientState({});
  const draft = parseAutoBuyerClientState({
    sourceCastleId: 10, allowRubyPackages: true,
    packages: [{ enabled: true, shopId: 'shop', packageId: 1, targetPurchasesPerReset: 1, maximumRubySpendPerReset: 100 }],
    specialists: [{ enabled: true, id: 0, minimumDays: 14, maximumRubyCostPerPurchase: 100 }],
  });
  assert.equal(autoBuyerOtherGoalsValid(draft, original, catalog), true);
  for (const change of [
    (value) => { value.allowRubyPackages = false; },
    (value) => { value.packages[0].maximumRubySpendPerReset = 99; },
    (value) => { value.packages[0].targetPurchasesPerReset = 11; },
    (value) => { value.specialists[0].maximumRubyCostPerPurchase = 99; },
    (value) => { value.specialists[0].minimumDays = 13; },
  ]) {
    const invalid = structuredClone(draft);
    change(invalid);
    assert.equal(autoBuyerOtherGoalsValid(invalid, original, catalog), false);
  }
});

test('old runtimes preserve unchanged specialist goals and allow disablement, but block new enablement', () => {
  const unsupportedCatalog = { ...catalog, specialistUpkeep: { supported: false } };
  const original = parseAutoBuyerClientState({
    specialists: [{ enabled: true, id: 0, minimumDays: 21, maximumRubyCostPerPurchase: 100 }],
  });
  const unchanged = structuredClone(original);
  unchanged.feast.minimumFoodReserve = 1;
  assert.equal(autoBuyerOtherGoalsValid(unchanged, original, unsupportedCatalog), true);

  const disabled = structuredClone(original);
  disabled.specialists[0].enabled = false;
  assert.equal(autoBuyerOtherGoalsValid(disabled, original, unsupportedCatalog), true);

  const newlyEnabled = parseAutoBuyerClientState({
    specialists: [{ enabled: true, id: 0, minimumDays: 21, maximumRubyCostPerPurchase: 100 }],
  });
  assert.equal(autoBuyerOtherGoalsValid(newlyEnabled, parseAutoBuyerClientState({}), unsupportedCatalog), false);
});
