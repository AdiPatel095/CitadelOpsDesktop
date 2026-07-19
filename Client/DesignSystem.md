# CitadelOps frontend design system

Status: living product and engineering standard
Last updated: 2026-07-15
Applies to: `Client/src` desktop interface

This document turns the recurring patterns in the current CitadelOps frontend and the findings in
[`CitadelOpsUiRedesignResearch.md`](../CitadelOpsUiRedesignResearch.md) into reusable design and
implementation rules. It is the source of truth for new frontend work and for the incremental
rewrite of existing screens.

The goal is not a visual reset. CitadelOps should continue to feel like the current application:
dark, game-adjacent, information-dense, and recognizably “liquid.” The rewrite makes that character
more deliberate by reducing decorative competition, separating action color from success, exposing
runtime state, and consolidating repeated code.

## 1. Product model

CitadelOps is an operations console, not a generic analytics dashboard. Each screen should answer,
in order:

1. What is the current state and how fresh is it?
2. What needs attention?
3. What is happening now?
4. What happens next?
5. What action can the user take?
6. Where is the execution evidence?

The global hierarchy is:

> State first. Action second. Execution trace third. Raw diagnostics last.

### Experience principles

- Preserve game vocabulary and game art because they support recognition.
- Keep scope explicit: empire, castle, automation, operation, and resource are never implied.
- Treat enabled, waiting, running, blocked, failed, paused, and complete as distinct states.
- Treat live, delayed, stale, offline, and unknown as distinct freshness states.
- Show explanations in the interface; never make a hover title the only source of operational truth.
- Accumulate routine activity and interrupt only for actionable exceptions.
- Use color, icon or shape, and text together for state.
- Prefer structured lists, sections, and split panes over a wall of interchangeable cards.
- Keep raw protocol data in diagnostics and explicit evidence views.

## 2. Continuity contract

The following traits belong to the current product and should remain recognizable during the
rewrite:

- Deep navy and graphite canvas.
- A compact fixed shell with header, navigation, task canvas, and optional diagnostics dock.
- Restrained translucent shell layers.
- Rounded but not uniformly pill-shaped controls.
- Lucide line icons.
- Official game imagery for troops, tools, construction items, and resources.
- Dense expert workspaces with lighter disclosure for first-use and settings flows.
- Light and dark themes.

The following traits are intentionally changing:

- Emerald is no longer both the primary action color and the success color.
- Main reading surfaces become more opaque; glass is reserved for shell and transient layers.
- Eighteen-pixel global rounding becomes a purposeful 4/8/12-pixel scale.
- Important labels do not rely on 9–11-pixel text.
- Runtime explanation moves out of native tooltips and into visible status rows.
- Large workspaces and editors load on demand.

## 3. Design-token architecture

Components consume semantic tokens. Raw values belong only in theme definitions or data-visualization
schemes.

### Token layers

| Layer | Purpose | Example |
| --- | --- | --- |
| Primitive | Perceptual scale | violet step, neutral step, spacing unit |
| Semantic | Product meaning | `--surface-canvas`, `--action-primary`, `--status-warning` |
| Component | Exceptional local contract | automation blocked marker, log error border |

The current CSS keeps legacy aliases such as `--bg-app` and `--primary` while screens migrate. New
components should prefer the semantic names when writing plain CSS. Tailwind utilities such as
`bg-bg-card` remain supported through the alias layer.

### Color roles

Color has separate jobs: structure, action/brand, semantic state, interaction, and data encoding.
One hue must not perform multiple jobs on the same screen.

#### Dark theme

| Role | Token | Value |
| --- | --- | --- |
| Canvas | `--surface-canvas` | `#07111F` |
| Surface | `--surface-default` | `#0D1B2A` |
| Subtle/raised surface | `--surface-subtle` | `#13263A` |
| Subtle border | `--border-subtle` | `#29425D` |
| Control border | `--border-control` | `#526D89` |
| Strong text | `--content-strong` | `#F4F8FC` |
| Muted text | `--content-muted` | `#9DB0C3` |
| Primary action | `--action-primary` | `#9B8CFF` |
| Primary hover | `--action-primary-hover` | `#B5AAFF` |
| Success | `--status-success` | `#4CD58A` |
| Warning | `--status-warning` | `#F4BE53` |
| Danger | `--status-danger` | `#FF6B76` |
| Information | `--status-info` | `#48C6E8` |
| Inactive/unknown | `--status-inactive` | `#8292A6` |

#### Light theme

| Role | Token | Value |
| --- | --- | --- |
| Canvas | `--surface-canvas` | `#F4F7FA` |
| Surface | `--surface-default` | `#FFFFFF` |
| Subtle surface | `--surface-subtle` | `#EAF0F5` |
| Subtle border | `--border-subtle` | `#C9D5E1` |
| Control border | `--border-control` | `#8292A4` |
| Strong text | `--content-strong` | `#102033` |
| Muted text | `--content-muted` | `#52687D` |
| Primary action | `--action-primary` | `#5A4BCB` |
| Primary hover | `--action-primary-hover` | `#493BAC` |
| Success | `--status-success` | `#147A46` |
| Warning | `--status-warning` | `#8B5800` |
| Danger | `--status-danger` | `#B42338` |
| Information | `--status-info` | `#007B99` |
| Inactive/unknown | `--status-inactive` | `#6F8194` |

Command Violet is a product hypothesis from the redesign research, not a completed identity decision.
It is adopted as the interface action color because it cleanly separates user action from confirmed
success while retaining the saturated accent and ambient depth of the current UI.

### Semantic color rules

- Violet: primary action, selected navigation, focus, and user-controlled emphasis.
- Green: confirmed success or healthy state only.
- Amber: waiting with consequence, caution, or near-term attention.
- Red/coral: failure, destructive action, or critical risk.
- Cyan: neutral information, live connection detail, or correlation.
- Slate: inactive, disabled, stale support information, or unknown.
- Data series use a separate qualitative/sequential/diverging palette and do not inherit the action
  color automatically.
- Healthy screens should be 80–90% neutral, 5–10% action color, and less than 5% semantic status
  color.

## 4. Type, spacing, shape, and elevation

### Typography

Use the system UI stack already configured in `index.css`.

| Size | Use |
| ---: | --- |
| 12 px | Rare metadata; never essential state or instructions |
| 14 px | Dense controls, table cells, supporting copy |
| 16 px | Primary body, forms, and comfortable density |
| 20 px | Section heading |
| 24–32 px | Page title or primary metric |

- Use tabular numerals for resources, timestamps, coordinates, counters, and timers.
- Use monospace only for IDs, protocol data, code, and aligned numeric entry.
- Use sentence case for controls and headings.
- Short eyebrow labels may use uppercase and deliberate tracking.
- Do not globally suppress letter spacing.
- Keep reading text around 65–75 characters wide.

### Spacing

Use a 4-pixel micro unit and an 8-pixel macro grid. Common values are 4, 8, 12, 16, 24, 32, and
48 pixels. A repeated layout that needs an unexplained one-off value is a candidate for a shared
component.

### Radius

| Token | Value | Use |
| --- | ---: | --- |
| `--radius-control` | 8 px | Buttons, inputs, compact selections |
| `--radius-surface` | 12 px | Panels, dialogs, primary surfaces |
| `--radius-pill` | Full | Badges, status pills, true segmented controls |

Four-pixel rounding is allowed for dense table selections. Sixteen pixels or more is reserved for
rare expressive frames. Circles and pills are semantic shapes, not the default for every control.
Radius is explicit at the component or utility level; global CSS must not flatten every radius class
or infer rounding merely because an element has a border or background.

### Elevation and transparency

Use only three levels:

1. Canvas and inline groups: no shadow.
2. Raised task surface: one compact shadow or boundary.
3. Transient layer: dialog, popover, or shell glass.

Do not stack blur on a translucent child inside a blurred parent. Dense text, tables, and logs use
opaque or near-opaque surfaces. Reduced-transparency and forced-color mappings must remove glass
without removing boundaries or focus.

### Frosted-glass composition

Frosted glass is a product texture, not a synonym for every panel. It belongs on the fixed shell,
transient layers, compact controls, and bounded picker workspaces. Reading-heavy content remains
opaque or nearly opaque inside that frame.

- Theme color and depth come from `--glass-nav`, `--glass-panel`, `--glass-panel-strong`,
  `--glass-control`, `--glass-highlight`, `--glass-lowlight`, and the `--glass-shadow-*` tokens.
- Compact switches and connected selectors consume `--frost-control-background`,
  `--frost-control-shadow`, and `--frost-control-backdrop`. Their selected surface consumes
  `--frost-active-background`.
- Do not copy those gradients, inset highlights, or blur values into a new selector. Extend the
  shared token only when the visual role is genuinely the same in both themes.
- A frosted parent gets at most one backdrop filter. Children may use an opaque or color-mixed fill,
  but must not add another large-area blur.
- Frost keeps a visible border in both themes. Blur and transparency never carry structure alone.

## 5. Motion

- Button and toggle feedback: 80–120 ms.
- Small reveal: 120–180 ms.
- Drawer or inspector: 180–240 ms.
- Toast: 200–280 ms.
- Major workspace transition: no more than 300 ms.
- Animate opacity and transform, not layout or large blur regions.
- No ambient pulse for healthy or running state.
- No bounce or elastic motion in operational flows.
- Respect `prefers-reduced-motion`.

## 6. Shell and responsive model

| Width | Layout |
| --- | --- |
| Wide | Navigation, main task canvas, optional inspector/dock |
| Medium | Compact navigation, main canvas, inspector as drawer |
| Narrow | One task surface; detail becomes a route or sheet |

Navigation remains labeled at normal desktop widths. Current-location state uses a native control
and `aria-current="page"`. Selection, filters, drafts, and useful scroll state should survive pane
collapse where practical.

Density has two modes in the target system:

- Comfortable: onboarding, forms, and settings.
- Compact: tables, queues, logs, and expert operations.

Density changes padding and row height, never font legibility, accessible target availability, or
information semantics.

## 7. Reusable component contracts

Every shared component owns purpose, anatomy, states, keyboard behavior, accessible naming,
responsive behavior, theme behavior, performance cautions, and migration guidance.

### Foundation inventory

| Component | Purpose | Current status |
| --- | --- | --- |
| `PageHeader` | One page title, description, icon, metadata, and actions | Available and used by routed workspaces |
| `ModalTitle` | Consistent icon, title, description, and trailing state | Available and used across editors and dialogs |
| `SettingsModal` | Settings title plus consistent Cancel/Save contract | Available and used by automation and schedule settings |
| `SectionCard` | Titled section with optional description and actions | Available across dashboard, settings, collection, analytics, and Rift sections |
| `StatusIndicator` | Redundant shape + label + optional detail | Available; first used for automation state |
| `Button` | Primary, secondary, ghost, danger, outline, glass actions | Available; conventional 12 px control radius |
| `Card` | Self-contained selectable or independently meaningful object | Available; reduce use for simple grouping |
| `Input` | Text/number entry with icons and inline error | Available; add description/error association |
| `Select` | Select-only/searchable choice | Available; resize cleanup and semantics improved; full arrow-key model remains |
| `Switch` | Immediate binary setting | Available; requires a visible label at call site |
| `PillSelector` | Small mutually exclusive choice set | Available; labeled radiogroup with arrow-key navigation |
| `ChoiceChipGroup` | Independent compact multi-select choices | Available; used by Storm and Sceat settings |
| `Modal` | Blocking task/dialog | Focus trap, Escape, focus return, title association, nested scroll lock available |
| `EmptyState` | Shared empty/loading explanation with optional recovery action | Available across preset, picker, castle, and Rift surfaces |
| `CollectionToolbar` | Collection summary plus consistently labeled search | Available in preset libraries |
| `CatalogPickerModal` | Search, selection summary, filters, results, and confirmation shell | Available across troop, tool, and TCI pickers |
| `SettingsToggleRow` | Labeled Boolean setting with detail, tone, and disabled reason | Available across automation settings |
| `ScheduleSummaryRow` | Readable schedule state plus edit action | Available in automation settings |
| `NamedPresetControls` | Name, load, apply, save-new, and delete lifecycle | Available in Auto Bird and Auto TCI |
| `QuantityAssetTile` | Game asset, quantity badge, and accessible removal | Available in Auto Bird and Auto Station |
| `AddSlot` | Dashed add/edit affordance for a collection slot | Available in troop-reserve grids |
| `MetricTile` | Comparable labeled value with semantic tone and caption | Available across presets, battle, equipment, and event score |
| Operation/activity item | Human-readable execution evidence | Next foundation component |

### CSS composition contracts

These classes expose repeated visual decisions that are smaller than a React component. They are
deliberately narrow so layout and domain behavior stay with the caller.

| Contract | Owns | Caller still owns |
| --- | --- | --- |
| `ui-frosted-workspace` | Frosted workspace border, theme-aware fill, compact shadow, clipping, and surface radius | Dimensions, layout direction, scrolling regions, and content |
| `ui-kicker` | Compact uppercase eyebrow typography | Spacing, alignment, and the label text |
| `--font-mono-ui` | Shared UI monospace stack | Deciding whether content is actually code, an ID, or aligned numeric data |
| `--radius-pill` | True pill geometry | Choosing a pill only for selection, status, or compact metadata |
| `--frost-control-*` / `--frost-active-background` | Shared glass recipes for switches and connected selectors | Control semantics, focus, disabled state, and interaction behavior |

`CatalogPickerModal` applies `ui-frosted-workspace`; new catalog pickers inherit it through the
shared shell. `ui-kicker` is used for picker mode labels, filter labels, and scheduler option labels.
Do not create a new utility for a one-off value or use these contracts to hide domain-specific
layout.

### `PageHeader`

Use once at the top of a routed workspace. Supply a short noun title, one-sentence task description,
an optional representative icon, and only the page-level actions. Do not put filters or routine
status chips into the heading unless they summarize the whole workspace.

### `ModalTitle` and `SettingsModal`

Use for settings and editors that need an icon and optional scope/description. The modal primitive
owns the actual `h2` and accessible title association; `ModalTitle` owns visual anatomy only.
`SettingsModal` is the higher-level contract when the dialog has the normal Cancel/Save lifecycle.
It owns button order, save loading/disabled state, consistent spacing, and the `ModalTitle`; callers
retain all domain validation and persistence. Use raw `Modal` for pickers, confirmations, and
editors whose footer has a materially different action model.

### `SectionCard`

Use for a titled section whose header anatomy is title, optional description/icon, and optional
actions. It replaces repeated `Card` + `CardHeader` + `CardTitle` + `CardContent` scaffolding but
does not justify turning every layout group into a card. Choose `variant="glass"` only when matching
an existing shell/dashboard surface; dense settings and reading surfaces use the solid default.

### Collection and empty-state contracts

`CollectionToolbar` owns the recurring summary-left/search-right library toolbar. The caller supplies
summary badges and search state; the component supplies search naming, responsive stacking, and
surface treatment. `EmptyState` occupies the same content location as the collection and explains
what is missing, why, and the next action. A loading state may use the same anatomy only when the
message clearly says that work is still in progress.

### Picker contract

`CatalogPickerModal` owns the common troop/tool/TCI picker frame: title, selection count, search,
optional command/filter areas, result region, Cancel, and disabled-until-selected confirmation.
Catalog querying, virtualization, item cards, selection rules, and quantity/level editing remain in
the domain picker. New catalog pickers extend this shell rather than copy its modal and toolbar.

### Settings rows and collection items

- `SettingsToggleRow` owns the visible label, explanation, semantic warning/danger treatment,
  disabled reason, accessible switch name, and row spacing.
- `ScheduleSummaryRow` owns the readable schedule summary and its edit action; the scheduler owns the
  schedule data and editing behavior.
- `NamedPresetControls` owns the preset lifecycle UI, not serialization or validation.
- `QuantityAssetTile` owns asset/quantity/remove anatomy and keeps removal available to keyboard
  focus, not only hover.
- `AddSlot` is an add or edit target inside a bounded collection. It is not a substitute for a
  primary page action.
- `MetricTile` is for values that are meaningfully compared. Tone describes value meaning; violet
  emphasis is not used as a generic decoration.

### `StatusIndicator`

Use for lifecycle, attention, freshness, or availability. The label states the dimension value and
the optional detail explains why or what happens next. Never collapse orthogonal dimensions into a
single “good/bad” dot.

### `Card`

Use a card when the object is independently selectable, movable, comparable, or self-contained.
Prefer a section, divider, table, structured list, or banded group for ordinary layout. A card must
not exist only to put another card inside it.

### Controls

- A switch changes state immediately and must not also imply running/healthy/complete.
- A button uses an action verb and names the object.
- Destructive confirmation names the consequence.
- Disabled controls expose a visible reason nearby.
- Icon-only buttons have an accessible name and a minimum 24×24 target; frequent or destructive
  actions should approach 44×44.

### Shape is semantic

Shape is part of the control vocabulary. Components that happen to contain short text are not all
interchangeable pills.

| Shape/pattern | Meaning | Interaction | Color rule |
| --- | --- | --- | --- |
| Rounded rectangle | A command such as Save, Refresh, Add, or Cancel | Activates once | Action/neutral/danger by consequence |
| Connected pill selector | Exactly one value from a small stable set | Changes selection immediately | Violet marks the selected value |
| Disconnected choice chip | Zero or more values in a compact filter set | Each chip toggles independently | Violet means selected, never “healthy” |
| Badge/status pill | Read-only metadata or state | No click behavior | Semantic status color plus text |
| Switch | One immediate Boolean setting | Toggles on/off | Action color means on; runtime health remains separate |
| Dot | Supporting state cue only | Never stands alone | Same tone as its adjacent state label |

Ordinary buttons use the control radius, not the pill radius. This preserves the pill silhouette as
a recognizable signal for selection, status, and compact metadata.

### Choosing a selection control

Use this decision order:

1. If the user is executing a verb, use `Button`.
2. If the value is Boolean and takes effect immediately, use `Switch` with a visible label.
3. If exactly one of two to five short, stable values is selected, use `PillSelector`.
4. If zero or more values may be selected, use disconnected choice chips with `aria-pressed`.
5. If there are more than five values, labels are long, or the set is dynamic, use `Select`.
6. If the choices navigate between durable peer pages or URL-addressable panels, use tabs or
   navigation, not `PillSelector`.
7. If the value is read-only, use plain text, `Badge`, or `StatusIndicator`; never a disabled
   selector as display output.

### `PillSelector`

`PillSelector` is the reference connected segmented control. It is appropriate for local mode,
time range, scope, view, and compact single-choice filters. Examples include Commander/Castellan,
Readable/Raw frames, 24H/7D/30D/All, and Units/Tools.

Contract:

- Two to five options is the normal range. A wave selector may exceed five only inside an explicit
  horizontally scrollable region.
- Labels are short, parallel nouns or values. Do not mix nouns and commands.
- Exactly one option is selected. Selection updates immediately; the control has no separate Apply.
- The selected segment uses action violet. Success, warning, and danger colors are not selection
  colors.
- The connected track stays on one line. Use horizontal scrolling or a `Select` instead of wrapping
  it into ambiguous rows.
- `fullWidth` is for balanced choices in a bounded form or modal, not for stretching a tiny filter
  across an entire workspace.
- Icons may aid recognition but never replace the text label.
- Every instance supplies `ariaLabel`. The component exposes a radiogroup, one tab stop, arrow-key
  movement with wrapping, and Home/End navigation.
- Focus, hover, selected, and disabled are visually distinct. Changing selection must not shift the
  surrounding layout.

Reference usage:

```tsx
<PillSelector
  ariaLabel="Log view"
  value={viewMode}
  options={[
    { value: 'readable', label: 'Readable' },
    { value: 'raw', label: 'Raw frames' },
  ]}
  onChange={(value) => setViewMode(value as LogViewMode)}
  size="sm"
/>
```

Do not use `PillSelector` for multi-select filters, feature enablement, status display, destructive
choices, or a toolbar of unrelated actions.

### Choice chips, tabs, badges, and switches

`ChoiceChipGroup` renders separate rounded items because each value is independent. Every group has
an accessible name and each selected chip uses `aria-pressed="true"`; the chips do not join into a
track and do not use radiogroup semantics. Keep an explicit “All” option only when it has clear reset
semantics. Use it for short multi-select sets, not arbitrary action toolbars or dynamic collections
with long labels.

Tabs describe navigation between peer content panels. They require tablist/tab/tabpanel semantics,
arrow-key behavior, and a persistent selected tab. A local display mode can remain a
`PillSelector`; a durable workspace destination should be a tab or sidebar item.

Badges and `StatusIndicator` are output. They must not look clickable unless they actually expose an
action, and their text must name the state. A switch is input. “On” means requested configuration,
not proof that an automation is running or healthy.

### Applied selection-pattern baseline

- Player Tracker metric, time-range, and troop-filter choices use `PillSelector` rather than local
  pill-shaped buttons.
- Equipment owner/category, movement mode, production scope, log view, attack wave/type, and unit
  picker choices share the same implementation.
- Storm target levels/resources/sizes/donors and Sceat transport skips use `ChoiceChipGroup`; they
  intentionally do not use `PillSelector` because more than one value may be active.
- The obsolete Logger-specific segmented-control styling is removed.
- General action buttons use the 12-pixel control radius so connected pill selectors remain visually
  distinct.

## 8. Emerging application patterns

### Page workspace

1. `PageHeader`.
2. Optional durable warning/stale banner.
3. State summary and primary task.
4. Filters or scope controls.
5. Structured content.
6. Empty/loading/error state in the same content location.

### Settings editor

1. `Modal` with `ModalTitle`.
2. Scope and mode before detailed values.
3. Group related fields into sections, not nested decorative cards.
4. Persistent field labels and constraints.
5. Cancel plus one consequence-named primary save action.
6. Schedule editing uses the shared weekly scheduler.

Auto Recruit and Auto Tool are the reference implementation for configurable variants of the same
interaction model. Their public wrappers choose `kind`; `QueueProductionSettingsModal` owns the
shared behavior and layout. New queue-production features should extend the definition model rather
than copy the editor.

### Automation row

An automation row contains:

- Independent enable switch.
- Name and plain-language purpose.
- Visible lifecycle label.
- Visible runtime explanation or next step.
- Settings action.
- Optional freshness, attention, and scope when those differ from the page summary.

“Enabled” never substitutes for runtime status. Routine running state is stable and does not pulse.

### Empty, stale, and failure states

Every state block answers:

- What happened or is missing?
- What data is still valid?
- How fresh is the conclusion?
- What can the user do next?

Empty is not error. Stale is not offline. Acknowledged is not resolved. Partial failure preserves
healthy scopes.

### Tables and large collections

- Use real headers, captions where useful, and programmatic sort state.
- Right-align comparable quantities and include units.
- Virtualize retained large collections and media grids.
- Preserve scroll anchor during live updates.
- Provide a narrow-screen alternative or scope horizontal scrolling to inherently tabular content.

### Applied extraction map

| Contract | Representative callers | What remains domain-owned |
| --- | --- | --- |
| `CatalogPickerModal` | Troop, tool, and construction-item pickers | Catalog data, virtualization, card details, selection rules |
| `SettingsModal` | Automation, queue, scheduler, and feature-schedule editors | Validation, persistence, dirty state, feature semantics |
| `SectionCard` | Castle dashboard, settings, currency, Rift, alliance targets, battle stats, event score, hospital, queue editor, patch notes | Section content and actions |
| `CollectionToolbar` + `EmptyState` | Attack and defense preset libraries | Filtering and creation behavior |
| `NamedPresetControls` | Auto Bird and Auto TCI | Stored shape, apply/save/delete handlers |
| `QuantityAssetTile` + `AddSlot` | Auto Bird and Auto Station | Picker invocation and domain IDs |
| `SettingsToggleRow` | Food, Station, Sceat, and Storm settings | Configuration state and dependencies |
| `ScheduleSummaryRow` | Equipment cleanup and Sceat settings | Schedule data and scheduler launch |
| `MetricTile` | Presets, attack setup, equipment, battle stats, event score | Metric calculation and interpretation |
| `PillSelector` | Equipment, movement, tracker, logs, queue scope, attack setup | Selected value and domain labels |
| `ChoiceChipGroup` | Storm targeting and Sceat transport skips | Set mutation and domain constraints |
| Frosted CSS contracts | Catalog picker, switches, pill selectors, scheduler/picker eyebrow labels | Layout, domain behavior, and semantic state |

### Second-pass pattern audit

The audit after this extraction deliberately separates a repeated visual wrapper from a repeated
interaction contract. These are the next evidence-backed candidates, not instructions to create a
generic component before its state and accessibility behavior are understood.

| Candidate | Current evidence | Extraction direction |
| --- | --- | --- |
| Form field anatomy | 52 compact uppercase field labels plus repeated help/error/suffix layouts | Create a labeled field contract that owns label/help/error IDs and supports text, number, select, and picker controls |
| Confirmation dialog | Seven `window.confirm` destructive/apply flows | Replace with an accessible consequence-named dialog that supports pending state; do not hide domain wording in the primitive |
| Schedule-window preview | Hospital and queue settings render closely related day/time rows | Extract a read-only preview list after aligning empty, overflow, and item-image variants |
| Catalog asset field | Weekly Scheduler repeats troop and tool picker-row anatomy | Extract one typed asset field with domain render/lookup callbacks |
| Collection/table frame | Alliance targets, spy reports, equipment, and battle collections repeat title/count/search/table/empty anatomy | Define caption, search, loading, empty row, sort state, and horizontal-scroll behavior together |
| Lifecycle/status badge | Movement and Spy Reports have local status badge mappers; other screens map inline | Wait for the canonical lifecycle/attention/freshness mapping, then provide a typed adapter over `StatusIndicator`/`Badge` |
| Collapsible section header | Battle detail and other dense cards use full-header disclosure buttons | Extract only after keyboard, heading, and persistent-open-state rules are shared |
| Block versus row empty state | 28 remaining “No …” messages include panels, table bodies, and inline fields | Continue using `EmptyState` for blocks; design separate table-row and inline-empty contracts instead of stretching one component |

Fifteen manual `CardHeader` instances remain, but many combine interactive disclosure, castle
state, or domain controls. They are audit targets, not automatic `SectionCard` migrations. Likewise,
the remaining raw modals include pickers and confirmation/edit flows whose footer semantics differ
from `SettingsModal`.

## 9. Operational state vocabulary

Track these dimensions independently:

| Dimension | Canonical values |
| --- | --- |
| Lifecycle | Draft, waiting, scheduled, running, paused, blocked, retrying, failed, completed, disabled |
| Attention | None, information, warning, critical |
| Freshness | Live, delayed, stale, offline, unknown |
| Scope | Global, account, castle, automation, execution, step |
| Actor | User, automation, AI, Citadel, game/upstream |
| Availability | Available, partially available, unavailable, unsupported |

State labels use text and a redundant symbol. Timestamp and cause appear where the state changes a
decision. Green is reserved for confirmed complete/healthy outcomes, not merely enabled settings.

## 10. Accessibility acceptance

Target WCAG 2.2 AA.

- Core tasks work with keyboard only.
- Focus order follows visual/task order and is never hidden by the logger or sticky shell.
- Dialogs set focus, trap focus, close with Escape when dismissible, restore focus, and expose title.
- Selected and focused remain visually distinct.
- Normal text meets 4.5:1; large text and meaningful graphics/controls meet 3:1.
- No meaning uses color alone.
- Essential labels are at least 12 px; primary body/control text is normally 14–16 px.
- Layout supports 200% text and 320 CSS-pixel reflow except for inherently two-dimensional content.
- Forms preserve valid input and explain both the problem and the fix.
- Live regions announce meaningful transitions, not every incoming frame or log row.
- Hover/focus content is supplemental, dismissible, and not the only path to critical information.
- Test light, dark, forced colors, reduced motion, reduced transparency, user text spacing, long
  strings, grayscale, and common color-vision variants.

## 11. Performance and resilience

### Budgets

| Metric | Directional budget |
| --- | ---: |
| Visual input acknowledgment | Under 100 ms |
| Common workspace useful after shell/cached state | Under 1 s |
| Interaction latency | Under 200 ms |
| Primary-task layout shift during live updates | 0 |
| Hidden workspace polling | 0 unless globally required |
| Long-session collections | Explicitly bounded |

### Current measured baseline

The pre-foundation snapshot on 2026-07-15 contained 91 TSX files, 46 TS files, one 4,982-line CSS
file, and 38,713 executable source lines under `Client/src`. The production build emitted:

| Asset | Minified | Gzip |
| --- | ---: | ---: |
| Main JavaScript | 859.54 kB | 232.26 kB |
| Main CSS | 251.67 kB | 34.50 kB |
| Attack editor chunk | 18.26 kB | 5.99 kB |

The same production build after the initial foundation emitted:

| Asset | Minified | Gzip | Change from baseline |
| --- | ---: | ---: | ---: |
| Main JavaScript | 359.19 kB | 111.09 kB | −58.2% minified / −52.2% gzip |
| Main CSS | 255.87 kB | 35.07 kB | +1.7% minified / +1.7% gzip |
| Attack editor chunk | 18.38 kB | 6.05 kB | Loaded only when opened |
| Shared queue editor chunk | 20.47 kB | 5.61 kB | Loaded only when opened |

The reusable-component extraction pass began at 38,462 executable lines and reached 38,446 while
adding and adopting the picker, settings-modal, section, empty-state, collection-toolbar,
toggle-row, schedule-row, preset, asset-tile, add-slot, metric, and choice-chip contracts. That small
net change exposed the next constraint: retired CSS remained in the shipped stylesheet even after
the JSX callers had moved to shared components.

The verified production build after this extraction emits:

| Asset | Minified | Gzip | Change from pre-foundation |
| --- | ---: | ---: | ---: |
| Main JavaScript | 359.24 kB | 111.87 kB | −58.2% minified / −51.8% gzip |
| Main CSS | 256.47 kB | 35.18 kB | +1.9% minified / +1.9% gzip |
| Attack editor chunk | 17.98 kB | 6.04 kB | Loaded only when opened |
| Shared queue editor chunk | 18.05 kB | 5.47 kB | Loaded only when opened |
| Feature schedule chunk | 21.87 kB | 6.76 kB | Loaded only when opened |

The CSS consolidation pass removed the verified-unreferenced shell, dashboard, picker, logger,
sidebar-feature, currency, castle, queue, and utility selectors. It also replaced copied pill,
monospace, kicker, and frost-control recipes with the shared contracts above. Runtime-generated log,
switch, toggle, and status variants remain intentionally; the post-pass selector audit reports no
other literal orphaned class.

The current tree has 107 TSX files, 47 TS files, one 4,640-line CSS file, and 37,894 executable lines
under `Client/src`. Relative to the immediate 38,446-line / 5,192-line pre-consolidation snapshot,
that is 552 fewer lines, all from CSS. It is 819 lines below the 38,713-line pre-foundation snapshot.

The verified production build after CSS consolidation emits:

| Asset | Minified | Gzip | Change from pre-consolidation |
| --- | ---: | ---: | ---: |
| Main JavaScript | 359.26 kB | 111.87 kB | Functionally unchanged |
| Main CSS | 233.20 kB | 32.72 kB | −23.27 kB / −2.46 kB gzip |
| Attack editor chunk | 17.98 kB | 6.04 kB | Unchanged |
| Shared queue editor chunk | 18.05 kB | 5.47 kB | Unchanged |
| Feature schedule chunk | 21.88 kB | 6.76 kB | Unchanged |

The stylesheet is 10.6% shorter, 9.1% smaller minified, and 7.0% smaller gzip than the immediate
pre-consolidation build. Visual checks covered dark and light desktop layouts, frosted switches,
the TCI settings modal, the shared catalog picker, and the 390-pixel breakpoint.

### Engineering rules

- Lazy-load top-level workspaces and large settings editors.
- Keep the startup shell independent from analytics, pickers, and feature editors where possible.
- Suspend workspace-specific polling while inactive.
- Prefer domain-scoped subscriptions/selectors over one broad context invalidation.
- Batch high-frequency state events and preserve the revision used by plans.
- Virtualize logs, tables, media grids, and long pickers.
- Bound logs, activity, receipts, and operation maps with an explicit retention policy.
- Reserve image dimensions and load game assets only for the visible workspace.
- Avoid stacked backdrop filters and full-scroll blurred surfaces.
- Measure parsed/executed and long-session cost, not only transfer size.

## 12. Rewrite architecture and sequence

The rewrite is incremental. A second frontend beside the live one would duplicate behavior and make
exact parity difficult to prove. Shared contracts are extracted first, then workspaces are replaced
behind the same imports and API calls.

### Phase A: foundation — established and still migrating

- Semantic color and shape tokens.
- Shared page, settings-modal, section, picker, collection, empty-state, status, metric, toggle,
  schedule, preset, asset-tile, and selection patterns.
- Accessible navigation and modal behavior.
- Route/workspace and settings-editor lazy loading.
- Remove exact duplicated feature implementations.

### Phase B: shell and navigation

- Introduce durable routes and deep links while preserving current `ViewId` behavior during
  migration.
- Group navigation by operations, workspaces, and system.
- Add System theme and explicit density preference.
- Collapse healthy connection detail into one quiet summary; expand degraded layers.

### Phase C: overview and automation

- Empire overview with attention queue, freshness, current work, next work, and significant outcomes.
- Automation control tower exposing lifecycle, detail, next check, last run, operation, error, and
  shared resource contention.
- Explicit Pause all/Resume semantics separate from game connection.

### Phase D: workspace migration

- Migrate presets, movement, Rift, equipment, intelligence, events, and analytics to shared page,
  state, filter, table, and empty-state patterns.
- Split oversized modules by user task and domain computation, not arbitrary line count.
- Keep game-specific art and functional parity through each replacement.

### Phase E: activity and diagnostics

- Human-readable activity and execution timeline.
- Recovery and safe redrive.
- Logger remains secondary and raw evidence remains opt-in.
- Explicit retention, redaction, and support bundle policy.

### Parity gate for every migrated screen

- All existing user actions and API intents remain reachable.
- Configuration wire shape and storage keys remain unchanged unless separately migrated.
- Loading, empty, disconnected, stale, partial, error, and success states are represented.
- Keyboard, focus, theme, narrow width, and long-text behavior are checked.
- Source duplication and total executable LOC do not increase without a documented reason.
- Startup bundle and workspace chunk changes are measured.

## 13. Code organization rules

- Shared visual primitives live in `Client/src/components/ui`.
- Domain components remain with their domain; do not put game rules into generic UI components.
- Generic feature variants use typed definitions/adapters instead of copied files.
- Keep state normalization separate from rendering.
- Prefer one component per repeated interaction contract, not one component for every wrapper `div`.
- A shared abstraction must have at least two real callers or represent a foundational accessibility
  contract.
- Preserve existing public component names with thin wrappers during migration.
- New hand-written source filenames use PascalCase.

## 14. Contribution checklist

Before adding or changing frontend UI:

- [ ] Does an existing primitive or pattern already own this behavior?
- [ ] Is current state and freshness visible before actions?
- [ ] Are scope and consequence explicit?
- [ ] Are action, success, warning, danger, information, and inactive colors used semantically?
- [ ] Is meaning redundant with text and shape/icon?
- [ ] Is essential content available without hover?
- [ ] Are controls labeled, keyboard-operable, and focus-visible?
- [ ] Are loading, empty, stale, error, and partial states covered?
- [ ] Is the surface opaque enough for its reading density?
- [ ] Does it work in light/dark, reduced motion, narrow width, and long text?
- [ ] Does it avoid new polling, broad rerenders, eager heavy imports, and unbounded collections?
- [ ] Did executable LOC or bundle cost increase, and is the reason documented?

## 15. Open decisions

- Durable routing library and URL scheme.
- System theme and density preference storage.
- Final high-contrast and reduced-transparency mappings.
- Shared loading, stale, partial-error, table-empty, and inline-empty APIs beyond block `EmptyState`.
- Canonical mapping from server automation status strings to lifecycle/attention/freshness dimensions.
- Labeled field/error/help association contract and its compact/comfortable variants.
- Accessible asynchronous confirmation dialog contract.
- Retention limits for operations, activity, and diagnostics.
- Whether game catalog loading can become workspace-scoped without delaying common tasks.
- Final Command Violet validation and broader identity/name decision.

These open decisions do not block incremental component extraction. They do block claiming the full
redesign or rewrite complete.
