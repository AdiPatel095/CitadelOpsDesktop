# Settings placement matrix (CIT-17)

Every automation settings editor shows an **Essentials** view first. **Advanced** groups start collapsed, and each one shows a one-line summary while collapsed. The browser remembers which groups a person expanded, per feature (`localStorage`, `citadelops.settings.disclosure.v1.<feature>`). Expanding or collapsing a group only changes what is visible. It never saves, resets, applies presets, changes permissions, or starts or stops anything.

- **Essentials** hold the task, the choices and dependencies it requires, its effective scope, operational limits, reserves, spending and consumable policy, and a reachable Stop.
- **Advanced** holds check timing, tooling, reserve tiers, filters and expert flags.
- Anything consequential inside an Advanced group appears in that group's collapsed summary: paid travel, time skips, premium or Ruby spending, demolition, restrictive filters, and custom values.
- When a readiness check points at a control inside a collapsed group, its Fix button expands the group and focuses the control. The check line also says which collapsed group holds the setting.

The code source of truth is `settings/disclosure/placement.ts`. `tests/settings-placement-matrix.test.mjs` fails when this table, the placement file, or any modal's `SettingsSection` usage disagree.

Every modal has a run line at the top of its Essentials view (`AutomationRunStrip`). It shows the current state (Running, Running until a time, or Stopped) and the weekly schedule where the feature supports one. It also has a **Stop now** button, which writes `automation.enabled` only when clicked. Saving never starts or stops an automation. Auto Buyer keeps its existing Run switch in place of the strip.

## Matrix

"Maya review" records product acceptance for each row; `pending` means not yet reviewed.

| Feature | Section | Tier | Contains | Collapsed summary | Maya review |
|---|---|---|---|---|---|
| autoNomad | setup | essentials | Source castle; Nomad and Samurai attack setups | — | pending |
| autoNomad | event | essentials | Event start difficulties; stop at score; stop before event end | — | pending |
| autoNomad | limits | essentials | Daily attack limit | — | pending |
| autoNomad | cooldown-skips | advanced | Clear landed-hit cooldowns with time skips; skip reserves | Cooldowns cleared or waited out; skips kept in reserve | pending |
| autoNomad | travel | advanced | Paid travel | Travel paid with coins, Rubies or the feather | pending |
| autoNomad | rbc-trial | advanced | Robber baron castle trial and its target | Trial on or off, and the target | pending |
| autoInvasion | setup | essentials | Source castle; attack setup | — | pending |
| autoInvasion | event | essentials | Difficulties; stop at score; stop before event end | — | pending |
| autoInvasion | fortify | essentials | Fortify each target and its currency (Rubies never default) | — | pending |
| autoInvasion | limits | essentials | Daily attack limit | — | pending |
| autoInvasion | travel | advanced | Paid travel | Travel payment | pending |
| autoKhan | setup | essentials | Attack castle; main castle; attack and main-castle defense setups; defense reapplication note | — | pending |
| autoKhan | policy | essentials | Lock automatic attacks; trigger rage; replenish defense tools (coin packages only); open-gate protection | — | pending |
| autoKhan | skips | essentials | Required cooldown skipping; skip reserves; stop before event end | — | pending |
| autoKhan | limits | essentials | Daily attack limit | — | pending |
| autoKhan | stop-limits | advanced | Rage chain limit; Rage points booster requirement; Nomad points stop | Rage chain limit; booster required or not; Nomad points stop (Ruby cost at the limit) | pending |
| autoKhan | travel | advanced | Paid travel | Travel payment | pending |
| autoBeriWorld | attack | essentials | Source castle; Gallantry booster requirement; tower attack setup | — | pending |
| autoBeriWorld | limits | essentials | Daily attack limit | — | pending |
| autoBeriWorld | building | essentials | Builder lane; built-in or captured camp target; capture tools; camp resource reserves | — | pending |
| autoBeriWorld | building-options | advanced | Construction time skips and reserves; premium costs; demolition | Premium allowed or not; demolition; time skips and reserve | pending |
| autoBeriWorld | attack-options | advanced | Attack check interval; armorer tool minimums (coins); troop transport skips; transfer check interval and minimum | Check interval; tool types bought with coins; transfer skips | pending |
| autoBeriWorld | travel | advanced | Paid travel | Travel payment | pending |
| autoTowers | castles | essentials | Castle on or off; tower troop and stock per castle; per-castle scope line | — | pending |
| autoTowers | limits | essentials | Daily attack limit | — | pending |
| autoTowers | advisor | advanced | Advisor chains; activation with a token; daily time-skip cap | Advisor on or off, token activation, daily time skips | pending |
| autoTowers | scan | advanced | Map scan interval; radius and maiden-supported filter per castle | Scan interval; radius range; maiden-only castles | pending |
| autoTowers | travel | advanced | Paid travel | Travel payment | pending |
| autoFortress | kingdoms | essentials | Kingdom targets, main castles and Direwolf stock | — | pending |
| autoFortress | supply | essentials | Direwolves per shop session (spend); Khan tablet reserve; price tiers | — | pending |
| autoFortress | limits | essentials | Daily attack limit; cache and ready-time note | — | pending |
| autoFortress | transfer-skips | advanced | Time skips for Direwolf transfers | Transfers use time skips or not | pending |
| autoFortress | travel | advanced | March speed advice; paid travel | Travel payment | pending |
| autoStorm | castle | essentials | Storm castle access and unlock plan | — | pending |
| autoStorm | targets | essentials | Forts and resource islands: levels, resources, sizes, attack setups, island defense units | — | pending |
| autoStorm | donors | essentials | Troop import and donor castles | — | pending |
| autoStorm | import-tuning | advanced | Minimum troops kept after launch; troop-cap preview | Import on or off and the kept minimum | pending |
| autoStorm | shop | essentials | Aquamarine reserve and prioritized purchase goals | — | pending |
| autoStorm | limits | essentials | Daily attack limit | — | pending |
| autoStorm | construction | advanced | Blueprints and capture; optional decoration; construction flags; castle and donor reserves; time-skip reserves; harbor | Target or combat only; premium; demolition; resource shipments; time skips; harbor level; decoration | pending |
| autoStorm | priority | advanced | Attack target priority | Number of ordered target types | pending |
| autoStorm | travel | advanced | Paid travel | Travel payment | pending |
| autoStorm | timing | advanced | Policy check interval; map refresh; map coverage | Check interval; map refresh | pending |
| autoFoodBalance | reserves | essentials | Donor reserve; coin reserve; kingdom transport; castle food table | — | pending |
| autoFoodBalance | timing | advanced | Polling interval; minimum kingdom shipment; minimum Storm delivery | Check interval; shipment minimums | pending |
| autoFoodBalance | transport-skips | advanced | Transport time skips and reserves | Time skips on or off; skips kept | pending |
| autoFoodBalance | travel | advanced | Market barrow travel | Travel payment | pending |
| autoStation | evacuation | essentials | Evacuate at N minutes; recall when clear | — | pending |
| autoStation | reserves | essentials | Troops left to defend per castle, with stock | — | pending |
| autoStation | filters | advanced | Minimum Bird protection days on target; open-gate fallback | Protection filter; open-gate fallback | pending |
| autoBird | targets | essentials | Minimum Bird protection days on target | — | pending |
| autoBird | castles | essentials | Active preset; kept units per castle, with stock | — | pending |
| autoBird | timing | advanced | Random delay range; minimum group size | Delay range; minimum per send | pending |
| autoRecruit | plan | essentials | Shared or per-castle mode; enabled castles; units and schedules; Glory title fallback | — | pending |
| autoRecruit | timing | advanced | Queue check interval | Check interval | pending |
| autoTool | plan | essentials | Shared or per-castle mode; enabled castles; tools and schedules | — | pending |
| autoTool | timing | advanced | Queue check interval | Check interval | pending |
| autoHospital | schedule | essentials | Scan windows (weekly schedule) | — | pending |
| autoHospital | timing | advanced | Queue check interval | Check interval | pending |
| autoTCI | items | essentials | Construction items and level floor and ceiling per castle | — | pending |
| autoTCI | presets | advanced | Saved item presets (apply, save as new, delete) | Number of presets; applied preset | pending |
| autoSceatRes | reserves | essentials | Coin and ruby reserves; logistics; Storm buffer; Ruby recipes; Ruby overflow skip | — | pending |
| autoSceatRes | crafting | essentials | Recipe cycles per building; queue slot rentals (coins) | — | pending |
| autoSceatRes | timing | advanced | Check interval; minimum shipment; overflow threshold | Check interval; shipment minimum; overflow threshold | pending |
| autoSceatRes | transport-skips | advanced | Transport time skips and reserves | Time skips on or off; skips kept | pending |
| autoSceatRes | storage | advanced | Additional storage nodes (read only) | Number of storage nodes | pending |
| autoBooster | purchase | essentials | Daily fortress-speed boost (2,500 Rubies); ruby reserve | — | pending |
| autoBooster | evidence | advanced | Purchase evidence; dispatch safeguards | Latest evidence recorded or not; checks run before every purchase | pending |
| autoBuyer | limits | essentials | Main castle; ruby reserve; allow Ruby-priced packages | — | pending |
| autoBuyer | goals | essentials | Shop limits; specialist floors; feast upkeep | — | pending |
| autoBuyer | timing | advanced | Check interval | Check interval | pending |
| autoAdvisor | access | essentials | Advisor access, tokens and explicit activation; live run | — | pending |
| autoAdvisor | setup | essentials | Source castle; attack preset; automated difficulties | — | pending |
| autoAdvisor | gates | essentials | Coin and Ruby cost per attack; coin, Ruby, feather and time-skip reserves | — | pending |
| autoAdvisor | run-sizing | advanced | Maximum attacks; stop before event end | Attacks per run; stop time | pending |
| autoAdvisor | travel | advanced | Paid travel | Travel payment | pending |
| autoEquipmentCleanup | schedule | essentials | Cleanup schedule | — | pending |
| autoEquipmentCleanup | timing | advanced | Poll interval | Check interval | pending |

## Proposed deltas from the brief matrix (for Maya)

1. **Daily attack limits are Essentials everywhere,** including Invasion, where the brief listed the limit as Advanced. The acceptance criteria put operational limits in Essentials, and one rule for every feature is easier to learn.
2. **Khan:** "Replenish defense tools" (purchase policy) and the cooldown-skip reserves stay in Essentials, next to the required cooldown-skip switch. The rage chain limit, the Rage points booster requirement and the Nomad points stop move to Advanced, and their summary states each one, including the Ruby cost at the Nomad points limit.
3. **Towers:** Advisor mode, including its token activation and daily time-skip cap, is one Advanced group. Its summary always states whether tokens and time skips can be used. Radius and the maiden-supported filter move from the castle cards into the "Map scan and target filters" group. Each castle card keeps a one-line scope summary.
4. **Fortress:** the Khan tablet reserve stays in Essentials, next to the Direwolf purchase ceiling it protects. The brief listed it as Advanced; the acceptance criteria put reserves in Essentials.
5. **Storm:** the Aquamarine purchase goals stay in Essentials with the reserve, because they are the spending policy. Construction, including its reserves, is Advanced, and its summary states premium, demolition, resource shipments, time skips, harbor and decoration. Import sizing is a separate Advanced group, so that choosing donors stays in Essentials.
6. **Bird:** the minimum protection days on the target are Essentials, as in the brief. Preset management stays in Essentials because it is the same control that selects the active preset.
7. **TCI:** there is no interval setting to collapse, so the Advanced group is the saved item presets.
8. **Equipment Cleanup:** its editor has only a schedule and a poll interval. Categories and keep thresholds, which the brief listed, belong to the Equipment view, not this editor.
9. **Recruit and Tool** share one modal with two entry points. Their rows are identical apart from the Glory title fallback, which exists only for Recruit.
10. **Buyer** keeps its existing Run switch instead of the run line (see above).

## Intentional platform differences

The desktop and hosted clients share the disclosure modules byte-for-byte: placement, summaries, fix targets, the section component and the run line. Their modal files differ only where they already differed before CIT-17. The hosted client uses compact editors in narrow layouts.
