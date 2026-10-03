# CORE_POL success replay

`core_pol_success.xt` comes from a retained `CORE_POL/10005` inbound WebSocket
push on `citadelops-prod-cell-001`, read on 2026-10-03 for CIT-125. The read-only
capture lookup exported only the sanitized frame, never the original payload
or profile identifiers. It corroborates investigation item 12 in
`Reports/opcode-errors-2026-10-02/opcode-gap-investigation.txt`.

The original header, offer-list root array, field names, nested offer details
(`OD`), visual components and reward components are preserved. Every scalar
payload value was replaced: strings with `sanitized`, numbers with `1`, and
booleans with `false`. Empty arrays and nulls retain their shape. No account
IDs, names, coordinates or original private offer values remain.

The replay decodes through `Protocol.Decode` and records through `Store.Record`,
checking both memory and persistent telemetry for successful receipt and zero
ERROR lines. Opcode-error inventories count those ERROR lines; Store has no
separate inbound response-error counter.
