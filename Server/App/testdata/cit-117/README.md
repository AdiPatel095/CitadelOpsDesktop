# CIT-117 queue replay provenance

Source: retained BUP success responses at 2026-09-21 20:15:26.903Z and
20:15:27.137Z, identified in Reports/opcode-errors-2026-10-02/opcode-gap-investigation.txt,
item 1. Read-only retrieval on 2026-10-03 confirmed the exact QS/P/SI fields,
unit amounts, help flags, and RUT/VIP values. These are raw inbound XT frames.

Sanitization keeps only the production snapshot, uses a synthetic route and sets production identifiers to zero. No account IDs,
names, castle IDs, coordinates, or unrelated payload data are retained. WID is the
public unit definition. The first frame has three occupied usable slots; the second
has four. Both contain five entries: two permanent, two active VIP, one locked.

English source provenance: Daniel's CIT-117 plan, Waiting (AC5), and Maya's
Opcode rejection fixes 2026-10-03 decision, story A. The new key
`server.automation.production_queues_full_at` renders
`All observed production queues are full at {used} of {usable}`. Only English
catalogs change; the epic's batched translation story owns other locales.
