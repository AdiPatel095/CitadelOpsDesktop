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

`automation.autoBird` is a version 2 document. `ignoreSettings` remains the
backward-compatible saved configuration, while named entries under
`presets.presets` can now be selected at runtime:

| Field | Meaning |
|---|---|
| `activePresetId` | Named preset used by default; `null` uses `ignoreSettings` |
| `presets.presets[]` | Stable-ID named reserve, delay, minimum-send, and RPT configurations |

Each preset and `ignoreSettings` contain:

| Field | Meaning |
|---|---|
| `settings` | Per-castle unit reserves that stay home |
| `minDelay` / `maxDelay` | Randomised delay band between sends |
| `minSend` | Minimum troop count worth sending |
| `minRPTDays` | Minimum remaining protection days for a destination |

The randomised delay is why the policy publishes `nextBirdUnixMs` and
`nextBirdCastleId` metrics — the UI shows when the next send is due.

Another feature can switch Auto Bird atomically by changing
`activePresetId`; it does not need to copy the preset's reserve map. The Auto
Bird weekly schedule can enable **Specify Preset Per Period**, which stores a
required `presetId` in each slot. An active scheduled slot overrides
`activePresetId`. A missing, deleted, or duplicated selected preset stops the
policy instead of falling back to a different troop configuration.

## Wake triggers

Domains: `alliance`, `movement-snapshot`, `movements`, `player-protection`,
`stationing`, `units`. Section: `automation.autoBird`.

## Decision ladder

1. **Protection Mode preparing or active** — `protected`; the game suppresses
   stationing, so the feature stands down entirely.
2. **Selected preset unavailable** — `waiting`; no stationing request is made.
3. **No protected alliance targets available** — that castle receives its own
   retry window without blocking another castle.
4. **Castle is ready** — fresh `AIN` target discovery, then a castle-scoped
   `JAA` inventory capture, then guarded `CDS` dispatch and `GAM` movement
   reconciliation.
5. **Everything reserved or already stationed** — `idle`.

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
- **Preset-bound manifests.** Target and prepared stationing state records the
  selected preset ID. A default change or schedule-period transition invalidates
  that preparation and forces a fresh target/inventory cycle.
- **Period-bound dispatch.** Scheduled intent arguments carry the end of the
  selected period, and planning plus the final dispatch guard reject a launch
  after that instant.

## Relationship to Auto Station

Same underlying `troops.station` intent and the same protected-holding
selection, but opposite triggers: Auto Bird moves troops out **routinely**, Auto
Station moves them out **because something is incoming** and then brings them
home. Auto Station runs at a higher priority so that, under threat, its
evacuation wins any contention.
