# Hosted cell profiling (CIT-42)

A hosted worker can expose Go's pprof endpoints and runtime metrics on a loopback-only listener. It is off unless `CITADEL_PPROF_ADDR` is set. It exists to answer "where does this cell's CPU go" with a profile instead of an estimate.

## Safety properties

- **Off by default.** With the variable unset there is no extra listener, no mutex or block sampling, and no profiler labels are applied (the labelled call sites cost one atomic load; ingest of a frame measured 9.27 vs 9.31 microseconds per frame with identical allocations).
- **Loopback only.** The address must be a loopback IP literal or `localhost` (for example `127.0.0.1:6060`, `[::1]:6060`). An empty host (`:6060`), an unspecified address (`0.0.0.0`, `[::]`), a routable address or a DNS name is refused: the worker logs `Profiling listener not started (CITADEL_PPROF_ADDR): ...` and keeps running without profiling. The bound address is checked again after `Listen`.
- **Its own server.** Its own mux serves only the routes below; none of the worker's application routes (dashboard API, `/orchestrator/`, tenant login) exist on it, and nothing here is added to the worker's own mux. It sets `ReadHeaderTimeout` (5 s), `IdleTimeout` (60 s) and `MaxHeaderBytes`; there is no write timeout because a profile or trace streams for its whole duration.
- **Sampling only while on.** Mutex (1 in 100) and block (1 ms) profiling are enabled when the listener starts and switched off when it stops.
- **Reaching it.** Operators reach the listener through an IAP SSH session to the cell VM, never through a public port or the Caddy proxy (see "Reaching the listener").

## Routes (loopback listener only)

| Route | Content |
|---|---|
| `/debug/pprof/` | index and named profiles (`heap`, `allocs`, `goroutine`, `mutex`, `block`, `threadcreate`) |
| `/debug/pprof/profile?seconds=N` | CPU profile |
| `/debug/pprof/trace?seconds=N` | execution trace |
| `/debug/pprof/symbol`, `/debug/pprof/cmdline` | symbol lookup, process command line |
| `/debug/runtime/metrics` | JSON: `gcCpuSeconds` (`/cpu/classes/gc/total:cpu-seconds`), `liveHeapBytes` (`/gc/heap/live:bytes`, post-GC), `schedLatencySeconds` (`/sched/latencies:seconds` count and p50/p90/p99/max, each the upper bound of the holding bucket) |

The token-protected `GET /orchestrator/v1/diagnostics` reports per-cell figures without a profile: post-GC live heap, `gcTotalCpuSeconds` and, new in CIT-42, `processCpuSeconds`, `cpuCores`, `cpuShare`, `gcCpuShare` and `cpuWindowSeconds`. `cpuShare` is the process CPU time (user+system, from the OS) over the last window as a fraction of `cpuCores` of wall time (`GOMAXPROCS`, which follows a container CPU limit); `gcCpuShare` is the garbage collector's part of it on the same denominator. The first sample after start covers the whole process lifetime (`cpuWindowSeconds` 0); later samples cover the time since the previous sample, at most one per five seconds. It is not clamped at 1: a value above 1 means more CPU than `cpuCores` was used.

## Profiler labels

Every CPU/goroutine/mutex/block sample can be sliced by these labels (present only while the listener is on):

| Label | Values | Set at |
|---|---|---|
| `runtime` | the account runtime id | `Accounts.Supervisor.AddAccount`, around `App.New`, `Application.Start` and the session start, so goroutines the runtime starts inherit it |
| `stage` | `ingest` | per committed frame (`Ingest.Pipeline.CommitFrameGuarded`), together with `opcode=<frame opcode>` |
| | `transport` | the direct WebSocket transport run loop (`DirectWebSocketTransport`) |
| | `automation` | the automation coordinator (`Application.start`) |
| | `persist` | the state persistence writer (`Application.persistState`) |
| | `worldmap-adopt` | each runtime's adoption of a shared world-map event (`Supervisor.runWorldMapPropagation`) |
| | `api` | dashboard HTTP/WebSocket requests through the account router (`Accounts.HandlerWithOrigins`) |
| `policy` | policy id | around each automation policy `Evaluate` and `recordDecision` |
| `opcode` | frame opcode | per ingested frame |

Not labelled: the Chromium transport (desktop only), goroutines a runtime starts later from an unlabelled goroutine, and the process-wide shared services.

## Reaching the listener

The listener lives inside the cell container's network namespace, so it is not visible on the VM's interfaces. From an IAP SSH session on the cell VM, run a short-lived container in the worker's network namespace:

```sh
gcloud compute ssh CELL_VM --zone ZONE --tunnel-through-iap

# on the VM: a 120 s CPU profile of the busiest period
sudo docker run --rm --network container:citadelops-cell -v /tmp:/out curlimages/curl \
  -sS -o /out/cell-cpu.pb.gz 'http://127.0.0.1:6060/debug/pprof/profile?seconds=120'
sudo docker run --rm --network container:citadelops-cell curlimages/curl \
  -sS http://127.0.0.1:6060/debug/runtime/metrics

# copy it off through the same tunnel
gcloud compute scp --tunnel-through-iap CELL_VM:/tmp/cell-cpu.pb.gz . --zone ZONE
```

Slice it locally (Go toolchain):

```sh
go tool pprof -tags cell-cpu.pb.gz                            # label values present
go tool pprof -tagroot=runtime,stage,policy -top cell-cpu.pb.gz
go tool pprof -tagfocus=runtime=RUNTIME_ID -tagfocus=stage=ingest -top cell-cpu.pb.gz
go tool pprof -tagfocus=opcode=gaa -top cell-cpu.pb.gz
```

Do not publish port 6060, add it to Caddy, or bind it to a non-loopback address; the worker refuses the latter.

## Enabling it on a cell

The variable must be present in the worker container's environment (`--env CITADEL_PPROF_ADDR=127.0.0.1:6060`). `deploy/gce-cell-startup.sh` in CitadelOpsBackend does not pass it through yet; that is deferred to the cell startup contract change, and until then it is not enabled on any cell. The hosted worker image is built in the private release repository, so a profile can only be taken on a cell running an image that contains this change.
