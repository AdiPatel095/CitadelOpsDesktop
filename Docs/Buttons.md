# Buttons

Use the shared `Button` for actions and `buttonAttributes()` for links that look like actions. Both clients use the same API and token styles. Import from `components/ui` (under `src/commandCenter` in the portal and `Client/src` in desktop).

```tsx
<Button variant="primary" leftIcon={<Save />}>Save</Button>
<Button variant="secondary" onClick={cancel}>Cancel</Button>
<Button variant="ghost" iconOnly aria-label="Settings"><Settings /></Button>
<Button variant="danger" leftIcon={<Trash2 />}>Delete</Button>
<a href="/accounts" {...buttonAttributes({ variant: 'secondary', size: 'lg' })}>Manage accounts</a>
```

The default variant is secondary and the default size is md. Primary must be explicit. Sizes are sm (32 px), md (40 px, 44 px on coarse pointers), and lg (48 px). `iconOnly` makes the button circular; supply an `aria-label`. Its tooltip defaults to that name. `outline`, `solid`, and `size="icon"` are retired.

Each card, dialog, toolbar, page header, or landing section has at most one primary action. Mark independent regions with `data-region` or `role="dialog"`. Nested regions own their actions. Every CIT-61 snapshot checks R11 before its screenshot. Destructive dialogs use danger confirmation and secondary Cancel, with no primary. Preserve existing confirmations and enablement.

All disabled variants use the same neutral fill, label, and border. Keyboard focus uses the shared ring. Hover and pressed states use tokens. Pressing changes the pill corner to the medium radius unless reduced motion is requested. Loading preserves the label, replaces the leading icon with a spinner, sets `aria-busy`, and disables the action. For buttons without a leading icon, a supplied `isLoading` prop reserves a leading slot even when false, keeping width stable. Pass the controlled loading value from the first render. Button forwards native attributes and refs, and preserves the browser default `type` when none is supplied.

## Overflow menus

`OverflowMenu` groups secondary and destructive actions. Mixed menus use a named ghost icon trigger. Menus containing only destructive actions use a danger trigger with a leading icon and chevron. Non-destructive items precede a divider and destructive items. Disabled entries are skipped by keyboard navigation; the trigger is disabled when all entries are disabled.

Enter, Space, or ArrowDown opens the menu and focuses the first enabled entry. Up/Down wrap; Home/End select the first/last enabled entry. Escape closes and restores trigger focus. Tab and outside clicks close. Selection closes the menu before invoking its callback. The popup is anchored at wide widths and becomes a bottom sheet with a scrim below 768 px. Motion honors reduced-motion preferences.

Equipment uses a primary Reconfigure action, secondary upgrade/event actions, and danger Unequip… and Sell… menus. Menu items open the existing confirmations; they do not submit an action directly.

## Documented raw-button patterns

Raw buttons must have a literal `data-button-pattern`:

| Pattern | Use |
| --- | --- |
| `row` | A complete selectable or toggleable list row, with its existing name and state |
| `card` | A whole card or picker option that acts as one control |
| `tab` | Existing tabs/segments with their selection and keyboard semantics |
| `disclosure` | A disclosure trigger with `aria-expanded` and its controlled panel |
| `nav` | Navigation items or the existing header navigation controls awaiting their own story |
| `tile` | A selectable compact tile/chip with its existing state |

These patterns retain their component behavior and styling; ordinary actions use Button. UI primitives, test files, and mock fixtures are exempt. `components/Header.tsx` remains exempt while CIT-69 owns its rewrite; CIT-65 changes only its retired Button props. The public header CTA keeps its existing styling until CIT-70.

Run `npm run check:buttons` from the portal or desktop Client directory. It fails on unmarked raw buttons, invalid patterns, danger buttons without `leftIcon`, icon-only buttons without names, or retired `m3-button` classes outside the primitive directory. The checker tests run with the project's unit suite.

## Styles and mirrors

`button.css` owns Button styles without requiring app utilities. `tokens.css` is public; `tokens-app.css` adds app-only compatibility aliases after it in the Account Center, command center, and desktop. The public build checks that all consumed variables and aliases resolve, and keeps the initial CSS under 16 KiB gzip. Shared primitives, styles, checker, visual rules, message definitions, and region primitives are checked byte-for-byte by `check:mirror`.
