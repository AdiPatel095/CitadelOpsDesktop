# Authored runtime message packs

These packs are incomplete. Current exact coverage is emitted by
`../scripts/validate-locales.mjs`; absent keys must fall back explicitly to English.
Every entry records source and translation SHA-256 plus authoring/reuse provenance.
No output from the rejected local MADLAD pilot is included. Agent review is not
native-speaker review.

The Client owner may run `../scripts/sync-client.mjs` with the explicit
`Client/src/i18n/server` destination and installed FormatJS parser module path.
It validates the complete ICU AST argument set and provenance before copying.
The copied coverage manifest contains every source-file hash for reproducibility.
Plural categories may vary by locale; source select enum branches and argument
identity must remain stable. Further pack work must preserve ICU semantics and numeric precision too;
parser/argument checks alone do not establish linguistic correctness.

The API batch covers all 73 current `server.api.*` keys in all 25 non-English
locales. Each locale currently contains 77 of 2,967 source messages. This is API
message coverage, not complete runtime or producer coverage. Protocol field names
and enum values remain exact; game role and kingdom terms follow verified official
v4357 dictionaries. Quantity templates may use locale-specific ICU plural rules.

The next batch covers all 25 `server.configuration.*` messages and the 20 canonical
feature channel labels in every locale: 122/2,967 entries per locale. Telemetry
descriptions and the remaining runtime templates are still incomplete. The
reviewed standalone feature glossary is `../feature-names.json`; its exact hash
and naming-review revision are recorded in provenance. Validation rejects copied
label drift even when an entry’s own translation hash was updated. Rift labels
come from official `event_title_133`; other labels are directly authored with
verified game terminology. Sync also copies the glossary and hashes it.
