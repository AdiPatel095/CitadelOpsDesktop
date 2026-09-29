# Settings placement matrix (CIT-17)

Every automation settings editor shows an **Essentials** view first. **Advanced** groups start collapsed, and each one shows a one-line summary while collapsed. The browser remembers which groups a person expanded, per feature (`localStorage`, `citadelops.settings.disclosure.v1.<feature>`). Expanding or collapsing a group only changes what is visible. It never saves, resets, applies presets, changes permissions, or starts or stops anything.

- **Essentials** hold the task, the choices and dependencies it requires, its effective scope, operational limits, reserves, spending and consumable policy, and a reachable Stop.
- **Advanced** holds check timing, tooling, reserve tiers, filters and expert flags.
- Anything consequential inside an Advanced group appears in that group's collapsed summary: paid travel, time skips, premium or Ruby spending, demolition, restrictive filters, and custom values.
- When a readiness check points at a control inside a collapsed group, its Fix button expands the group and focuses the control. The check line also says which collapsed group holds the setting.

The code source of truth is `settings/disclosure/placement.ts`. `tests/settings-placement-matrix.test.mjs` fails when this table, the placement file, or any modal's `SettingsSection` usage disagree.

Every modal has a run line at the top of its Essentials view (`AutomationRunStrip`). It shows the current state (Running, Running until a time, or Stopped) and the weekly schedule where the feature supports one. It also has a **Stop now** button, which writes `automation.enabled` only when clicked. Saving never starts or stops an automation. Auto Buyer keeps its existing Run switch in place of the strip.

## Matrix

"Maya review" records product acceptance for each row. Evidence for every `accepted` row: `Maya matrix review 2026-09-29, Desktop 1c2fa93 / Hosted 745eef2, Product/Simpler automation setup.md § CIT-17 placement matrix review`. `accepted (adjusted)` rows are the ones Maya moved in that review; the placement below is her exact decision and is implemented as written.

| Feature | Section | Tier | Contains | Collapsed summary | Maya review |
|---|---|---|---|---|---|
| autoNomad | setup | essentials | Source castle; Nomad and Samurai attack setups | — | accepted |
| autoNomad | event | essentials | Event start difficulties; stop at score; stop before event end | — | accepted |
| autoNomad | limits | essentials | Daily attack limit | — | accepted |
| autoNomad | cooldown-skips | advanced | Clear landed-hit cooldowns with time skips; skip reserves | Cooldowns cleared or waited out; skips kept in reserve | accepted |
| autoNomad | travel | advanced | Paid travel | Travel paid with coins, Rubies or the feather | accepted |
| autoNomad | rbc-trial | advanced | Robber baron castle trial and its target | Trial on or off, and the target | accepted |
| autoInvasion | setup | essentials | Source castle; attack setup | — | accepted |
| autoInvasion | event | essentials | Difficulties; stop at score; stop before event end | — | accepted |
| autoInvasion | fortify | essentials | Fortify each target and its currency (Rubies never default) | — | accepted |
| autoInvasion | limits | essentials | Daily attack limit | — | accepted |
| autoInvasion | travel | advanced | Paid travel | Travel payment | accepted |
| autoKhan | setup | essentials | Attack castle; main castle; attack and main-castle defense setups; defense reapplication note | — | accepted |
| autoKhan | policy | essentials | Lock automatic attacks; trigger rage; replenish defense tools (coin packages only); open-gate protection; Nomad points stop (Ruby cost at the limit) | — | accepted (adjusted) |
| autoKhan | skips | essentials | Required cooldown skipping; skip reserves; stop before event end | — | accepted |
| autoKhan | limits | essentials | Daily attack limit | — | accepted |
| autoKhan | stop-limits | advanced | Rage chain limit; Rage points booster requirement | Rage chain limit; booster required or not | accepted (adjusted) |
| autoKhan | travel | advanced | Paid travel | Travel payment | accepted |
| autoBeriWorld | attack | essentials | Source castle; Gallantry booster requirement; tower attack setup | — | accepted |
| autoBeriWorld | limits | essentials | Daily attack limit | — | accepted |
| autoBeriWorld | building | essentials | Builder lane; built-in or captured camp target; capture tools; camp resource reserves; premium (Ruby) construction costs | — | accepted (adjusted) |
| autoBeriWorld | building-options | advanced | Construction time skips and reserves; demolition | Demolition; time skips and reserve | accepted (adjusted) |
| autoBeriWorld | attack-options | advanced | Attack check interval; armorer tool minimums (coins); troop transport skips; transfer check interval and minimum | Check interval; tool types bought with coins; transfer skips | accepted |
| autoBeriWorld | travel | advanced | Paid travel | Travel payment | accepted |
| autoTowers | castles | essentials | Castle on or off; tower troop, stock and radius per castle; per-castle scope line; Copy to other castles | — | pending |
| autoTowers | limits | essentials | Daily attack limit | — | accepted |
| autoTowers | advisor | advanced | Advisor chains; activation with a token; daily time-skip cap | Advisor on or off, token activation, daily time skips | accepted |
| autoTowers | scan | advanced | Map scan interval; maiden-supported filter per castle | Map scan every N; maiden-supported commanders only at N castles | accepted (adjusted) |
| autoTowers | travel | advanced | Paid travel | Travel payment | accepted |
| autoFortress | kingdoms | essentials | Kingdom targets, main castles and Direwolf stock | — | accepted |
| autoFortress | supply | essentials | Direwolves per shop session (spend); Khan tablet reserve; price tiers | — | accepted |
| autoFortress | limits | essentials | Daily attack limit; cache and ready-time note | — | accepted |
| autoFortress | transfer-skips | advanced | Time skips for Direwolf transfers | Transfers use time skips or not | accepted |
| autoFortress | travel | advanced | March speed advice; paid travel | Travel payment | accepted |
| autoStorm | castle | essentials | Storm castle access and unlock plan | — | accepted |
| autoStorm | targets | essentials | Forts and resource islands: levels, resources, sizes, attack setups, island defense units | — | accepted |
| autoStorm | donors | essentials | Troop import and donor castles; minimum troops kept after launch; troop-cap preview | — | accepted (adjusted) |
| autoStorm | shop | essentials | Aquamarine reserve and prioritized purchase goals; premium (Ruby) construction costs | — | accepted (adjusted) |
| autoStorm | limits | essentials | Daily attack limit | — | accepted |
| autoStorm | construction | advanced | Blueprints and capture; optional decoration; construction flags (demolition, resource shipments, time skips); castle and donor reserves; time-skip reserves; harbor | Target or combat only; demolition; resource shipments; time skips; harbor level; decoration | accepted (adjusted) |
| autoStorm | priority | advanced | Attack target priority | Number of ordered target types | accepted |
| autoStorm | travel | advanced | Paid travel | Travel payment | accepted |
| autoStorm | timing | advanced | Policy check interval; map refresh; map coverage | Check interval; map refresh | accepted |
| autoFoodBalance | reserves | essentials | Donor reserve; coin reserve; kingdom transport; castle food table; Account-wide settings only; no per-castle copy. | — | accepted |
| autoFoodBalance | timing | advanced | Polling interval; minimum kingdom shipment; minimum Storm delivery | Check interval; shipment minimums | accepted |
| autoFoodBalance | transport-skips | advanced | Transport time skips and reserves | Time skips on or off; skips kept | accepted |
| autoFoodBalance | travel | advanced | Market barrow travel | Travel payment | accepted |
| autoStation | evacuation | essentials | Evacuate at N minutes; recall when clear; open-gate fallback | — | accepted (adjusted) |
| autoStation | reserves | essentials | Troops left to defend per castle, with stock; Copy to other castles | — | pending |
| autoStation | filters | advanced | Minimum Bird protection days on target | Protection filter | accepted (adjusted) |
| autoBird | targets | essentials | Minimum Bird protection days on target | — | accepted |
| autoBird | castles | essentials | Active preset; kept units per castle, with stock; Copy to other castles | — | pending |
| autoBird | timing | advanced | Random delay range; minimum group size | Delay range; minimum per send | accepted |
| autoRecruit | plan | essentials | Shared or per-castle mode; enabled castles; units and schedules; Glory title fallback; Copy to other castles | — | pending |
| autoRecruit | timing | advanced | Queue check interval | Check interval | accepted |
| autoTool | plan | essentials | Shared or per-castle mode; enabled castles; tools and schedules; Copy to other castles | — | pending |
| autoTool | timing | advanced | Queue check interval | Check interval | accepted |
| autoHospital | schedule | essentials | Scan windows (weekly schedule); Account-wide settings only; no per-castle copy. | — | accepted |
| autoHospital | timing | advanced | Queue check interval | Check interval | accepted |
| autoTCI | items | essentials | Construction items and level floor and ceiling per castle | — | accepted |
| autoTCI | presets | advanced | Saved item presets (apply, save as new, delete) | Number of presets; applied preset | accepted |
| autoSceatRes | reserves | essentials | Coin and ruby reserves; logistics; Storm buffer; Ruby recipes; Ruby overflow skip | — | accepted |
| autoSceatRes | crafting | essentials | Recipe cycles per building; queue slot rentals (coins) | — | accepted |
| autoSceatRes | timing | advanced | Check interval; minimum shipment; overflow threshold | Check interval; shipment minimum; overflow threshold | accepted |
| autoSceatRes | transport-skips | advanced | Transport time skips and reserves | Time skips on or off; skips kept | accepted |
| autoSceatRes | storage | advanced | Additional storage nodes (read only) | Number of storage nodes | accepted |
| autoBooster | purchase | essentials | Daily fortress-speed boost (2,500 Rubies); ruby reserve | — | accepted |
| autoBooster | evidence | advanced | Purchase evidence; dispatch safeguards | Latest evidence recorded or not; checks run before every purchase | accepted |
| autoBuyer | limits | essentials | Main castle; ruby reserve; allow Ruby-priced packages | — | accepted |
| autoBuyer | goals | essentials | Shop limits; specialist floors; feast upkeep | — | accepted |
| autoBuyer | timing | advanced | Check interval | Check interval | accepted |
| autoAdvisor | access | essentials | Advisor access, tokens and explicit activation; live run | — | accepted |
| autoAdvisor | setup | essentials | Source castle; attack preset; automated difficulties | — | accepted |
| autoAdvisor | gates | essentials | Coin and Ruby cost per attack; coin, Ruby, feather and time-skip reserves | — | accepted |
| autoAdvisor | run-sizing | essentials | Maximum attacks; stop before event end | — | accepted (adjusted) |
| autoAdvisor | travel | advanced | Paid travel | Travel payment | accepted |
| autoEquipmentCleanup | schedule | essentials | Cleanup schedule | — | accepted |
| autoEquipmentCleanup | timing | advanced | Poll interval | Check interval | accepted |

## Deltas from the brief matrix (reviewed by Maya, 2026-09-29)

Items 1, 4, 6, 7, 8, 9 and 10 were accepted as proposed; items 2, 3 and 5 were accepted with the adjustments stated in them.

1. **Daily attack limits are Essentials everywhere,** including Invasion, where the brief listed the limit as Advanced. The acceptance criteria put operational limits in Essentials, and one rule for every feature is easier to learn.
2. **Khan:** "Replenish defense tools" (purchase policy) and the cooldown-skip reserves stay in Essentials, next to the required cooldown-skip switch. The Nomad points stop is Essentials too, beside open-gate protection, because both open the gates and pay the Ruby open-gate cost. The rage chain limit and the Rage points booster requirement are Advanced ("Rage chain and booster limits"), and their summary states each one.
3. **Towers:** Advisor mode, including its token activation and daily time-skip cap, is one Advanced group. Its summary always states whether tokens and time skips can be used. The radius stays on each castle card in Essentials, because it is the castle's effective scope. The "Map scan and target filters" group keeps the scan interval and the maiden-supported filter per castle, and its summary states both. Each castle card keeps a one-line scope summary.
4. **Fortress:** the Khan tablet reserve stays in Essentials, next to the Direwolf purchase ceiling it protects. The brief listed it as Advanced; the acceptance criteria put reserves in Essentials.
5. **Storm:** the Aquamarine purchase goals stay in Essentials with the reserve, because they are the spending policy. The premium (Ruby) construction switch moves there too ("Aquamarine and Ruby spending"), so every Ruby permission stays outside a fold. Construction, including its reserves, is Advanced, and its summary states demolition, resource shipments, time skips, harbor and decoration. The minimum troops kept after launch and the troop-cap preview are reserves, so they sit in Essentials with the donors; there is no separate import-sizing group.
6. **Bird:** the minimum protection days on the target are Essentials, as in the brief. Preset management stays in Essentials because it is the same control that selects the active preset.
7. **TCI:** there is no interval setting to collapse, so the Advanced group is the saved item presets.
8. **Equipment Cleanup:** its editor has only a schedule and a poll interval. Categories and keep thresholds, which the brief listed, belong to the Equipment view, not this editor.
9. **Recruit and Tool** share one modal with two entry points. Their rows are identical apart from the Glory title fallback, which exists only for Recruit.
10. **Buyer** keeps its existing Run switch instead of the run line (see above).

Other placements Maya moved in the same review: Beri World's premium (Ruby) construction switch is Essentials (Camp construction), and Auto Station's open-gate fallback is Essentials (Evacuation), because it is what happens when evacuation cannot finish; its Advanced group is the protection filter. Auto Advisor's run sizing (maximum attacks per run, stop before event end) is Essentials, because it caps paid attacks.

## Intentional platform differences

The desktop and hosted clients share the disclosure modules byte-for-byte: placement, summaries, fix targets, the section component and the run line. Their modal files differ only where they already differed before CIT-17. The hosted client uses compact editors in narrow layouts.
