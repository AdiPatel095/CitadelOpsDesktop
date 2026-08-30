# Auto Sceat Resources

Keeps the sovereign crafting queues fed: starts crafting recipes and moves the
resources they need into place. Two lanes — one crafts, one does logistics.

Source: `Server/Automation/CraftingPolicy.go`,
`Server/Automation/CraftingLogisticsPolicy.go`,
`Server/Automation/CraftingLogistics.go`, `Server/Automation/CraftingOverflow.go`.

## Identity

| Lane | Policy ID | Purpose |
|---|---|---|
| Crafting | `autoSceatRes` | Refresh queues, start recipes |
| Logistics | (schedule/actor `autoSceatRes`) | Move resources so recipes can start |

| | |
|---|---|
| Enabled key | `auto_sceat_resources` (both lanes) |
| Schedule key | `autoSceatRes` (both) |
| Actor | `automation:autoSceatRes` — the logistics lane declares `ActorID()` |
| Priority | 45 (`PriorityAutoSceat`) |
| Config section | `automation.autoSceatResources` |

The logistics lane is the reference example of `ActorIDPolicy`: it is an
independent coordinator lane that attributes its operations to the parent
feature so priority, schedule, and activity routing stay unified.

## Settings

| Field | Meaning |
|---|---|
| `castles` | Per-castle crafting plan |
| `minimumShipmentSize` | Floor for a logistics move |
| `sourceReservePercent` | Percentage a donor castle retains |
| `overflowThresholdPercent` | When a castle counts as overflowing |
| `autoKingdomTransport` | Allow cross-kingdom moves |
| `useKingdomTimeSkips` + `allowedTimeSkips` + `timeSkipReserve` | Time-skip policy for transports |
| `useStormBuffer` | Use the Storm base as a resource buffer |
| `allowRubyRecipes` | Permit recipes that cost rubies |
| `useRubyOverflowSkip` | Permit a ruby skip on overflow |
| `minimumCoinReserve` / `minimumRubyReserve` | Currency floors |

## Wake triggers

| Lane | Domains |
|---|---|
| Crafting | `crafting`, `currencies`, `resources` |
| Logistics | `crafting`, `currencies`, `kingdom-transport`, `market`, `movements`, `resources` |

Both: section `automation.autoSceatResources`.

## Decision ladders

### Crafting lane

1. **No plan configured** — `waiting`.
2. **Queues stale** — `crafting.refresh`.
3. **A recipe can start** — `crafting.start`.
4. **Otherwise** — `idle`.

### Logistics lane

1. **No plan configured** — `waiting`.
2. **Leased market barrows still out** — `waiting` for their return.
3. **Logistics stale** — `resource.logistics.refresh`.
4. **Automatic logistics disabled in settings** — `idle`.
5. **No move needed** — `idle`.

## Guards

- **Ruby recipes are opt-in.** `allowRubyRecipes` is off by default; a recipe
  that costs rubies is never started implicitly. `useRubyOverflowSkip` is a
  second, separate opt-in.
- **Currency floors.** `minimumCoinReserve` and `minimumRubyReserve` are hard
  floors.
- **Source reserve percentage.** A donor castle keeps `sourceReservePercent`,
  so feeding a recipe cannot strip a castle.
- **Barrow leases.** Same model as Auto Food Balance — the lane waits for leased
  barrows to return rather than planning against committed capacity.
- **Overflow threshold.** Moves are driven by genuine overflow rather than
  small imbalances, which keeps transport traffic down.
- **Logistics can be disabled independently.** Turning off automatic logistics
  leaves the crafting lane running; the two are separable.

## Why two lanes

Crafting and logistics operate at different rhythms — a recipe start is quick
and event-driven, while a resource move involves market barrows or kingdom
transport and can be in flight for a long time. Splitting them means a long
transport never blocks a recipe that is already fundable, which is the same
reasoning behind [Auto Khan](AutoKhan.md)'s lane split.
