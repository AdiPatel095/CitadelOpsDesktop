# Hosted placement load

The hosted orchestrator samples worker process load every 10 seconds. It keeps
at most 60 successful CPU intervals (10 minutes at the normal cadence),
independently of requests to status or diagnostics. The sampler starts only with
the hosted orchestrator; the Windows desktop lane does not run it.

An optional `load` object appears in `/orchestrator/v1/status`, control-fence and
reconcile responses, status events, and `/orchestrator/v1/diagnostics`. It is
omitted until the first CPU interval exists, normally 10 seconds after startup.
Older workers omit it as well. Failed CPU reads and zero-length intervals add
no sample; after a failed read, the next successful read establishes a new
baseline. The reported timestamp remains at the latest successful interval.

| Field | Meaning |
| --- | --- |
| `cpuMeanShare` | Arithmetic mean of the retained process CPU shares. |
| `cpuP95Share` | Nearest-rank 95th percentile: sorted index `ceil(0.95 * n) - 1`. |
| `cpuWindowSeconds` | Sum of retained interval durations in seconds. |
| `cpuSamples` | Number of retained CPU intervals, up to 60. |
| `cpuCores` | `GOMAXPROCS` used as the CPU capacity for the newest interval. |
| `postGcHeapBytes` | Latest `/gc/heap/live:bytes`, live heap after the last GC. |
| `memoryLimitBytes` | Latest `/gc/gomemlimit:bytes`; values at least `MaxInt64` are unset and reported as 0. |
| `observedAt` | UTC timestamp of the newest successful interval. |

Each interval's share is cumulative process user+system CPU seconds consumed
divided by elapsed monotonic wall seconds and CPU cores. A share of 0.70 means
70% of that capacity. CPU covers the worker process and all its runtimes; it
excludes Caddy and the OS and can read below VM utilization. Memory is read with
`runtime/metrics` without `ReadMemStats` or a forced GC. The object contains no
account identifiers, paths, labels or payloads.

The paired CIT-56 backend guard uses this object to withhold new placements
when mean CPU exceeds 0.70, p95 exceeds 0.90, or post-GC heap exceeds 1.5 GiB.
Absent, insufficient (fewer than six samples), stale (over two minutes), or
invalid load uses the configured count limit. Headroom deferral does not
provision a VM or stop, move or drain existing runtimes. The backend guard can
be disabled with `HOSTED_CELL_LOAD_GUARD=off`. These are backend behavior from
the paired plan, not changes implemented by the desktop sampler.

The signal can lag placements by up to its 10-minute window. The runtime count
limit remains the hard bound. CIT-57 stop conditions use Cloud Monitoring VM
utilization, which includes work outside this process.
