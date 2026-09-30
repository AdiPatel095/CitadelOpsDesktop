# CIT-63 PR-A: waiting for Oscar's provenance

The TTF and WOFF2 files are deliberately absent. `npm run build` runs the
fail-closed provenance/integrity check first. It currently exits with:

    Font provenance incomplete. Waiting for Oscar to pin google/fonts commit.

Do not fill the commit with a guessed hash or disable `prebuild`. Oscar must
provide the exact google/fonts commit for the accepted TTF and OFL bytes.
The TTF must remain 164,700 bytes with SHA-256
`3ae11c49db0455a3cc33e37d380f20fdb8c7f8b41dc07625c177e3d87a9d6ae6`.

## Completing the gate on this branch

1. Replace `scripts/fonts/font-provenance.json` with Oscar's completed record.
   Update `source` and `license` in `public/fonts/manrope/fonts-manifest.json`
   (desktop: `Client/public/fonts/manrope/`) to match it, including both commits.
2. In an isolated Python venv, install `scripts/fonts/requirements.txt`.
   Run the repo-root command below using that venv's Python. No download occurs:

       python scripts/fonts/build-manrope --source '/path/to/Manrope[wght].ttf'

   This writes the five `manrope-<script>.woff2` files and their output SHA-256,
   byte sizes and source commit into `fonts-manifest.json`. `wght` stays variable
   at 400–700; shaping features, including `tnum`, are retained. Source timestamp
   is preserved and the pure Python OpenType serializer is used for repeatability.
3. Alternatively, drop the five externally generated WOFF2 files into the same
   directory and copy their complete output records into `fonts-manifest.json`.
   Keep every filename, Unicode range and weight contract unchanged. Copy the
   same assets and manifest to both repositories and run `npm run check:fonts`.
   It checks all five hashes/sizes, source commits, WOFF2 signatures and the OFL.
4. Commit both asset sets on these same branches, run full builds and activate
   the registered `tests/visual/fonts.spec.ts` by completing the provenance.
   Then run the font specimen tests, Lighthouse median-of-three comparison,
   and the 54 portal / 20 desktop baseline checks, labelled "Manrope".

`--font-sans` and language-specific font stacks remain gated on CIT-62 PR-1.
The preload deliberately points to the pending Latin asset; this branch must
not merge or ship while the build gate is failing. Desktop `Client/dist` is
embedded by the existing Go client bundle; no packaging change is needed.

## Fallback metrics

`fonts.css` registers `Manrope Fallback` via local Arial (400–500) and
Arial Bold (600–700). No system font is copied. At each of the four weights,
Manrope has units/em 2000, glyph x-height 1080, and OS/2 typo metrics
2132 ascent, -600 descent, zero line gap. Arial and Arial Bold x-height is
1062/2048. `size-adjust = (1080/2000)/(1062/2048)`, and each override is
`100 * Manrope metric / 2000 / size-adjust`. Geometry/Lighthouse verification
remains pending font-stack integration; matching these metrics does not prove
identical string widths or zero CLS.

## Mirror check

From the portal: `npm run check:mirror -- --desktop /path/to/desktop`.
From desktop `Client`: `npm run check:mirror -- --portal /path/to/portal`.
This branch adds only `styles/fonts.css` to the shared file list. When CIT-62
PR-1 lands, preserve its token list/checker and append this font entry.
