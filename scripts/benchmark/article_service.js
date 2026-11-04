import http from "k6/http";
import { check } from "k6";
import { Counter, Rate, Trend } from "k6/metrics";

const baseURL = __ENV.BASE_URL || "http://localhost:8080";
const dominantAuthorName = __ENV.DOMINANT_AUTHOR || "Benchmark Author 0001";
const longTailAuthorName = __ENV.LONG_TAIL_AUTHOR || "Benchmark Author 0002";
const writeAuthorID = Number(__ENV.WRITE_AUTHOR_ID || 1);

export const options = {
  summaryTrendStats: ["avg", "min", "med", "max", "p(90)", "p(95)", "p(99)"],
  scenarios: {
    listing: { executor: "constant-arrival-rate", rate: 2, timeUnit: "1s", duration: "30s", preAllocatedVUs: 2, exec: "listing" },
    dominant_author: { executor: "constant-arrival-rate", rate: 2, timeUnit: "1s", duration: "30s", preAllocatedVUs: 2, exec: "dominantAuthor" },
    long_tail_author: { executor: "constant-arrival-rate", rate: 2, timeUnit: "1s", duration: "30s", preAllocatedVUs: 2, exec: "longTailAuthor" },
    keyword: { executor: "constant-arrival-rate", rate: 2, timeUnit: "1s", duration: "30s", preAllocatedVUs: 2, exec: "keyword" },
    combined: { executor: "constant-arrival-rate", rate: 2, timeUnit: "1s", duration: "30s", preAllocatedVUs: 2, exec: "combined" },
    cache_hit: { executor: "constant-arrival-rate", rate: 4, timeUnit: "1s", duration: "30s", preAllocatedVUs: 2, exec: "cacheHit" },
    cache_miss: { executor: "constant-arrival-rate", rate: 2, timeUnit: "1s", duration: "30s", preAllocatedVUs: 2, exec: "cacheMiss" },
    bursty_reads: { executor: "ramping-arrival-rate", startRate: 1, timeUnit: "1s", preAllocatedVUs: 4, stages: [{ target: 20, duration: "5s" }, { target: 20, duration: "10s" }, { target: 1, duration: "5s" }], exec: "burstyReads" },
    mixed_read_write: { executor: "constant-arrival-rate", rate: 2, timeUnit: "1s", duration: "30s", preAllocatedVUs: 2, exec: "mixedReadWrite" },
  },
  thresholds: { http_req_failed: ["rate<0.01"] },
};

const errors = new Rate("benchmark_errors");
const searchLatency = new Trend("benchmark_search_latency", true);
const requests = new Counter("benchmark_requests");

function get(path, isSearch = false) {
  const response = http.get(`${baseURL}${path}`, { tags: { endpoint: path.split("?")[0] } });
  requests.add(1);
  errors.add(response.status >= 400);
  if (isSearch) searchLatency.add(response.timings.duration);
  check(response, { "status is successful": (r) => r.status >= 200 && r.status < 400 });
  return response;
}

export function listing() { get("/articles?limit=20"); }
export function dominantAuthor() { get(`/articles?author=${encodeURIComponent(dominantAuthorName)}&limit=20`); }
export function longTailAuthor() { get(`/articles?author=${encodeURIComponent(longTailAuthorName)}&limit=20`); }
export function keyword() { get("/articles?query=benchmark%20keyword&limit=20", true); }
export function combined() { get(`/articles?query=benchmark%20keyword&author=${encodeURIComponent(dominantAuthorName)}&limit=20`, true); }
export function cacheHit() { get("/articles?query=benchmark%20keyword&limit=20", true); }
export function cacheMiss() { get(`/articles?query=benchmark%20keyword%20${__ITER}&limit=20`, true); }
export function burstyReads() { get("/articles?limit=20"); }

export function mixedReadWrite() {
  if (__ITER % 100 === 0) {
    const body = JSON.stringify({ author_id: writeAuthorID, title: `benchmark write ${Date.now()}`, body: "benchmark write body" });
    const response = http.post(`${baseURL}/articles`, body, { headers: { "Content-Type": "application/json" } });
    requests.add(1);
    errors.add(response.status >= 400);
    check(response, { "write is successful": (r) => r.status >= 200 && r.status < 400 });
    return;
  }
  listing();
}

export function handleSummary(data) {
  return { stdout: textSummary(data) };
}

function textSummary(data) {
  const metric = data.metrics.http_req_duration;
  const failed = data.metrics.http_req_failed;
  return `p50=${metric.values.med}ms p95=${metric.values["p(95)"]}ms p99=${metric.values["p(99)"]}ms throughput=${data.metrics.http_reqs.values.rate}/s error_rate=${failed.values.rate}\n`;
}
