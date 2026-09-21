# Backend message translation source

The portal repository owns these backend-namespaced client catalogs. `en.json`
is extracted from CitadelOpsBackend PR95 at e8e34993 (282 explicit keys), not an
independently edited translation. The server sends keys, scalar parameters and
safe context descriptors; each viewer selects a pack locally.

German and French translations were authored by the implementation model in this
task on 2026-09-20. No source was uploaded to a translation service. They have
key/ICU-argument validation; independent native-speaker review remains pending.
Coverage records must distinguish authored translations, fallback and review.

## Glossary and register

- CitadelOps, Ops Credits, Google, US1, RFC3339, JSON and API paths retain their
  identity. Generic credits may use the language's normal financial noun.
- API identifiers (commanderID, savedAtUnix, worldId, CRA, AIN, admin/edit,
  players/alliances) remain literal code, even inside translated prose.
- German uses informal singular `du` and imperative `Gib`/`Prüfe`. French uses
  polite `vous` and imperatives `Saisissez`/`Vérifiez`.
- Runtime means an executing account instance; launch means a captured Rift
  launch record. Tombstone means a deletion marker, not an in-game grave.
- A hosted tenant is an access-isolated account; it is not a player alliance.
- Credential validation must tell the user what to fix. Conflict messages retain
  expected/current revisions and record context. Never translate a diagnostic
  string by reverse matching it.

Every translated template must retain exactly its source ICU arguments. Context
labels translate independently; their identifiers remain scalar user data.
Source hash provenance and reproducible sync are maintained by the catalog scripts.
