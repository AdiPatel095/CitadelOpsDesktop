# Desktop localization coverage ledger

Status: partial foundation, not a translated release. Baseline: e77ed022c2f926ae3509527fdabf54dca9e22586. 31 shell/shared-control messages have model-authored translations in all 25 non-English catalogs. Human linguistic review is pending. This small authored set is not full interface coverage. The full visible-text conversion remains open.

## Source inventory

Run `node scripts/inventory-localization.mjs` from Client. `source-inventory.json` contains conservative AST candidates with source references and sink categories. It includes branch values and technical strings in rendered expressions, so counts are not translation counts. Exclusions retain location/reason without duplicating technical text. Missing source candidates, dynamic string composition and false exclusions require semantic review before coverage certification. No fallback English counts as translated.

## Surface routes and remaining work

| Surface | Source ownership | Required message route | Current gap |
| --- | --- | --- | --- |
| Shell/navigation/header/mobile controls | App, Header, Sidebar, ui components | Explicit typed custom keys for JSX, aria and titles | Navigation/sidebar/theme converted; header status literals remain |
| Settings/forms/confirmation/modals | settings/components, SettingsView | Explicit keys; interpolate values once; preserve user input | Labels, validation, help and templates remain |
| Automation/event views | views, events, attackAnalytics | Custom descriptors; official `localizationKey` first for event names | EventsView currently overrides event IDs with English names |
| Units/buildings/decorations/equipment/gems/effects/currencies | MetadataContext, equipment components | Official keys via per-request viewer locale; custom generic fallback descriptors | Metadata locale invalidation implemented; display fallback provenance needs consumer presentation |
| Dynamic server errors/operation notifications | CitadelClient, ApiContext, OperationNotifications | Adjacent `messageDescriptor` / `detailDescriptor`; legacy English fallback marked uncovered | Existing notification conversion still English; server contract additive work pending |
| Activity dock/channel names | LoggerDock | Structured server message descriptors, preserve timestamps/identities | Current tail is raw text parsed by regex; cannot reconstruct localization semantics safely |
| Event/world history and rankings | WorldEventHistory, FeatureEventHistory, ranking components | Official event keys + typed summary descriptors; Intl values | Server-built names and historic English strings remain |
| Exports/copy | LoggerDock, SettingsView, SettingsTransfer | Localized display copy; settings JSON keys and values remain machine/user data | Activity copy should use same localized descriptor rendering; settings filenames are product identifiers |
| Empty/loading/error/status states | All views/components | Explicit custom keys and ICU plural/select | Mostly uncovered |
| Patch notes/support | config/PatchNotes, PatchNotesView, SupportView | Authored custom catalog content | All historic prose remains English |
| Date/number/duration/count formatters | views, components, equipment, analytics | Intl formatter using viewer locale and ICU | Provider formatter available; legacy formatters remain |
| User-created castle/player/alliance/preset/account names | Dynamic payloads and editable state | Verbatim isolated text; never translate | Preserve during conversion |
| Protocol IDs, settings keys, operation IDs, diagnostic console | Contracts and technical code | Excluded from translation | Console exclusion applies only to private diagnostics, not displayed LoggerDock content |

## Completion gate

Every visible sink needs a reviewed key or a specific exclusion. Each supported custom catalog must have exactly the English keys and matching recursively parsed ICU argument sets. Official game positional placeholders are separate from ICU. Runtime unknown descriptors, unavailable official keys and retained English history must remain visible as coverage gaps. Check all 26 locales, long text, Arabic direction, locale changes during in-flight fetches, offline fallback, account switching, exports and accessibility. Existing physical left/right CSS and all non-header responsive surfaces still need RTL audit.
