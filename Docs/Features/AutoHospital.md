# Auto Hospital

Heals wounded units by feeding the hospital queue, and requests alliance help to
speed the queue along.

Source: `Server/Automation/HospitalPolicy.go`,
`Server/App/AllianceHelpIntents.go`.

## Identity

| | |
|---|---|
| Policy ID | `autoHospital` |
| Enabled key | `auto_hospital` |
| Priority | 50 (`PriorityHospital`) |
| Config section | `automation.autoHospital` |

## Settings

`hospitalSettings` has a single field, `checkIntervalSec`. Everything else is
derived from observed game state — which units are wounded, what the hospital
queue capacity is, and how many alliance-help requests are outstanding.

## Wake triggers

Domains: `production`, `subscriptions`, `units`. Section:
`automation.autoHospital`.

`subscriptions` matters because a premium subscription changes queue capacity.

## Decision ladder

1. **A hospital queue can take alliance help** — `alliance.help.request`.
2. **A wounded stack can be queued** — heal it (`ReevaluateOnSuccess`, so
   healing chains through the wounded stacks back to back).
3. **Otherwise** — one of:
   - `idle` — no wounded units need healing
   - `idle` — waiting for hospital queues to be observed
   - `waiting` — waiting for one of N outstanding alliance-help requests to
     finish
   - `idle` — queues are full or their capacity is not yet known

## Guards

- **Capacity must be known.** The feature distinguishes "queue is full" from
  "capacity not yet observed" and refuses to queue against an unknown capacity
  rather than guessing.
- **Outstanding help cap.** It will not pile up alliance-help requests; when the
  outstanding count reaches the cap it waits for one to complete. Alliance help
  is a social resource and spamming it is costly to the player.
- **Chaining is bounded by the coordinator.** `ReevaluateOnSuccess` means
  healing proceeds stack by stack without waiting out the interval, but the
  coordinator's `maxImmediatePolicyRuns` (32) and repeated-decision pause
  prevent a runaway loop if a heal silently fails to change state.
- **Pacing.** The heal decision uses `coordinatorTick` (2 s) as its
  `NextCheckAt`, so even without a wake the queue is revisited promptly.
