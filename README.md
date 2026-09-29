# Distributed Rate Limiter

An educational, public project for building a shared rate limiter with Go, Redis, gRPC, and a thin HTTP gateway. M1 is complete: admission runs as a token bucket in a Redis Lua script, shared by every service instance that uses the same Redis.

## M1: walking skeleton

Each check goes through the HTTP gateway, over loopback TCP to the service's own gRPC server, and into [`redischeck`](redischeck/checker.go), which runs [`admission.lua`](redischeck/admission.lua) atomically in Redis. M1 was merged in [PR #7](https://github.com/danielcaze/distributed-rate-limiter/pull/7). [Issue #6](https://github.com/danielcaze/distributed-rate-limiter/issues/6) records its verification: the full test suite against real Redis, a 1,000-call contention run across two checker instances (`allowed=100 denied=900 errors=0`), the allowed, allowed, denied, allowed-after-refill `curl` sequence through Compose, and a Redis outage reported as HTTP 504 and 503 rather than as a quota denial.

M1 does not include retry, idempotency, or failure policy design. These are deferred to a later milestone.

The [M1 admission contract](docs/m1-contract.md) defines the interface, semantics, error mapping, Lua boundary, regeneration command, and wiring. The `curl` example simulates a trusted backend that derives the bucket identifier from a verified session; M1 does not provide public authentication or access enforcement.

## Run the service

From the repository root in PowerShell:

```powershell
docker compose up --build -d
curl.exe -i http://127.0.0.1:8080/healthz
curl.exe -i http://127.0.0.1:8080/readyz
Set-Content -Path .check-request.json -Value '{"key":"demo"}' -NoNewline
1..3 | ForEach-Object { curl.exe -s -w " HTTP %{http_code}`n" -H 'Content-Type: application/json' --data-binary '@.check-request.json' http://127.0.0.1:8080/v1/check }
Start-Sleep -Seconds 2
curl.exe -s -w " HTTP %{http_code}`n" -H 'Content-Type: application/json' --data-binary '@.check-request.json' http://127.0.0.1:8080/v1/check
Remove-Item .check-request.json
docker compose down
```

Compose sets `CAPACITY=2` and `REFILL_TOKENS_PER_SECOND=1`. The first two checks are allowed with `remaining` 1 and then 0, the third is denied with HTTP 429 and a `retryAfter` under one second, and the check after a two-second pause, enough to refill the whole bucket, is allowed again with `remaining` 1. Decision times depend on when the calls run, so `resetAt` and `retryAfter` differ between runs.

`/healthz` reports process liveness; `/readyz` reports whether Redis answers a PING. Compose publishes only the HTTP port on host loopback. The HTTP endpoint assumes a trusted backend supplies `key`; it is not an authenticated public endpoint.

For local Go execution with an existing Redis on `127.0.0.1:6379`, run `go run ./cmd/limiter`. Settings, units, validation, and error codes are in [the M1 contract](docs/m1-contract.md). Sample environment values are in [`.env.example`](.env.example).

Optional Git hooks use [Lefthook](https://lefthook.dev/). Install the pinned Go 1.27 compatible version for this checkout with `go run github.com/evilmartians/lefthook@v1.11.1 install`. The hook runs `gofmt`, `go vet ./...`, and `go build ./...`. Tests are not part of the hook. Run them with `go test ./... -count=1` while Docker is available, because the `redischeck` tests start a real Redis through [Testcontainers](https://golang.testcontainers.org/).

## Project guidance

- [Agent guidance](AGENTS.md)
- [Domain glossary](CONTEXT.md)
- [Issue tracker conventions](docs/agents/issue-tracker.md)
- [Domain documentation conventions](docs/agents/domain.md)
- [Triage labels](docs/agents/triage-labels.md)

Licensed under [Apache-2.0](LICENSE).
