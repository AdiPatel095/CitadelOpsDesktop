# Synthetic localization UI fixture

Run from Client: `npx vite --config tests/localization-browser/vite.config.ts`.
Open http://127.0.0.1:41882/. All runtime API requests return503 except two synthetic telemetry GET responses; no real runtime is proxied. Confirmation callbacks capture JSON locally and never dispatch game actions.

The fixture uses production LocaleProvider, equipment modals, Alerts and LoggerDock. Alt+1/2/3 switches English/German/Arabic even while a modal remains open. Native radio controls provide a browser behavior reference. Fixture instructions themselves are test controls, not production localization scope.

Verified during this checkpoint:
- A persistent notification changes English→German→Arabic without republishing.
- An open sell confirmation keeps Relic2.0 selected and threshold17 across German→Arabic→German. Captured payload remains `{"category":"relic2_equipment","sellLookItems":false,"sellPost2026":false,"keepStars":17}`.
- Equal-width ToggleGroup highlight initially stayed at the previous LTR position after switching direction. Group-local reactive direction fixes it. Home/End select first/last logical options in both directions; horizontal arrows move visually, matching Chromium native RTL radio behavior. Up/Down retain logical order.
- Arabic rich swap text keeps `Player {0} <b>literal</b>` literal and isolated. Numeric fraction1/5 needed explicit LTR isolation; it now displays in the correct order.
- Open activity rows change Arabic→German, including timestamp presentation. Synthetic original-only history remains unchanged and visibly marked untranslated.
- Screenshots inspected at1280×720: Arabic sell and swap modals, highlight before/after and fraction before/after. Reproduce with the controls above; no complete responsive or all-route RTL acceptance is claimed.

Official label requests deliberately fail, so official-only buttons retain honest English fallback in this fixture. Full dictionaries, remaining UI text, shared accessibility labels and broad RTL layout migration remain unfinished.
