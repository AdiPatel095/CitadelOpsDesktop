# Authored runtime message packs

These packs are incomplete: every non-English locale contains 228 of 3,181 source
messages. All 73 API messages, 25 configuration messages and 46 telemetry channel
labels/descriptions are covered, alongside 45 intent failure/recovery messages,
four explicitly reviewed source-key reuses, 20 Storm/runtime messages, 11 lane lock/ruby guard messages, and four earlier runtime messages.
The remaining 2,953 keys per locale must fall back explicitly to English. Exact
coverage is emitted by `../scripts/validate-locales.mjs`.

Every entry records source and translation SHA-256 plus authoring/reuse provenance.
Exact reviewed source-key reuse additionally records the origin key, immutable
revision, both origin hashes, and semantic review; validation rejects drift or
unmatched templates. It is an authoring aid, never runtime English matching.
The catalog baseline is approved source revision 97535d9. No output from the
rejected local MADLAD pilot is included. Agent review is not native-speaker review.
Protocol fields and enum values remain exact; game terminology follows verified
v4357 dictionaries. ICU plural categories may vary by locale, while argument
identity, numeric precision and source select branches remain stable.

The canonical standalone feature glossary is `../feature-names.json`. Provenance
records its exact hash and reviewed naming revision. Validation rejects copied
label drift even when an entry's translation hash is updated. Rift labels come
from official `event_title_133`; other labels are directly authored. Whole prose
may inflect feature names naturally instead of concatenating label fragments.

Activity descriptions preserve launched attacks versus completed actions. Run
`../scripts/channel-stages.test.mjs` for authorship vocabulary fixtures and the
dispatch-as-completion negative fixture. These checks supplement semantic review;
parser, argument and vocabulary checks alone do not establish linguistic quality.

Intent failure/recovery checks in `../scripts/intent-safety.test.mjs` preserve
partial versus unconfirmed outcomes, blocked purchase/retry conditions, official
commander/feast terms, and timed versus manually reviewed lock recovery. They
render all 45 messages in all 25 locales, including literal brace-containing
error data. Vocabulary fixtures supplement review, not native-speaker validation.

The Client owner may run `../scripts/sync-client.mjs` with the explicit
`Client/src/i18n/server` destination and installed FormatJS parser module path.
It validates packs before copying them, the English catalog, provenance and the
feature glossary. The coverage manifest records every copied source-file hash.
Final integration must compare the exported English catalog hash with the actual
integrated Server catalog; an older baseline is not complete integration evidence.
