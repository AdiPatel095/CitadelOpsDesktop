# Runtime localization coverage

Source baseline: approved `97535d9bbfc1271f8cc28e04508187dfbd10954a` (PR #81).
The pack branch also includes bootstrap PR #74 at
`dca87226959be84b703de9fe226c23a48548e9a3`. These are source revisions,
not merge, deployment, or live-verification claims.

## Authored custom packs

All 25 non-English packs contain **228 of 3,181 source keys** (5,700 values).
Each locale still has **2,953 missing keys**. Missing, stale, or unsupported
messages must keep explicit English/raw fallback provenance; they earn no
translation credit.

| Bounded module | Keys per locale | Evidence |
| --- | ---: | --- |
| API errors | 73 | source/translation hashes and ICU gate |
| Configuration messages | 25 | source/translation hashes and ICU gate |
| Telemetry channel labels/descriptions | 46 | official feature glossary, stage fixtures |
| Intent failure/recovery messages (excluding operation descriptions) | 45 | actual ICU rendering and safety/terminology fixtures |
| Storm purchases, outcomes, castle identity and guarded actions | 20 | actual ICU/list rendering and source-bound authoring |
| Lane safety locks, lifecycle and ruby guard messages | 11 | persisted binding, typed numeric guard fixtures and ICU rendering |
| Reviewed exact source-key reuse | 4 | explicit origin key/revision/hashes and semantic review |
| Earlier runtime messages | 4 | source/translation hashes and ICU gate |

The four reuse targets are the removed Experimental Battle Research settings
App guard, two unavailable-official-data Automation guards, and the matching Auto Buyer catalog guard. Their reviewed
API origins have identical complete meanings and no arguments. Validation
requires the origin and target templates/translations to match and checks both
origin hashes. This is a build-time authoring aid, never reverse translation.

Agent-authored and reviewed text is not native-speaker validation. Local MT
pilot output was rejected and is not used. Semantic checks preserve partial
versus unconfirmed actions, read-only retries, blocked repeat purchases, manual
lock review versus 30-minute eligibility, and official commander/feast terms.
The vocabulary fixtures supplement review; substring matching cannot prove
linguistic correctness.

## Source producer audit

The source-bound syntax inventory at this baseline records 1,193 descriptor
sites, 46 explicitly registered telemetry sites, and 34 unclassified sites.
Some unclassified sites forward descriptors or finite event identifiers; others
remain source-owned visible prose. These counts are not a complete audit of
assignments, errors, compound strings, or game-noun parameters.

Remaining source groups include Storm castle cost/skip summaries, Storm/Berimond
dependency reasons, combat-return aggregates and source-owned error paths.
Most syntax candidates are forwarding boundaries; this is not an exhaustive
reachability or noun-provenance audit. Descriptor
presence does not prove translated parameter provenance or pack coverage.
Historical opaque text remains whole and untranslated rather than guessed.

The missing-client browser response has a separate complete 26-locale embedded
bootstrap catalog. Root process-only startup/flag logs are operator diagnostics;
they are not part of the browser message catalog. Current battle-report effects
carry positive definition IDs and numeric values; historical objects without
those identities cannot be safely reconstructed by matching English labels.

## Integration gate

Run `scripts/validate-locales.mjs`, its negative fixtures, channel-stage fixtures,
intent-safety fixtures, and storm-stages fixtures. The Client owner runs `scripts/sync-client.mjs` into
its owned destination. Compare the exported English catalog hash with the
actual integrated Server catalog and retain both PR #74 and #75 lineage.
An older baseline or successful dictionary fetch is not full localization.

Current follow-on integrates PR78 ruby guard lineage without changing whitelist, timing or premium authorization. Updated lock detail omits operation IDs, carries exact protocol code strings, and falls back entirely for unknown or stale context metadata. Source/pack candidate is not deployment evidence.
