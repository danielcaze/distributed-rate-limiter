# M1 admission contract

This is the contract slice of [issue #1](https://github.com/danielcaze/distributed-rate-limiter/issues/1). It defines the caller interface and wire shape. The service, HTTP gateway, Redis connection, Lua, configuration, Compose setup, and integration tests are later work.

## Caller and wire interface

The Go seam is [`limiter.Checker`](../limiter/contract.go):

```go
Check(ctx context.Context, key string) (Decision, error)
```

`Decision` contains `Allowed bool`, `Remaining uint64`, `ResetAt time.Time`, and `RetryAfter time.Duration`. The gRPC method is `limiter.v1.Limiter/Check`; its request contains only `key`, and its response contains the corresponding `allowed`, `remaining`, `reset_at`, and `retry_after` fields. The policy belongs to environment configuration in the later wiring slice, not to each request. A request costs one admission. There is no public capacity, refill rate, request ID, or clock input.

`key` is a nonempty, opaque bucket identifier supplied by the trusted backend. The backend derives identity from a verified session. The M1 `curl` flow simulates that backend; it does not establish public authentication. A user must not be allowed to choose their own identity. Authentication and access enforcement are outside M1.

Each key has a separate bucket that starts full under the one shared policy. Refill is continuous. An allowed request consumes one admission; a denied request consumes none. A decision reports the state **after** that check. `remaining` is the count of whole admissions possible immediately, even when the internal bucket holds a fraction. Thus an allowed last admission can return `allowed=true, remaining=0`; the next check can return `allowed=false, remaining=0`.

`reset_at` is the absolute UTC time when this bucket would be full if no more requests arrive. If already full, it is the decision time. `retry_after` is the nonnegative time until the earliest possible next admission after a denial; it is zero when allowed. The wire uses standard protobuf [`Timestamp`](https://protobuf.dev/reference/protobuf/google.protobuf/#timestamp) and [`Duration`](https://protobuf.dev/reference/protobuf/google.protobuf/#duration) so the absolute instant and elapsed interval have distinct types. Go uses `time.Time` and `time.Duration` for the same distinction. Subsecond values are supported on the wire; no rounding to whole seconds is part of the contract.

## Decision and error rules

Quota denial is a successful check: gRPC `OK` with `allowed=false`, eventually HTTP 429. A malformed request, including an empty key, is gRPC `InvalidArgument`, eventually HTTP 400. A Redis connection failure is gRPC `Unavailable`, eventually HTTP 503; an expired deadline is gRPC `DeadlineExceeded`, eventually HTTP 504. These failures leave admission **unknown** to the caller. They cannot be represented as `allowed=false` or used to infer that no token was consumed. Other unexpected failures also remain errors; their eventual HTTP handling belongs with the gateway. No retry, idempotency, or failure policy is defined in M1.

## Redis Lua boundary

The later Redis Lua implementation will receive a bucket key and the shared configured policy values. In production it obtains time from Redis `TIME`; a deterministic time override, if needed for tests, stays internal to the test path and is never accepted through the public request or production path. It returns the admission decision and the values needed for `remaining`, `reset_at`, and `retry_after`. Its invariants are atomic admission across instances sharing Redis, initially full independent buckets, one admission consumed on success, none on denial, continuous refill up to capacity, and the response meanings above. The script and its tests are separate work.

## Regeneration and current verification

[`proto/limiter/v1/limiter.proto`](../proto/limiter/v1/limiter.proto) is the authored source. The checked-in [`gen/limiter/v1/limiter.pb.go`](../gen/limiter/v1/limiter.pb.go) and [`limiter_grpc.pb.go`](../gen/limiter/v1/limiter_grpc.pb.go) are outputs from `protoc` and its Go plugins; do not edit them by hand. From the repository root on Windows with Go and PowerShell, run:

```powershell
./scripts/generate.ps1
go build ./...
go vet ./...
```

The script downloads `protoc` 29.3 for Windows x64 and checks its SHA-256, installs `protoc-gen-go` v1.36.6 and `protoc-gen-go-grpc` v1.5.1 into ignored `.tools/bin`, then regenerates both files. The module pins `grpc-go` v1.71.0 and protobuf v1.36.6. Repeating generation should leave the generated files unchanged. The compiler download and plugin cache are local to this project; no global tool installation is required. This follows the [protobuf Go generation guide](https://protobuf.dev/reference/go/go-generated/#compiler-invocation), [official compiler installation guidance](https://github.com/protocolbuffers/protobuf#protobuf-compiler-installation), and [gRPC Go quick start](https://grpc.io/docs/languages/go/quickstart/#regenerate-grpc-code). The [gRPC status code reference](https://grpc.io/docs/guides/status-codes/) defines the error codes used above.

Compilation verifies only the contract and generated code. There is no running admission path in this slice.
