# Desktop visual snapshots (CIT-61)

The canonical renderer is Playwright 1.63.0 in `mcr.microsoft.com/playwright:v1.63.0-noble`, Linux amd64. The version in the lockfile and the image tag must match. The suite opens the CIT-22 `rich-account` harness on 127.0.0.1:41736 and captures Castle, Automation, Equipment, Feature Stats and Auto Towers settings at 1440×900 and 1024×768, in dark and light: 20 viewport PNGs.

Each capture starts a fresh context with scale 1, reduced motion, en-US, UTC, a seeded random generator and a clock fixed at 2026-09-29T12:00:00Z while timers run. The harness banner and dock are hidden only by test CSS. The ready marker, network idle, fonts and a two-second settle precede each capture. External images get the committed neutral placeholder; other external traffic is aborted. Escaped requests and unhandled mock paths fail the test. Product images come from `public/game-data`.

Run commands from `Client/`:

- `npm run test:visual`: compare in CI or the canonical container, zero retries, threshold 0.2 and zero differing pixels.
- `npm run test:visual:update`: update in the canonical renderer for inspection. Committed baselines must come from a failed PR run's artifact.
- `npm run test:visual:docker`: optional pinned amd64 container with an anonymous node_modules volume, npm ci and host IPC. Local Docker is not needed for delivery.
- `npm run test:visual:host -- --update-snapshots=changed`: local before/after comparisons only, written to the ignored `tests/visual/.host/`. Never commit these images.

## CI baseline bootstrap and updates

Open the PR into develop without baselines. The compare run fails for missing images, then captures missing/changed baselines and uploads `visual-baselines` for seven days. The job remains failed. `visual-report` always contains the compare/capture HTML reports, traces and diff screenshots.

Download with `gh run download <run1> -n visual-baselines -D Client/tests/visual/__screenshots__` from the repository root. Verify exactly the expected 20 PNGs named `<case>-<width>-<theme>.png`, with nothing else. Commit as `CIT-61: Linux baselines from run <run1>` and push. Run 2 must pass; rerun it and require that attempt to pass too. Record all three run URLs, duration and baseline size in the PR.

A successful compare also checks one throwaway CSS property (`filter: grayscale(1)`) against the Castle dark capture. That change exists only in the CI workspace, is restored, and must produce a failing assertion and a visible `-diff.png`. The report artifact includes that screenshot as the negative-control evidence.

For later intended changes, review the failing compare report and download the failed run's baseline artifact. Commit only intended changes. Every PR changing baselines needs a **Visual baseline changes** heading listing every changed PNG and a one-line reason; the guard verifies every filename. Sophie reviews the snapshots and reports before approval. Part B merges after CIT-61 Part A (portal).

The suite changes no UI design. Its only product-source change is the explicit Tailwind source root needed by the fixture harness; production built CSS must remain byte-identical. Production build, deployment configuration and the desktop client flow otherwise stay unchanged. Visual outputs and baselines are excluded from packaging; no network request goes outside loopback during captures.
