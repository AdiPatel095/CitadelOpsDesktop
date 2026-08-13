# CitadelOpsDesktop project rules

## Patch notes

- Keep releases newest-first in `Client/src/config/PatchNotes.ts`.
- Group every new or edited release's items by change type in this exact order: `added`, `fixed`, `security`, `changed`, `removed`, `deprecated`.
- Keep related items together within their change-type group; do not interleave groups.
- Treat `PATCH_NOTE_KIND_ORDER` as the canonical order and preserve the central normalization that applies it to every exported release.
- Keep the Patch Notes page grouped under visible change-type headings in that same canonical order, omitting empty groups.
- When adding a change type, update the type order, label mapping, badge mapping, project rule, and sorting verification together.
