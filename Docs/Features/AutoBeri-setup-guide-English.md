# Auto Beri setup guide — English source draft

Auto Beri supplies your owned Berimond camp from a Great Empire castle, attacks towers with a saved formation, and can maintain tools and build from confirmed loot. Recommended choices describe regular use; they do not change saved settings. The illustrations for this guide should be code-rendered examples of the current settings, since older screenshots show the removed manual camp ID, transfer troop, and KUT CID controls.

## 1. Choose your attack and supply source

- **Open settings:** On the Automation page, open the Auto Beri World card settings.
- **Attack preset:** Choose a saved Berimond attack formation; Auto Beri uses its troop mix for transfers as well as tower attacks. Recommended: create and select your usual Berimond preset first.
- **Source castle:** Choose the Great Empire castle or outpost supplying those troops. Recommended: select the castle that holds your attack army.
- **Horse travel boost:** Choose feather, coin, or ruby travel for tower attacks. Recommended: Travel Feather for fast travel without selecting ruby travel.
- **Attack check interval:** Set how often Auto Beri checks for another tower attack. Recommended: keep the usual 30-second interval unless you need fewer checks.
- **Daily normal attack limit:** Stop new Auto Beri tower attacks at the account-wide daily normal-attack count; 0 removes this added limit. Recommended: 0 unless you use an account-wide cap.

Illustrative panel: saved attack preset, Great Empire donor castle, Travel Feather, attack check, and daily limit.

## 2. Set proportional troop transfers

- **Owned Berimond camp:** Auto Beri finds your owned kingdom-10 camp automatically. There is no camp ID to enter.
- **Preset troop mix:** Auto Beri sums the troops in the selected preset's waves and courtyard, including repeated unit types, and replenishes one underrepresented type at a time toward that mix. Only troops that the official catalog says consume Food can transfer; an unsupported troop or unresolved troop family pauses transfers with a reason.
- **Check interval:** Set how often Auto Beri refreshes free transfer capacity and both castle inventories. Recommended: keep the usual 30-second interval.
- **Minimum free capacity:** Begin transfers only when reported free capacity reaches this threshold; an individual proportional shipment can be smaller. Recommended: use the smallest free-capacity threshold that suits your usual transfer pace.
- **Use troop transport time skips:** Apply the selected skip to a confirmed transfer while it is still travelling. Recommended: off unless faster camp resupply is worth the saved skip items.
- **Troop transport skip:** Choose the duration of the item used when transport skipping is on. Recommended: reserve longer skips for other tasks if you need them.

Auto Beri waits for an in-flight transfer to arrive before recalculating the camp mix; if the donor has some of the needed type, it sends that smaller amount, then refreshes. If the donor has none of the needed type, it waits instead of filling the camp with a different type. KUT sends its fixed `CID=-1` internally; there is no wire ID to enter.

Illustrative panel: a fictional 2:1 saved troop mix, current camp stock, reported free capacity, and the next single-type shipment; show the numbers as examples, not saved defaults.

## 3. Set tower attack readiness

- **Only run with a Gallantry booster:** Require an active Gallantry booster before transfers, tool purchases, camp setup, tower attacks, or construction proceed. Recommended: on if you want every Auto Beri lane gated by the booster.
- **Attack preset loadout:** Review the selected preset's waves, troops, and tools before enabling attacks. Recommended: keep enough of each selected troop and tool at the donor and camp for your normal attack pace.

Illustrative panel: booster gate status and preset loadout, with no implied live booster or customer data.

## 4. Maintain camp tools

- **Scaling ladders minimum:** Replenish this many stationed ladders at the camp using eligible coin offers; 0 leaves them unmanaged. Recommended: match the number your usual preset and attack pace need.
- **Battering rams minimum:** Replenish this many stationed rams under the same rule. Recommended: match your usual preset's use.
- **Mantlets minimum:** Replenish this many stationed mantlets under the same rule. Recommended: match your usual preset's use.

Illustrative panel: the three tool minimums and a clearly fictional camp inventory.

## 5. Choose optional camp construction

- **Auto Beri Builder lane:** Build and upgrade the built-in exact camp target or an active captured target from confirmed returned loot. Recommended: off until you have reviewed the target and spending permissions.
- **Stable target:** Choose the desired Faction Stable level from 1 through 5 for the built-in target. Recommended: choose the level your current event resources can support.
- **Optional custom camp target:** Capture, preview, save, and activate a layout only if you want it to replace the built-in target. Recommended: keep the built-in target unless you have inspected a custom layout.
- **Use construction time skips:** Advance a confirmed build timer while keeping the selected skip reserves. Recommended: off unless faster construction is worth those items.
- **Allow premium costs:** Permit eligible construction steps that spend premium currency. Recommended: off unless you intentionally want premium construction.
- **Allow demolition:** Permit exact-target cleanup of unmanaged buildings that cannot be moved or stored. Recommended: off unless you have reviewed the target's removals.
- **Camp resource reserves:** Keep the chosen amounts of Berimond wood and stone unavailable to the builder. Recommended: reserve what you need for other camp work.
- **Construction time-skip reserves:** Keep the chosen number of each skip item unavailable to the builder. Recommended: reserve what you need for other activities.

Illustrative panel: built-in target, stable target, construction permissions, and reserve groups.

## 6. Save and enable

- **Save settings:** Save the preset, donor, transfer pacing, attack limits, tools, and optional build choices before enabling the automation.
- **Automation switch:** Turn on Auto Beri World from its Automation card after confirming your camp, donor troops, preset, and optional spending settings. Recommended: on after setup.
- **Calendar:** Leave the schedule off for continuous operation, or set your usual hours and save the schedule if you need a time window. Recommended: off for ongoing event operation.

Illustrative panel: saved setup checklist, switch, and optional schedule, without showing a live account.
