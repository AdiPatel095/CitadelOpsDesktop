# Plain descriptor lists

`LocalizedMessage.listParams` maps an ICU argument name to ordered leaf descriptors. The root must bind complete original display text in `fallbackText`; an outer `officialKey` is invalid. Leaves allow the existing plain fields, including official names and primitive/game parameters, but no context or nested lists. Rich tags and HTML callbacks are not supported.

Bounds: 1–4 lists, 1–32 leaves per list, at most 64 total leaves, at most 65,536 UTF-8 bytes of serialized list metadata. The byte count applies Go JSON escaping for `<`, `>`, `&`, U+2028 and U+2029. List argument names cannot collide with primitive/game parameters. Current Storm's 17 allowlisted deduplicated product IDs fit within the per-list bound; the producer owns that cardinality regression.

Every list must be a plain ICU string argument in the source and selected translation. All possible conditional branches must retain it; typed number/date/select/plural use is invalid. Leaves resolve first in the requested locale, then `Intl.ListFormat` with long conjunction joins their plain text. Inserted values are never parsed again. Missing translations, invalid parameters, empty rendered items, malformed metadata and overflow restore the complete root `fallbackText` before context composition. English source leaves are valid for English; English fallbacks do not satisfy a non-English request.

The API parser rejects malformed metadata so existing consumers retain their separate raw legacy text. Direct formatter calls without the mandatory fallback binding return an empty untranslated string rather than an ICU template.
