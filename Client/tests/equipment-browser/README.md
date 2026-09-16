# CIT-6 local equipment browser fixture

This fixture mounts the production `EquipmentOptimizer` and production CSS. Vite aliases only `ApiContext` and `MetadataContext` to deterministic fixture providers. The browser provider sends one real HTTP request to the production `/api/v2/equipment/optimize` mux, whose handler invokes `Server/Equipment.Optimize` against synthetic `State.GameState` and official-data-shaped `GameData`.

It never connects to a game session and cannot mutate live equipment. Apply is a browser-only transport simulation that captures the chosen alternative arguments and returns a terminal success, authoritative failure, or stale rejection.

Run from the repository root in two terminals:

```sh
go run ./cmd/cit6-equipment-fixture -listen 127.0.0.1:41732
cd Client && npm exec vite -- --config tests/equipment-browser/vite.config.ts --host 127.0.0.1 --port 41733 --strictPort
```

Open `http://127.0.0.1:41733/`.

The fixed control dock stays above production modal portals. It exposes commander/castellan inventories, fewer/no candidates, HTTP error, 1.5-second delayed success, and 9-second timeout, unrelated background updates, candidate ID churn within the same semantic groups, relevant equipment and off-mode socket changes, catalog addition/digest change, v1-v4 stored-profile seeds, and simulated apply outcomes.

`Run 5 warm full-path samples` clicks the production Preview button five times. A capture-phase document listener records the production button click before its React handler runs; the fixture API provider records the HTTP completion, then a `MutationObserver` waits for all ten real alternative buttons and two animation frames. HTTP time and click-handler-to-painted-results time remain visible in the DOM. Initial Vite/catalog/component loading is outside the measured window. These are synthetic local regression measurements, not live account or production latency evidence.

## Recorded review evidence

On 2026-09-15, the mounted fixture was exercised in local Chrome on an Apple M4 machine through browser clicks against the production HTTP handler and optimizer. The sample lists below are rounded to the nearest millisecond:

- Commander, 334 equipment and 16 gems: 10 warm click-to-ten-results samples were 100, 79, 68, 80, 67, 81, 89, 91, 90, and 69 ms; exact maximum and p95 were 99.7 ms.
- Castellan, 253 equipment and no PvP gems: 10 warm samples were 74, 76, 82, 82, 39, 40, 55, 73, 95, and 76 ms; exact maximum and p95 were 94.8 ms.
- Selecting alternative 7 was instant and submitted that alternative's equipment, gems, fingerprint, and intended leader. For the commander case, the captured request matched `alternatives[6]` from the real HTTP response.
- Unrelated state and candidate-ID churn retained the selected batch. Catalog expansion and attached off-mode socket changes retained it as stale with Apply disabled.
- Regeneration showed an HTTP error or timeout inside the retained preview, ended loading, and recovered on retry. Canceling a delayed successful request kept the preview closed after the response arrived.
- Adding, removing, reordering, and moving groups between tiers persisted v4 preferences under the selected player, leader, and target key. A v1 seed migrated to official groups, and returning to PvP restored its saved groups.
- A priority edit during a delayed request cleared loading and ignored the late result. Switching from PvP to PvE during a delayed PvP request did not open the old batch under the new target.
- Simulated authoritative apply failure kept the preview open with recovery text; simulated terminal success closed it. No game transport or equipment mutation was used.

The handler-level performance check is reproducible with:

```sh
go test ./Server/API -run 'TestEquipmentOptimize(HTTPReturnsBoundedAlternatives|RepresentativeHTTPPerformance)' -count=1 -v
```

That test recorded p95 values of 17.638 ms for the same commander shape and 11.844 ms for the castellan shape across 12 warm samples each. Timings vary by machine. Both the browser and handler measurements use deterministic synthetic inventory and cannot establish production network latency or live-account apply behavior.
