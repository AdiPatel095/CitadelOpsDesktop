# Auto Advisor

Uses the event advisor to run a batch of server-managed attacks against a Nomad
or Samurai camp, rather than launching each hit individually. One advisor
activation buys a run of N attacks that the game executes on its own.

Source: `Server/Automation/AutoAdvisorPolicy.go`, `Server/App/AdvisorIntents.go`.

## Identity

| | |
|---|---|
| Policy ID | `autoAdvisor` |
| Enabled key | `auto_advisor` |
| Priority | 10 (background) |
| Config section | `automation.autoAdvisor` |

Declares no `WakeSections()`, so it is not woken by its own configuration
changes — only by state and by its check interval.

## Settings

| Field | Meaning |
|---|---|
| `sourceCastleId` | Castle the run launches from |
| `presetId` | Attack formation |
| `nomadDifficultyId` / `samuraiDifficultyId` | Event difficulty |
| `maxAttackCount` | Attacks requested per advisor run |
| `minimumRemainingSec` | Stop starting runs near event end |
| `coinCostPerAttack` / `minimumCoinReserve` | Coin budget and floor |
| `rubyCostPerAttack` / `minimumRubyReserve` | Ruby budget and floor |
| `minimumFeatherReserve` | Advisor token floor |
| `timeSkipReserve` | Per-item time-skip reserves |
| `mapRefreshIntervalSec` | Camp scan staleness |
| `horseTravelBoostId` | Travel boost |

## Wake triggers

Domains: `advisor`, `map`, `commanders`, `units`, `events`, `event-scores`,
`nomad-camps`, `currencies`, `resources`, `attack_dialog`, `achievements`.

## Decision ladder

1. **Difficulty not selected** — `nomad.difficulty.select` before activating the
   advisor.
2. **Run in progress, overview stale** — `advisor.overview.refresh` to read
   progress.
3. **Run in progress** — `running`, attack N of M.
4. **Run complete** — `complete`.
5. **Run ended early** — `idle`; a stopped run is deliberately **not**
   restarted.
6. **Camps not discovered** — `nomad.map.scan`.
7. **Ready** — `advisor.run.launch` for N attacks at the chosen camp.
8. **Otherwise** — `waiting`.

## Guards

- **Currency floors.** Coin, ruby, and feather (advisor token) reserves are all
  checked before a run is started, and the per-attack costs are multiplied by
  the requested attack count — the feature will not start a run it cannot
  finish paying for.
- **No auto-restart.** A run that was stopped or interrupted is reported and
  left alone. Restarting could double-spend tokens against a run the server may
  still consider live.
- **Progress is read, not assumed.** `advisor.overview.refresh` reads
  cumulative gains, costs, losses, wins, and remaining attacks from the game
  rather than counting locally.
- **Event window.** `minimumRemainingSec` prevents starting a run that cannot
  finish before the event closes.
- **Admission.** `advisor.run.launch` carries the `autoAdvisor` attack module,
  so it competes for the shared attack-launch admission slot with every other
  attacking feature.
