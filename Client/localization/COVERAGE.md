# Desktop localization coverage ledger

Status: partial implementation, not a translated release. Baseline: `e77ed022c2f926ae3509527fdabf54dca9e22586`. Current source has **1,890 typed keys: 1,812 custom and 78 official routes**. Each of 25 non-English UI packs contains **304 authored entries**, leaving **1,508 custom keys missing per locale**. Complete authored groups are equipment (63), activity (32), and battle presentation (66), and event/history presentation (41), and automation lane presentation (11 compound templates); those group counts do not certify their entire screens. Native-speaker review remains pending. English fallbacks are not credited as translations; legitimate language-neutral numeric/ID templates preserve their semantics.

Backend catalogs contain 25 complete 329-key packs from approved Backend98 `3f55544cadc758f69154e3e0b9d815b3e983a275` through Frontend63 `93607ec68898e0d4b649a5dee70e8941ed9ed3d4`. Server catalogs contain 228 of 3,181 entries per locale, synchronized from approved runtime PR84 `e0f6bf1e9acdbf676bc26815d240e03ce6157dac`. Its full source lineage, including PR78 ruby notification behavior, is integrated. The strict gate compares the copied English catalog against the actual Server catalog in this checkout, so internally consistent stale copies cannot pass.

## Source inventory

The latest inventory has **6,696 unreviewed conservative candidates**, 47 source-bound reviewed records and 10,065 structural exclusions. Candidate count is not a count of visible messages: property selectors, raw identity values and other dataflow candidates still need review. Run `npm run check:localization` from Client to regenerate the inventory and exact coverage counts.

Source assignments are retained in `static-migrations.json` (729 standalone sinks), `attribute-migrations.json` (505 static attributes), `icon-label-migrations.json` (79 whole icon-adjacent labels) and `patch-note-migrations.json` (263 release subtitles/items). `common-key-map.json` records reviewed semantic consolidation. Source keys live in `messages.ts`, `sourceMessages.ts` and `richMessages.ts`; `ui.en.json` is generated for tooling.

`source-reviews.json` records exact context-bound classifications; stale records fail. Narrow syntax rules exclude intrinsic SVG rendering attributes and known keys in explicitly imported `MessageKey` state. Negative fixtures retain SVG titles/descriptions/ARIA, custom component properties, untyped state and arbitrary key objects. No blanket waiver covers dynamic display values or historical logs.

## Remaining surfaces

| Surface | Current state and remaining work |
| --- | --- |
| Shell/navigation/shared controls | Viewer-local persisted selection and offline official common controls work. Remaining accessibility strings, select empty/search text and full responsive/RTL audit remain. |
| Settings/forms/confirmation/help | Static keys and selected rich sentences are assigned. Dynamic validation, descriptions, date/duration formatting and most language entries remain. |
| Equipment | Modal authored group and canonical calculation invariance are covered. Remaining pure summary labels and display provenance consumers need migration. |
| Battle reports | New presentation group covers result/filter labels, aggregate counts, dates, plural summaries and whole item tooltips. Lane/location fallbacks are localized, with historical resolved-only effects marked original. Remaining effect accessibility catalogs, effect provenance and older source-assigned labels remain. |
| Automation/events/history/rankings | Official event keys must replace remaining English display overrides. Feature-name glossary is available for explicit reuse; operational IDs, classification and UTC boundaries must remain unchanged. |
| Activity/notifications | Activity controls and finite row labels have all-language entries. Structured messages render reactively; unknown historical text remains explicitly original. Server runtime packs remain partial. |
| API errors | Validated adjacent descriptors are preserved at common boundaries. Remaining consumers must retain reactive descriptors rather than flattening them into strings. |
| Exports/copy | Activity copy uses rendered display text. Remaining human-readable exports need audit; settings JSON keys/values and user-created filenames remain machine/user data. |
| Patch notes/support | Historic release prose is keyed and original order/text is tested. Release kind/count/date presentation is reactive; all release prose translations remain outstanding. |
| Game data | Official per-viewer projections and cache invalidation exist. Remaining game-derived unit/tool/effect/quest/event labels need explicit verified-key routing and fallback provenance. |
| Dates/numbers/durations | Viewer formatters and several consumers are integrated. Browser-default formatters elsewhere remain. Numeric form values and dispatch parameters must remain canonical. |
| User names/IDs/private diagnostics | User text and protocol identities remain verbatim. Only private diagnostics are excluded; displayed logs remain in scope. |

## Invariants and provenance

Equipment calculations, semantic deduplication and default priorities use canonical English effect metadata independently of viewer display text. Real items786.03/language4357 evidence covers 830 definitions: 671 usable templates, 156 absent descriptions and three retired descriptions. Unexpected empty/partial canonical results retain prior same-scope state. Exact identity-compatible absences preserve baseline behavior; a version bump alone does not disable actions. `canonicalEffectCoverage.v4357.json` records provenance. Narrow versioned display corrections cover two verified Japanese sign defects without rewriting official source.

`official-effect-argument-audit.json` records the two raid-boss templates with positional index2 and evidence that no current Client consumer displays them. Unexpected generic arrivals preserve unresolved tokens. `hollow-moon-official-link.json` proves the equipment-set to event128 mapping. Bundled official common controls carry v4357 source/subset hashes and remain available offline. Authored packs retain source hashes and explicit model-authored provenance; official equality with English is not evidence of missing translation.

## Verification and completion gate

The existing Client build and 181 tests pass at the type-contract correction checkpoint. The explicit app typecheck remains failed with the same **43 baseline diagnostics**, with zero new diagnostics after six localization errors were corrected. Node-project typechecking passes. See `TYPECHECK.md`; a successful bare build `tsc` invocation is not a full app typecheck.

Strict completeness remains failed until source review, six dynamic surface groups, every required locale entry and actual Server integration are complete. Authored-subset tests never waive missing coverage. Browser fixtures in `tests/localization-browser/README.md` exercise locale changes with open equipment modals, retained numeric selection and captured-only payloads, notifications/activity, Arabic text isolation and German numeric formatting. `/responsive.html` verifies English/German/Arabic navigation positioning with production CSS. Synthetic routes cannot mutate a game. Full connected integration and remaining layout/source acceptance are outstanding.

## Event/history checkpoint

The 41-key event module has all 25 authored locale packs with exact source and translation hashes. Official event IDs drive viewer-local display; nonempty historical names retain exact bytes on fallback. Event date-only values retain UTC boundaries, while display dates/numbers use the selected locale. League thresholds, score availability and canonical IDs are unchanged. Root/list boundary tests include Arabic literal isolation. Event-history source review is restricted to four account-scope/filter identity candidates; broader dynamic coverage remains open. The full app still has the same 43 baseline diagnostics. Native-speaker review and browser review of this batch remain pending.

## Automation lane consumer checkpoint

Lane and overall runtime details accept descriptors only when `fallbackText` exactly equals the adjacent raw detail. Known status selectors cover all finite status assignments found in current `Server/Automation` producers; unknown values remain verbatim with original-language metadata. All 25 packs include status/lane labels, waiting policies, missing-decoration counts, attack/day counters, accessible status labels, and rounded countdown presentation. Countdown day/hour/minute granularity remains unchanged. 202 tests and build pass; app diagnostics remain the same 43 baseline failures. Authentic synthetic CRA90/EUP440 browser fixtures are prepared, but final browser proof awaits reviewed runtime source and pack integration. No runtime/state behavior changes or dispatches are authorized by these fixtures.

## Integrated lane verification

The runtime PR84 lineage is integrated without changes to its Server tree. Client notification test conflict resolution preserves both localized-error and PR78 ruby deduplication tests. Ruby policy-skip notices additionally carry only exact-bound descriptors; polling and recovery identities remain unchanged. All 204 Client tests, build, node typecheck and full `go test ./...` pass. App typecheck remains the same 43 baseline errors. Actual runtime and backend catalog validation has no nonmissing catalog errors; completeness still fails for 1,508 custom keys per locale, 2,953 runtime keys per locale, 6,696 source candidates and six dynamic groups.

Browser `/lanes.html` on the integrated working tree uses actual candidate packs and primary-source official mocks. English → German → Arabic changes the two CRA90/EUP440 lockouts reactively, including German cost 3.100/threshold 2.500, while exact opcode/code/deadline and literal synthetic identity remain. Unknown lane/status/detail retain English provenance and exact text. The toggle remained enabled with zero callbacks and fixture telemetry reported zero operational requests. Arabic screenshot inspection showed contained layout at the default 1280×720 viewport. This is synthetic consumer verification, not a live runtime or native-speaker approval.
