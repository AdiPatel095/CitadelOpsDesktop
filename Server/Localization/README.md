# Runtime message descriptors

Server responses retain their existing raw English fields and add optional structured
presentation metadata. `fallback` is a static ICU template; primitive parameters are
never reinterpreted as templates. `fallbackText` is the complete sanitized legacy
message. A client choosing that verbatim fallback must not prepend `context` again.
Context contains at most four leaf descriptors, in prefix order. Official game keys
and one-level `gameParams` use the official locale catalog; user names stay ordinary
parameters. Unknown external error prose and historical unstructured messages remain
explicitly `untranslated`; a generic translation must not conceal the actual reason.

`WithError` preserves Error and Unwrap behavior. Presentation fields do not participate
in dispatch fingerprints. Identifier humanization discards an incompatible descriptor
rather than reintroducing identifiers which the raw visible field already removed.

## Catalog maintenance

Run from the repository root:

```
go run ./Server/Localization/cmd/catalog .
go test ./Server/Localization ./Server/Telemetry
```

`en.json` contains live static producer templates plus the explicitly enumerated
`dynamic.json` telemetry channel family. Source/catalog tests check both directions;
telemetry tests check dynamic values against actual channel definitions. Translation
packs are separate work and are not included in this producer change.

## Activity persistence

New structured feature activity is one queued JSON line with schema
`citadel.activity.v1`, the complete original `line`, and its descriptor/status.
There is no separately ordered sidecar. Existing plaintext records remain supported.
Tail preserves the old string array; the API additionally returns aligned entries.
Attack recovery, timestamp extraction, retention, rotation, and feature filtering
unwrap the same record. Truncated structured records are skipped, never paired with
a neighboring line. Diagnostic traffic retains its existing format. External tools
which read raw activity files must recognize the versioned record format.

## Current coverage boundary

Descriptors cover static API errors, source-known validation errors, intent plans and
steps, intent descriptions, automation decisions/state details, failure presentation,
stationing status, and structured activity/channel labels. Unknown causes, opaque
custom/durable historical prose, and incomplete source context remain untranslated.
Game-derived nouns still passed as ordinary parameters require further producer
provenance migration to `gameParams`; this contract alone does not translate them.
The English catalog and descriptors do not mean non-English packs are complete.

The official noun extension attaches verified keys at building, Berimond tool,
production/healing unit, resource shipment, and Auto Buyer price/feast producers.
Keys are selected from official item identities and source-defined candidates;
rendered English labels are never used as lookup keys. Feast names follow the
verified official `PremiumFestivalItemVO.nameTextID` expression:
`"dialog_festival_" + type + "Event"`, including case-sensitive `bigLevel2` types.
Automation snapshots carry the shared runtime language only to verify those keys;
viewer preferences never change automation language or dispatch inputs.

Compound custom nouns (overseer variants, bundle labels, level suffixes) still need
structured custom composition. Missing official keys retain the exact fallback.
