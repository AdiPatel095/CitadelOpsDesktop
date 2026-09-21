# Explicit TypeScript verification

The existing `build` command invokes bare `tsc` against the root configuration, whose `files` list is empty and whose app/node configurations are references. That invocation does not typecheck the referenced app. Vite builds still run successfully. Use `npm run typecheck:app` and `npm run typecheck:node` explicitly.

Baseline `e77ed022c2f926ae3509527fdabf54dca9e22586` was exported to an isolated temporary directory and checked with the same installed TypeScript compiler and dependencies as this branch. It has 43 app diagnostics. Before correction, localization added six diagnostics: three equipment notification calls still supplied strings, a removed event label was referenced, its replacement descriptor was unused, and LocalizedText accepted booleans outside the own-message parameter type. These are corrected; the current diagnostic multiset matches all 43 baseline errors after removing line/column offsets. Node-project typechecking passes.

`localization-type-contracts.test.mjs` separately requires zero app diagnostics in the new i18n modules and EquipmentView action callsites. It is a focused regression test, not an assertion that the entire app typecheck passes. Overall app typechecking remains failed because of the following baseline diagnostics.

| Baseline file | Diagnostics |
| --- | ---: |
| `src/Movement/components/CommanderRequirementModal.tsx` | 6 |
| `src/Movement/types/MovementState.ts` | 1 |
| `src/Rift/context/RiftMapContext.tsx` | 1 |
| `src/allianceTargets/components/AllianceTargetsView.tsx` | 2 |
| `src/battleStats/components/BattleStatsView.tsx` | 6 |
| `src/components/AttackSetupModal.tsx` | 2 |
| `src/components/CastleFocusSwitcher.tsx` | 1 |
| `src/components/DefensePresetEditor.tsx` | 1 |
| `src/components/ui/Badge.tsx` | 1 |
| `src/components/ui/Button.tsx` | 1 |
| `src/components/ui/Card.tsx` | 1 |
| `src/components/ui/ChoiceChipGroup.tsx` | 1 |
| `src/components/ui/EmptyState.tsx` | 1 |
| `src/components/ui/Input.tsx` | 1 |
| `src/context/MetadataContext.tsx` | 3 |
| `src/dashboard/components/EventScoreCard.tsx` | 3 |
| `src/playerTracker/components/PlayerTrackerView.tsx` | 1 |
| `src/settings/components/AutoInvasionSettingsModal.tsx` | 1 |
| `src/settings/components/AutoKhanSettingsModal.tsx` | 1 |
| `src/settings/components/AutoNomadSettingsModal.tsx` | 1 |
| `src/settings/components/AutoStormSettingsModal.tsx` | 1 |
| `src/settings/components/HorseTravelBoostSelect.tsx` | 1 |
| `src/settings/components/WeeklyScheduler.tsx` | 3 |
| `src/views/SettingsView.tsx` | 2 |
