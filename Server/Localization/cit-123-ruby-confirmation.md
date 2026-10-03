# CIT-123 English message provenance

Source: Daniel's CIT-123 plan (2026-10-03, Addenda 1 and 2) and Product/Opcode rejection fixes 2026-10-03 decision, story G, Option 2. English source only; locale translations belong to the later batched story.

| Key | Producer | Meaning |
| --- | --- | --- |
| `server.intent.ruby_confirmation_required` | `Server/Intent/LaneSafety.go` | Scoped AGB/440 quote; the purchase needs confirmation in the game. |
| `server.automation.auto_booster_ruby_setting_unknown` | `Server/Automation/AutoBoosterPolicy.go`, shared by `Server/App/AutoBoosterIntents.go` | The current-session game confirmation setting is unavailable. |
| `server.automation.auto_booster_ruby_confirmation_required` | Same shared notice producer | The game threshold blocks the fixed 2,500-ruby boost; `threshold` is the validated setting amount. |
| `server.automation.auto_booster_ruby_confirmation_hold` | Same shared notice producer | A prior quote holds this occurrence until a newer permissive setting observation. |

All messages use descriptors and new keys. No translations or existing English strings are changed. Quotes never authorize spending or replace the saved game setting.
