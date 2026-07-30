# Auto Bird

Continuously stations spare troops onto protected alliance castles, so troops
sit somewhere safe rather than at home where they can be farmed. Unlike Auto
Station this is not threat-driven — it runs steadily.

Source: `Server/Automation/AllianceStationPolicies.go:49`.

## Identity

| | |
|---|---|
| Policy ID | `autoBird` |
| Enabled key | `auto_bird` |
| Priority | 80 (`PriorityAutoBird`) |
| Config section | `automation.autoBird` |

## Settings

Under `ignoreSettings`:

| Field | Meaning |
|---|---|
| `settings` | Per-castle unit reserves that stay home |
| `minDelay` / `maxDelay` | Randomised delay band between sends |
| `minSend` | Minimum troop count worth sending |
| `minRPTDays` | Minimum remaining protection days for a destination |

The randomised delay is why the policy publishes `nextBirdUnixMs` and
`nextBirdCastleId` metrics — the UI shows when the next send is due.

## Wake triggers

Domains: `alliance`, `movement-snapshot`, `movements`, `player-protection`,
`stationing`, `units`. Section: `automation.autoBird`.

## Decision ladder

1. **Protection Mode preparing or active** — `protected`; the game suppresses
   stationing, so the feature stands down entirely.
2. **No protected alliance targets available** — `idle`.
3. **A castle has sendable troops** — `troops.station`, sending from that castle
   to a protected holding.
4. **Everything reserved or already stationed** — `idle`.

## Guards

- **Protection Mode.** Stationing is refused outright while Protection Mode is
  preparing or active — the game would reject or mis-handle it.
- **Reserves.** Per-castle unit reserves are subtracted before anything is
  considered sendable.
- **`minSend` floor.** Avoids a stream of tiny, pointless transfers.
- **`minRPTDays`.** Destinations must keep protection long enough to be worth
  using.
- **Randomised pacing.** `minDelay`/`maxDelay` spread sends out instead of
  emitting a burst the moment troops appear.
- **Per-castle return tracking.** `birdReturnUnixMs.<castle>` metrics track when
  troops come back, so a castle is not re-sent while a send is outstanding.

## Relationship to Auto Station

Same underlying `troops.station` intent and the same protected-holding
selection, but opposite triggers: Auto Bird moves troops out **routinely**, Auto
Station moves them out **because something is incoming** and then brings them
home. Auto Station runs at a higher priority so that, under threat, its
evacuation wins any contention.
