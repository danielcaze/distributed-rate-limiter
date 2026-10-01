# M2 comparison contract

Status: agreed in [issue #9](https://github.com/danielcaze/distributed-rate-limiter/issues/9). It extends the [M1 admission contract](m1-contract.md); anything not changed here keeps its M1 meaning. [ADR 0001](adr/0001-cluster-compatible-key-layout.md) records the key layout decision.

## Scope

M2 compares four admission algorithms under the same policy and the same workloads, and publishes the measurements. The service runs one algorithm at a time, chosen by configuration. The token bucket from M1 stays the default.

| Algorithm | Why it is in the comparison |
| --- | --- |
| Token bucket | M1 baseline. Continuous refill; the burst is bounded by the bucket size. |
| Fixed window | Cheapest state (one integer). Allows up to twice the limit around a window boundary. |
| Sliding window log | Exact count over the last period, with no boundary burst. Stores one entry per allowed call. |
| Sliding window counter | Two integers per key. Estimates the last period from the previous and current windows, with a bounded error that depends on how calls are spread. |

The fixed window boundary burst: with 10 per minute, 10 calls at 12:00:59.5 and 10 at 12:01:00.2 are all allowed, 20 in 0.7 s, because each clock minute has its own counter.

## Interface and admission path

The caller interface is unchanged: `limiter.Checker`, the `limiter.v1.Limiter/Check` gRPC method, and `POST /v1/check` over the HTTP gateway, with only `key` in the request. HTTP requests still reach the checker through the gateway's gRPC client, so HTTP and direct gRPC callers go through the same admission path for every algorithm. Each algorithm has its own `limiter.Checker` implementation and Lua script; startup wires exactly one, and only its script runs.

## Configuration

| Variable | Default | Meaning |
| --- | --- | --- |
| `ALGORITHM` | `token_bucket` | One of `token_bucket`, `fixed_window`, `sliding_window_log`, `sliding_window_counter`. Any other value fails at startup. |
| `LIMIT` | `2` | Positive integer: admissions allowed per period. |
| `PERIOD` | `2s` | Positive Go duration, such as `1m` or `500ms`. |

`CAPACITY` and `REFILL_TOKENS_PER_SECOND` are removed. The token bucket derives `capacity = LIMIT` and `refill = LIMIT / PERIOD` tokens per second, so all four algorithms allow the same average rate and the same largest burst. Any M1 capacity and refill pair can still be expressed: `LIMIT = capacity`, `PERIOD = capacity / refill`. The refill can now be fractional. The defaults reproduce M1's Compose policy (capacity 2, refill 1 per second), so the README `curl` sequence still gives allowed, allowed, denied, allowed. Go converts `PERIOD` to microseconds for the scripts. `PERIOD` is used rather than `WINDOW` because the token bucket has no window: for it, `PERIOD` is the time an empty bucket takes to fill.

## Response semantics

Every decision carries all four fields, allowed or denied, and reports the state after the decision.

- `allowed`: the admission decision.
- `remaining`: admissions left after this decision, from 0 to `LIMIT`. Never negative.
- `reset_at`: when the algorithm has fully restored capacity if no more calls arrive.
- `retry_after`: zero when allowed; otherwise the delay until the earliest possible admission.

| Algorithm | `reset_at` | `retry_after` on denial |
| --- | --- | --- |
| Token bucket | when the bucket would be full (M1) | time to refill one token (M1) |
| Fixed window | start of the next window | start of the next window minus now |
| Sliding window log | newest entry + `PERIOD` (on an allowed call, now + `PERIOD`) | oldest entry + `PERIOD` minus now |
| Sliding window counter | current window start + 2 × `PERIOD` | earliest instant the estimate drops below `LIMIT` (formula settled with the script) |

Fixed windows align to the clock: `window_start = floor(now / PERIOD) × PERIOD`. A first call at 12:00:50 still resets at 12:01:00, and a call exactly at a boundary counts in the new window.

## State layout and lifetime

Keys follow `rl:<algorithm>:<key>`. `rl` separates the rate limiter from anything else in a shared Redis; the algorithm segment keeps each algorithm's state apart, so switching algorithms never reads another's state. Window starts (`ws`) and timestamps are Unix microseconds.

Every script declares all the keys it touches in `KEYS`, so the layout is compatible with Redis Cluster, which routes a script to a node by its keys ([ADR 0001](adr/0001-cluster-compatible-key-layout.md)). Key names never depend on time. The sliding window log's two keys share the hash tag `{<key>}`: Redis Cluster hashes only the part inside the braces, so both keys land in the same slot.

| Algorithm | Keys | Type | Records denied calls |
| --- | --- | --- | --- |
| Token bucket | `rl:tb:<key>` | hash with `tokens` and `ts` | no |
| Fixed window | `rl:fw:<key>` | hash with `ws` and `count` | yes |
| Sliding window log | `rl:swl:{<key>}`, `rl:swl:{<key>}:seq` | sorted set, integer | no |
| Sliding window counter | `rl:swc:<key>` | hash with `ws`, `prev`, and `cur` | no |

- **Token bucket.** Moves from the bare key to `rl:tb:<key>`. Existing M1 bucket state is abandoned at deploy; buckets start full, so this grants at most one extra burst per key.
- **Fixed window.** The script compares the stored `ws` with the current window: in the same window it increments `count`; in a new window it writes the new `ws` and `count = 1`. Every call increments and is then compared with the limit; over-counting changes no decision, and the count resets when the next window starts.
- **Sliding window log.** Each check removes entries scored below `now - PERIOD`, counts the rest, and on `count < LIMIT` adds an entry and allows. The member is `<timestamp>:<seq>` with a per-key sequence that only grows, because `ZADD` overwrites an equal member: two allowed calls at the same microsecond would otherwise leave one entry and admit past the limit. Denied calls are not recorded; recording them would grow memory under abusive traffic and keep a client that keeps retrying locked out indefinitely.
- **Sliding window counter.** The script first rotates the stored window: same window, unchanged; exactly the next window, `prev = cur` and `cur = 0`; two or more windows later, both 0. It allows when `prev × (share of the previous window still inside the last PERIOD) + cur < LIMIT`. Denied calls are not counted, because the current count becomes the next window's previous term and counting denials would carry a lockout into the next window.

Every key expires at its algorithm's `reset_at` (`PEXPIREAT`), renewed on each write. A denial that writes nothing leaves the expiry valid, because `reset_at` did not change. A fixed window's `reset_at` is fixed for the whole window, so its expiry is set when a new window starts. A sliding window counter key expires at `ws + 2 × PERIOD`, since its current count is still read as the previous window during the next one. An expired key and a missing key produce the same decision. Expiry cleans up idle keys; it does not replace the algorithm.

## Validation and errors

The M1 error mapping is unchanged: a denial is gRPC `OK` with `allowed=false` (HTTP 429); a Redis outage is `Unavailable` (503) or `DeadlineExceeded` (504) and is never reported as a denial. Every script returns five integers: `allowed`, `remaining`, `reset_at`, `retry_after`, and `now`, the time the script used (Redis `TIME`, or the test-only override), in Unix microseconds. `now` exists only between the script and the Go checker, so the checker can validate the time fields against the script's own clock rather than its own, which may differ by clock skew. The checker drops it after validation; the `Decision`, the gRPC response, and the HTTP JSON keep their four fields. The Go checker validates every script reply before building a `Decision`: a five-element array of integers, `allowed` 0 or 1, `remaining >= 0`, `retry_after >= 0`, `retry_after = 0` when allowed, `reset_at >= now`, and `now + retry_after <= reset_at` (the next admission never comes after full capacity returns; this holds for all four algorithms). Any violation is gRPC `Internal` (HTTP 500) naming the broken rule, not a denial and not a silent clamp. Each script also keeps `remaining` from going negative. The tests additionally fix the time through `checkAt` and assert each algorithm's exact `reset_at`.

## Scenarios

Correctness tests run for every algorithm against real Redis, with 10 per minute unless stated:

1. Same-key contention: two checker instances send 1,000 concurrent calls for one key; exactly the limit is allowed.
2. Cross-key independence: exhausting one key does not affect another.
3. Window rollover: 10 calls at 12:00:59.5 and 10 at 12:01:00.2. Fixed window allows 20, sliding window log 10, sliding window counter about 10, token bucket 10 plus what refilled in 0.7 s.
4. Outage as error: a Redis failure is reported as HTTP 504 or 503, never as a denial.
5. Selection: each `ALGORITHM` value starts its checker, and an unknown value fails at startup.

## Benchmark and reporting

Workloads with k6 and ghz: steady load, bursty load, and load at window boundaries, including calls bunched at the start of a window, the sliding window counter's worst case. Each run reports allowed, denied, and error counts, p50 and p99 latency, Redis memory per key, and accuracy: the number of admissions above `LIMIT` in any `PERIOD`-long interval. Results state the tool versions, commands, configuration, concurrency, duration, key distribution, and machine. A result is evidence for that scenario, not a general ranking.

## Limitations

- Sliding window log memory grows with `LIMIT`: up to `LIMIT` entries per active key.
- The sliding window counter assumes calls in the previous window were evenly spread. Calls bunched at its start cause extra denials; calls bunched at its end admit a few extra. The benchmark measures this error.
- The fixed window admits up to twice `LIMIT` across a boundary.
- One policy applies to every key. Per-key policies, failure policy, retries, and deduplication stay out of M2.
- The key layout is compatible with Redis Cluster, but M2 runs on a single Redis and is not tested on a cluster.
