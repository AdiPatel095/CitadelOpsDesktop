# Auto Beri

Transfers troops to the Berimond world castle, attacks towers, maintains camp
tools, and spends confirmed returned loot through an independent builder lane.

Source: `Server/Automation/BeriPolicy.go`,
`Server/Automation/BeriBuildPolicy.go`, `Server/App/BeriIntents.go`, and
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
| `build.enabled` | Enables loot-funded camp construction and upgrades |
| `build.stableLevel` | Stable target level from 1 through 5; resolved to the current official Berimond WoD |
| `build.allowPremium` | Allows target actions with official premium costs |
| `build.allowDemolition` | Allows exact-target removal of unmanaged destructible buildings |
| `build.allowTimeSkips` | Allows construction skips above configured reserves |

Premium spending, demolition, and time skips remain opt-in. The builder never
changes those permissions. Resource and time-skip reserves are applied in every
phase, so "use all resources" means all currently spendable resources above the
user's explicit reserves.

## Default builder target

With no custom blueprint active, the builder uses the built-in exact reference
camp layout: 17 ground tiles, 92 ordinary buildings, 64 decorations, and 22
fixed targets. It binds this template to the current owned Berimond camp each
season.

The snapshot's mixed Small tent WoDs, Large tent WoD, and Auxiliaries'
headquarters WoD are not retained as level targets. Every camp and tent family
is followed through the current official upgrade chain to its terminal WoD.
The Stable is the exception: `build.stableLevel` selects its official level-1
through level-5 WoD. Because the game has no safe Stable downgrade path, an
already-higher Stable satisfies the built-in target and is retained. An
explicitly active captured blueprint replaces the built-in target until the
user switches back to the default. Terminal Large-tent steps that carry an
official premium cost remain blocked unless `build.allowPremium` is enabled.

## Builder phase order

The builder recomputes its phase from each fresh castle snapshot and emits at
most one action before reevaluating:

1. Build or upgrade the selected Stable level in place. An existing higher
   Stable is retained, and Stables are never stored or demolished, including
   when an exact custom blueprint omits one.
2. Buy every missing ground tile required by the active target, one confirmed
   expansion at a time. If the next official resource expansion is not yet
   affordable, the builder waits and saves returned loot. It does not spend on
   later phases. A storage action is allowed here only when the official cost
   cannot fit the observed capacity.
3. For exact targets, remove buildings outside the target multiplicities.
   Matching follows official upgrade paths, so a retained lower-level target
   building is upgraded later rather than mistaken for an extra. Storeable
   extra decorations are stored first. Other eligible extras require
   `build.allowDemolition`; protected and Stable definitions are preserved.
   Functional and layout custom targets keep their non-exact unmanaged-building
   behavior.
4. Move retained target buildings and decorations to their exact positions,
   one confirmed move at a time. A collision or move cycle waits with a clear
   blocker instead of deleting a retained building.
5. Place or build every target decoration. An unavailable or unaffordable
   decoration blocks this phase, even when a tent is already affordable.
6. Construct and upgrade the remaining tents, camps, and fixed target
   structures.

An occupied construction queue, malformed target, unavailable permission,
resource shortage, or blocked placement waits in the active phase. It never
falls through to a cheaper later action. Restarting the service or changing the
active blueprint is safe because no phase cursor is persisted; the next phase
is derived from the latest authoritative layout, queue, and resource state.

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
