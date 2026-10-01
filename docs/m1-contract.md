# M1 admission contract

This records the M1 admission contract defined in [issue #1](https://github.com/danielcaze/distributed-rate-limiter/issues/1) and implemented through [PR #7](https://github.com/danielcaze/distributed-rate-limiter/pull/7). [Issue #6](https://github.com/danielcaze/distributed-rate-limiter/issues/6) records the verification evidence.

## Caller and wire interface

The Go seam is [`limiter.Checker`](../limiter/contract.go):

```go
Check(ctx context.Context, key string) (Decision, error)
```

`Decision` contains `Allowed bool`, `Remaining uint64`, `ResetAt time.Time`, and `RetryAfter time.Duration`. The gRPC method is `limiter.v1.Limiter/Check`; its request contains only `key`, and its response contains the corresponding `allowed`, `remaining`, `reset_at`, and `retry_after` fields. The policy belongs to environment configuration, not to each request. A request costs one admission. There is no public capacity, refill rate, request ID, or clock input.

`key` is a nonempty, opaque bucket identifier supplied by the trusted backend. The backend derives identity from a verified session. The M1 `curl` flow simulates that backend; it does not establish public authentication. A user must not be allowed to choose their own identity. Authentication and access enforcement are outside M1.

Each key has a separate bucket that starts full under the one shared policy. Refill is continuous. An allowed request consumes one admission; a denied request consumes none. A decision reports the state **after** that check. `remaining` is the count of whole admissions possible immediately, even when the internal bucket holds a fraction. Thus an allowed last admission can return `allowed=true, remaining=0`; the next check can return `allowed=false, remaining=0`.

`reset_at` is the absolute UTC time when this bucket would be full if no more requests arrive. If already full, it is the decision time. `retry_after` is the nonnegative time until the earliest possible next admission after a denial; it is zero when allowed. The wire uses standard protobuf [`Timestamp`](https://protobuf.dev/reference/protobuf/google.protobuf/#timestamp) and [`Duration`](https://protobuf.dev/reference/protobuf/google.protobuf/#duration) so the absolute instant and elapsed interval have distinct types. Go uses `time.Time` and `time.Duration` for the same distinction. Subsecond values are supported on the wire; no rounding to whole seconds is part of the contract.

## Decision and error rules

Quota denial is a successful check: gRPC `OK` with `allowed=false`, HTTP 429. A malformed request, including an empty key, is gRPC `InvalidArgument`, HTTP 400. A Redis connection failure is gRPC `Unavailable`, HTTP 503; an expired deadline is gRPC `DeadlineExceeded`, HTTP 504. These failures leave admission **unknown** to the caller. They cannot be represented as `allowed=false` or used to infer that no token was consumed. Other unexpected failures, including a script reply of an unexpected type, become gRPC `Internal` and HTTP 500. A `grpcserver.Server` built without a `Checker` returns gRPC `Unimplemented` and HTTP 501; the production binary always wires the Redis checker, so only tests exercise that path. No retry, idempotency, or failure policy is defined in M1.

## Redis Lua boundary

[`redischeck/admission.lua`](../redischeck/admission.lua) receives the bucket key as `KEYS[1]`, the shared capacity as `ARGV[1]`, and the shared refill rate as `ARGV[2]`. In production it reads time from Redis `TIME`. The optional `ARGV[3]` supplies a fixed time in Unix microseconds; only the unexported `checkAt` test path passes it, and neither the public request nor the production path can set it. The bucket is a Redis hash with `tokens` and `ts` (the last write time, in microseconds). A missing key is a full bucket, and only an allowed check writes the hash. The keys have no expiry in M1. The script returns `allowed` (0 or 1), `remaining`, `reset_at` in Unix microseconds, and `retry_after` in microseconds.

Its invariants are atomic admission across instances sharing Redis, initially full independent buckets, one admission consumed on success, none on denial, continuous refill up to capacity, and the response meanings above. Redis runs each script call atomically, so concurrent checks for one key cannot interleave.

The checker loads the script with `SCRIPT LOAD` at startup and calls it with `EVALSHA`. If Redis answers `NOSCRIPT`, for example after a restart that cleared the script cache, the checker loads the script again and repeats the call once. That repeat is safe because Redis did not run the script on the failed call.

## Regeneration and wiring

[`proto/limiter/v1/limiter.proto`](../proto/limiter/v1/limiter.proto) is the authored source. The checked-in [`gen/limiter/v1/limiter.pb.go`](../gen/limiter/v1/limiter.pb.go) and [`limiter_grpc.pb.go`](../gen/limiter/v1/limiter_grpc.pb.go) are outputs from `protoc` and its Go plugins; do not edit them by hand. From the repository root on Windows with Go and PowerShell, run:

```powershell
./scripts/generate.ps1
go build ./...
go vet ./...
```

The script downloads `protoc` 29.3 for Windows x64 and checks its SHA-256, installs `protoc-gen-go` v1.36.6 and `protoc-gen-go-grpc` v1.5.1 into ignored `.tools/bin`, then regenerates both files. The module pins `grpc-go` v1.71.0 and protobuf v1.36.6. Repeating generation should leave the generated files unchanged. The compiler download and plugin cache are local to this project; no global tool installation is required. This follows the [protobuf Go generation guide](https://protobuf.dev/reference/go/go-generated/#compiler-invocation), [official compiler installation guidance](https://github.com/protocolbuffers/protobuf#protobuf-compiler-installation), and [gRPC Go quick start](https://grpc.io/docs/languages/go/quickstart/#regenerate-grpc-code). The [gRPC status code reference](https://grpc.io/docs/guides/status-codes/) defines the error codes used above.

The HTTP gateway uses `protojson` to decode only the generated `CheckRequest` fields and encode `CheckResponse`, preserving standard protobuf `Timestamp` and `Duration` JSON values. Unknown fields, including any clock override, are rejected. The gateway invokes a generated gRPC client connected to the loopback TCP listener of the actual gRPC server. `grpcserver.Server` accepts an injected `limiter.Checker`, so gateway tests can provide a fixed Checker without bypassing the HTTP to gRPC path.

## Runtime settings and health

`config.Load` reads environment variables. All settings are validated before listeners start. The defaults are shown in [`.env.example`](../.env.example); Compose supplies its own container addresses. Address values must be `host:port` with a port in 1–65535. `GRPC_ADDR` must have a loopback host.

| Variable | Default | Meaning |
| --- | --- | --- |
| `HTTP_ADDR` | `127.0.0.1:8080` | HTTP listener; Compose uses `0.0.0.0:8080` inside the container and publishes only to host loopback. |
| `GRPC_ADDR` | `127.0.0.1:9090` | Private loopback gRPC listener. |
| `REDIS_ADDR` | `127.0.0.1:6379` | Redis TCP address. |
| `CAPACITY` | `2` | Positive integer maximum admissions per bucket. Replaced in M2. |
| `REFILL_TOKENS_PER_SECOND` | `1` | Positive integer continuous refill rate, in tokens per second. Replaced in M2. |
| `REQUEST_TIMEOUT` | `3s` | Positive Go duration for Redis startup/readiness PING and each HTTP check's gRPC call. |
| `SHUTDOWN_TIMEOUT` | `5s` | Positive Go duration bounding graceful HTTP and gRPC shutdown. |

M2 replaces `CAPACITY` and `REFILL_TOKENS_PER_SECOND` with `ALGORITHM`, `LIMIT`, and `PERIOD`; the token bucket reads them as capacity `LIMIT` refilling `LIMIT` per `PERIOD`. See the [M2 contract](m2-contract.md#configuration).

Redis is pinged at startup and on each readiness request. `/healthz` reports only process liveness; `/readyz` returns HTTP 200 when Redis answers a PING and HTTP 503 otherwise. A ready process does not mean a particular check will be admitted. Each check sends a Redis PING before `EVALSHA`, so an outage maps to `Unavailable` or `DeadlineExceeded` before the script runs. The adapter uses go-redis `MaxRetries: -1`, which disables its command retries; the loopback gRPC client disables configured retries. gRPC may transparently retry calls that the server did not process. There is no application retry of an uncertain admission.

## Tests

The `redischeck` tests run the checked-in script against a real Redis started by Testcontainers. [`checker_clock_test.go`](../redischeck/checker_clock_test.go) covers independent full buckets, the exact post-decision `remaining` count, subsecond `reset_at` and `retry_after`, continuous refill with no consumption on denial, Redis `TIME` in production alongside the test-only time override, and a closed Redis client reported as `Unavailable` rather than as a denial. [`checker_contention_test.go`](../redischeck/checker_contention_test.go) sends 1,000 concurrent checks for one key through two checker instances with capacity 100, and passes only with 100 allowed, 900 denied, no errors, and each `remaining` value from 0 to 99 seen exactly once. The [gateway tests](../transport/httpgateway/handler_test.go) use a fixed checker to cover allowed and denied status mapping, a missing key, a clock field in the request, deadline and outage mapping, the `Unimplemented` path, and the health endpoints. [Issue #6](https://github.com/danielcaze/distributed-rate-limiter/issues/6) records the allowed, allowed, denied, allowed-after-refill `curl` sequence through Compose.
