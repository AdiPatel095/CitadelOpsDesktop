# Runtime localization coverage

Source baseline: approved `df5a66733e25a664be7f58de299b56cd1dd813e3` (PR #75).
The pack branch also includes bootstrap PR #74 at
`dca87226959be84b703de9fe226c23a48548e9a3`. These are source revisions,
not merge, deployment, or live-verification claims.

## Authored custom packs

All 25 non-English packs contain **196 of 3,072 source keys** (4,900 values).
Each locale still has **2,876 missing keys**. Missing, stale, or unsupported
messages must keep explicit English/raw fallback provenance; they earn no
translation credit.

| Bounded module | Keys per locale | Evidence |
| --- | ---: | --- |
| API errors | 73 | source/translation hashes and ICU gate |
| Configuration messages | 25 | source/translation hashes and ICU gate |
| Telemetry channel labels/descriptions | 46 | official feature glossary, stage fixtures |
| Intent failure/recovery messages (excluding operation descriptions) | 45 | actual ICU rendering and safety/terminology fixtures |
| Reviewed exact source-key reuse | 3 | explicit origin key/revision/hashes and semantic review |
| Earlier runtime messages | 4 | source/translation hashes and ICU gate |

The three reuse targets are the removed Experimental Battle Research settings
App guard and two unavailable-official-data Automation guards. Their reviewed
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

The source-bound syntax inventory at this baseline records 1,176 descriptor
sites, 46 explicitly registered telemetry sites, and 48 unclassified sites.
Some unclassified sites forward descriptors or finite event identifiers; others
remain source-owned visible prose. These counts are not a complete audit of
assignments, errors, compound strings, or game-noun parameters.

Remaining concrete groups include Storm/Tower plan summaries and purchases,
Auto Buyer package/specialist decisions, Invasion refresh/inventory decisions,
Storm/Berimond dependency reasons, and source-owned error paths. Descriptor
presence does not prove translated parameter provenance or pack coverage.
Historical opaque text remains whole and untranslated rather than guessed.

The missing-client browser response has a separate complete 26-locale embedded
bootstrap catalog. Root process-only startup/flag logs are operator diagnostics;
they are not part of the browser message catalog. Current battle-report effects
carry positive definition IDs and numeric values; historical objects without
those identities cannot be safely reconstructed by matching English labels.

## Integration gate

Run `scripts/validate-locales.mjs`, its negative fixtures, channel-stage fixtures,
and intent-safety fixtures. The Client owner runs `scripts/sync-client.mjs` into
its owned destination. Compare the exported English catalog hash with the
actual integrated Server catalog and retain both PR #74 and #75 lineage.
An older baseline or successful dictionary fetch is not full localization.
