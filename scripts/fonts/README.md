# CIT-63 PR-A: self-hosted Manrope

The five WOFF2 subsets and unmodified OFL ship under public/fonts/manrope/
(desktop: Client/public/fonts/manrope/). No source TTF is copied into the repos.
npm run build runs the fail-closed provenance/integrity gate first.

## Verified source

Commit: b31870aff700ab7a1d74fa0c6887d95beb9e0037
TTF: 164,700 bytes; SHA-256 3ae11c49db0455a3cc33e37d380f20fdb8c7f8b41dc07625c177e3d87a9d6ae6
TTF blob: 75274da58537d6123b14f2cd0c355ad4681fc2b3
OFL: 4,387 bytes; SHA-256 58172e0c0fac2cda8a37b348164bb55e44b0e69051e557e92b1d3f6910141f7b
OFL blob: e271172a9ed0cddc895aabb6509f1c7d880b492d

## Reproduction and gate

Install scripts/fonts/requirements.txt in a dedicated Python venv. From the
repo root, run with that venv's Python:

    python scripts/fonts/build-manrope --source '/path/to/Manrope[wght].ttf'

The script checks the exact accepted source pin before generation. It subsets
before limiting the variable axis to wght 400-700, retains all OpenType features
including tnum, preserves timestamps, and uses the pure Python serializer.
fonts-manifest.json records source and output hashes, sizes and source commits.

Alternatively, drop independently generated WOFF2 files and complete output
records into the same directory. Preserve filenames, Unicode ranges and weight
contracts. npm run check:fonts verifies hashes/sizes, WOFF2 signatures, source
commits and OFL. Missing metadata exits with:

    Font provenance incomplete. Waiting for Oscar to pin google/fonts commit.

npm run test:fonts verifies the fail-closed gate. test:visual includes every
script specimen from self with no fallback glyphs and equal tnum digit widths
at 400, 500, 600 and 700. CIT_VISUAL_PORT_OFFSET=100 npm run test:visual uses
separate local servers when unrelated previews occupy the default ports.

--font-sans uses Manrope and the metric-matched fallback in shared tokens.css.
Arabic and CJK stacks follow the element language via :lang(). Font synthesis
is disabled: Manrope supplies upright faces only. Client/dist is embedded in Go desktop
builds; the fonts require no packaging change.

## Fallback metrics

fonts.css registers Manrope Fallback via local Arial (400-500) and Arial Bold
(600-700). No system font is copied. At each weight, Manrope units/em is 2000,
x-height 1080, OS/2 typo ascent 2132, descent -600 and line gap zero.
Arial/Arial Bold x-height is 1062/2048. size-adjust = (1080/2000)/(1062/2048);
each override = 100 * Manrope metric / 2000 / size-adjust. Matching metrics
does not guarantee identical string widths. Repeat performance checks after
application font-stack changes.

## Mirror

Portal: npm run check:mirror -- --desktop /path/to/desktop
Desktop Client: npm run check:mirror -- --portal /path/to/portal
The CIT-62 explicit path manifest covers tokens.css and fonts.css in both clients.
