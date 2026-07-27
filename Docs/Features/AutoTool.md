# Auto Tool

Keeps siege/defence tool production queues busy at the configured castles.

Source: `Server/Automation/ProductionPolicy.go` (shared with
[Auto Recruit](AutoRecruit.md)).

## Identity

| | |
|---|---|
| Policy ID | `autoTool` |
| Enabled key | `auto_tool` |
| Priority | 40 (`PriorityAutoTool`) |
| Config section | `automation.autoTool` |
| Production line | `lineID: 1`, definition key `tool` |

Auto Tool is the same `ProductionPolicy` implementation as Auto Recruit,
constructed for production line 1 instead of line 0
(`ProductionPolicy.go:54`). Everything in [Auto Recruit](AutoRecruit.md) applies
here, with two differences:

1. It queues **tools**, resolved through the `tool` definition key.
2. It does **not** rotate. Rotation is gated on `policy.id == "autoRecruit"`
   (`ProductionPolicy.go:103`), so a tool castle with several targets always
   works the first entry rather than cycling.

## Settings

Identical shape to Auto Recruit: `mode` (`global` / `perCastle`),
`globalItems`, `castles` (`enabled`, `items`, `cursor`), `checkIntervalSec`
(default 300).

## Wake triggers

Domains: `production`, `subscriptions`. Section: `automation.autoTool`.

## Guards

Same set as Auto Recruit:

- Queue capacity computed from base capacity 2 plus the stack-capacity effect;
  castles with unknown capacity are skipped, not guessed.
- Per-castle `enabled` respected.
- Missing castles and invalid target ids skipped.
- Re-evaluates when subscription status changes queue capacity.

## Relationship to Auto Storm

Auto Storm has its own build and tool handling for the storm base and does not
route through Auto Tool. If both are enabled they operate on different castles
and do not contend, beyond ordinary claim serialization on any castle they
happen to share.
