# Distributed Rate Limiter

An educational, public project for building a shared rate limiter with Go, Redis, gRPC, and a thin HTTP gateway. The repository currently contains setup and work tracking only; the service is not runnable yet.

## M1: walking skeleton

M1 targets a Go service whose HTTP gateway calls its own loopback gRPC endpoint, with Redis-backed admission through a checked-in Lua script. A single Compose command should start the service and Redis. The done check is a `curl` sequence that is allowed, then denied, then allowed after refill. See [M1 issues](https://github.com/danielcaze/distributed-rate-limiter/issues) for the work units and evidence gates.

M1 does not include retry, idempotency, or failure policy design. The linked issues define the remaining work and its dependencies.

## Project guidance

- [Agent guidance](AGENTS.md)
- [Domain glossary](CONTEXT.md)
- [Issue tracker conventions](docs/agents/issue-tracker.md)
- [Domain documentation conventions](docs/agents/domain.md)
- [Triage labels](docs/agents/triage-labels.md)

Licensed under [Apache-2.0](LICENSE).
