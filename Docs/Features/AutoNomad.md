# Auto Nomad

Farms the Nomad / Samurai event: level the four regular camps to max, lock the
weakest maxed camp, then chain attacks against it with time skips clearing the
cooldown between hits. Optionally runs a resource-sized trial against a
robber-baron castle.

Source: `Server/Automation/AutoNomadPolicy.go`, `Server/App/NomadIntents.go`.

## Identity

| | |
|---|---|
| Policy ID | `autoNomad` |
| Enabled key | `auto_nomad` |
| Priority | 10 (background — no entry in the priority table) |
| Config section | `automation.autoNomad` |

## Settings

| Field | Meaning |
|---|---|
| `sourceCastleId` | Castle the chain attacks from |
| `presetId` / `nomadPresetId` / `samuraiPresetId` | Attack formations per event type |
| `nomadDifficultyId` / `samuraiDifficultyId` | Event difficulty to select |
| `scoreTarget` | Stop once event score reaches this |
| `minimumRemainingSec` | Stop launching when the event is nearly over |
| `dailyAttackLimit` | Server daily attack cap |
| `skipCooldowns` + `timeSkipReserve` | Whether to spend time skips, and per-item reserves |
| `horseTravelBoostId` | Travel boost |
| `rbcTest` | Optional robber-baron trial (`enabled`, `runId`, `targetX`, `targetY`) |

## Wake triggers

Domains: `attacks`, `map`, `movements`, `commanders`, `units`, `events`,
`event-scores`, `nomad-camps`, `tower-cooldowns`, `currencies`,
`attack_dialog`, `achievements`. Section: `automation.autoNomad`.

## The loop

```
select difficulty
      │
scan map for the four camps
      │
level all four to max ──(cooldowns cleared with time skips between hits)
      │
lock the weakest maxed camp
      │
chain attacks against the locked camp ──▶ refresh after each confirmed victory
                                     └──▶ time-skip its cooldown
```

## Decision ladder

1. **Difficulty not selected** — `nomad.difficulty.select`.
2. **Score target reached** — `complete`, stop.
3. **Event nearly over** (`minimumRemainingSec`) — `idle`, no new attacks.
4. **Locked camp just won** — `map.query` to refresh it immediately after the
   confirmed victory.
5. **Locked camp on cooldown, skips unavailable** — `waiting`.
6. **Locked camp on cooldown** — clear it before the next chained arrival.
7. **Camps not discovered** — `nomad.map.scan` for the four camps.
8. **Fewer than four found** — `waiting`.
9. **Camp won, needs refresh** — `map.query`.
10. **Leveling in progress** — `waiting` while the current hit resolves.
11. **No single skip can clear a camp while leveling** — `waiting`.
12. **Camp cooldown clearable** — skip it and continue leveling.
13. **All four maxed** — `nomad.target.lock` on the weakest.
14. **Locked camp cooldown** — `nomad.cooldown.minute_skip`.
15. **No uncommitted skip above reserves** — `waiting`.
16. **Preset shortage / no launchable troops** — `waiting` with the shortage detail.
17. **Ready** — `nomad.camp.attack`.

RBC trial path, when enabled: clear the RBC cooldown → refuse on unsafe arrival
order → `nomad.rbc_test.attack`.

## Guards

- **Four-camp completeness.** The chain refuses to lock a target until all four
  regular camps are discovered and maxed, so it cannot lock a suboptimal camp.
- **Post-victory refresh.** After every confirmed victory the camp is re-pinged
  before any cooldown decision, so a skip is never sized against a stale
  cooldown reading. `PendingCooldownRefresh` marks camps in that window and the
  CRA send guard refuses to launch at them.
- **Uncommitted skips.** Skips already committed to in-flight hits are
  subtracted before deciding another launch is affordable.
- **Time-skip reserves.** Per-item reserves are never spent below.
- **Unsafe arrival order.** The RBC trial stops if hits would land out of order.
- **Daily attack limit.** Enforced as a plan step at launch, not just at
  decision time.
- **Preset capacity.** Refuses when the source castle cannot field the preset,
  reporting the exact shortage (item id, required, available).

## Notes

Auto Nomad and Auto Khan both use the shared nomad-camp cooldown machinery
(`nomad.cooldown.minute_skip`, `NomadCamps.Cooldowns`,
`PendingCooldownRefresh`) and both attack via `cra`, so they contend for the
`AdmissionAttackLaunch` admission slot and the 4–6 s attack lane. They do not
otherwise block each other.
