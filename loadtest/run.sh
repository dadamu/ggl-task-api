#!/usr/bin/env bash
# Step load test: for each rate, start a fresh API container with fixed resources,
# run k6 at that constant rate, and keep the k6 summary.
#
#   loadtest/run.sh                       # default rates
#   RATES="1000 5000" loadtest/run.sh     # custom rates
#
# Env: IMAGE (ggl-task-api:local), CPUS (2), MEMORY (512m), DURATION (30s),
#      RATES, OUT (loadtest/results),
#      PROFILE_RATES (rates at which to capture a 15s pprof CPU profile; default none),
#      STOP_ON_FAIL (true: stop at the first rate that misses the pass criteria),
#      HEAP_AT (seconds after start at which to capture a heap profile; default none).
set -euo pipefail

IMAGE=${IMAGE:-ggl-task-api:local}
CPUS=${CPUS:-2}
MEMORY=${MEMORY:-512m}
DURATION=${DURATION:-30s}
RATES=${RATES:-"1000 2500 5000 7500 10000 15000"}
OUT=${OUT:-loadtest/results}
PROFILE_RATES=${PROFILE_RATES:-}
STOP_ON_FAIL=${STOP_ON_FAIL:-true}
HEAP_AT=${HEAP_AT:-}
K6_IMAGE=grafana/k6:2.4.0
NAME=ggl-loadtest
PORT=18080

mkdir -p "$OUT"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

for rate in $RATES; do
  cleanup
  docker run -d --name "$NAME" --cpus="$CPUS" --memory="$MEMORY" \
    -p "$PORT:8080" -p 16060:6060 \
    -e PPROF_ENABLED=true -e PPROF_ADDR=:6060 \
    "$IMAGE" >/dev/null
  curl -fsS --retry 20 --retry-all-errors --retry-delay 1 -o /dev/null "http://localhost:$PORT/healthz"

  echo "=== rate=${rate}/s duration=${DURATION} cpus=${CPUS} memory=${MEMORY}"

  # Sample container CPU and memory once per second while k6 runs: "<elapsed s> <cpu%> <mem used> / <limit>".
  start=$(date +%s)
  (while line=$(docker stats --no-stream --format '{{.CPUPerc}} {{.MemUsage}}' "$NAME" 2>/dev/null); do
    echo "$(($(date +%s) - start)) $line"
    sleep 1
  done) >"$OUT/stats-$rate.txt" &
  sampler=$!

  heaper=
  if [[ -n "$HEAP_AT" ]]; then
    (sleep "$HEAP_AT" && curl -fsS -o "$OUT/heap-$rate.pprof" "http://localhost:16060/debug/pprof/heap") &
    heaper=$!
  fi

  profiler=
  if [[ " $PROFILE_RATES " == *" $rate "* ]]; then
    (sleep 15 && curl -fsS -o "$OUT/cpu-$rate.pprof" "http://localhost:16060/debug/pprof/profile?seconds=15") &
    profiler=$!
  fi

  status=0
  docker run --rm --network host -v "$PWD/loadtest:/scripts:ro" "$K6_IMAGE" run --quiet \
    -e BASE_URL="http://localhost:$PORT" -e RATE="$rate" -e DURATION="$DURATION" \
    /scripts/tasks.js >"$OUT/rate-$rate.txt" 2>&1 || status=$?
  [[ -n "$profiler" ]] && { wait "$profiler" || true; }
  [[ -n "$heaper" ]] && { kill "$heaper" 2>/dev/null || true; wait "$heaper" 2>/dev/null || true; }
  kill "$sampler" 2>/dev/null || true

  # Container state before it is removed: was it OOM-killed or restarted?
  docker inspect -f 'container running={{.State.Running}} oomkilled={{.State.OOMKilled}} exit={{.State.ExitCode}} restarts={{.RestartCount}}' \
    "$NAME" | tee "$OUT/state-$rate.txt"

  grep -E "scenario:mixed" "$OUT/rate-$rate.txt" | sed 's/^ *//' || true
  awk -v cpus="$CPUS" '{gsub("%", "", $2); if ($2 + 0 > cpu) cpu = $2 + 0; mem = $3}
    END {printf "container peak cpu=%.0f%% (limit %d00%%) mem=%s\n", cpu, cpus, mem}' "$OUT/stats-$rate.txt"

  # k6 exit codes: 0 = all thresholds passed, 99 = thresholds failed (also when aborted early).
  case $status in
    0) echo "result: PASS" ;;
    99)
      grep -q 'stopping test prematurely' "$OUT/rate-$rate.txt" && echo "result: FAIL (aborted early: service collapsed)" || echo "result: FAIL"
      if [[ $STOP_ON_FAIL == true ]]; then
        echo "stopping: rate ${rate}/s missed the pass criteria (set STOP_ON_FAIL=false to continue)"
        break
      fi
      ;;
    *) echo "result: ERROR (k6 exit $status)"; exit "$status" ;;
  esac
done
