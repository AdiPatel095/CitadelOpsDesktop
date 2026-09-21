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
Translation plural/select options may vary by locale while argument identity stays
stable. Further pack work must preserve ICU semantics and numeric precision too;
parser/argument checks alone do not establish linguistic correctness.
