CIT-119 sanitized retained October 1, 2026 CRA/327 and CDS/327 replay.

Extracted read-only from retained wire logs on October 3. Each request was the
last outbound command of its exact opcode before the captured 327 response in
the same profile. The retained CDS receipt independently confirms its shape.
Only these sanitized fixtures were returned from the source host.

Castle/object/account identities are replaced with 10, commanders with 3,
source coordinates with 100:100 and destination coordinates with 101:101.
Catalog unit/tool IDs, counts, flags, array order, and the full constructor key
sets are retained. No account identity, name, or original coordinates is retained.
Balances are synthetic SCE raw frames: [["PTT",0]] and [["PTT",5]]. They exercise
applyPlayerCurrencies, not a hand-built currency observation.

Official payload and per-movement rule: CIT Doc Vault/Engineering/Plans/CIT-119
Travel tickets checked for attack and support.md; cached client bundle
Game.bundle.ffc3a3f14892d62ec53c.js. The CDS keys are exactly
SID TX TY A LID WT HBW BPC PTT SD.
