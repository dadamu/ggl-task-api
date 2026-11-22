// k6 load test for the Task API: a fixed request rate with a mixed workload.
//
//   k6 run -e RATE=2000 -e DURATION=30s -e BASE_URL=http://localhost:8080 loadtest/tasks.js
//
// DURATION must be given in seconds (e.g. 30s).
// GET_PAGE=first (default) always lists the first page; GET_PAGE=last lists the newest tasks by
// passing after = (highest ID this VU has seen) - 20.
// Mix per iteration: 70% GET /tasks, 20% POST /tasks, 10% PUT /tasks/{id}.
// Each VU updates only tasks it created, so 409 conflicts are rare; a 409 is
// still treated as an expected outcome of optimistic locking, not a failure.
import http from 'k6/http';
import { check } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const RATE = Number(__ENV.RATE || 1000);
const DURATION = __ENV.DURATION || '30s';
const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } };
const GET_PAGE = __ENV.GET_PAGE || 'first';
const PAGE_SIZE = 20;

http.setResponseCallback(http.expectedStatuses({ min: 200, max: 299 }, 409));

const WARMUP = '5s';

// Planned iterations in the measured phase, for the dropped-iteration budget.
const DURATION_SECONDS = Number(DURATION.replace(/s$/, ''));
const PLANNED = RATE * DURATION_SECONDS;
const arrival = {
  executor: 'constant-arrival-rate',
  rate: RATE,
  timeUnit: '1s',
  // Generous pre-allocation: allocating VUs mid-test drops iterations during latency spikes.
  preAllocatedVUs: Math.max(100, Math.ceil(RATE / 5)),
  maxVUs: Math.max(200, RATE),
};

export const options = {
  scenarios: {
    // Connection setup and VU start-up inflate the first seconds; excluded from thresholds.
    warmup: { ...arrival, duration: WARMUP },
    mixed: { ...arrival, duration: DURATION, startTime: WARMUP },
  },
  // Measured on the "mixed" scenario only. Two kinds of threshold:
  //  - pass criteria: the step fails but runs to completion, so its numbers are complete;
  //  - collapse guards (abortOnFail): stop the step early once the service is clearly overloaded.
  thresholds: {
    'http_req_failed{scenario:mixed}': [
      'rate<0.001',
      { threshold: 'rate<0.05', abortOnFail: true, delayAbortEval: '5s' },
    ],
    'http_req_duration{scenario:mixed}': [
      'p(99)<50',
      { threshold: 'p(99)<1000', abortOnFail: true, delayAbortEval: '5s' },
    ],
    // Dropped iterations are requests k6 could not start on time; up to 1% is tolerated so a
    // single latency spike near capacity does not fail a step that kept up overall.
    'dropped_iterations{scenario:mixed}': [`count<${Math.ceil(PLANNED * 0.01)}`],
  },
  summaryTrendStats: ['avg', 'p(50)', 'p(95)', 'p(99)', 'max'],
};

// Per-VU state: the last task this VU created and its current version, and the
// highest task ID this VU has seen (approximates the newest task for GET_PAGE=last).
let mine = null;
let maxSeenID = 0;

function seen(id) {
  if (id > maxSeenID) maxSeenID = id;
}

function list() {
  const after = GET_PAGE === 'last' ? Math.max(0, maxSeenID - PAGE_SIZE) : 0;
  const url = after > 0 ? `${BASE_URL}/tasks?after=${after}` : `${BASE_URL}/tasks`;
  const res = http.get(url, { tags: { name: 'GET /tasks' } });
  // PARSE_GET=1 parses the body in first-page mode too: a control for client-side parsing cost.
  if (check(res, { 'GET 200': (x) => x.status === 200 }) && (GET_PAGE === 'last' || __ENV.PARSE_GET === '1')) {
    const data = res.json().data;
    if (data.length > 0) seen(data[data.length - 1].id);
  }
}

function create() {
  const res = http.post(`${BASE_URL}/tasks`, JSON.stringify({ name: `load ${__VU}-${__ITER}` }), {
    ...JSON_HEADERS,
    tags: { name: 'POST /tasks' },
  });
  if (check(res, { 'POST 201': (r) => r.status === 201 })) {
    const t = res.json();
    mine = { id: t.id, version: t.version };
    seen(t.id);
  }
}

function update() {
  if (!mine) {
    create();
    return;
  }
  const res = http.put(
    `${BASE_URL}/tasks/${mine.id}`,
    JSON.stringify({ name: `updated ${__ITER}`, status: __ITER % 2, version: mine.version }),
    { ...JSON_HEADERS, tags: { name: 'PUT /tasks/{id}' } },
  );
  check(res, { 'PUT 200 or 409': (r) => r.status === 200 || r.status === 409 });
  if (res.status === 200) {
    mine.version = res.json().version;
  } else if (res.status === 409) {
    mine.version = res.json().error.current.version;
  }
}

export default function () {
  const r = Math.random();
  if (r < 0.7) {
    list();
  } else if (r < 0.9) {
    create();
  } else {
    update();
  }
}
