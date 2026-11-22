# Soak test: sustained load and memory growth

- **Date:** 2026-11-22
- **Raw output:** this folder: `k6-rate-5000.txt` (k6 summary), `stats-rate-5000.txt` (container
  CPU and memory, one sample per ~2 s with elapsed seconds), `state-rate-5000.txt` (container state
  at the end), `heap-480s.pprof` (heap profile at 480 s)
- **Reproduce:** `RATES=5000 DURATION=600s STOP_ON_FAIL=false HEAP_AT=480 loadtest/run.sh`
- **Related:** [load test](../2026-11-22-load-test/) (short steps, CPU capacity)

## Question

The [load test](../2026-11-22-load-test/) ran 30 s per step and found the CPU limit (~10,000
req/s on 2 CPUs). Storage is in memory and every `POST` adds a task that is never evicted, so:
does the service hold up under sustained load, how fast does memory grow, and what bounds a long
run?

This is a measurement record only; no optimization is proposed or decided.

## Setup

Same as the [load test](../2026-11-22-load-test/): one container with `--cpus=2 --memory=512m`,
k6 2.4.0 on the same host (WSL2), workload 70% `GET` / 20% `POST` / 10% `PUT`, same pass
criteria. Rate **5,000 req/s** (half the CPU capacity) for **600 s** after a 5 s warm-up, so about
1,000 tasks are created per second.

## Results

| Metric | Value |
|---|---|
| Requests | 3,000,001; **0 failed**, 0 dropped iterations |
| Latency | p50 0.41 ms, p95 0.56 ms, **p99 1.86 ms**, max 750 ms |
| Container CPU | ~70% for the first 7 min, ~83% afterwards (limit 200%) |
| Container memory | 124 MiB → **332 MiB** after 10 min (limit 512 MiB) |
| Container state | running, `OOMKilled=false`, no restarts |
| Pass criteria | **Passed** |

Latency did not degrade over the run.

### Memory over time

| Minute | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 |
|---|---|---|---|---|---|---|---|---|---|---|---|
| RSS (MiB) | 124 | 141 | 157 | 191 | 212 | 214 | 216 | 275 | 328 | 332 | 332 |

Memory grows in steps rather than linearly: the Go GC target scales with the live heap, so the
heap expands in jumps and then plateaus until the next GC cycle raises it again.

### What the memory holds (heap profile at 480 s, ~485,000 tasks)

| In use | Size | Share |
|---|---|---|
| Stored tasks (`TaskRepository.Create`: map entries and values) | 98.9 MB | 80% |
| Swagger UI assets held in memory by `swaggo/files` | 8.3 MB | 7% |
| Request buffers and JSON decoding (in-flight requests) | ~13 MB | 11% |
| **Live heap total** | **123.7 MB** | |
| Container RSS at the same time | ~328 MiB | ≈ 2.6× live heap |

- Each stored task costs about **205 bytes** (98.9 MB / ~485,000).
- RSS exceeds the live heap by ~2.6×: the default `GOGC=100` lets the heap reach about twice the
  live size before collecting, and freed memory is not immediately returned to the OS.

## Interpretation

- **Throughput is not the long-run limit.** At half the CPU capacity the service ran 10 minutes
  with flat latency and no errors.
- **The number of stored tasks is.** Only task data accumulates (~205 B per task); request garbage
  is reclaimed. At 1,000 creates per second the live heap grows by ~12 MB per minute.
- **Estimated time to the 512 MiB limit at this rate:** with RSS ≈ 2.6× live heap, the limit is
  reached at roughly 195 MB of live data, about 950,000 tasks, i.e. **~16 minutes** at 5,000 req/s
  with 20% writes. This is an extrapolation from the 10-minute run; the run was not continued to
  the limit.
- **Short high-rate steps overstate memory growth.** In the load test, 10,000 req/s reached
  217 MiB after 35 s with only ~70,000 tasks (~14 MB of data); most of that RSS was GC headroom
  for allocation churn, not stored tasks. Extrapolating such short runs linearly predicts the limit
  far too early.

## Capacity depends on stored tasks

The ~10,000 req/s capacity and the ~950,000-task memory bound were measured separately and do not
combine. Capacity is a function of both request rate and stored task count:

| Stored tasks | Request rate within pass criteria | Basis |
|---|---|---|
| Up to ~70,000 | ~10,000 req/s | Measured (load test, fresh container per step) |
| ~400,000–600,000 | 5,000 req/s passes; 10,000 req/s **likely not** | Measured at 5,000 req/s: CPU rose from ~70% to ~83% as the store grew; 10,000 req/s not measured |
| Near ~950,000 | **Not sustainable** at high rates | Inferred: live data alone reaches the memory limit, leaving no headroom for request allocations |

Why capacity falls as the store grows:

- **Per-request work does not grow.** A page walks IDs (O(limit)); create and update are O(1).
- **Memory headroom shrinks.** High request rates need transient allocation headroom (10,000
  req/s showed 217 MiB RSS with only ~14 MB of task data). Stored tasks consume the same 512 MiB,
  so a large store leaves less room before GC pressure or an OOM kill.
- **CPU per request rises.** A larger heap means more GC marking work and more cache misses on
  the map; observed as +13 points of CPU at the same 5,000 req/s between minutes 0–7 and 7–10.

Measuring capacity at a given store size would require preloading N tasks and then running the
step test; not done.

## System limits and bottlenecks (load test + soak test)

| Limit | Value on 2 CPUs / 512 MiB | Bottleneck |
|---|---|---|
| Request rate within p99 < 50 ms, small store (≤ ~70,000 tasks) | ~10,000 req/s | CPU: network syscalls (~28%), access logging (~16%), JSON (~13%); not the repository or its lock |
| Request rate, large store | Lower; 5,000 req/s sustained with ~600,000 tasks | Memory headroom and GC work grow with the store |
| Behavior past the CPU limit | Queueing, then connections closed after the 10 s `WriteTimeout`; throughput collapses | No admission control |
| Sustained run | Bounded by stored task count, not rate: ~950,000 tasks (~16 min at 1,000 creates/s) | In-memory storage never evicts; RSS ≈ 2.6× live heap |
| Fixed memory cost | ~8 MB for Swagger UI when `SWAGGER_ENABLED=true` | `swaggo/files` keeps assets in memory |
