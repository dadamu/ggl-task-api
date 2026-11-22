# Load test: capacity on 2 CPUs

- **Date:** 2026-11-22
- **Raw output:** this folder: `k6-rate-*.txt` (k6 summaries), `stats-rate-*.txt` (container CPU and
  memory samples), `cpu-rate-20000.pprof` (CPU profile at 20,000 req/s)
- **Reproduce:** `make loadtest` (or `RATES="..." PROFILE_RATES="..." loadtest/run.sh`); page
  comparison: `GET_PAGE=last RATES=10000 STOP_ON_FAIL=false loadtest/run.sh`

## Question

How much traffic can one instance handle within the latency target, which resource runs out
first, and how does the service fail beyond that point?

## Setup

| | |
|---|---|
| API | Docker image from this repo, `--cpus=2 --memory=512m`, `GIN_MODE=release`, pprof on a separate port |
| Load generator | k6 2.4.0 in Docker on the same host (`--network host`), not CPU-limited |
| Host | AMD Ryzen 7 9700X, 16 threads, Linux (WSL2); Go 1.27.4 |
| Workload | Constant arrival rate; 70% `GET /tasks` (page of 20), 20% `POST /tasks`, 10% `PUT /tasks/{id}` on tasks created by the same VU |
| Steps | 5 s warm-up (excluded) + 30 s measured per rate; a fresh container per rate |
| Pass criteria | p99 < 50 ms, failed requests < 0.1%, dropped iterations < 1% of planned; `409` counts as expected |
| Early stop | A step aborts once failures exceed 5% or p99 exceeds 1 s; the run stops at the first failing step (`STOP_ON_FAIL=false` to continue) |

The load generator shares the host, and Docker on WSL2 adds a userland network hop, so absolute
numbers are a lower bound for a dedicated machine. The shape of the curve is the useful part.

## Results

| Target rate | Achieved | p50 | p95 | p99 | Failed | Container CPU (limit 200%) | Memory | Pass |
|---|---|---|---|---|---|---|---|---|
| 1,000/s | 1,000/s | 0.38 ms | 0.49 ms | 0.62 ms | 0% | 20% | 44 MiB | ✅ |
| 5,000/s | 5,000/s | 0.41 ms | 0.52 ms | 0.88 ms | 0% | 81% | 124 MiB | ✅ |
| **10,000/s** | **10,000/s** | 0.43 ms | 1.0 ms | **19.5 ms** | 0% | 138% | 217 MiB | ✅ |
| 15,000/s | 14,600/s | 0.61 ms | 26 ms | 131 ms | 0% | 180% | 307 MiB | ❌ p99 |
| 20,000/s | 19,000/s | 2.6 ms | 137 ms | 398 ms | 0% | 205% | 454 MiB | ❌ p99 |
| 25,000/s | 11,400/s | 122 ms | 3.2 s | 15.8 s | 4.8% | 210% | 512 MiB | ❌ |
| 30,000/s | 2,700/s | 3.6 s | 14 s | 24 s | 47% | 109% | 512 MiB | ❌ |

"Achieved" includes the warm-up seconds; dropped iterations make it lower than the target.
This sweep ran with `STOP_ON_FAIL=false` and before the early-stop guards existed, so every step
ran its full 30 s.

**Run-to-run variance at 10,000/s.** Five runs: p99 16–34 ms in all of them; four dropped no
iterations and one dropped 0.25% (a 600 ms latency spike briefly exhausted k6's VUs). A
zero-dropped criterion therefore flapped near capacity, which is why the criterion tolerates
up to 1%.

**Capacity: about 10,000 req/s on 2 CPUs within p99 < 50 ms** (passed 5 of 5 runs under the
1% dropped budget), roughly 5,000 req/s per core for
this mix. The knee is between 10,000 and 15,000 req/s.

This holds for a **small data set**: every step started from an empty store and accumulated at
most ~70,000 tasks. Capacity with a large store was not measured; the
[soak test](../2026-11-22-soak-test/) shows it is lower (see "Capacity depends on stored tasks"
there).

## First page vs last page

In the sweep above every `GET /tasks` requested the first page (IDs 1–20): no cursor, and no
deletions in the mix. That is the most favorable case (no deleted IDs to skip, the same tasks hot in
CPU caches). To check, the 10,000 req/s step was repeated with `GET_PAGE=last`: each VU tracks the
highest ID it has seen and requests `after = highest - 20`, so 91% of GETs hit the newest region of
a growing store. Because that mode parses GET responses in k6, a control run parsed them in
first-page mode too (`PARSE_GET=1`).

| Mode (10,000 req/s, 2 runs each) | p50 | p95 | p99 | Peak CPU | Pass |
|---|---|---|---|---|---|
| First page | 427–428 µs | 1.05–1.18 ms | 18.9–23.6 ms | 136–149% | ✅ ✅ |
| First page + k6 parses body (control) | 426–431 µs | 1.80–1.89 ms | 16.7–19.1 ms | 135–142% | ✅ ✅ |
| Last page | 440 µs | 3.54–4.26 ms | 19.0–19.3 ms | 140–146% | ✅ ✅ |

Raw output: `k6-page-*.txt`, `stats-page-*.txt` in this folder.

- **Capacity is unchanged:** p50, p99, CPU and the pass result are the same for both pages.
- **p95 is higher for the last page.** About 0.7 ms of the difference is client-side (k6 parsing
  responses on the shared host, per the control). The remaining ~2 ms is **not explained**:
  server CPU is unchanged, the repository uses one lock for every page, and the
  [list benchmark](../2026-11-22-list-pagination/) shows equal in-process cost for first and
  middle pages. Not investigated further.
- The sparse worst case (many deleted IDs before the cursor) was not exercised under load; it was
  measured only in-process (2.3 ms per page with 99.98% of 100,000 IDs deleted).

## Where the CPU goes (profile at 20,000 req/s)

| Share of CPU | Work |
|---|---|
| ~28% | Network I/O syscalls: writing responses (`net.(*conn).Write`, 21%) and reading requests (6%) |
| ~16% | **Access logging**: `gin.Logger` formatting (`fmt.Fprint`, 8%) and writing to stdout (`os.(*File).Write`, 8%) |
| ~13% | JSON: encoding responses (~10%) and decoding request bodies (~3%) |
| ~4% | Garbage collection (background marking and allocation assists) |
| ~1.6% | Repository `List` |
| not visible | `sync.RWMutex` contention |

Application logic and the repository lock are not bottlenecks, which confirms the
[ADR-0002](../../adr/0002-in-memory-storage.md) assumption that a single `RWMutex` is not the
limit. The cost is HTTP plumbing: syscalls, logging and serialization.

## How it fails

1. **Up to ~10,000/s:** latency flat; CPU grows linearly.
2. **15,000–20,000/s:** CPU saturates at the 2-CPU limit; requests queue, so p95/p99 grow by
   orders of magnitude while p50 stays low. No errors yet.
3. **Above ~20,000/s:** queues grow without bound. Requests waiting longer than the 10 s
   `WriteTimeout` ([ADR-0008](../../adr/0008-runtime-and-deployment.md)) have their connections
   closed, which clients see as `connection reset by peer`. Throughput collapses (11,400/s at
   25,000/s offered, 2,700/s at 30,000/s) because time is spent on requests that end up failing.

The container was **not** OOM-killed (verified with `docker inspect`: `OOMKilled=false`, no
restart), but memory reached the 512 MiB limit. Two causes: tasks accumulate (20% of requests
create one), and the Go runtime does not know the container limit, so the heap grows with the
default `GOGC=100` headroom.

## Findings and options

| Finding | Option | Expected effect |
|---|---|---|
| Access logging is ~16% of CPU | Buffered or sampled logging, or structured logging to an async writer | Roughly +15–20% throughput at saturation |
| Go is unaware of the 512 MiB limit | Set `GOMEMLIMIT` (e.g. 450 MiB) in the image | GC works harder near the limit instead of risking an OOM kill |
| Overload degrades everyone, then resets connections after 10 s | Admission control: cap in-flight requests and return `503` immediately when full | Fails fast and keeps latency bounded for accepted requests |
| Memory grows with every task | Inherent to in-memory storage | Bounded only by a persistent store or a task limit |

None of these are implemented. For the expected load of this service (well under 1,000 req/s),
one 2-CPU instance has about 10× headroom.

## Revisit when

- Expected traffic approaches a few thousand requests per second per instance.
- The workload changes (e.g. larger pages, filtering) or the storage moves to a database, which
  shifts the bottleneck away from HTTP plumbing.
- Measuring on dedicated hardware, to remove the shared-host and WSL2 network effects.
