# Desktop visual snapshots (CIT-61 Revision 2)

The reference renderer is the studio Mac (macOS arm64), with Playwright 1.63.0 and its pinned Chromium headless shell. Ethan generates the baselines and Sophie verifies them on this same Mac. There is no CI workflow or container path.

The suite opens the CIT-22 `rich-account` harness on 127.0.0.1:41736 and captures Castle, Automation, Equipment, Feature Stats and Auto Towers settings at 1440×900 and 1024×768, in dark and light: 20 viewport PNGs. A busy port fails the run; no existing server is reused. Never run two desktop visual suites at once.

Each capture starts a fresh context with scale 1, reduced motion, en-US, UTC, a seeded random generator and a clock fixed at 2026-09-29T12:00:00Z while timers run. Test CSS hides the harness banner and dock and resets the root viewport. The ready marker, network idle, fonts and a two-second settle precede each capture. External images get the committed neutral placeholder; other external traffic is aborted. Escaped requests and unhandled mock paths fail the test. Product images come from `public/game-data`. Captures disable animations and hide the caret; the comparator uses threshold 0.2 and zero differing pixels.

Run from `Client/` on the studio Mac:

1. `npm ci`
2. `npx playwright install chromium`
3. `npm run test:visual:update` (refuses baseline generation outside macOS arm64).
4. Verify exactly 20 PNGs under `tests/visual/__screenshots__/`, named `<case>-<width>-<theme>.png`, with nothing else; commit them.
5. Run `npm run test:visual` twice, fresh each time. Both runs must pass.
6. Make one throwaway CSS property change, run the suite and review the visible diff in `playwright-report/` and `test-results/`. Restore the CSS; never commit the probe change.
7. Run `node scripts/visual/check-baseline-changes.mjs --base origin/develop --body-file <file>` against the final PR body. Every changed baseline filename belongs under the **Visual baseline changes** heading with a one-line reason.

The PR records a **Visual environment** block: studio Mac, macOS version/build/arch, Node version, Playwright version, Chromium headless-shell revision, and commands with results and timestamps. Record run time and total baseline size. When macOS or Playwright changes, re-baseline in a separate PR with no product change and a new environment block.

Sophie runs `npm ci` and `npm run test:visual` at the exact PR head on this Mac. When baselines change, she compares against the base's images, reviews intended differences, restores the PR images and verifies a passing run, then runs the local baseline guard. Initial baselines have no base images; their complete captures require review. Part B merges after Part A (portal #98).

No redesign is included. The only product-source change is the explicit Tailwind source root needed by the fixture harness; production built CSS must remain byte-identical. Build/deployment configuration stays unchanged. Visual outputs and baselines are excluded from packaging, and capture requests remain on loopback.
