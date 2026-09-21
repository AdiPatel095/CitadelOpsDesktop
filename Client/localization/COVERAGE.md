# Desktop localization coverage ledger

Status: partial implementation, not a translated release. Baseline: `e77ed022c2f926ae3509527fdabf54dca9e22586`. Current source has **1,838 typed keys: 1,760 custom and 78 official routes**. Each of 25 non-English UI packs contains **252 authored entries**, leaving **1,508 custom keys missing per locale**. Complete authored groups are equipment (63), activity (32), and battle presentation (66); those group counts do not certify their entire screens. Native-speaker review remains pending. English fallbacks are not credited as translations; legitimate language-neutral numeric/ID templates preserve their semantics.

Backend catalogs contain 25 complete 324-key packs. Server catalogs contain 196 of 3,072 entries per locale, synchronized from approved `9e17ed7012356b2de5ae8976fe6a1f420186bbdc`. Final approved Server source integration is outstanding. The strict gate compares the copied English catalog against the actual Server catalog in this checkout, so internally consistent stale copies cannot pass.

## Source inventory

The latest inventory has **6,734 unreviewed conservative candidates**, 43 source-bound reviewed records and 10,026 structural exclusions. Candidate count is not a count of visible messages: property selectors, raw identity values and other dataflow candidates still need review. Run `npm run check:localization` from Client to regenerate the inventory and exact coverage counts.

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
