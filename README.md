# Distributed Rate Limiter

An educational, public project for building a shared rate limiter with Go, Redis, gRPC, and a thin HTTP gateway. The M1 contract and runnable service wiring are present. Admission is deliberately unimplemented until the Redis Lua script is added.

## M1: walking skeleton

M1 targets Redis-backed admission through a checked-in Lua script. A single Compose command starts the service and Redis now; the HTTP gateway calls the actual gRPC server over loopback TCP. The eventual done check is a `curl` sequence that is allowed, then denied, then allowed after refill. For now, a valid check returns HTTP 501 and gRPC `Unimplemented`. See [M1 issues](https://github.com/danielcaze/distributed-rate-limiter/issues) for the work units and evidence gates.

M1 does not include retry, idempotency, or failure policy design. The linked issues define the remaining work and its dependencies.

The [M1 admission contract](docs/m1-contract.md) defines the interface, semantics, error mapping, regeneration command, and current wiring. It does not claim working rate limiting. The eventual M1 `curl` demonstration simulates a trusted backend that derives the bucket identifier from a verified session; M1 does not provide public authentication or access enforcement.

## Run the scaffolding

From the repository root in PowerShell:

```powershell
docker compose up --build -d
curl.exe -i http://127.0.0.1:8080/healthz
curl.exe -i http://127.0.0.1:8080/readyz
Set-Content -Path .check-request.json -Value '{"key":"demo"}' -NoNewline
curl.exe -i -H 'Content-Type: application/json' --data-binary '@.check-request.json' http://127.0.0.1:8080/v1/check
Remove-Item .check-request.json
docker compose down
```

`/healthz` reports process liveness; `/readyz` checks Redis reachability. Neither asserts that admission is implemented. The check above currently returns HTTP 501. Compose publishes only the HTTP port on host loopback. The HTTP endpoint assumes a trusted backend supplies `key`; it is not an authenticated public endpoint.

For local Go execution with an existing Redis on `127.0.0.1:6379`, run `go run ./cmd/limiter`. Settings, units, validation, and error codes are in [the M1 contract](docs/m1-contract.md). Sample environment values are in [`.env.example`](.env.example).

Optional Git hooks use [Lefthook](https://lefthook.dev/). Install the pinned Go 1.23 compatible version for this checkout with `go run github.com/evilmartians/lefthook@v1.11.1 install`. The hook runs `gofmt`, `go vet ./...`, and `go build ./...`; it does not require admission tests before their implementation.

## Project guidance

- [Agent guidance](AGENTS.md)
- [Domain glossary](CONTEXT.md)
- [Issue tracker conventions](docs/agents/issue-tracker.md)
- [Domain documentation conventions](docs/agents/domain.md)
- [Triage labels](docs/agents/triage-labels.md)

Licensed under [Apache-2.0](LICENSE).
