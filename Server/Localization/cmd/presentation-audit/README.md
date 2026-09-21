# Presentation producer inventory

Run `go run ./Server/Localization/cmd/presentation-audit .` from the repository root.
The JSON report inventories known visible struct fields, elided slice elements,
Step/Decision helper calls discovered from function and lexical closure signatures, and writes to
`details`/`.Details` maps. It recognizes explicit fields, helper descriptor
arguments, step decorators, and adjacent map-descriptor assignments.

This is an inventory tool, not a declaration that localization is complete.
`unclassifiedSites` includes real missing producers, forwarding boundaries, and
other unclassified boundaries. Verified finite telemetry registry sites are counted
separately and require an exact channel ID/key/source match. Other field
assignments, source-owned error factories, and reachability of map values require
additional audit. Opaque historical or third-party text remains distinct from
source-owned prose. Descriptor presence proves neither complete authored packs
nor official noun provenance. Catalog parity remains a separate check.

The regression fixture covers elided fields, helper calls, decorators, and map
assignments, lexically scoped multi-result closures, and stale telemetry registry
entries so those previously missed boundaries remain detectable.
