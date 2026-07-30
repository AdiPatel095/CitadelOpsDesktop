# Auto Recruit

Keeps unit recruitment queues busy at the configured castles, either from one
global list or a per-castle plan with rotation.

Source: `Server/Automation/ProductionPolicy.go` (shared with
[Auto Tool](AutoTool.md)).

## Identity

| | |
|---|---|
| Policy ID | `autoRecruit` |
| Enabled key | `recruit_troops` |
| Priority | 40 (`PriorityRecruit`) |
| Config section | `automation.recruitTroops` |
| Production line | `lineID: 0`, definition key `unit` |

Auto Recruit and Auto Tool are the *same* `ProductionPolicy` type constructed
with different ids, enabled keys, sections, and production line ids
(`ProductionPolicy.go:47`). Behaviour is identical; only the queue differs.

## Settings

| Field | Default | Meaning |
|---|---|---|
| `mode` | `global` | `global` uses one item list everywhere; `perCastle` uses each castle's own list |
| `globalItems` | — | Item targets used in global mode |
| `castles` | — | Per-castle plans: `enabled`, `items`, `cursor` |
| `checkIntervalSec` | 300 | Pacing |

Each target is `{id, amount}`.

## Wake triggers

Domains: `production`, `subscriptions`. Section: `automation.recruitTroops`.

## How it works

For each configured, enabled castle in a stable order, it picks the target item
and queues it if the queue has room.

**Rotation** is the one behaviour unique to Auto Recruit: when
`mode == perCastle` and a castle has more than one target, a per-castle cursor
rotates through them (`productionRotationCursor`), so a castle with several unit
types cycles rather than always recruiting the first entry.

## Guards

- **Queue capacity is computed, not assumed.** Base queue capacity is 2
  (`productionBaseQueueCapacity`), extended by the recruitment stack-capacity
  effect (`effect id 189`). A castle whose stack capacity is not yet known is
  counted as `unknownStackCapacity` and skipped rather than over-queued.
- **Per-castle enable.** A castle plan with `enabled: false` is skipped even if
  it has targets.
- **Missing castles are skipped.** A configured castle id not present in state
  is ignored rather than erroring.
- **Invalid targets are skipped.** A target with `id <= 0` is skipped.
- **Subscription awareness.** `subscriptions` is a wake domain because premium
  status changes queue capacity — the policy re-evaluates when it changes.

## Status reporting

The policy reports counts of configured, observed, and full castles, so the UI
can distinguish "nothing to do because everything is busy" from "nothing to do
because nothing is configured".
