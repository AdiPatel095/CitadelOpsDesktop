# Onboarding preview runbook: desktop (CIT-22)

This is the local preview of the whole integrated onboarding experience: goal-led first setup, inline event setup, readiness, explicit simulated Start, first-result feedback, Stop, castle copy, draft recovery and connection repair. **It runs the production UI against synthetic data. It cannot reach a game, an account, a purchase, an attack, a transfer or any production API.** Everything you see is simulated, and the page says so all the time.

## What "simulated" means

- A persistent banner that cannot be dismissed says: **Simulated preview: sample data only. Nothing here reaches the game or an account.**
- The dock at the bottom shows the candidate revision (`Candidate <short SHA>`) and counts what it intercepted.
- Receipts and first results read **Simulated: ...** (for example "Simulated: attack on tower 12"). A first result is shown as confirmed only for a scenario-declared simulated receipt attributed to that automation; no intent you trigger in the preview can ever become a first result (they are recorded, answered with a receipt whose actor is `ui`, and sent nowhere).
- Nothing here is runtime evidence, proof of durable reliability, a deployment check or a business result. The Go server is not running; see "Known limitations".

## Candidate revision

- Candidate: the head of PR branch `ethan/cit-22-onboarding-preview` in this repository (`git rev-parse HEAD`; the dock shows the same short SHA). The PR description records the full SHA under test.
- Stack: CIT-19 `845bcc89f7f220aa2a0fa5f96fa44776b18339c3` (desktop) over CIT-21 `e5fa8376f1c1f40cd85beedf8d4f2101cef63f74` over CIT-20 `eb614948b1d7b5f469764abde1ab2b66c5b8fa09`, on `develop` `6a5078c77905186004aeef17fc9983199f281ba7`.
- The hosted candidate is in the hosted repository (`Docs/OnboardingPreview.md`).

## Requirements

- Node.js 20.19 or newer, or 22.12 or newer, and npm (the same as the rest of the repository).
- No Go toolchain, browser automation or credentials are needed for this preview. No new dependency is added.

## Install

```sh
git checkout ethan/cit-22-onboarding-preview
npm ci --prefix Client
```

## Run and stop

One command, from the repository root (or `npm run preview:onboarding` inside `Client/`):

```sh
npm run preview:onboarding
```

Open **http://127.0.0.1:41734/** (the first scenario is `new-user`). The port is fixed; if it is taken the command fails instead of picking another. Stop with Ctrl-C. Ports used: 41734 only.

## Pick a scenario

Use the dock's Scenario list, or the address bar:

```
http://127.0.0.1:41734/?scenario=<id>
```

Optional parameters (all combinable): `session=live|disconnected|awaiting-baseline|checkpoint`, `fail=start|stop` (one rejected `automation.enabled` write per page load), `locale=en|de|ar`, `account=2` (the second account of scenarios that have one), `draft=changed` (the recorded draft was made against settings that were saved again), `reset=1`.

The dock also has: **Toggle connection**, **Advance runtime** (applies the scenario's next simulated game report), **Switch account** (scenarios with two accounts), **Reset scenario**, and an **Intercepted** list that shows every write the preview recorded and every request it refused. The scenario list and what each one shows are in `OnboardingCoverage.md`.

## Reset

- **Reset scenario** (dock) or `?reset=1` clears every `citadelops*` key in this browser's local storage and reloads the scenario from its start. Settings you saved during a session live only in the page's memory and are gone on reload; recorded drafts, chosen goals and turn-on times are re-seeded from the scenario on every load.
- Reloading always returns to the scenario's starting state.

## 10-minute CEO path

Steps 1, 2, 9, 6 and 13 below, in that order.

## Walkthrough (about 25 minutes)

Each step lists what must be visible and what must not. No step shows a green success unless a simulated first result is shown as such, and no player-facing text says "runtime".

1. **Start.** Run the command, open the URL. See the banner and `Candidate <short SHA>` in the dock. Nothing else is needed.
2. **First use, goal-led (`onboarding-disconnected-first-use`, then `new-user`).** Automation page: "No automation is on yet" and "Set up with a goal". Choose "Play the Nomad or Samurai event". The checklist shows *Connected to the game* Blocked with "Check the connection": open it; the repair dialog appears over the page and offers Start Bot; press Start Bot (the dock's Intercepted list records it; the fixture connects). Then open `new-user`: Auto Nomad opens with a recommended one-wave setup from the source castle's most numerous attack troops; customize one slot; read the consequence lines; Save. Open Attack Presets: the preset is marked "Created by app". Reopen the editor: hidden Advanced values are kept. "Save as preset" makes a user preset. *Must not:* Save or opening an editor starting anything; a step marked done because you visited a page.
3. **Reuse and promotion (`custom-presets`, `presets-edit-promotion`).** Pick the shared preset in Invasion; rename or edit the app-created preset and accept the promotion prompt (observation: do you understand that renaming makes it yours?); duplicate copies only the record; deleting a preset in use is refused with the modules named.
4. **Khan (`khan-defense-fresh`, `khan-defense-stale`, `khan-missing-main`, `khan-missing-defense`, `khan-skip-dependency`).** Fresh: the defense starter is previewed and applied ("Created by app"). Stale: refused with the reason. Missing main castle / defense: blocked lines with an in-context Fix.
5. **Storm (`storm-offer`, `storm-no-offer`, `storm-donor-unavailable`).** With an offered starter castle the unlock plan is valid and Save goes through; with none offered the accepted copy appears, the toggle stays operable and Save is blocked with its reason; donor lines wait or name the missing castle.
6. **Contrast and disclosure (`towers-food-contrast`, `support-production`, `disclosure-matrix`).** Towers castle cards with stock lines; Food Balance roles and "Last known"; every editor with Essentials first and collapsed Advanced groups with summaries; a Fix into a collapsed group expands and focuses the control.
7. **Stale data (`stale-data-disconnected`, `stale-data-awaiting-baseline`, `stale-data-checkpoint`, `zero-castles`).** "Waiting for account data" everywhere, never "blocked"; "Check connection" opens the repair and returns to the editor.
8. **Shortages and conflicts (`shortages-conflicts`).** Commander conflict panel, castle not in this world (reselect), shortage lines. Answer for the record: *does FREE on a commander that is switched off read as "will be used"?*
9. **Readiness, Start, first result, Stop (`phases-enabled-waiting`, `start-rejected`, `stop-failed`, plus `phases-*`, `first-result`, `first-failed`, `rich-account`).** Auto Towers "Before you start" is Ready; turn it on (the switch is the only Start). Press **Advance runtime** three times: waiting, running, then a first result that reads "Simulated: attack on tower 12" and confirms it. Stop and read what it says about an attack already on its way. In `start-rejected` the blocked check opens "Start with unresolved setup?" (Fix first is the default); "Start anyway" is rejected once, then Retry succeeds. `stop-failed`: the switch keeps following the saved value, with the failure and Retry.
10. **Copy (`copy-compatible`, `copy-mixed`, `copy-absent-unit`, `copy-differences`, `copy-unobserved`, `copy-queue-production`, `copy-conflict`, `copy-conflict-changed`).** The dialog states, explicit includes, Apply to draft, Save. In `copy-conflict` the first Save conflicts: use "Load latest and re-apply copy"; the status line appears and the next Save succeeds. In `copy-conflict-changed` the dialog reopens for review.
11. **Goal-led recovery (`onboarding-hosted-runtime-no-feature`, `onboarding-interrupted-return`, `onboarding-wrong-world`, `onboarding-existing-custom-setup`).** The checklist changes only with state; the recovered-draft banner (Restore / Discard; add `&draft=changed` for Compare); the repair names a typed cause only when the game reported one. The hosted Account Center step is in the hosted runbook.
12. **Account switch (`onboarding-account-switch`, `account-switch-editor`).** Open an editor, press **Switch account**: saved castles that are not in the other world ask to reselect; a recovered draft belongs to one account only.
13. **Locales and sizes (`locale-de` at 1280 wide, `locale-ar` at 375 wide).** The adjusted Khan, Towers, Beri, Storm, Station and Advisor groups in German and in right-to-left Arabic; then a keyboard pass: Tab through one editor, Enter or Space on a disclosure header, and check focus returns after each dialog. Use your browser's responsive mode for 375 px; the dock shows the suggested width.
14. **Stop.** Ctrl-C the command. Port 41734 is released.

## Evidence

Screenshots and one recording per platform are taken by whoever runs the walkthrough (Sophie for QA) and stored under `QA/Evidence/CIT-22/` in the vault, with a step index. File names: `desktop-<step>-<scenario>-<width>-<locale>.png`. Every image of a first result must show the "Simulated" label. This evidence is not runtime proof.

## Known limitations

- Fixture evidence is not runtime evidence. The Go server is not exercised; the preview runs the production UI against fixtures served in the browser. (Optional proof through the real server with a synthetic frame log was **not** attempted: see the PR description.)
- The hosted demo account is fictional. The turn-on time used for first results is per browser.
- Building layout capture and blueprint previews are not available in the preview (they answer "not available").
- "Advance runtime" applies the scenario's scripted game reports; there is no simulated clock, so schedule and timed-lock expiry are only shown as scripted.
- Screenshots and the recording need someone to run a browser; the agents that built this could not.
- The fixture holds saved settings in the page's memory only; reload starts the scenario again.
