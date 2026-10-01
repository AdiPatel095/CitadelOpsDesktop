# CIT-63 B: size and weight migration preparation

Contract: Daniel's **CIT-63 Self-hosted Manrope and type scale**, part B,
and CIT-60 §§4.3–4.4, R4/R5/R13. This branch adds tooling and tests only.
Application, role utilities, locale rules, lint enforcement, the zero ratchet,
and visual baselines wait for **CIT-62 PR-3 to merge in both repositories**.
No font delivery work is included here. No PR is opened for this preparation.

## Size / role mapping

CSS output references CIT-62's `--font-size-N` primitives. TSX output uses the
role utilities below; B must define them in `@theme` before application.
Size alone never determines whether text is a title or body. All sizes ≥12 px
require an explicit reviewed role decision, even exact matches. The nearest
size is a suggestion; select the nearest role **that fits the element's use**.

| Role / TSX utility | Size / line px | Weight | Use |
| --- | --- | --- | --- |
| `text-display-lg` | 64 / 68 | 700 | Wide landing hero |
| `text-display` | 48 / 52 | 700 | Medium landing hero |
| `text-display-sm` | 32 / 40 | 700 | Compact hero, landing section titles, headline metrics |
| `text-headline` | 24 / 32 | 700 | Page title |
| `text-headline-compact` | 20 / 28 | 700 | Page title at compact; decision is `headline`, `compact: true` |
| `text-title` | 20 / 28 | 600; metric 700 | Section / dialog title, metric value |
| `text-title-sm` | 16 / 24 | 600 | Card / list-item title |
| `text-body-lg` | 16 / 24 | 400 | Landing / reading text, compact text inputs |
| `text-body` | 14 / 20 | 400; controls 500 or 600 | Default UI text |
| `text-body-sm` | 13 / 18 | 400, 500, 600 | Dense tables, labels (500), headers (600), secondary lines |
| `text-caption` | 12 / 16 | 400, 500, 600 | Metadata, helpers, badges (600), tags (500), eyebrows (600) |

| Old size | Numeric nearest candidates (semantic choice still required) |
| --- | --- |
| Any positive size below 12 px, including 9 / 10 / 11 | Caption, 12 / 16 (automatic) |
| 12 | Caption |
| 13 | Body-sm |
| 14 | Body |
| 15 | Body or title-sm / body-lg (tie; title must keep a title role) |
| 16 | Title-sm or body-lg |
| 17–18 | Title-sm / body-lg; at 18, title also ties |
| 19–21 | Title; compact headline when it is a page title |
| 22 | Title or headline (tie) |
| 23–27 | Headline |
| 28 | Headline or display-sm (tie) |
| 29–39 | Display-sm |
| 40 | Display-sm or display (tie) |
| 41–55 | Display |
| 56 | Display or display-lg (tie) |
| 57 and above | Display-lg |

Fractional sizes use the same absolute-distance calculation, with all ties
reported. `rem` converts only with an explicit, verified `--rem-base 16`.
`em`, percentages, viewport sizes, `clamp()`, `calc()`, dynamic expressions and
font shorthand require manual review. Existing variable/inheritance values
remain intact. Tokens themselves are excluded.

Legacy Tailwind `text-xs/sm/base/lg/xl/2xl/3xl/4xl/5xl/6xl/7xl/8xl/9xl`
use default sizes 12/14/16/18/20/24/30/36/48/60/72/96/128 for suggestions.
Verify these defaults against the current `@theme` before choosing a role.

## Weight mapping

| Old weight | Target | TSX utility | CSS / inline style output |
| --- | --- | --- | --- |
| 400 / normal | 400 | `font-normal` | `var(--font-weight-400)` |
| 500 / 520 | 500 | `font-medium` | `var(--font-weight-500)` |
| 600 / 620–650 | 600 | `font-semibold` | `var(--font-weight-600)` |
| 700–1000 / bold | 700 | `font-bold` | `var(--font-weight-700)` |

This covers the story's observed 630/640/650 and
720/740/750/760/780/790/800/820/830/850/900/950.
`font-extrabold` and `font-black` become `font-bold`.
Existing allowed weight utilities remain intact. Other intermediate or light
weights, `bolder`, `lighter`, and dynamic weights are reported; the plan does
not supply a mapping, so return those choices to Daniel.

## Run / review / apply later

Run from the portal root or desktop `Client/`. TypeScript and PostCSS use the
existing development installation (PostCSS is already in the npm lockfile).
The three prep files are byte-identical between the two clients.

```sh
node --test tests/type-scale.test.mjs
node scripts/typography/type-scale.mjs src > /tmp/cit-63-type-scale-report.json
```

The default is a JSON dry run; no input file is written. Each proposed change
and unresolved finding names `file:sourceOffset:kind`, the original value and,
for numeric sizes, nearest candidate roles. Offsets are UTF-16 offsets in the
original file, not line numbers. Review current element semantics and create
a decisions JSON file, for example (use actual keys from your report):

```json
{
  "src/example.css:14:size": { "expected": "18px", "role": "title" },
  "src/example.tsx:22:size": { "expected": "24px", "role": "headline", "compact": true }
}
```

Re-run with `--decisions /path/to/decisions.json` and, only if the root font
size has been checked, `--rem-base 16`. Stale values, unknown roles, unused
keys, invalid syntax, path escapes and symlinks stop the batch. Parse and
resolve every selected file before writing; `--write` refuses any unresolved
finding. Select narrow files rather than the whole tree for application.
Do not run `--write` on application sources until the merge gate has cleared.

The AST scanner changes only JSX `className`, literal inline `style` font
properties and SVG `fontSize/fontWeight`. It follows static branches,
`cn/clsx/classNames`, arrays and object class keys. It reports interpolated
templates and class references; it never changes UI text, comments or catalog
strings. Class constants outside those expressions, referenced/spread style
objects, computed property names and other attributes require manual audit
with the later ratchet and DOM scan. Custom Tailwind leading is reported.
CSS comments, `!important`, selectors, colors and unrelated declarations stay
intact. Overlapping input paths are deduplicated. Re-running resolved output
is idempotent; discard/regenerate decisions when source changes.

## Remaining B application checks

The codemod handles size/weight replacements only. CSS sizes and inline styles
use primitives: separately apply their paired line heights and role weights.
Add role utilities to `@theme`; review existing `leading-*` / `font-*` overrides.
Do not infer eyebrows from uppercase text. A reviewed eyebrow is caption,
600, uppercase, +0.06em, secondary text. Remove other CSS uppercase transforms
without re-casing strings or translations. For ar/ja/ko/zh-*, disable uppercase
and tracking, and add 4px line height for roles from 12–16px. Display tracking
is −0.02em and headline −0.01em only in Latin/Cyrillic/Greek; otherwise zero.
Apply tabular figures to stats, tables, timers, counters, currency and coordinates.

After applying B: enforce raw-size/weight lint errors and a zero font-size
ratchet, run the all-view DOM size scan, rebaseline all CIT-61 images as
“type scale”, check clipping and mirror parity, and run Daniel's standard
checks in both clients. This prep does not establish those application results.
