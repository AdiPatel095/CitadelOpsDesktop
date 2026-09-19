# Coin affordability at dispatch

Coin affordability is checked against the current-session authoritative `C1`
balance after a command has been fully resolved and immediately before the
transport sends it. The shared gate subtracts all unresolved process-local
coin reservations and preserves each feature's configured reserve. A normal
shortage is an availability wait: it does not create a 30-minute lane safety
lock or a toast, and a fresh authoritative balance wakes the lane.

Successful debits are released only when the command-correlated response
contains the authoritative `C1` observation. Otherwise the debit remains
reserved while a bounded account-inventory refresh reconciles it. Definitive
no-send and non-consuming rejection paths release the reservation; uncertain
sends retain it.

| Opcode / intent | Cost source at final dispatch | Reserve | Resolution |
| --- | --- | --- | --- |
| `BUP` production | Official item `costC1` × resolved stack; absent `costC1` on a valid item is the official zero default | Existing feature reserve where declared | Per resolved stack; base price is a conservative upper bound when modeled effects are discounts |
| `HRU` hospital | Official unit `healingCostC1` × resolved amount | Declared reserve | Per resolved batch; base price is a conservative upper bound over title/global discounts |
| `CDS` support | Official distance/item formula plus selected horse `costFactorC1` | Declared reserve | Resolved manifest, route, and horse |
| `CRA` attack | Official distance/item formula, current daily-attack surcharge, and selected horse cost | Advisor uses the configured coin reserve | Resolved formation and route; advisor's configured batch budget is a floor, not an extra fee |
| `CSM` spy | Official spy travel formula using the documented maximum risk | Declared reserve | Conservative upper bound because the resolved wire command does not carry risk |
| `CRM` market shipment | Official current caravan capacity, route/barrow formula, and selected horse cost | Auto Food and crafting minimum coin reserve | Resolved goods, route, live market effects, and horse |
| `CRUN` crafting rental | Official fixed production/queue slot price used by the planner | Crafting minimum coin reserve | Resolved slot |
| `CRST` crafting start | Official recipe `C1` cost | Crafting minimum coin reserve | Resolved recipe |
| `SBP` shop packages, including Berimond tools | Official package `packagePriceC1` × resolved amount; absent coin price on a valid package is the official zero default | None | Resolved package and amount; coin-priced buy-all variants block because their total-price semantics are not authoritative |
| `ERE` relic enchantment | Official `relicEnchanters` next-level `c1Cost` | Equipment upgrade coin threshold | Recomputed for every roll/retry from committed item level |
| `EQE` equipment enchantment | Official base formula `10 × round(17 × nextLevel^1.7)` | Equipment upgrade coin threshold | Recomputed for every roll/retry; base price is an upper bound over discounts |
| `BGE` gem insertion | Official normal-gem level `insertCostC1`, or the official 95,000-coin relic-gem constant | None | Resolved gem; normal base price is an upper bound over insertion discounts |
| `KUT` kingdom troop transfer | Official target-kingdom travel tax × unit travel cost; tools use the official 100-coin constant | None | Resolved manifest and target kingdom; undiscounted upper bound |
| `EUP` building upgrade | Official resolved target-building `costC1` | None | Resolved target level immediately before dispatch |
| `EBU` building construction | Official resolved building `costC1` | None | New construction only; placing an owned stored building remains coin-free |

The shared gate does not guess a price when the official record, dynamic state,
or numeric field is malformed. It blocks the command as unavailable. Ordinary
Construction item removal has an official `removalCostC1` field, but CitadelOps
currently has no removal intent. Read-only and proven coin-free commands bypass
the gate.
