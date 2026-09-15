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

`Run 5 warm full-path samples` clicks the production Preview button five times. Each sample starts inside the fixture API provider immediately before its fetch, then a `MutationObserver` waits for all ten real alternative buttons and two animation frames. HTTP time and click-handler-to-painted-results time remain visible in the DOM. Initial Vite/catalog/component loading is outside the measured window. These are synthetic local regression measurements, not live account or production latency evidence.
