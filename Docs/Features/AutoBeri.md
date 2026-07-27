# Auto Beri

Transfers troops to the Berimond world castle in batches, applying the fixed
speed-up to each transfer.

Source: `Server/Automation/BeriPolicy.go`, `Server/App/BeriIntents.go`,
`Server/GameFeatures/FeatureView/AutoBeriWorld.go`.

## Identity

| | |
|---|---|
| Policy ID | `autoBeriWorld` |
| Enabled key | `auto_beri_world` |
| Priority | 30 (`PriorityAutoBeri`) |
| Config section | `automation.autoBeriWorld` |

The smallest automation policy in the codebase (~116 lines) and a good place to
start reading if you want the shape of a policy without the complexity.

## Settings

| Field | Meaning |
|---|---|
| `beriCastleId` | Destination Berimond castle |
| `sourceCastleId` | Castle troops come from |
| `wireCastleId` | Castle id used on the wire, where it differs from the model id |
| `transferTroopId` | Which troop type to transfer |
| `minTroopsToTransfer` | Minimum batch size |
| `troopSpaceCheckIntervalSec` | How often to re-check available capacity |

## Wake triggers

Domains: `beri`, `castles`, `units`. Section: `automation.autoBeriWorld`.

## Decision ladder

1. **No Berimond castle or transfer troop configured** — `waiting`.
2. **Source castle not yet observed** — `waiting`.
3. **Capacity stale** — `beri.capacity.refresh`.
4. **Capacity below `minTroopsToTransfer`** — `idle`, reporting the observed
   capacity against the configured minimum.
5. **Ready** — `beri.transfer` with the batch size.

## Guards

- **Capacity is read from the game.** `beri.capacity.refresh` asks the game for
  the current transfer capacity rather than computing it locally, and the
  transfer is sized against that authoritative answer.
- **Minimum batch size.** Prevents a stream of tiny transfers, each of which
  costs a speed-up.
- **Source castle must be observed.** The feature refuses to size a transfer
  against a castle it has not seen.
- **Validated batch.** `beri.transfer` is an `EffectLaunch` intent that
  validates the batch before applying the fixed speed-up, so the speed-up is not
  spent on a transfer the game rejects.
- **Distinct wire castle id.** `wireCastleId` exists because the Berimond
  castle's id on the wire differs from its id in the model. Conflating them
  sends troops to the wrong place — this is the feature's main correctness trap.
