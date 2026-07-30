# Feature Reference

One file per automation feature: what it does, how its loop works, and every
guard that can stop it.

For the layer underneath — how a command actually reaches the game, what context
it needs, and the engine-level guard stack — see
[`IntentEngineCommandGuards.md`](../../IntentEngineCommandGuards.md) at the repo
root. This directory documents what each feature does *on top of* that.

For measured performance findings, the cross-feature mechanism inventory, and
the open architecture questions, see
[`Docs/ArchitectureReview.md`](../ArchitectureReview.md).

---

## Roster

19 policy lanes across 16 features. Lanes that share an enabled key are one
feature to the user.

| Feature | Policy ID | Enabled key | Config section | Doc |
|---|---|---|---|---|
| Auto Recruit | `autoRecruit` | `recruit_troops` | `automation.recruitTroops` | [AutoRecruit](AutoRecruit.md) |
| Auto Tool | `autoTool` | `auto_tool` | `automation.autoTool` | [AutoTool](AutoTool.md) |
| Auto Hospital | `autoHospital` | `auto_hospital` | `automation.autoHospital` | [AutoHospital](AutoHospital.md) |
| Auto TCI | `autoTCI` | `auto_tci` | `automation.constructionItems` | [AutoTCI](AutoTCI.md) |
| Auto Sceat Resources | `autoSceatRes` + logistics lane | `auto_sceat_resources` | `automation.autoSceatResources` | [AutoSceatResources](AutoSceatResources.md) |
| Auto Bird | `autoBird` | `auto_bird` | `automation.autoBird` | [AutoBird](AutoBird.md) |
| Auto Station | `autoStation` | `auto_station` | `automation.autoStation` | [AutoStation](AutoStation.md) |
| Auto Beri | `autoBeriWorld` | `auto_beri_world` | `automation.autoBeriWorld` | [AutoBeri](AutoBeri.md) |
| Auto Food Balance | `autoFoodBalance` | `auto_food_balance` | `automation.autoFoodBalance` | [AutoFoodBalance](AutoFoodBalance.md) |
| Auto Towers | `autoTowers` | `auto_towers` | `automation.autoTowers` | [AutoTowers](AutoTowers.md) |
| Auto Invasion | `autoInvasion` | `auto_invasion` | `automation.autoInvasion` | [AutoInvasion](AutoInvasion.md) |
| Auto Nomad | `autoNomad` | `auto_nomad` | `automation.autoNomad` | [AutoNomad](AutoNomad.md) |
| Auto Advisor | `autoAdvisor` | `auto_advisor` | `automation.autoAdvisor` | [AutoAdvisor](AutoAdvisor.md) |
| Auto Khan | `autoKhan` + 3 lanes | `auto_khan` | `automation.autoKhan` | [AutoKhan](AutoKhan.md) |
| Auto Storm | `autoStorm` | `auto_storm` | `automation.autoStorm` | [AutoStorm](AutoStorm.md) |

Registration order is in `Server/App/Application.go:206`. The coordinator sorts
policies by ID, so evaluation order is alphabetical, not registration order.

---

## The shared model

Every feature is a `Policy` (`Server/Automation/Types.go:39`) that answers one
question when asked: **given this snapshot of the world, what is the single next
thing to do?** It returns a `Decision`, never a command. It cannot send
anything, cannot mutate state, and cannot block.

```
Coordinator ──Snapshot──▶ Policy.Evaluate ──Decision──▶ Intent.Engine ──▶ game
```

A `Decision` carries a status and detail for the UI, a `NextCheckAt`, optional
metrics, and optionally an `Intent.Request` to submit. That request is the only
way a feature causes anything to happen.

### Guard layers

Every feature is protected at four levels. Feature docs describe levels 3 and 4;
levels 1 and 2 are identical for everyone and are documented here.

| Level | Guard | Applies to |
|---|---|---|
| 1 | Coordinator gating | every feature, before `Evaluate` is called |
| 2 | Engine guard stack | every intent — see the root doc |
| 3 | Policy decision ladder | per feature: the ordered conditions it refuses on |
| 4 | In-plan guard actions | per feature: live rechecks immediately before the effect |

### Level 1 — coordinator gating

Before any policy is asked for a decision
(`Server/Automation/Coordinator.go:259`), all of these must hold:

1. **Enabled** — its `EnabledKey()` is on in `automation.enabled`, and any timed
   enable has not expired.
2. **Session ready** — websocket connected, logged in, and the session baseline
   generation matches the current generation. A feature never acts on a
   half-loaded account.
3. **In schedule** — inside the configured weekly window for its
   `ScheduleKey()`.
4. **Not in a safety pause** — no `failureBlockedUntil` from a previous failed
   operation (30 s default).
5. **Not already running** — one operation per lane at a time.

After a decision, two anti-runaway guards apply:

- An identical repeated decision sets a 30 s pause (`rejectRepeatedDecision`).
- A chain of immediate re-runs is capped at 32 (`maxImmediatePolicyRuns`).

### Level 1b — when a feature is woken

A policy declares `WakeDomains()` (state domains) and `WakeSections()`
(configuration sections). A state change in a declared domain clears the
policy's `nextCheck` so it re-evaluates early, rather than waiting out its
interval. Wakes are coalesced through a 250 ms debounce; a policy can opt a
domain out of that window with `UrgentWakeDomains()` when the game-side
opportunity is short. Auto Khan's rage lane is currently the only user.

A policy that is mid-operation records the wake as pending rather than
interrupting itself.

### Continuation flags

Two `Decision` flags control what happens after an operation finishes:

- `ReevaluateOnSuccess` — on success, ask the policy again immediately instead
  of waiting for `NextCheckAt`. This is how chains advance.
- `ReevaluateOnStale` — if the operation stopped before dispatch because its
  plan went stale, ask again immediately so the policy can pick a different
  action.

### Reading a decision ladder

Each feature doc lists its decision ladder in evaluation order. The first
matching condition wins and everything below it is unreachable that pass. Order
is the feature's real priority ordering — that is usually the most important
thing to understand about a feature.

Common statuses:

| Status | Meaning |
|---|---|
| `idle` | Nothing to do; healthy |
| `waiting` | Blocked on something expected to clear on its own |
| `ready` / feature-specific | About to submit a request |
| `blocked` | Misconfigured or an error the user must resolve |
| `protected` / `yielding` | Deliberately standing down for a safety reason |
