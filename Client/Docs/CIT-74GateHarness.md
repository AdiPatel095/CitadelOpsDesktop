# CIT-74 harness and report inventory

PR-1 is report-only. Product findings are collected; fixture failures still fail the run.
Run only through `/Users/nebulabot/Desktop/CIT/tools/visual-lock.sh <portal|desktop> npm run test:gate`.
The sealed harness uses synthetic/local recordings and intercepts external requests. No login fields are filled.

## Coverage and enforcement

`tests/gate/views.ts` joins existing snapshot cases and all 15 command-center views, all 19 current automation settings panels (the plan says 18), success/failure toasts and, on portal, authentication and four Account Center dialogs.
Layout runs at 390/768/1024/1440/1920 in dark. Axe runs at 390/1440 in both themes; keyboard runs at 390/1440 in dark. All gate suites run in English and Arabic. Touch widths use touch/mobile contexts.
Compact-hidden views are rendered via the existing `citadelops:open-view` event; navigation visibility is still tested by the existing view suite.
Axes are included in report filenames. `test:gate` deletes only its prior generated findings before each run. Browser/fixture errors fail even in report mode.
Set each completed area's letter in `tests/gate/enforcement.json` to enforce its cases; final requires a/b/c/d. This switch is empty in PR-1.

## Existing report-only or ratcheted checks

- `tests/visual/rules.ts::reportAccentUsage` is R2 report-only and swallows reporting errors. The gate calls the shared color scan directly, records findings and propagates scan failures. R3 remains enforced in existing visual snapshots. New PR-1 coverage records R3/R11 findings until its area is enabled.
- `scripts/check-arbitrary-values.mjs` compares against `arbitrary-values.baseline.json`, rather than zero. Fresh develop has 233 desktop values and 254 portal values. Area fixes must remove them before the final PR deletes the baseline.
- `scripts/check-retired-classes.mjs` defaults to reporting unless `--enforce` is passed. Both package scripts already enforce cit-67,cit-73.
- Current `.stylelintrc.json` rules already have error severity; no warning severity remains. R7 spacing is not currently listed and must be covered by the final source gate.
- Typography has a zero baseline. R4/R5/R13, R11, R12, R15 and reduced-motion R16 also run on every rendered gate case.
- Gate layout, keyboard, axe and bidi findings are report-only until their area is enabled. Existing snapshot suites keep their enforced assertions.

## Snapshot additions

The dedicated `coverage.spec.ts` adds 768 English cases, missing command-center views, portal Auto Bird/Auto Storm settings, and portal Arabic landing/Account Center/Castle/Automation at 390/1440 dark. Existing matrices are retained. Only newly added baselines are generated in PR-1.
All exact paths and reasons belong in the PR's Visual baseline changes section.

## Ownership and next gate

Daniel resolves plan discrepancies and splits any area exceeding one day. Sophie verifies the exact PR heads; Claire merges approved pairs. Product fixes, final enforcement, manual QA and release remain subsequent stages.
