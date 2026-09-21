# Battle presentation translations

The 58 `battle.*` entries in each of 25 non-English packs are directly model-authored. Native-speaker review remains pending. This group does not represent complete BattleStats screen coverage; existing source-assigned labels and dynamic data paths remain tracked by the strict inventory.

Terminology was checked against the official v4357 dictionaries: `battlelog`, `tools`, `dialog_battleLogDetail_courtyard`, `dialog_battleLog_attacker`, `dialog_battleLog_defender`, and `dialog_combatAnimation_wave`. Contextual inflection is retained in whole sentences. In particular, Dutch tools use Tuigen, Swedish courtyard uses Gårdsplan, Italian attacker uses Assalitore, and Japanese waves use 波状攻撃. The support phase means reinforcing troops, not customer support.

Canonical authored files are copied through `scripts/import-authored-ui-module.mjs`; `module-authorship.json` records each source hash. The all-locale authored-group test checks catalog equality, source hashes, literal parameter handling and rendering across every select branch and representative plural counts. The overall completeness command remains independent and fails while any required source or translation coverage is missing.
