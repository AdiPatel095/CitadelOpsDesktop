# Guide locale workflow

Run these commands from `Client/`; the web client has the same `npm run guide:locales` command from its root. The script reads the configured locale list and derives stable leaf paths from the English guide source and `guideLocales/en.json` for Auto Towers, Auto Bird, Auto Station, or Auto Fortress. It never calls a translation service.

1. `npm run --silent guide:locales -- export autoFortress > fortress-en.json` writes the current flat English template with a source hash. Author one locale by replacing every value in `entries` with a reviewed translation; keep keys, feature and hash unchanged.
2. `npm run --silent guide:locales -- import autoFortress fr fortress-fr.json --mirror /absolute/path/to/CitadelOpsFrontend` validates the whole feature before writing both clients. It rejects missing, extra, blank and unchanged-English packs, stale source hashes, or unrelated mirror differences. Repeat the same import safely after interruption; it preserves other guide content.
3. `npm run --silent guide:locales -- status autoFortress --mirror /absolute/path/to/CitadelOpsFrontend` reports each configured locale as missing, stale, or complete. A saved pack without matching source-bound progress is stale. `check` uses the same report and exits nonzero until all locales are complete.
4. After an English change, run `npm run --silent guide:locales -- provenance autoFortress --mirror /absolute/path/to/CitadelOpsFrontend` to pin the English bytes; re-export and review any locale now marked stale. Imports update per-locale pack hashes in `guideLocales/provenance.json` and source-bound progress in `guideLocales/progress.json`.

The English source and pack must mirror step and item IDs, titles, descriptions, recommendations, and image text. The public guide remains English in no-JS output; client-side locale selection uses completed locale packs. This workflow covers guide content only, not the whole application catalog or native-speaker review.
