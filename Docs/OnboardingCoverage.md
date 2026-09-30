# Onboarding coverage matrix (CIT-22)

Traceability from every automation module and shared surface to the scenario that exercises it. The matrix is derived from the scenario files in `Client/tests/onboarding-browser/scenarios/`; `Client/tests/onboarding-coverage.test.mjs` fails when this document, the scenarios or the module list disagree, and when a module has no scenario in a column it supports. A "not applicable" cell says why. Nothing here claims live success: every scenario is simulated (see `OnboardingPreview.md`).

Evidence screenshots and recordings are stored by Sophie in the vault under `QA/Evidence/CIT-22/` and indexed there; the Evidence column stays `pending` until that index exists.

## Modules by column

<!-- coverage-table:start -->
| Module | Inline setup (CIT-15/16) | Readiness and repair (CIT-18) | Disclosure (CIT-17) | Before you start, phases, first result, Stop (CIT-20) | Copy (CIT-21) | Goal, checklist, recovery, repair (CIT-19) | Disclosure rows | Platform notes | Evidence |
|---|---|---|---|---|---|---|---|---|---|
| `autoNomad` Auto Nomad / Samurai | `custom-presets`, `new-user`, `presets-edit-promotion` | `disclosure-matrix`, `onboarding-disconnected-first-use`, `onboarding-wrong-world` | `disclosure-matrix`, `new-user` | `rich-account` | n/a: no per-castle setup to copy | `new-user`, `onboarding-disconnected-first-use`, `onboarding-hosted-runtime-no-feature`, `onboarding-wrong-world` | 6 | both | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `autoInvasion` Auto Invasion | `custom-presets`, `new-user` | `disclosure-matrix` | `disclosure-matrix`, `new-user` | `rich-account` | n/a: no per-castle setup to copy | `new-user` | 5 | both | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `autoBeriWorld` Auto Beri World | `custom-presets`, `new-user` | `disclosure-matrix` | `disclosure-matrix`, `locale-ar`, `locale-de`, `new-user` | `rich-account` | n/a: no per-castle setup to copy | `new-user` | 6 | both | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `autoKhan` Auto Khan | `custom-presets`, `khan-defense-fresh`, `khan-defense-stale`, `khan-missing-defense`, `khan-missing-main`, `khan-skip-dependency`, `new-user` | `disclosure-matrix`, `khan-missing-defense`, `khan-missing-main`, `khan-skip-dependency`, `shortages-conflicts` | `disclosure-matrix`, `locale-ar`, `locale-de`, `new-user` | `rich-account` | n/a: no per-castle setup to copy | `new-user` | 6 | both | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `autoStorm` Auto Storm | `new-user`, `storm-donor-unavailable`, `storm-no-offer`, `storm-offer` | `disclosure-matrix`, `storm-donor-unavailable`, `storm-no-offer` | `disclosure-matrix`, `locale-ar`, `locale-de`, `new-user` | `rich-account` | n/a: no per-castle setup to copy | `new-user` | 9 | both | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `autoTowers` Auto Towers | n/a: not preset-dependent: settings are edited directly | `disclosure-matrix`, `shortages-conflicts`, `stale-data-awaiting-baseline`, `stale-data-checkpoint`, `stale-data-disconnected`, `towers-food-contrast`, `zero-castles` | `disclosure-matrix`, `locale-ar`, `locale-de`, `new-user`, `towers-food-contrast` | `first-failed`, `first-result`, `phases-blocked`, `phases-completed`, `phases-enabled-waiting`, `phases-locked`, `phases-running`, `phases-stopped`, `rich-account`, `start-rejected`, `stop-failed` | `copy-differences` | `account-switch-editor`, `new-user`, `onboarding-account-switch`, `onboarding-existing-custom-setup`, `onboarding-interrupted-return` | 5 | both; hosted gates the Baron Advisor description behind `towerAdvisor` | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `autoFortress` Auto Fortress | n/a: not preset-dependent: settings are edited directly | `disclosure-matrix`, `shortages-conflicts`, `stale-data-awaiting-baseline`, `stale-data-checkpoint`, `stale-data-disconnected`, `support-production`, `zero-castles` | `disclosure-matrix`, `new-user`, `support-production` | `rich-account` | n/a: no per-castle setup to copy | `new-user` | 5 | both | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `autoFoodBalance` Auto Food Balance | n/a: not preset-dependent: settings are edited directly | `disclosure-matrix`, `stale-data-awaiting-baseline`, `stale-data-checkpoint`, `stale-data-disconnected`, `towers-food-contrast`, `zero-castles` | `disclosure-matrix`, `new-user`, `towers-food-contrast` | `rich-account` | n/a: account-wide settings only; no per-castle copy | `new-user` | 4 | both | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `autoStation` Auto Station | n/a: not preset-dependent: settings are edited directly | `disclosure-matrix`, `stale-data-awaiting-baseline`, `stale-data-checkpoint`, `stale-data-disconnected`, `support-production`, `zero-castles` | `disclosure-matrix`, `locale-ar`, `locale-de`, `new-user`, `support-production` | `rich-account` | `copy-absent-unit`, `copy-compatible`, `copy-conflict`, `copy-conflict-changed`, `copy-differences`, `copy-unobserved` | `account-switch-editor`, `new-user` | 3 | both | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `autoBird` Auto Bird | n/a: not preset-dependent: settings are edited directly | `disclosure-matrix`, `stale-data-awaiting-baseline`, `stale-data-checkpoint`, `stale-data-disconnected`, `support-production` | `disclosure-matrix`, `new-user`, `support-production` | `rich-account` | `copy-mixed` | `new-user` | 3 | both; hosted gates Bird presets and castle controls behind `birdRuntimePresets` / `birdCastleControls` (2.4.0-beta.2) | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `autoRecruit` Recruit Troops | n/a: not preset-dependent: settings are edited directly | `disclosure-matrix`, `support-production` | `disclosure-matrix`, `new-user`, `support-production` | `rich-account` | `copy-queue-production` | `new-user` | 2 | both | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `autoTool` Auto Tool | n/a: not preset-dependent: settings are edited directly | `disclosure-matrix`, `support-production` | `disclosure-matrix`, `new-user`, `support-production` | `rich-account` | `copy-queue-production` | `new-user` | 2 | both | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `autoHospital` Auto Hospital | n/a: not preset-dependent: settings are edited directly | `disclosure-matrix`, `support-production` | `disclosure-matrix`, `new-user`, `support-production` | `rich-account` | n/a: account-wide settings only; no per-castle copy | `new-user` | 2 | both | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `autoTCI` Auto TCI | n/a: not preset-dependent: settings are edited directly | `disclosure-matrix`, `support-production` | `disclosure-matrix`, `new-user`, `support-production` | `rich-account` | n/a: no per-castle setup to copy | `new-user` | 2 | both | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `autoSceatRes` Auto Sceat Resources | n/a: not preset-dependent: settings are edited directly | `disclosure-matrix`, `support-production` | `disclosure-matrix`, `new-user`, `support-production` | `rich-account` | n/a: no per-castle setup to copy | `new-user` | 5 | both | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `autoBooster` Auto Booster | n/a: not preset-dependent: settings are edited directly | `disclosure-matrix`, `support-production` | `disclosure-matrix`, `new-user`, `support-production` | `rich-account` | n/a: no per-castle setup to copy | `new-user` | 2 | both | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `autoBuyer` Auto Buyer | n/a: not preset-dependent: settings are edited directly | `disclosure-matrix`, `support-production` | `disclosure-matrix`, `new-user`, `support-production` | `rich-account` | n/a: no per-castle setup to copy | `new-user` | 3 | both | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `autoAdvisor` Auto Advisor | n/a: not preset-dependent: settings are edited directly | `disclosure-matrix`, `support-production` | `disclosure-matrix`, `locale-ar`, `locale-de`, `new-user`, `support-production` | `rich-account` | n/a: no per-castle setup to copy | `new-user` | 5 | both; hosted gates behind `towerAdvisor` | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `autoEquipmentCleanup` Auto Equipment Cleanup | n/a: not preset-dependent: settings are edited directly | `disclosure-matrix`, `support-production` | `disclosure-matrix`, `new-user`, `support-production` | `rich-account` | n/a: no per-castle setup to copy | `new-user` | 2 | both | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `presets-views` Attack and Defense Presets views | `custom-presets`, `new-user`, `presets-edit-promotion` | n/a: not part of this surface | n/a: not part of this surface | n/a: not part of this surface | n/a: not part of this surface | n/a: not part of this surface | n/a: not a settings editor | both | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `movement-assignments` Movement commander assignments | n/a: not part of this surface | `shortages-conflicts` | n/a: not part of this surface | n/a: not part of this surface | n/a: not part of this surface | n/a: not part of this surface | n/a: not a settings editor | both | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `account-center-hosted` Account Center (hosted) | n/a: not part of this surface | n/a: not part of this surface | n/a: not part of this surface | n/a: not part of this surface | n/a: not part of this surface | `hosted-account-center`, `onboarding-hosted-runtime-no-feature`, `onboarding-wrong-world` | n/a: not a settings editor | hosted only | pending: `QA/Evidence/CIT-22/` (Sophie) |
| `settings-connection-desktop` Settings connection (desktop) | n/a: not part of this surface | `onboarding-disconnected-first-use`, `onboarding-wrong-world`, `stale-data-awaiting-baseline`, `stale-data-checkpoint`, `stale-data-disconnected` | n/a: not part of this surface | n/a: not part of this surface | n/a: not part of this surface | `onboarding-disconnected-first-use`, `onboarding-wrong-world` | n/a: not a settings editor | desktop only (Start Bot, background login); hosted uses the connection repair panel | pending: `QA/Evidence/CIT-22/` (Sophie) |
<!-- coverage-table:end -->

## Scenarios

`Step` is the walkthrough step in `OnboardingPreview.md`. Scenario ids are identical on desktop (JSON files) and hosted (`?mockScenario=` overlays in `scenarios.ts`).

| Scenario | Title | Step | Platforms | Platform differences |
|---|---|---|---|---|
| `new-user` | New user: nothing saved, no presets | 2 | desktop, hosted | - |
| `onboarding-disconnected-first-use` | Goal-led first use: disconnected, nothing saved | 2 | desktop, hosted | hosted: Hosted shows the panel with 'Start the hosted runtime' (Account Center) instead of Start Bot. |
| `custom-presets` | Existing custom and shared presets | 3 | desktop, hosted | - |
| `presets-edit-promotion` | Editing and renaming an app-created preset | 3 | desktop, hosted | - |
| `khan-defense-fresh` | Khan: main-castle defense observed after this connection started | 4 | desktop, hosted | - |
| `khan-defense-stale` | Khan: defense read before this connection started | 4 | desktop, hosted | - |
| `khan-missing-defense` | Khan: saved defense preset is missing | 4 | desktop, hosted | - |
| `khan-missing-main` | Khan: saved main castle is not in this account | 4 | desktop, hosted | - |
| `khan-skip-dependency` | Khan: time-skip dependency | 4 | desktop, hosted | - |
| `storm-donor-unavailable` | Storm: donor castles short or not in this account | 5 | desktop, hosted | - |
| `storm-no-offer` | Storm: no starter castle on offer | 5 | desktop, hosted | - |
| `storm-offer` | Storm: an official starter castle is on offer | 5 | desktop, hosted | - |
| `disclosure-matrix` | Disclosure matrix: a custom Advanced value in every module | 6 | desktop, hosted | - |
| `support-production` | Support and production dependencies | 6 | desktop, hosted | - |
| `towers-food-contrast` | Contrast: Towers and Food Balance (no presets) | 6 | desktop, hosted | - |
| `stale-data-awaiting-baseline` | Stale data: Waiting for the first sync | 7 | desktop, hosted | - |
| `stale-data-checkpoint` | Stale data: Saved checkpoint | 7 | desktop, hosted | hosted: On hosted the checkpoint mode is the account's saved dashboard; desktop shows it as a disconnected session. |
| `stale-data-disconnected` | Stale data: Disconnected | 7 | desktop, hosted | - |
| `zero-castles` | Zero observed castles | 7 | desktop, hosted | - |
| `shortages-conflicts` | Shortages, commander conflict and castles not in this world | 8 | desktop, hosted | - |
| `first-failed` | First attempt failed | 9 | desktop, hosted | - |
| `first-result` | First result confirmed | 9 | desktop, hosted | - |
| `phases-blocked` | Blocked | 9 | desktop, hosted | - |
| `phases-completed` | Completed | 9 | desktop, hosted | - |
| `phases-enabled-waiting` | Start a ready automation and watch it progress | 9 | desktop, hosted | - |
| `phases-locked` | Locked after a rejected action | 9 | desktop, hosted | - |
| `phases-running` | Running | 9 | desktop, hosted | - |
| `phases-stopped` | Stopped with an action still in flight | 9 | desktop, hosted | - |
| `rich-account` | Existing account with every automation configured | 9 | desktop, hosted | - |
| `start-rejected` | Start with unresolved setup, then a rejected write | 9 | desktop, hosted | - |
| `stop-failed` | Failed Stop | 9 | desktop, hosted | - |
| `copy-absent-unit` | Copy: a unit the game does not know | 10 | desktop, hosted | - |
| `copy-compatible` | Copy: compatible destinations | 10 | desktop, hosted | - |
| `copy-conflict` | Copy: the first Save conflicts | 10 | desktop, hosted | - |
| `copy-conflict-changed` | Copy: the other window changed a destination | 10 | desktop, hosted | - |
| `copy-differences` | Copy: destinations with their own values | 10 | desktop, hosted | - |
| `copy-mixed` | Copy: Direwolves reserved at an outer main castle | 10 | desktop, hosted | - |
| `copy-queue-production` | Copy: Recruit Troops and Auto Tool per-castle plans | 10 | desktop, hosted | - |
| `copy-unobserved` | Copy: troop counts not current on this connection | 10 | desktop, hosted | - |
| `hosted-account-center` | Hosted Account Center with the fictional Demo Keep account | 11 | hosted | hosted: No desktop equivalent: the desktop shows Start Bot and Settings. |
| `onboarding-existing-custom-setup` | Existing custom setup: everything already done | 11 | desktop, hosted | - |
| `onboarding-hosted-runtime-no-feature` | Runtime up, no feature configured | 11 | desktop, hosted | hosted: Hosted Account Center shows the fictional 'Demo Keep' account with the three-way copy (Start the hosted runtime / Turn on <Feature> / First result); the Add-account form is never opened. |
| `onboarding-interrupted-return` | Interrupted return: an unsaved draft is waiting | 11 | desktop, hosted | - |
| `onboarding-wrong-world` | Login rejected with a typed reason | 11 | desktop, hosted | hosted: Hosted wording: 'The saved server does not match this login. Change the server in the Account Center.' |
| `account-switch-editor` | Switch account with an editor open | 12 | desktop, hosted | - |
| `onboarding-account-switch` | Two accounts: a draft belongs to one | 12 | desktop, hosted | - |
| `locale-ar` | Arabic right-to-left at 375: the adjusted disclosure groups | 13 | desktop, hosted | - |
| `locale-de` | German at 1280: the adjusted disclosure groups | 13 | desktop, hosted | - |
