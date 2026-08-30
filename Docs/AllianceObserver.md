# Alliance Observer — Design Notes

> **Status: future feature, not started.** Less worked-through than
> [`RiftRaidCoordination.md`](RiftRaidCoordination.md) — this records the shape,
> what already exists, and the open questions. Not a committed plan.

Watches for incoming attacks and for alliance property being captured, alerts
the alliance through Discord, and can optionally launch a clearing attack
against occupying forces.

---

## 1. Scope

| Part | What it does |
|---|---|
| **A** | Detect incoming attacks on the operator's own castles |
| **B** | Detect capture of alliance property — **outpost, metropolis, capital** |
| **C** | Alert to a Discord channel, alliance-wide |
| **D** | Optionally launch a clearing attack to wipe occupying forces |

Discord doubles as the coordination bus: humans read the alerts, and other app
instances can read the same channel for sync. That avoids building a bespoke
inter-instance transport, and reuses a channel the alliance already runs.

---

## 2. What already exists

### Castle type identification ✅

`castleTypeName` (`Server/AllianceTargets/Projection.go:188`) already maps the
exact property set in scope:

| SlotType | Meaning |
|---|---|
| 1 | Main castle |
| 3, 6 | **Capital** |
| 4 | **Outpost** |
| 5, 22 | **Metropolis** |

### Alliance holdings carry those types ✅

`reduceAllianceInfo` (`Server/Ingest/AccountReducers.go:725`) parses the `ain`
frame and builds one `AllianceHolding` per member holding from the member's `AP`
rows:

```go
type AllianceHolding struct {
    CastleID, PlayerID, KingdomID  // owner
    X, Y                            // position
    SlotType                        // ← capital / outpost / metropolis
}
```

**This is the key enabler.** Every member's capitals, outposts and metropolises
are already in state with position and type, refreshed by the `alliance.refresh`
intent. Capture detection can therefore be a **diff of `AllianceState.Holdings`
between refreshes** — one call covering the whole alliance — rather than
per-coordinate map scanning, which was the expensive approach I first assumed.

### Incoming attack detection ✅

`incomingThreats()` (`Server/Automation/AllianceStationPolicies.go:413`) returns
per-castle threat windows with earliest and latest impact, filtered through
`State.IsIncomingPlayerAttack` — which correctly excludes NPC and event attacks
(it requires a player-owned source castle). Auto Station consumes this today.

### Guards ✅

- **Protection Mode** — `ProtectionModeState.PreparingOrActive(now)`, already
  used by Auto Bird and Auto Invasion
- **Troops away under Auto Bird** — the policy tracks `birdReturnUnixMs` per
  castle, so "are my troops actually home" is answerable
- **Owner confirmation** — `map.query` on the lost coordinates returns
  `MapObservation.OwnerID`

### Clearing attack ✅

`alliance.target.attack` is registered as an `EffectLaunch` intent —
*"Launch a selected CitadelOps preset against an alliance target."*

---

## 3. What does not exist

1. **No notification infrastructure at all.** Nothing server-side. Discord
   integration is entirely net-new.
2. **No holdings snapshot or diff.** `AllianceState.Holdings` is overwritten on
   each refresh; nothing retains the previous set to compare against.
3. **No watch/alert policy.** No lane observes any of this.
4. **No cross-member incoming visibility.** Movements targeting another player's
   castles are private to that player. Alliance-wide *incoming* alerts require
   every member running an instance and reporting in — the same multi-instance
   dependency as the rift scheduler. **Capture** alerts do not have this problem,
   because holdings are alliance-visible.

---

## 4. Proposed flow

```
1  alliance.refresh on a cadence
2  diff Holdings against the previous snapshot
3  a capital / outpost / metropolis disappeared → capture suspected
4  map.query those coordinates → confirm new OwnerID
5  post alert to Discord (who lost what, where, to whom)
6  if auto-clear enabled and guards pass → alliance.target.attack with the
   configured preset to wipe occupying forces
```

Guards on step 6:

- not in Protection Mode (preparing or active)
- troops actually home (not out under Auto Bird)
- sufficient troops for the configured preset
- per-castle / per-day launch caps

---

## 5. Design decisions still open

**Detection cadence and loss.** Holdings diffing is polling. A property captured
and retaken between refreshes is invisible. How fresh does this need to be, and
does that justify a tighter `alliance.refresh` interval than other consumers
want?

**Disappeared ≠ captured.** A holding leaving the list could mean captured,
abandoned, or the member left the alliance. Step 4 disambiguates by reading the
new owner, but the alert should not claim "captured" before that confirms.

**Discord as a bus — direction.** Read-only alerting is simple. Having instances
*read* the channel for sync makes Discord a command path into the app, which
needs the same trust boundary thinking as the rift scheduler: an instance should
only act on messages from a channel and author it has been explicitly paired
with.

**Clearing attack semantics.** Confirmed intent is to **wipe occupying forces**
with a preset, not to retake the property. Worth stating explicitly in the
config so the behaviour is not mistaken for a recapture attempt.

**Who launches?** If several instances see the same alert, they could all launch
at once. Either the alert names a specific responder, or instances need a claim
protocol so exactly one responds.

---

## 6. Effort read

- **A (own incomings)** — small. Detection is done; only alerting is new.
- **B (capture detection)** — small-to-medium. Snapshot + diff over data already
  in state, plus a confirmation query.
- **C (Discord)** — medium, and net-new. Outbound alerting is straightforward;
  inbound command reading needs auth and pairing.
- **D (clearing attack)** — small. A policy wrapping an existing launch intent
  with guards that already exist.

The dependency worth noting: **B is cheap because holdings already carry slot
types**. If the scope later widens to monuments, alliance towers or villages,
none of those are modelled in state and the cost changes substantially.
